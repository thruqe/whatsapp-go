package builder

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestPollBuilder_Basic(t *testing.T) {
	pb := NewPoll(nil, "Choose an option:")
	if pb.question != "Choose an option:" {
		t.Errorf("question = %q, want 'Choose an option:'", pb.question)
	}
	if !pb.single {
		t.Errorf("expected single choice by default")
	}
	if !pb.autoDelete {
		t.Errorf("expected autoDelete = true by default")
	}
	if pb.timeout != DefaultPollTimeout {
		t.Errorf("expected timeout = %v, got %v", DefaultPollTimeout, pb.timeout)
	}

	pb.AddOption("Option 1").
		AddOptions("Option 2", "Option 3").
		MultiChoice().
		Timeout(10 * time.Second)

	if len(pb.options) != 3 {
		t.Errorf("len(options) = %d, want 3", len(pb.options))
	}
	if pb.single {
		t.Errorf("expected multi-choice")
	}
	if pb.timeout != 10*time.Second {
		t.Errorf("expected timeout = 10s, got %v", pb.timeout)
	}

	pb.SingleChoice().AutoDelete(false)
	if !pb.single {
		t.Errorf("expected single choice after SingleChoice()")
	}
	if pb.autoDelete {
		t.Errorf("expected autoDelete = false after AutoDelete(false)")
	}
}

func TestPollRoute_Lifecycle(t *testing.T) {
	msgID := types.MessageID("TEST_POLL_MSG_ID_1")
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:  msgID,
		Options:    []string{"Opt A", "Opt B"},
		Once:       true,
		AutoDelete: true,
		Timeout:    100 * time.Millisecond,
	})

	pollRoutesMu.RLock()
	route, ok := pollRoutes[msgID]
	pollRoutesMu.RUnlock()

	if !ok {
		t.Fatalf("expected route to be registered")
	}
	if len(route.options) != 2 {
		t.Errorf("expected 2 options in route, got %d", len(route.options))
	}

	// Wait for the timeout to fire and auto-deregister
	time.Sleep(150 * time.Millisecond)

	pollRoutesMu.RLock()
	_, okAfter := pollRoutes[msgID]
	pollRoutesMu.RUnlock()

	if okAfter {
		t.Errorf("expected route to be removed after timeout expiry")
	}
}

type mockSender struct {
	client *whatsmeow.Client
	chat   types.JID
	sender types.JID
}

func (m *mockSender) GetSendContext() context.Context { return context.Background() }
func (m *mockSender) GetClient() *whatsmeow.Client    { return m.client }
func (m *mockSender) GetChat() types.JID              { return m.chat }
func (m *mockSender) GetSender() types.JID            { return m.sender }
func (m *mockSender) GetBotName() string              { return "WhatsRook" }
func (m *mockSender) StopAutoLoader()                 {}
func (m *mockSender) ReplyContextInfo() *waE2E.ContextInfo { return nil }
func (m *mockSender) FormatTextResponse(text string) string { return text }
func (m *mockSender) Reply(text string) error         { return nil }
func (m *mockSender) ReplyWithMentions(text string, mentions []types.JID) error { return nil }
func (m *mockSender) ReplyWithImage(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error { return nil }
func (m *mockSender) ReplyWithVideo(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) ReplyWithVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error { return nil }
func (m *mockSender) ReplyWithVideoGif(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) ReplyWithAudio(data []byte, mimetype string) error { return nil }
func (m *mockSender) ReplyWithDocument(data []byte, mimetype, filename, caption string) error { return nil }
func (m *mockSender) ReplyWithSticker(data []byte) error { return nil }
func (m *mockSender) ReplyWithGroupMention(text string) error { return nil }
func (m *mockSender) SendText(text string) error { return nil }
func (m *mockSender) SendTextWithMentions(text string, mentions []types.JID) error { return nil }
func (m *mockSender) SendImage(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error { return nil }
func (m *mockSender) SendVideo(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) SendVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error { return nil }
func (m *mockSender) SendVideoGif(data []byte, mimetype, caption string) error { return nil }
func (m *mockSender) SendAudio(data []byte, mimetype string) error { return nil }
func (m *mockSender) SendDocument(data []byte, mimetype, filename, caption string) error { return nil }
func (m *mockSender) SendSticker(data []byte) error { return nil }
func (m *mockSender) SendTextWithGroupMention(text string) error { return nil }
func (m *mockSender) React(emoji string) error { return nil }
func (m *mockSender) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	return whatsmeow.SendResponse{}, nil
}

func TestIsSameUser_LID(t *testing.T) {
	// 1. Same user and server
	pn1 := types.NewJID("2348011111111", types.DefaultUserServer)
	pn2 := types.NewJID("2348011111111", types.DefaultUserServer)
	if !isSameUser(context.Background(), nil, pn1, pn2) {
		t.Errorf("expected same user for identical JIDs")
	}

	// 2. Non-AD comparison
	pn1AD := types.NewADJID("2348011111111", 0, 1)
	if !isSameUser(context.Background(), nil, pn1, pn1AD) {
		t.Errorf("expected same user for AD and NonAD JIDs")
	}
}
