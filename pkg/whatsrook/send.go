package whatsrook

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Sender encapsulates the target destination and client instance for sending messages.
type Sender struct {
	Client *whatsmeow.Client
	Chat   types.JID
	Ctx    context.Context
}

// NewSender creates a new Sender instance.
func NewSender(client *whatsmeow.Client, chat types.JID, ctx ...context.Context) *Sender {
	c := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		c = ctx[0]
	}
	return &Sender{
		Client: client,
		Chat:   chat,
		Ctx:    c,
	}
}

func (s *Sender) getContext() context.Context {
	if s == nil || s.Ctx == nil || s.Ctx.Err() != nil {
		return context.Background()
	}
	return s.Ctx
}

// ResolveMentionJIDStrings resolves user JIDs to their string representations (including PN/LID lookups).
func ResolveMentionJIDStrings(ctx context.Context, client *whatsmeow.Client, mentions []types.JID) []string {
	if len(mentions) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	seen := make(map[string]bool)
	var mentionStrs []string
	for _, m := range mentions {
		if m.IsEmpty() {
			continue
		}
		norm := m.ToNonAD()
		key := norm.String()
		if !seen[key] {
			seen[key] = true
			mentionStrs = append(mentionStrs, key)
		}
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			switch norm.Server {
			case types.HiddenUserServer:
				if pn, err := client.Store.LIDs.GetPNForLID(ctx, norm); err == nil && !pn.IsEmpty() {
					pnStr := pn.ToNonAD().String()
					if !seen[pnStr] {
						seen[pnStr] = true
						mentionStrs = append(mentionStrs, pnStr)
					}
				}
			case types.DefaultUserServer:
				if lid, err := client.Store.LIDs.GetLIDForPN(ctx, norm); err == nil && !lid.IsEmpty() {
					lidStr := lid.ToNonAD().String()
					if !seen[lidStr] {
						seen[lidStr] = true
						mentionStrs = append(mentionStrs, lidStr)
					}
				}
			}
		}
	}
	return mentionStrs
}

// SendText sends a plain text message without quoting.
func SendText(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted ...*waE2E.ContextInfo) error {
	_, err := SendTextWithID(ctx, client, chat, text, quoted...)
	return err
}

var Text = SendText

// SendTextWithID sends a plain text message and returns the assigned message ID.
func SendTextWithID(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted ...*waE2E.ContextInfo) (string, error) {
	if client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var msg *waE2E.Message
	if len(quoted) > 0 && quoted[0] != nil {
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        &text,
				ContextInfo: quoted[0],
			},
		}
	} else {
		msg = &waE2E.Message{
			Conversation: &text,
		}
	}

	resp, err := client.SendMessage(ctx, chat, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

var TextWithID = SendTextWithID

// SendTextWithMentions sends a text message with user mentions tagged in ContextInfo.
func SendTextWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, mentions []types.JID, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	mentionStrs := ResolveMentionJIDStrings(ctx, client, mentions)
	var ci *waE2E.ContextInfo
	if len(quoted) > 0 && quoted[0] != nil {
		ci = quoted[0]
		ci.MentionedJID = mentionStrs
	} else if len(mentionStrs) > 0 {
		ci = &waE2E.ContextInfo{
			MentionedJID: mentionStrs,
		}
	}

	var msg *waE2E.Message
	if ci != nil {
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        &text,
				ContextInfo: ci,
			},
		}
	} else {
		msg = &waE2E.Message{
			Conversation: &text,
		}
	}

	_, err := client.SendMessage(ctx, chat, msg)
	return err
}

var TextWithMentions = SendTextWithMentions

// SendReply sends a quoted text reply to a previous message.
func SendReply(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted *waE2E.ContextInfo) error {
	_, err := SendReplyWithID(ctx, client, chat, text, quoted)
	return err
}

var Reply = SendReply

// SendReplyWithID sends a quoted text reply and returns the assigned message ID.
func SendReplyWithID(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted *waE2E.ContextInfo) (string, error) {
	if client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	resp, err := client.SendMessage(ctx, chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &text,
			ContextInfo: quoted,
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

var ReplyWithID = SendReplyWithID

// SendReplyWithMentions sends a quoted text reply mentioning specified JIDs.
func SendReplyWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, mentions []types.JID, quoted *waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ci := quoted
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.MentionedJID = ResolveMentionJIDStrings(ctx, client, mentions)

	_, err := client.SendMessage(ctx, chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &text,
			ContextInfo: ci,
		},
	})
	return err
}

var ReplyWithMentions = SendReplyWithMentions

// SendReact sends an emoji reaction to a target message.
func SendReact(ctx context.Context, client *whatsmeow.Client, chat types.JID, targetID types.MessageID, targetSender types.JID, emoji string) error {
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

var React = SendReact

// SendReactMessage sends an emoji reaction specifying only targetID.
func SendReactMessage(ctx context.Context, client *whatsmeow.Client, chat types.JID, targetID types.MessageID, emoji string) error {
	return SendReact(ctx, client, chat, targetID, types.EmptyJID, emoji)
}

var ReactMessage = SendReactMessage

// SendEdit edits an existing message.
func SendEdit(ctx context.Context, client *whatsmeow.Client, chat types.JID, msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
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

var Edit = SendEdit

// SendDelete revokes/deletes a message for everyone in the chat.
func SendDelete(ctx context.Context, client *whatsmeow.Client, chat types.JID, msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
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

var Delete = SendDelete

// Methods on *Sender
func (s *Sender) Text(text string) error {
	return SendText(s.getContext(), s.Client, s.Chat, text)
}

func (s *Sender) Reply(text string, quoted *waE2E.ContextInfo) error {
	return SendReply(s.getContext(), s.Client, s.Chat, text, quoted)
}

func (s *Sender) React(targetID types.MessageID, targetSender types.JID, emoji string) error {
	return SendReact(s.getContext(), s.Client, s.Chat, targetID, targetSender, emoji)
}

func (s *Sender) Edit(msgID types.MessageID, content any) (whatsmeow.SendResponse, error) {
	return SendEdit(s.getContext(), s.Client, s.Chat, msgID, content)
}

func (s *Sender) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	return SendDelete(s.getContext(), s.Client, s.Chat, msgID, senderJID...)
}
