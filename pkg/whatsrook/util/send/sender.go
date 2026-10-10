package send

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

// New creates a new Sender instance.
func New(client *whatsmeow.Client, chat types.JID, ctx ...context.Context) *Sender {
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

// Text sends a plain text message without quoting.
func Text(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted ...*waE2E.ContextInfo) error {
	_, err := TextWithID(ctx, client, chat, text, quoted...)
	return err
}

// TextWithID sends a plain text message and returns the assigned message ID.
func TextWithID(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted ...*waE2E.ContextInfo) (string, error) {
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

// TextWithMentions sends a text message with user mentions tagged in ContextInfo.
func TextWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, mentions []types.JID, quoted ...*waE2E.ContextInfo) error {
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

// Reply sends a quoted text reply to a previous message.
func Reply(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted *waE2E.ContextInfo) error {
	_, err := ReplyWithID(ctx, client, chat, text, quoted)
	return err
}

// ReplyWithID sends a quoted text reply and returns the assigned message ID.
func ReplyWithID(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, quoted *waE2E.ContextInfo) (string, error) {
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

// ReplyWithMentions sends a quoted text reply mentioning specified JIDs.
func ReplyWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string, mentions []types.JID, quoted *waE2E.ContextInfo) error {
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

// Methods on *Sender
func (s *Sender) Text(text string) error {
	return Text(s.getContext(), s.Client, s.Chat, text)
}

func (s *Sender) Reply(text string, quoted *waE2E.ContextInfo) error {
	return Reply(s.getContext(), s.Client, s.Chat, text, quoted)
}
