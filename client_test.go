package whatsrook

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestReplyContextInfo_NoCircularReference(t *testing.T) {
	text := ".ping"
	incomingMsg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &text,
			ContextInfo: &waE2E.ContextInfo{
				StanzaID: proto.String("PREV123"),
			},
		},
	}

	pctx := &PluginContext{
		Chat:   types.NewJID("123456789", types.DefaultUserServer),
		Sender: types.NewJID("987654321", types.DefaultUserServer),
		Evt: &events.Message{
			Info: types.MessageInfo{
				ID:        "MSG_PING_123",
				Sender:    types.NewJID("987654321", types.DefaultUserServer),
				Timestamp: time.Now(),
			},
			Message: incomingMsg,
		},
	}

	ci := pctx.replyContextInfo()
	if ci == nil {
		t.Fatalf("expected ContextInfo, got nil")
	}
	if ci.GetStanzaID() != "MSG_PING_123" {
		t.Errorf("expected StanzaID MSG_PING_123, got %s", ci.GetStanzaID())
	}
	if ci.QuotedMessage == nil {
		t.Fatalf("expected QuotedMessage, got nil")
	}

	replyMsg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("Ping..."),
			ContextInfo: ci,
		},
	}

	// Marshal to bytes - this must succeed in microseconds without stack overflow
	data, err := proto.Marshal(replyMsg)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("marshaled data is empty")
	}
}
