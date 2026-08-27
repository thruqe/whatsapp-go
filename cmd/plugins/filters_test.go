package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestRenderFilterTemplate_Placeholders(t *testing.T) {
	ctx := context.Background()
	sender := types.NewJID("1234567890", types.DefaultUserServer)
	chat := types.NewJID("9876543210", types.DefaultUserServer)

	evt := &events.Message{
		Info: types.MessageInfo{
			ID:        "TEST_STANZA_123",
			Chat:      chat,
			Sender:    sender,
			Timestamp: time.Now(),
			PushName:  "TestUser",
		},
	}

	tpl := "Hello @user! Name: {name}, Phone: [phone], Bot: @bot, Uptime: {uptime}, RAM: @ram, Time: @time, Date: @date, Day: @day, Version: @version"

	rendered, mentions := RenderFilterTemplate(ctx, nil, evt, tpl)

	if !strings.Contains(rendered, "@1234567890") {
		t.Errorf("Expected @user to be replaced by sender phone tag, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Name: TestUser") {
		t.Errorf("Expected {name} to be replaced by PushName, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Phone: 1234567890") {
		t.Errorf("Expected [phone] to be replaced by sender phone, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Bot: WhatsRook") {
		t.Errorf("Expected @bot to be replaced by bot name, got: %s", rendered)
	}
	if strings.Contains(rendered, "{uptime}") || strings.Contains(rendered, "@uptime") {
		t.Errorf("Expected uptime to be replaced, got: %s", rendered)
	}
	if strings.Contains(rendered, "@ram") {
		t.Errorf("Expected @ram to be replaced, got: %s", rendered)
	}
	if strings.Contains(rendered, "@time") {
		t.Errorf("Expected @time to be replaced, got: %s", rendered)
	}
	if strings.Contains(rendered, "@date") {
		t.Errorf("Expected @date to be replaced, got: %s", rendered)
	}
	if strings.Contains(rendered, "@day") {
		t.Errorf("Expected @day to be replaced, got: %s", rendered)
	}

	foundSenderMention := false
	for _, m := range mentions {
		if m == sender.String() {
			foundSenderMention = true
			break
		}
	}
	if !foundSenderMention {
		t.Errorf("Expected sender %q in mentions slice, got: %v", sender.String(), mentions)
	}
}

func TestApplyFilterPlaceholders_ConversationConversion(t *testing.T) {
	ctx := context.Background()
	sender := types.NewJID("2348000000000", types.DefaultUserServer)
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:       "MSG_ABC",
			Chat:     sender,
			Sender:   sender,
			PushName: "Tester",
		},
	}

	convText := "Welcome @user to the chat!"
	msg := &waE2E.Message{
		Conversation: &convText,
	}

	ApplyFilterPlaceholders(ctx, nil, evt, msg)

	// Since @user mention is present, Conversation should be converted to ExtendedTextMessage with ContextInfo.MentionedJID
	if msg.Conversation != nil {
		t.Errorf("Expected msg.Conversation to be nil after conversion, got: %v", *msg.Conversation)
	}
	if msg.ExtendedTextMessage == nil {
		t.Fatalf("Expected msg.ExtendedTextMessage to be populated")
	}
	if !strings.Contains(*msg.ExtendedTextMessage.Text, "@2348000000000") {
		t.Errorf("Expected rendered text with phone tag, got: %s", *msg.ExtendedTextMessage.Text)
	}
	if msg.ExtendedTextMessage.ContextInfo == nil || len(msg.ExtendedTextMessage.ContextInfo.MentionedJID) == 0 {
		t.Fatalf("Expected MentionedJID to contain sender JID")
	}
	if msg.ExtendedTextMessage.ContextInfo.MentionedJID[0] != sender.String() {
		t.Errorf("Expected MentionedJID %s, got: %v", sender.String(), msg.ExtendedTextMessage.ContextInfo.MentionedJID)
	}
}

func TestApplyFilterPlaceholders_MediaCaptions(t *testing.T) {
	ctx := context.Background()
	sender := types.NewJID("111222333444", types.DefaultUserServer)
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:       "IMG_MSG",
			Chat:     sender,
			Sender:   sender,
			PushName: "MediaUser",
		},
	}

	imgCaption := "Image for @user with bot @bot"
	imgMsg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption: &imgCaption,
		},
	}

	ApplyFilterPlaceholders(ctx, nil, evt, imgMsg)

	if !strings.Contains(*imgMsg.ImageMessage.Caption, "@111222333444") {
		t.Errorf("Expected image caption to have user tag, got: %s", *imgMsg.ImageMessage.Caption)
	}
	if !strings.Contains(*imgMsg.ImageMessage.Caption, "WhatsRook") {
		t.Errorf("Expected image caption to have bot name, got: %s", *imgMsg.ImageMessage.Caption)
	}
	if imgMsg.ImageMessage.ContextInfo == nil || len(imgMsg.ImageMessage.ContextInfo.MentionedJID) == 0 {
		t.Fatalf("Expected ImageMessage ContextInfo.MentionedJID to be set")
	}
}

func TestAppendUniqueMentions(t *testing.T) {
	existing := []string{"123@s.whatsapp.net", "456@s.whatsapp.net"}
	newMentions := []string{"456@s.whatsapp.net", "789@s.whatsapp.net", ""}

	result := appendUniqueMentions(existing, newMentions)

	if len(result) != 3 {
		t.Fatalf("Expected 3 unique mentions, got: %d (%v)", len(result), result)
	}
	if result[0] != "123@s.whatsapp.net" || result[1] != "456@s.whatsapp.net" || result[2] != "789@s.whatsapp.net" {
		t.Errorf("Unexpected result slice: %v", result)
	}
}

func TestSetReplyContextInfo_PreservesMentions(t *testing.T) {
	sender := types.NewJID("12345", types.DefaultUserServer)
	triggerText := "test trigger"
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:     "STANZA_XYZ",
			Chat:   sender,
			Sender: sender,
		},
		Message: &waE2E.Message{
			Conversation: &triggerText,
		},
	}

	text := "Hello @12345"
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &text,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{"12345@s.whatsapp.net"},
			},
		},
	}

	setReplyContextInfo(msg, evt)

	ci := msg.ExtendedTextMessage.ContextInfo
	if ci == nil {
		t.Fatalf("ContextInfo is nil")
	}
	if *ci.StanzaID != "STANZA_XYZ" {
		t.Errorf("Expected StanzaID STANZA_XYZ, got: %s", *ci.StanzaID)
	}
	if len(ci.MentionedJID) != 1 || ci.MentionedJID[0] != "12345@s.whatsapp.net" {
		t.Errorf("Expected MentionedJID to be preserved, got: %v", ci.MentionedJID)
	}
}
