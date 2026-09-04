// dispatch package provides the primary runtime message router and command execution pipeline.
//
// it intercepts incoming WhatsApp message events, parses message bodies against active command prefixes,
// checks chat permissions (public mode, group restrictions, admin requirements, sudo/owner authorization),
// displays animated loaders for long-running operations, and dispatches to registered native handlers
// or external plugins.
package dispatch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	utils "whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/external"
	Logger "whatsrook/logger"
	"whatsrook/system"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
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
	msgProto := utils.UnwrapMessageProto(evt.Message)
	if msgProto == nil {
		msgProto = evt.Message
	}
	if pollUpdate := msgProto.GetPollUpdateMessage(); pollUpdate != nil {
		targetID := ""
		if key := pollUpdate.GetPollCreationMessageKey(); key != nil {
			targetID = key.GetID()
		}
		Logger.Debug("Dispatcher: incoming poll vote message detected",
			"targetPollMsgID", targetID,
			"chat", evt.Info.Chat.String(),
			"sender", evt.Info.Sender.String(),
		)
		if utils.DispatchPollVoteEvent(cctx, evt) {
			Logger.Debug("Dispatcher: poll vote event successfully dispatched to reactive route",
				"targetPollMsgID", targetID,
				"chat", evt.Info.Chat.String(),
				"sender", evt.Info.Sender.String(),
			)
			return true
		}
		Logger.Debug("Dispatcher: poll vote event has no matching reactive route",
			"targetPollMsgID", targetID,
			"chat", evt.Info.Chat.String(),
			"sender", evt.Info.Sender.String(),
		)
	}

	s, okStore := GetSQLStore(client)
	if okStore {
		store.InitTables(ctx, s.SQLStore)
	}

	if evt.Info.Chat.Server == "g.us" && okStore {
		store.LogGroupMessage(ctx, s.SQLStore, evt.Info.Chat, evt.Info.Sender)
	}

	// 1. Status Broadcast Auto-Save
	if (evt.Info.Chat.String() == "status@broadcast" || evt.Info.Chat.Server == "broadcast") && okStore {
		raw, _ := s.GetSetting(ctx, "autostatussave")
		if raw == "on" && client.Store.ID != nil {
			ownerJID := client.Store.ID.ToNonAD()
			_, _ = client.SendMessage(ctx, ownerJID, evt.Message)
		}
	}

	// 2. ViewOnce Auto-Save / Forwarding (autovv)
	if (evt.IsViewOnce || evt.IsViewOnceV2 || utils.IsViewOnceMessage(evt.Message)) && okStore {
		handleAutoViewOnce(ctx, client, s.SQLStore, evt)
	}

	// 3. Asynchronous Post-Processors (AutoRead, AutoReact)
	if okStore && !evt.Info.IsFromMe {
		postProcessorsMu.RLock()
		for _, pp := range postProcessors {
			go pp.fn(ctx, client, s, evt)
		}
		postProcessorsMu.RUnlock()
	}

	// 4. Sticker Commands
	if evt.Message.StickerMessage != nil && okStore {
		if handleStickerCommand(ctx, client, s.SQLStore, evt) {
			return true
		}
	}

	// 5. Bot Tagged / Mention Proto
	if isBotMentioned(client, evt) && okStore {
		if mentionProto, err := s.GetSetting(ctx, "mention_proto"); err == nil && mentionProto != "" {
			if msg, err := utils.DecodeProtoMessage(mentionProto); err == nil {
				setReplyContextInfo(msg, evt)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
				return true
			}
		}
	}

	// 6. Filters & BGM Trigger Words
	if text != "" && okStore {
		if handleFiltersAndBGM(ctx, client, s.SQLStore, evt, text) {
			return true
		}
	}

	// 7. Run registered Pre-Interceptors (Group Moderation, AFK, Games, Shell)
	preInterceptorsMu.RLock()
	preList := make([]interceptorEntry, len(preInterceptors))
	copy(preList, preInterceptors)
	preInterceptorsMu.RUnlock()

	for _, it := range preList {
		if it.fn(cctx, text) {
			return true
		}
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
			fields := strings.Fields(body)
			if len(fields) == 0 {
				continue
			}
			if runCommand(ctx, client, evt, body) {
				return true
			}
			cmdName := strings.ToLower(fields[0])
			if clean := strings.TrimRight(cmdName, ",:;!? \t"); clean != "" {
				cmdName = clean
			}
			if isLikelyCommandName(cmdName) {
				if _, handled := HandleUnknownCommand(cctx, p, cmdName); handled {
					return true
				}
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

	// 9. Run registered Fallback Interceptors (AutoAI)
	fallbackInterceptorsMu.RLock()
	fallbackList := make([]interceptorEntry, len(fallbackInterceptors))
	copy(fallbackList, fallbackInterceptors)
	fallbackInterceptorsMu.RUnlock()

	for _, it := range fallbackList {
		if it.fn(cctx, text) {
			return true
		}
	}

	return false
}

// ResolveSenderMention resolves the triggering sender to an interactive WhatsApp mention tag (@phone)
// and all associated non-AD JIDs (including Phone Number and LID mappings) for ContextInfo.MentionedJID.
func ResolveSenderMention(cctx *Context) (string, []types.JID) {
	if cctx == nil {
		return "@User", nil
	}

	sender := cctx.Sender
	if sender.IsEmpty() && cctx.Evt != nil && !cctx.Evt.Info.Sender.IsEmpty() {
		sender = cctx.Evt.Info.Sender
	}
	if sender.IsEmpty() {
		sender = cctx.Chat
	}
	if sender.IsEmpty() && cctx.Client != nil && cctx.Client.Store != nil && cctx.Client.Store.ID != nil {
		sender = *cctx.Client.Store.ID
	}
	sender = sender.ToNonAD()
	if sender.IsEmpty() || (sender.Server != types.DefaultUserServer && sender.Server != types.HiddenUserServer) {
		return "@User", nil
	}

	ctx := cctx.GetSendContext()
	client := cctx.Client

	pnJID := sender
	var lidJID types.JID

	if sender.Server == types.HiddenUserServer {
		lidJID = sender
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, sender); err == nil && !pn.IsEmpty() {
				pnJID = pn.ToNonAD()
			}
		}
	} else if sender.Server == types.DefaultUserServer {
		pnJID = sender
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if lid, err := client.Store.LIDs.GetLIDForPN(ctx, sender); err == nil && !lid.IsEmpty() {
				lidJID = lid.ToNonAD()
			}
		}
	}

	// In WhatsApp protocol, interactive text mentions require @<phone_number> (the JID User part).
	// WhatsApp clients match this against ContextInfo.MentionedJID to highlight and render the clickable mention.
	tagUser := pnJID.User
	if tagUser == "" {
		tagUser = sender.User
	}
	if tagUser == "" || tagUser == "User" {
		return "@User", nil
	}
	userTag := "@" + tagUser

	var mentions []types.JID
	seen := make(map[string]bool)
	for _, j := range []types.JID{pnJID, lidJID, sender} {
		if !j.IsEmpty() {
			norm := j.ToNonAD()
			key := norm.String()
			if !seen[key] {
				seen[key] = true
				mentions = append(mentions, norm)
			}
		}
	}

	return userTag, mentions
}

