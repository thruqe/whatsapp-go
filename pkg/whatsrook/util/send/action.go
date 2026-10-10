package send

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// React sends an emoji reaction to a target message.
func React(ctx context.Context, client *whatsmeow.Client, chat types.JID, targetID types.MessageID, targetSender types.JID, emoji string) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if targetID == "" {
		return fmt.Errorf("no target message ID for reaction")
	}
	reactionMsg := client.BuildReaction(chat, targetSender, targetID, emoji)
	_, err := client.SendMessage(ctx, chat, reactionMsg)
	return err
}

// ReactMessage sends an emoji reaction specifying only targetID.
func ReactMessage(ctx context.Context, client *whatsmeow.Client, chat types.JID, targetID types.MessageID, emoji string) error {
	return React(ctx, client, chat, targetID, types.EmptyJID, emoji)
}

// Edit edits an existing message.
func Edit(ctx context.Context, client *whatsmeow.Client, chat types.JID, msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var msg *waE2E.Message
	switch v := content.(type) {
	case string:
		msg = &waE2E.Message{
			Conversation: &v,
		}
	case *waE2E.Message:
		msg = v
	default:
		return whatsmeow.SendResponse{}, fmt.Errorf("unsupported content type: %T", content)
	}
	editMsg := client.BuildEdit(chat, msgID, msg)
	return client.SendMessage(ctx, chat, editMsg)
}

// Delete revokes/deletes a message for everyone in the chat.
func Delete(ctx context.Context, client *whatsmeow.Client, chat types.JID, msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	if client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sJID := types.EmptyJID
	if len(senderJID) > 0 {
		sJID = senderJID[0]
	}
	revokeMsg := client.BuildRevoke(chat, sJID, msgID)
	return client.SendMessage(ctx, chat, revokeMsg)
}

// Action methods on *Sender
func (s *Sender) React(targetID types.MessageID, targetSender types.JID, emoji string) error {
	return React(s.getContext(), s.Client, s.Chat, targetID, targetSender, emoji)
}

func (s *Sender) Edit(msgID types.MessageID, content any) (whatsmeow.SendResponse, error) {
	return Edit(s.getContext(), s.Client, s.Chat, msgID, content)
}

func (s *Sender) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	return Delete(s.getContext(), s.Client, s.Chat, msgID, senderJID...)
}
