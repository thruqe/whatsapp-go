package plugins

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	clistore "whatsrook/cmd/store"
	utils "whatsrook/src"
	Logger "whatsrook/src/logger"
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

// mapOptionToCommandArgs maps a plain poll option label back to its intended subcommand payload.
func mapOptionToCommandArgs(cmdName, option string) string {
	optTrim := strings.TrimSpace(option)
	optLower := strings.ToLower(optTrim)

	// Clean emojis like ▶️, ◀️
	cleaned := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(optLower, "▶️", ""), "◀️", ""))

	// If option contains font indicator e.g. "(#14)"
	if strings.Contains(optLower, "(#") {
		idx := strings.Index(optLower, "(#")
		end := strings.Index(optLower[idx:], ")")
		if end > 2 {
			fontNum := strings.TrimSpace(optLower[idx+2 : idx+end])
			return cmdName + " " + fontNum
		}
	}

	// If option starts with a number like "1. Option Name"
	if strings.Contains(optLower, ".") {
		parts := strings.SplitN(optLower, ".", 2)
		if idx, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && idx > 0 {
			switch cmdName {
			case "timezone":
				return "timezone setidx " + strconv.Itoa(idx)
			case "csai":
				return "csai " + strconv.Itoa(idx)
			case "why":
				rest := strings.TrimSpace(parts[1])
				if rest != "" {
					return "why " + rest
				}
			case "setbot":
				rest := strings.TrimSpace(parts[1])
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					return "setbot " + fields[0]
				}
				return "setbot " + strconv.Itoa(idx)
			default:
				return cmdName + " " + strings.TrimSpace(parts[1])
			}
		}
	}

	// Page navigation
	if strings.HasPrefix(cleaned, "next (page ") || strings.HasPrefix(cleaned, "page ") {
		if _, after, ok := strings.Cut(cleaned, "page "); ok {
			numStr := strings.TrimRight(strings.TrimSpace(after), ")")
			return cmdName + " page " + numStr
		}
	}
	if cleaned == "first page" {
		return cmdName + " page 1"
	}
	if cleaned == "next" {
		if cmdName == "setbot" {
			return "setbot page 2"
		}
		return cmdName + " next"
	}
	if cleaned == "back" {
		if cmdName == "setbot" {
			return "setbot page 1"
		}
		return cmdName + " back"
	}

	switch cleaned {
	case "1 min", "1 minute":
		return cmdName + " time 60"
	case "2 mins", "2 minutes":
		return cmdName + " time 120"
	case "3 mins", "3 minutes":
		return cmdName + " time 180"
	case "activate":
		return cmdName + " activate"
	case "deactivate":
		return cmdName + " deactivate"
	case "action mode":
		return cmdName + " action"
	case "customize", "customize mode":
		switch cmdName {
		case "antilink":
			return "antilink mode"
		case "vv":
			return "vv customize"
		default:
			return cmdName + " customize"
		}
	case "delete only":
		return cmdName + " action delete"
	case "kick user":
		return cmdName + " action kick"
	case "warn user":
		return cmdName + " action warn"
	case "default links":
		return cmdName + " default"
	case "custom urls":
		return cmdName + " custom"
	case "set limit (3)":
		return cmdName + " setwarn 3"
	case "target list", "list words", "list targets":
		return cmdName + " list"
	case "clear targets":
		return cmdName + " clear"
	case "set timeout":
		return cmdName + " time"
	case "help / guide", "guide", "format guide", "help":
		return cmdName + " guide"
	case "preview":
		return cmdName + " preview"
	case "change timezone":
		return "timezone"
	case "set owner dm":
		return "vv dm"
	case "switch to dm (private)":
		return cmdName + " mode dm"
	case "switch to public (chat)":
		return cmdName + " mode public"
	case "call audio", "audio call":
		return "callaudio"
	case "call video", "video call":
		return "callvideo"
	case "voicemail":
		return "voicemail"
	case "start match":
		return cmdName + " start"
	case "leaderboard":
		return cmdName + " leaderboard"
	case "confirm leave":
		return "leave confirm"
	case "cancel":
		return cmdName + " cancel"
	case "wizard", "start wizard", "customize bot":
		return "reconfigure"
	case "keep default":
		return "setbot setup_ignore"
	case "skip":
		return "setbot 0"
	case "continue":
		return "continue"
	default:
		return cmdName + " " + cleaned
	}
}

func sendPollReplyWithMentions(ctx *Context, question string, options []string, jids []types.JID, fn ...func(req PollRequest, res *utils.Response)) error {
	cmdName := ctx.Command
	Logger.Debug("sendPollReplyWithMentions: creating poll reply",
		"command", cmdName,
		"question", question,
		"optionsCount", len(options),
		"options", options,
		"mentionsCount", len(jids),
		"hasCallback", len(fn) > 0 && fn[0] != nil,
	)
	builder := ctx.Rook().NewPoll(question).Mentions(jids...)
	for _, opt := range options {
		builder.AddOption(opt)
	}
	if len(fn) > 0 && fn[0] != nil {
		return builder.Reply(fn[0])
	}

	client := ctx.Client

	return builder.Reply(func(req PollRequest, res *utils.Response) {
		for _, selected := range req.SelectedOptions {
			cmdLine := mapOptionToCommandArgs(cmdName, selected)
			Logger.Debug("Interactive poll selection triggered",
				"command", cmdName,
				"selected", selected,
				"dispatchedCommand", cmdLine,
				"sender", req.Sender.String(),
				"chat", req.Chat.String(),
			)
			if cmdLine == "" || strings.TrimSpace(cmdLine) == cmdName {
				continue
			}
			fakeEvt := &events.Message{
				Info: types.MessageInfo{
					Chat:      req.Chat,
					Sender:    req.Sender,
					IsGroup:   req.Chat.Server == "g.us",
					ID:        req.PollMsgID,
					Timestamp: time.Now(),
				},
				Message: &waE2E.Message{
					Conversation: &cmdLine,
				},
			}
			voteCtx := req.Ctx
			if voteCtx == nil || voteCtx.Err() != nil {
				voteCtx = context.Background()
			}
			runCommand(voteCtx, client, fakeEvt, cmdLine)
		}
	})
}

func sendPollReply(ctx *Context, question string, options []string, fn ...func(req PollRequest, res *utils.Response)) error {
	return sendPollReplyWithMentions(ctx, question, options, nil, fn...)
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