// HandleUnknownCommand handles incoming messages where the prefix matched but the command does not exist.
// It finds the closest registered command, formats the suggestion with user mention, and replies.
func HandleUnknownCommand(cctx *Context, prefix, cmdName string) (string, bool) {
	if cctx == nil {
		return "", false
	}

	sendCtx := cctx.GetSendContext()
	if s, okStore := GetSQLStore(cctx.Client); okStore {
		botMode, _ := s.GetSetting(sendCtx, "mode")
		if botMode == "private" && !cctx.IsSudo() {
			_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
			return "The bot is currently in private mode. Only sudoers/owners can use it.", true
		}
	}

	closest := ClosestCommand(cmdName)
	if closest == "" {
		return "", false
	}

	userTag, mentions := ResolveSenderMention(cctx)

	msg := FormatUnknownCommandSuggestion(userTag, prefix, closest)

	Logger.Debug("Dispatcher: unknown command with prefix detected, suggesting closest", "prefix", prefix, "input", cmdName, "suggestion", closest, "user", userTag, "mentions", mentions)
	_ = cctx.ReplyWithMentions(msg, mentions)
	return msg, true
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

	if s, okStore := GetSQLStore(client); okStore {
		botMode, _ := s.GetSetting(ctx, "mode")
		if botMode == "private" && !cctx.IsSudo() {
			_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
			return true
		}

		raw, _ := s.GetSetting(ctx, "disabled_commands")
		if raw != "" {
			for disabled := range strings.FieldsSeq(raw) {
				if strings.EqualFold(disabled, cmdName) {
					_ = cctx.Replyf("⚠️ Command %q is currently disabled.", cmdName)
					return true
				}
			}
		}
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
			_ = cctx.Replyf("⚠️ %v", err)
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

func handleAutoViewOnce(ctx context.Context, client *whatsmeow.Client, s *sqlstore.SQLStore, evt *events.Message) {
	raw, _ := store.GetSetting(ctx, s, "autovv")
	if raw != "on" {
		return
	}
	mode, _ := store.GetSetting(ctx, s, "autovv_mode")
	var targetJID types.JID
	if (mode == "public" || mode == "chat") && !evt.Info.Chat.IsEmpty() {
		targetJID = evt.Info.Chat
	} else if client.Store.ID != nil {
		targetJID = client.Store.ID.ToNonAD()
	}

	if !targetJID.IsEmpty() {
		go func() {
			_ = utils.UnwrapAndSendViewOnceMessage(context.Background(), client, evt.Message, evt.Info.Sender, evt.Info.PushName, targetJID, evt.Info.ID, evt.Info.Chat)
		}()
	}
}

func handleStickerCommand(ctx context.Context, client *whatsmeow.Client, s *sqlstore.SQLStore, evt *events.Message) bool {
	stk := evt.Message.StickerMessage
	if stk == nil || len(stk.FileSHA256) == 0 {
		return false
	}

	shaHex := hex.EncodeToString(stk.FileSHA256)
	cmdName, err := store.GetStickerCmd(ctx, s, shaHex)
	if err != nil || cmdName == "" {
		return false
	}

	cmd, exists := Get(cmdName)
	if !exists {
		return false
	}

	var args []string
	var rawArgs string

	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			quotedText := utils.ExtractTextFromProto(ci.QuotedMessage)
			if quotedText != "" {
				args = strings.Fields(quotedText)
				rawArgs = quotedText
			}
		}
	} else if ci := stk.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
		quotedText := utils.ExtractTextFromProto(ci.QuotedMessage)
		if quotedText != "" {
			args = strings.Fields(quotedText)
			rawArgs = quotedText
		}
	}

	cctx := &Context{
		Ctx:     ctx,
		Client:  client,
		Evt:     evt,
		Command: cmdName,
		Args:    args,
		RawArgs: rawArgs,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
	}

	go func() {
		botMode, _ := store.GetSetting(ctx, s, "mode")
		if botMode == "private" && !cctx.IsSudo() {
			_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
			return
		}

		raw, _ := store.GetSetting(ctx, s, "disabled_commands")
		if raw != "" {
			for disabled := range strings.FieldsSeq(raw) {
				if strings.EqualFold(disabled, cmdName) {
					_ = cctx.Replyf("⚠️ Command %q is currently disabled.", cmdName)
					return
				}
			}
		}

		if err := cmd.Handler(cctx); err != nil {
			LogHandlerErrWithContext(cctx, cmdName, err)
			_ = cctx.Replyf("⚠️ %v", err)
		}
	}()

	return true
}

