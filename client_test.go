package whatsrook

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestResolveMentionPN(t *testing.T) {
	ctx := context.Background()
	pnJID := types.NewJID("2348012345678", types.DefaultUserServer)

	resolvedJID, tagUser := ResolveMentionRaw(ctx, nil, pnJID)
	if tagUser != "2348012345678" {
		t.Fatalf("expected tagUser to be '2348012345678', got '%s'", tagUser)
	}
	if resolvedJID.User != "2348012345678" || resolvedJID.Server != types.DefaultUserServer {
		t.Fatalf("expected resolvedJID to be %s, got %s", pnJID.String(), resolvedJID.String())
	}

	jids, tagUser2 := ResolveMentionJIDs(ctx, nil, pnJID)
	if tagUser2 != "2348012345678" {
		t.Fatalf("expected tagUser2 to be '2348012345678', got '%s'", tagUser2)
	}
	if len(jids) != 1 || jids[0].User != "2348012345678" {
		t.Fatalf("expected 1 jid with user '2348012345678', got %+v", jids)
	}
}

func TestResolveMentionLIDFallback(t *testing.T) {
	ctx := context.Background()
	lidJID := types.NewJID("123456789012345", types.HiddenUserServer)

	resolvedJID, tagUser := ResolveMentionRaw(ctx, nil, lidJID)
	if tagUser != "123456789012345" {
		t.Fatalf("expected tagUser to be '123456789012345', got '%s'", tagUser)
	}
	if resolvedJID.User != "123456789012345" || resolvedJID.Server != types.HiddenUserServer {
		t.Fatalf("expected resolvedJID to be %s, got %s", lidJID.String(), resolvedJID.String())
	}
}

func TestFormatMention(t *testing.T) {
	pctx := &PluginContext{
		Chat:   types.NewJID("12345", "g.us"),
		Sender: types.NewJID("2348012345678", types.DefaultUserServer),
	}

	target := types.NewJID("2348099999999", types.DefaultUserServer)
	tag, resolved := pctx.FormatMention(target)

	if tag != "@2348099999999" {
		t.Fatalf("expected tag '@2348099999999', got '%s'", tag)
	}
	if resolved.User != "2348099999999" {
		t.Fatalf("expected resolved.User '2348099999999', got '%s'", resolved.User)
	}

	tagJIDs, jids := pctx.FormatMentionJIDs(target)
	if tagJIDs != "@2348099999999" {
		t.Fatalf("expected tagJIDs '@2348099999999', got '%s'", tagJIDs)
	}
	if len(jids) == 0 || jids[0].User != "2348099999999" {
		t.Fatalf("expected jids containing target, got %+v", jids)
	}
}

func TestResolveContactNameFallback(t *testing.T) {
	ctx := context.Background()
	pnJID := types.NewJID("2348012345678", types.DefaultUserServer)

	name := ResolveContactName(ctx, nil, pnJID)
	if name != "2348012345678" {
		t.Fatalf("expected fallback to user '2348012345678', got '%s'", name)
	}
}

func TestUnwrapMessageProto(t *testing.T) {
	inner := &waE2E.Message{
		Conversation: new("deep secret text"),
	}

	// 1. Nested Ephemeral -> ViewOnce -> Message
	wrapped1 := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ViewOnceMessage: &waE2E.FutureProofMessage{
					Message: inner,
				},
			},
		},
	}
	if unwrapped := UnwrapMessageProto(wrapped1); unwrapped.GetConversation() != "deep secret text" {
		t.Fatalf("expected 'deep secret text', got %+v", unwrapped)
	}

	// 2. ProtocolMessage EditedMessage
	wrapped2 := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			EditedMessage: inner,
		},
	}
	if unwrapped := UnwrapMessageProto(wrapped2); unwrapped.GetConversation() != "deep secret text" {
		t.Fatalf("expected 'deep secret text', got %+v", unwrapped)
	}

	// 3. BotInvokeMessage -> GroupMentionedMessage
	wrapped3 := &waE2E.Message{
		BotInvokeMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				GroupMentionedMessage: &waE2E.FutureProofMessage{
					Message: inner,
				},
			},
		},
	}
	if unwrapped := UnwrapMessageProto(wrapped3); unwrapped.GetConversation() != "deep secret text" {
		t.Fatalf("expected 'deep secret text', got %+v", unwrapped)
	}
}

