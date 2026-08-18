package plugins

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	"whatsrook/utils"
)

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
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
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
