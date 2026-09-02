// dispatch package provides the primary runtime message router and command execution pipeline.
//
// it intercepts incoming WhatsApp message events, parses message bodies against active command prefixes,
// checks chat permissions (public mode, group restrictions, admin requirements, sudo/owner authorization),
// displays animated loaders for long-running operations, and dispatches to registered native handlers
// or external plugins.
package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	utils "whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/external"
	Logger "whatsrook/logger"
	"whatsrook/system"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

// Dispatch evaluates an incoming message event against all registered commands and routing middleware.
// returns true if the message was handled by a command or reactive route.
func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	RecordRecentMessage(evt)

	if evt == nil || evt.Message == nil || client == nil || client.Store == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return false
	}

	if evt.Info.Category == "peer" {
		return false
	}

	chatStr := evt.Info.Chat.String()
	senderStr := evt.Info.Sender.String()
	text := extractText(evt)

	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		var respJSON struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(text), &respJSON); err == nil && respJSON.ID != "" {
			text = respJSON.ID
		}
	}

	Logger.Debug("Incoming message received", "chat", chatStr, "sender", senderStr, "is_from_me", evt.Info.IsFromMe, "text", text)

	if after, ok := strings.CutPrefix(text, "cancel_loader_"); ok {
		loaderID := after
		if utils.CancelLoader(loaderID) {
			return true
		}
	}

	// Reactive dispatch: list button responses and poll votes
	cctx := &Context{
		Ctx:    ctx,
		Client: client,
		Evt:    evt,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
	}

	displayText := extractInteractionDisplayText(evt)
	if text != "" {
		if utils.DispatchListSelection(cctx, text, displayText) {
			return true
		}
	}
	if pollUpdate := evt.Message.GetPollUpdateMessage(); pollUpdate != nil {
		if utils.DispatchPollVoteEvent(cctx, evt) {
			return true
		}
	}

	s, okStore := GetSQLStore(client)
	if okStore {
		store.InitTables(ctx, s.SQLStore)
	}

	if evt.Info.Chat.Server == "g.us" && okStore {
		store.LogGroupMessage(ctx, s.SQLStore, evt.Info.Chat, evt.Info.Sender)
	}

	if text == "" {
		return false
	}

	prefixes := activePrefixes(ctx, client)
	Logger.Debug("Checking active prefixes", "prefixes", prefixes, "text", text)

	hasEmpty := false
	for _, p := range prefixes {
		if p == "" {
			hasEmpty = true
			continue
		}
		if matchesPrefix(text, p) {
			body := strings.TrimLeft(strings.TrimSpace(text[len(p):]), ",:;! \t")
			if runCommand(ctx, client, evt, body) {
				return true
			}
		}
	}

	if hasEmpty {
		body := strings.TrimSpace(text)
		fields := strings.Fields(body)
		if len(fields) > 0 {
			first := fields[0]
			if _, exists := Get(strings.ToLower(first)); exists {
				return runCommand(ctx, client, evt, body)
			}
		}
	}

	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) > 0 {
		cmdName := strings.ToLower(fields[0])
		args := fields[1:]
		rawArgs := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
		if external.DefaultDispatcher.IsInstalled(cmdName) {
			return external.DefaultDispatcher.Dispatch(ctx, client, evt, cmdName, args, rawArgs)
		}
	}
	return false
}

func runCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message, cmdLine string) bool {
	fields := strings.Fields(cmdLine)
	if len(fields) == 0 {
		return false
	}

	cmdName := strings.ToLower(fields[0])
	args := fields[1:]
	rawArgs := strings.TrimSpace(strings.TrimPrefix(cmdLine, fields[0]))

	cmd, exists := Get(cmdName)
	if !exists {
		if external.DefaultDispatcher.IsInstalled(cmdName) {
			return external.DefaultDispatcher.Dispatch(ctx, client, evt, cmdName, args, rawArgs)
		}
		return false
	}

	cctx := &Context{
		Ctx:     ctx,
		Client:  client,
		Evt:     evt,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
		Command: cmdName,
		Args:    args,
		RawArgs: rawArgs,
	}

	// Permission checks
	if cmd.GroupOnly && evt.Info.Chat.Server != "g.us" {
		_ = cctx.Reply("⚠️ This command can only be used in group chats.")
		return true
	}

	if !cmd.IsPublic && !cctx.IsSudo() {
		_ = cctx.Reply("⚠️ This command is restricted to sudoers/bot owners only.")
		return true
	}

	if !cmd.NoLoader {
		cctx.StartAutoLoader(1200 * time.Millisecond)
	}

	go func() {
		defer func() {
			cctx.StopAutoLoader()
			if r := recover(); r != nil {
				crashPath := system.RecordCrash(r, "command: "+cmdName, "user: "+cctx.Sender.String(), "chat: "+cctx.Chat.String())
				Logger.Error("Panic recovered in command handler", "command", cmdName, "panic", r, "crash_log", crashPath)
				_ = cctx.Reply("⚠️ An unexpected internal error occurred while executing this command.")
			}
		}()

		err := cmd.Handler(cctx)
		if err != nil {
			LogHandlerErrWithContext(cctx, cmdName, err)
			_ = cctx.Reply(fmt.Sprintf("⚠️ %v", err))
		}
	}()

	return true
}

func activePrefixes(ctx context.Context, client *whatsmeow.Client) []string {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return []string{"."}
	}
	s, ok := client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return []string{"."}
	}
	raw, err := s.GetSetting(ctx, "prefix")
	if err != nil || raw == "" {
		return []string{"."}
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return []string{"."}
	}
	var res []string
	for _, p := range parts {
		if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
			res = append(res, "")
		} else {
			res = append(res, p)
		}
	}
	return res
}

func matchesPrefix(text, prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.HasPrefix(text, prefix) {
		return true
	}
	return false
}

func extractText(evt *events.Message) string {
	if evt == nil || evt.Message == nil {
		return ""
	}
	return utils.ExtractMessageText(evt)
}

func extractInteractionDisplayText(evt *events.Message) string {
	if evt == nil || evt.Message == nil {
		return ""
	}
	msg := utils.UnwrapMessageProto(evt.Message)
	if msg == nil {
		return ""
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil {
		return btnResp.GetSelectedDisplayText()
	}
	if listResp := msg.GetListResponseMessage(); listResp != nil {
		return listResp.GetTitle()
	}
	if templResp := msg.GetTemplateButtonReplyMessage(); templResp != nil {
		return templResp.GetSelectedDisplayText()
	}
	return ""
}