func TestExtractTextFromProto(t *testing.T) {
	cases := []struct {
		name     string
		msg      *waE2E.Message
		expected string
	}{
		{
			name:     "Conversation",
			msg:      &waE2E.Message{Conversation: new("hello world")},
			expected: "hello world",
		},
		{
			name: "ExtendedTextMessage",
			msg: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: new("extended info"),
				},
			},
			expected: "extended info",
		},
		{
			name: "ImageMessage caption",
			msg: &waE2E.Message{
				ImageMessage: &waE2E.ImageMessage{
					Caption: new("sunset photo"),
				},
			},
			expected: "sunset photo",
		},
		{
			name: "VideoMessage caption",
			msg: &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					Caption: new("video clip"),
				},
			},
			expected: "video clip",
		},
		{
			name: "PtvMessage caption",
			msg: &waE2E.Message{
				PtvMessage: &waE2E.VideoMessage{
					Caption: new("instant video"),
				},
			},
			expected: "instant video",
		},
		{
			name: "PollCreationMessage",
			msg: &waE2E.Message{
				PollCreationMessage: &waE2E.PollCreationMessage{
					Name: new("Which framework?"),
				},
			},
			expected: "Which framework?",
		},
		{
			name: "PollCreationMessageV3",
			msg: &waE2E.Message{
				PollCreationMessageV3: &waE2E.PollCreationMessage{
					Name: new("Lunch location?"),
				},
			},
			expected: "Lunch location?",
		},
		{
			name: "InteractiveMessage body text",
			msg: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Interactive question"),
					},
				},
			},
			expected: "Interactive question",
		},
		{
			name: "ButtonsResponseMessage",
			msg: &waE2E.Message{
				ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
					Response: &waE2E.ButtonsResponseMessage_SelectedDisplayText{
						SelectedDisplayText: "Option A",
					},
				},
			},
			expected: "Option A",
		},
		{
			name: "TemplateButtonReplyMessage",
			msg: &waE2E.Message{
				TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{
					SelectedDisplayText: new("Click Here"),
				},
			},
			expected: "Click Here",
		},
		{
			name: "ListResponseMessage",
			msg: &waE2E.Message{
				ListResponseMessage: &waE2E.ListResponseMessage{
					Title: new("Menu Item 1"),
				},
			},
			expected: "Menu Item 1",
		},
		{
			name: "GroupInviteMessage",
			msg: &waE2E.Message{
				GroupInviteMessage: &waE2E.GroupInviteMessage{
					Caption: new("Join our group"),
				},
			},
			expected: "Join our group",
		},
		{
			name: "EventMessage",
			msg: &waE2E.Message{
				EventMessage: &waE2E.EventMessage{
					Name: new("Team Standup"),
				},
			},
			expected: "Team Standup",
		},
		{
			name: "Edited ProtocolMessage",
			msg: &waE2E.Message{
				ProtocolMessage: &waE2E.ProtocolMessage{
					EditedMessage: &waE2E.Message{
						Conversation: new("edited payload"),
					},
				},
			},
			expected: "edited payload",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractTextFromProto(tc.msg)
			if got != tc.expected {
				t.Fatalf("expected '%s', got '%s'", tc.expected, got)
			}
		})
	}
}

func TestExtractMessageTextFallback(t *testing.T) {
	evt := &events.Message{
		RawMessage: &waE2E.Message{
			Conversation: new("fallback text"),
		},
	}
	if text := ExtractMessageText(evt); text != "fallback text" {
		t.Fatalf("expected 'fallback text', got '%s'", text)
	}
}

func TestConfigureCompanionPlatform(t *testing.T) {
	tests := []struct {
		name         string
		clientType   ClientType
		isBusiness   bool
		wantPlatform waWa6.ClientPayload_UserAgent_Platform
		wantProps    waCompanionReg.DeviceProps_PlatformType
	}{
		{
			name:         "Android normal",
			clientType:   ClientAndroid,
			isBusiness:   false,
			wantPlatform: waWa6.ClientPayload_UserAgent_ANDROID,
			wantProps:    waCompanionReg.DeviceProps_ANDROID_PHONE,
		},
		{
			name:         "Android business (SMB_ANDROID)",
			clientType:   ClientAndroid,
			isBusiness:   true,
			wantPlatform: waWa6.ClientPayload_UserAgent_SMB_ANDROID,
			wantProps:    waCompanionReg.DeviceProps_ANDROID_PHONE,
		},
		{
			name:         "iOS normal",
			clientType:   ClientIos,
			isBusiness:   false,
			wantPlatform: waWa6.ClientPayload_UserAgent_IOS,
			wantProps:    waCompanionReg.DeviceProps_IOS_PHONE,
		},
		{
			name:         "iOS business (SMB_IOS)",
			clientType:   ClientIos,
			isBusiness:   true,
			wantPlatform: waWa6.ClientPayload_UserAgent_SMB_IOS,
			wantProps:    waCompanionReg.DeviceProps_IOS_PHONE,
		},
		{
			name:         "Chrome default",
			clientType:   ClientChrome,
			isBusiness:   false,
			wantPlatform: waWa6.ClientPayload_UserAgent_WEB,
			wantProps:    waCompanionReg.DeviceProps_CHROME,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configureCompanionPlatform(tc.clientType, tc.isBusiness)
			if store.BaseClientPayload.UserAgent.GetPlatform() != tc.wantPlatform {
				t.Fatalf("expected Platform = %v, got %v", tc.wantPlatform, store.BaseClientPayload.UserAgent.GetPlatform())
			}
			if store.DeviceProps.GetPlatformType() != tc.wantProps {
				t.Fatalf("expected PlatformType = %v, got %v", tc.wantProps, store.DeviceProps.GetPlatformType())
			}
		})
	}
}

func TestParseClientType(t *testing.T) {
	cases := []struct {
		input      string
		wantType   ClientType
		wantParsed bool
	}{
		{"chrome", ClientChrome, true},
		{"Chrome", ClientChrome, true},
		{"android", ClientAndroid, true},
		{"Android", ClientAndroid, true},
		{"ios", ClientIos, true},
		{"iOS", ClientIos, true},
		{"smb_android", ClientAndroid, true},
		{"smba", ClientAndroid, true},
		{"smb_ios", ClientIos, true},
		{"smbi", ClientIos, true},
		{"unknown_platform", ClientChrome, false},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			ct, ok := ParseClientType(c.input)
			if ok != c.wantParsed {
				t.Fatalf("input %q: expected ok=%v, got %v", c.input, c.wantParsed, ok)
			}
			if ok && ct != c.wantType {
				t.Fatalf("input %q: expected type=%v, got %v", c.input, c.wantType, ct)
			}
		})
	}
}
