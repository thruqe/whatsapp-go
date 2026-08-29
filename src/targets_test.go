package src

import (
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestGetTargets_PriorityInDM(t *testing.T) {
	botJID := types.NewJID("1111111111", types.DefaultUserServer)
	dmChatJID := types.NewJID("2222222222", types.DefaultUserServer)
	argJID := types.NewJID("3333333333", types.DefaultUserServer)
	quotedJID := types.NewJID("4444444444", types.DefaultUserServer)

	mockClient := &whatsmeow.Client{
		Store: &store.Device{
			ID: &botJID,
		},
	}

	t.Run("DM with args and no reply uses args JID rather than chat ID", func(t *testing.T) {
		ctx := &PluginContext{
			Client:  mockClient,
			Chat:    dmChatJID,
			Sender:  dmChatJID,
			Args:    []string{"3333333333"},
			RawArgs: "3333333333",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:    dmChatJID,
					Sender:  dmChatJID,
					IsGroup: false,
				},
				Message: &waE2E.Message{},
			},
		}

		targets := ctx.GetTargets()
		if len(targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(targets))
		}
		if targets[0].User != argJID.User {
			t.Errorf("expected target to be args %s, got %s", argJID.User, targets[0].User)
		}
	})

	t.Run("DM with reply and args prioritizes reply", func(t *testing.T) {
		quotedUser := quotedJID.String()
		ctx := &PluginContext{
			Client:  mockClient,
			Chat:    dmChatJID,
			Sender:  dmChatJID,
			Args:    []string{"3333333333"},
			RawArgs: "3333333333",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:    dmChatJID,
					Sender:  dmChatJID,
					IsGroup: false,
				},
				Message: &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						ContextInfo: &waE2E.ContextInfo{
							Participant: &quotedUser,
							QuotedMessage: &waE2E.Message{
								Conversation: new(string),
							},
						},
					},
				},
			},
		}

		targets := ctx.GetTargets()
		if len(targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(targets))
		}
		if targets[0].User != quotedJID.User {
			t.Errorf("expected target to be quoted sender %s, got %s", quotedJID.User, targets[0].User)
		}
	})

	t.Run("DM with no args and no reply falls back to chat JID", func(t *testing.T) {
		ctx := &PluginContext{
			Client:  mockClient,
			Chat:    dmChatJID,
			Sender:  dmChatJID,
			Args:    nil,
			RawArgs: "",
			Evt: &events.Message{
				Info: types.MessageInfo{
					Chat:    dmChatJID,
					Sender:  dmChatJID,
					IsGroup: false,
				},
				Message: &waE2E.Message{},
			},
		}

		targets := ctx.GetTargets()
		if len(targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(targets))
		}
		if targets[0].User != dmChatJID.User {
			t.Errorf("expected target to be chat %s, got %s", dmChatJID.User, targets[0].User)
		}
	})
}