func isBotMentioned(client *whatsmeow.Client, evt *events.Message) bool {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return false
	}
	ourJID := client.Store.ID.ToNonAD()

	var mentions []string
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil {
			mentions = ci.MentionedJID
		}
	}

	ourLID := ourJID
	if !client.Store.LID.IsEmpty() {
		ourLID = client.Store.LID.ToNonAD()
	} else if ourJID.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(context.Background(), ourJID); err == nil && !lid.IsEmpty() {
			ourLID = lid.ToNonAD()
		}
	} else if ourJID.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(context.Background(), ourJID); err == nil && !pn.IsEmpty() {
			ourLID = pn.ToNonAD()
		}
	}

	for _, m := range mentions {
		mj, err := types.ParseJID(m)
		if err == nil {
			mj = mj.ToNonAD()
			if mj == ourJID || mj == ourLID {
				return true
			}
		}
	}
	return false
}

func handleFiltersAndBGM(ctx context.Context, client *whatsmeow.Client, s *sqlstore.SQLStore, evt *events.Message, text string) bool {
	trigger := strings.TrimSpace(strings.ToLower(text))
	if trigger == "" {
		return false
	}

	bgmProto, err := store.GetBGM(ctx, s, trigger)
	if err == nil && bgmProto != "" {
		if msg, err := utils.DecodeProtoMessage(bgmProto); err == nil {
			setReplyContextInfo(msg, evt)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	filterProto, err := store.GetFilter(ctx, s, trigger)
	if err == nil && filterProto != "" {
		if msg, err := utils.DecodeProtoMessage(filterProto); err == nil {
			setReplyContextInfo(msg, evt)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	return false
}

func setReplyContextInfo(msg *waE2E.Message, evt *events.Message) {
	if msg == nil || evt == nil {
		return
	}
	ci := &waE2E.ContextInfo{
		StanzaID:      &evt.Info.ID,
		Participant:   new(string),
		QuotedMessage: evt.Message,
	}
	senderStr := evt.Info.Sender.ToNonAD().String()
	*ci.Participant = senderStr

	if ext := msg.ExtendedTextMessage; ext != nil {
		ext.ContextInfo = ci
	} else if img := msg.ImageMessage; img != nil {
		img.ContextInfo = ci
	} else if vid := msg.VideoMessage; vid != nil {
		vid.ContextInfo = ci
	} else if aud := msg.AudioMessage; aud != nil {
		aud.ContextInfo = ci
	} else if doc := msg.DocumentMessage; doc != nil {
		doc.ContextInfo = ci
	} else if stk := msg.StickerMessage; stk != nil {
		stk.ContextInfo = ci
	}
}
