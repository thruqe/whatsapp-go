package plugins

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	clistore "whatsrook/cmd/store"
	utils "whatsrook/src"
)

type StoreWrapper struct {
	*sqlstore.SQLStore
}

func wrap(s *sqlstore.SQLStore) *StoreWrapper {
	if s == nil {
		return nil
	}
	return &StoreWrapper{SQLStore: s}
}

func (w *StoreWrapper) GetSetting(ctx context.Context, key string) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return clistore.GetSetting(ctx, w.SQLStore, key)
}

func (w *StoreWrapper) PutSetting(ctx context.Context, key, value string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return clistore.PutSetting(ctx, w.SQLStore, key, value)
}

func (w *StoreWrapper) DeleteSetting(ctx context.Context, key string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return clistore.DeleteSetting(ctx, w.SQLStore, key)
}

func (w *StoreWrapper) GetCallMediaConfig(ctx context.Context, jid types.JID, kind clistore.CallMediaKind) (string, error) {
	if w == nil || w.SQLStore == nil {
		return "", nil
	}
	return clistore.GetCallMediaConfig(ctx, w.SQLStore, jid, kind)
}

func (w *StoreWrapper) PutCallMediaConfig(ctx context.Context, jid types.JID, kind clistore.CallMediaKind, filePath string) error {
	if w == nil || w.SQLStore == nil {
		return nil
	}
	return clistore.PutCallMediaConfig(ctx, w.SQLStore, jid, kind, filePath)
}

func getSQLStore(client *whatsmeow.Client) (*StoreWrapper, bool) {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return nil, false
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok || s == nil {
		return nil, false
	}
	return wrap(s), true
}

func getStore(ctx *Context) (*StoreWrapper, bool) {
	if ctx == nil {
		return nil, false
	}
	return getSQLStore(ctx.Client)
}

func sendText(ctx *Context, text string) error {
	return ctx.Rook().NewMessage(text).Send()
}

// sendTextRaw is like sendText but usable before a *Context exists (e.g. inside
// HandlePendingAudioReply, which runs ahead of normal command dispatch).
func sendTextRaw(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string) error {
	pctx := &Context{Ctx: ctx, Client: client, Chat: chat}
	return pctx.Rook().NewMessage(text).To(chat).Send()
}

func sendInteractiveButtonsWithMentions(ctx *Context, bodyText, footerText string, buttons []struct{ ID, Text string }, jids []types.JID) error {
	builder := ctx.Rook().NewButton(bodyText).Footer(footerText).Mentions(jids...)
	for _, b := range buttons {
		builder.Add(b.ID, b.Text)
	}
	return builder.Send(ctx.Chat)
}

func sendInteractiveButtons(ctx *Context, bodyText, footerText string, buttons []struct{ ID, Text string }) error {
	return sendInteractiveButtonsWithMentions(ctx, bodyText, footerText, buttons, nil)
}

func isWordPrefix(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

func GetBotName(ctx context.Context, client *whatsmeow.Client) string {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return "WhatsRook"
	}
	s, ok := getSQLStore(client)
	if !ok {
		return "WhatsRook"
	}
	raw, err := s.GetSetting(ctx, "bot_name")
	if err != nil || strings.TrimSpace(raw) == "" {
		return "WhatsRook"
	}
	return strings.TrimSpace(raw)
}

func NormalizeUserJID(ctx context.Context, client *whatsmeow.Client, jid types.JID) types.JID {
	return utils.ResolvePN(ctx, client, jid)
}

func resolveUserPushName(ctx *Context, pnjid types.JID, rawJID types.JID) string {
	if !rawJID.IsEmpty() && ctx.Evt != nil && ctx.Evt.Info.Sender.ToNonAD().User == rawJID.ToNonAD().User && ctx.Evt.Info.PushName != "" {
		return ctx.Evt.Info.PushName
	}

	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, pnjid); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			}
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.BusinessName != "" {
				return contact.BusinessName
			}
		}
		// Fallback to raw LID contact lookup if PN contact was not found
		if rawJID != pnjid && !rawJID.IsEmpty() {
			if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, rawJID); err == nil && contact.Found {
				if contact.PushName != "" {
					return contact.PushName
				}
				if contact.FullName != "" {
					return contact.FullName
				}
				if contact.BusinessName != "" {
					return contact.BusinessName
				}
			}
		}
	}

	if pnjid.User != "" {
		return pnjid.User
	}
	return "User"
}
