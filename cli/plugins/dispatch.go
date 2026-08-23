package plugins

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	clistore "whatsrook/cli/store"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func DismissBotNamePrompt(ctx context.Context, s *StoreWrapper) {
	if s != nil {
		cliutils.DismissBotNamePrompt(ctx, s.SQLStore)
	}
}

func ResetBotNamePromptDismissed(ctx context.Context, s *StoreWrapper) {
	if s != nil {
		cliutils.ResetBotNamePromptDismissed(ctx, s.SQLStore)
	}
}

func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	RecordRecentMessage(evt)

	if evt == nil || evt.Message == nil || client == nil || client.Store == nil {
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
			slog.Debug("Parsed JSON interactive response ID", "original", text, "extracted_id", respJSON.ID)
			text = respJSON.ID
		}
	}
	slog.Debug("Incoming message received", "chat", chatStr, "sender", senderStr, "is_from_me", evt.Info.IsFromMe, "text", text)

	if after, ok := strings.CutPrefix(text, "cancel_loader_"); ok {
		loaderID := after
		slog.Info("Cancel interactive loader button pressed", "loaderID", loaderID)
		if utils.CancelLoader(loaderID) {
			return true
		}
	}

	// WARook reactive dispatch: button clicks, list selections, and poll votes
	// are resolved before command routing so they work with any prefix configuration.
	{
		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: evt.Info.Sender,
		}
		displayText := extractInteractionDisplayText(evt)
		if text != "" {
			if utils.DispatchButtonClick(cctx, text, displayText) {
				return true
			}
			if utils.DispatchListSelection(cctx, text, displayText) {
				return true
			}
		}
		if evt.Message.GetPollUpdateMessage() != nil {
			if utils.DispatchPollVoteEvent(cctx, evt) {
				return true
			}
		}
	}

	s, okStore := getSQLStore(client)
	if okStore {
		clistore.InitTables(ctx, s.SQLStore)
		if fontStyle, err := s.GetSetting(ctx, "font_style"); err == nil && fontStyle != "" {
			cliutils.SetFontStyle(fontStyle)
		}
	}

	if evt.Message.StickerMessage != nil {
		if handleStickerCommand(ctx, client, evt) {
			return true
		}
	}

	if evt.Info.Chat.Server == "g.us" && okStore {
		slog.Debug("Processing group message", "chat", chatStr, "sender", senderStr)
		clistore.LogGroupMessage(ctx, s.SQLStore, evt.Info.Chat, evt.Info.Sender)
	}

	if handleGroupModeration(ctx, client, evt, text) {
		return true
	}

	if evt.Info.Chat.String() == "status@broadcast" {
		if okStore {
			raw, _ := s.GetSetting(ctx, "autostatussave")
			if raw == "on" && client.Store.ID != nil {
				ownerJID := client.Store.ID.ToNonAD()
				_, _ = client.SendMessage(ctx, ownerJID, evt.Message)
			}
		}
	}

	if (evt.IsViewOnce || evt.IsViewOnceV2 || utils.IsViewOnceMessage(evt.Message)) && okStore {
		raw, _ := s.GetSetting(ctx, "autovv")
		mode, _ := s.GetSetting(ctx, "autovv_mode")
		slog.Info("[AutoVV] Intercepted ViewOnce message",
			"msg_id", evt.Info.ID,
			"chat", evt.Info.Chat.String(),
			"sender", evt.Info.Sender.String(),
			"is_from_me", evt.Info.IsFromMe,
			"push_name", evt.Info.PushName,
			"autovv_setting", raw,
			"autovv_mode", mode,
		)
		if raw == "on" {
			var targetJID types.JID
			if (mode == "public" || mode == "chat") && !evt.Info.Chat.IsEmpty() {
				targetJID = evt.Info.Chat
			} else if client.Store.ID != nil {
				targetJID = client.Store.ID.ToNonAD()
			}

			slog.Info("[AutoVV] Target JID resolved", "target_jid", targetJID.String(), "mode", mode)

			if !targetJID.IsEmpty() {
				go func() {
					err := utils.UnwrapAndSendViewOnceMessage(context.Background(), client, evt.Message, evt.Info.Sender, evt.Info.PushName, targetJID, evt.Info.ID, evt.Info.Chat)
					if err != nil {
						slog.Error("[AutoVV] AutoVV forwarding failed", "chat", evt.Info.Chat.String(), "err", err)
					}
				}()
			}
		}
	}

	if HandleAFKAutoResponse(ctx, client, evt, text) {
		return true
	}

	if isBotMentioned(client, evt) && okStore {
		if mentionProto, err := s.GetSetting(ctx, "mention_proto"); err == nil && mentionProto != "" {
			if msg, err := utils.DecodeProtoMessage(mentionProto); err == nil {
				setReplyContextInfo(msg, evt)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
				return true
			}
		}
	}

	if text == "" {
		return false
	}

	if handleFiltersAndBGM(ctx, client, evt, text) {
		return true
	}

	prefixes := activePrefixes(ctx, client)
	slog.Debug("Checking active prefixes", "prefixes", prefixes, "text", text)

	if okStore {
		senderUser := evt.Info.Sender.ToNonAD().User
		awaitingInput, _ := s.GetSetting(ctx, cliutils.BotNameAwaitingInputPrefix+senderUser)
		if awaitingInput == "true" {
			newName := strings.TrimSpace(text)
			if newName != "" {
				_ = s.PutSetting(ctx, cliutils.BotNameSettingKey, newName)
				DismissBotNamePrompt(ctx, s)
				_ = s.PutSetting(ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")

				p := prefixes[0]
				if p == "" {
					p = cliutils.DefaultPrefix
				}
				cctx := &Context{
					Ctx:    ctx,
					Client: client,
					Evt:    evt,
					Chat:   evt.Info.Chat,
					Sender: evt.Info.Sender,
				}
				_ = cctx.Replyf("Bot name updated successfully to \"%s\"! 🎉\n\nYou can change it anytime later using the %sbotname command (e.g. `%sbotname <name>`).", newName, p, p)
				return true
			}
		}
	}

	hasEmpty := false

	for _, p := range prefixes {
		if p == "" {
			hasEmpty = true
			continue
		}
		if matchesPrefix(text, p) {
			body := strings.TrimLeft(strings.TrimSpace(text[len(p):]), ",:;! \t")
			slog.Debug("Prefix matched, executing command", "prefix", p, "body", body)
			if runCommand(ctx, client, evt, body) {
				return true
			}
		}
	}

	trimmedText := strings.TrimSpace(text)
	if IsTTTGameActive(chatStr) && len(trimmedText) == 1 && trimmedText >= "1" && trimmedText <= "9" {
		slog.Debug("Direct move matched active Tic-Tac-Toe game", "chat", chatStr, "move", trimmedText)
		return runCommand(ctx, client, evt, "ttt "+trimmedText)
	}

	cctx := &Context{
		Ctx:    ctx,
		Client: client,
		Evt:    evt,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
	}

	if HandleShellInput(cctx, text) {
		return true
	}

	if cliutils.GetUnscrambleGame(chatStr) != nil {
		if HandleUnscrambleLobbyInput(cctx, text) {
			return true
		}
		if HandleUnscrambleInput(cctx, text) {
			return true
		}
	}

	if cliutils.GetWCGGame(chatStr) != nil {
		if HandleWCGLobbyInput(cctx, text) {
			return true
		}
		if HandleWCGInput(cctx, text) {
			return true
		}
	}

	if hasEmpty {
		body := strings.TrimSpace(text)
		fields := strings.Fields(body)
		if len(fields) > 0 {
			first := fields[0]
			if _, exists := Get(strings.ToLower(first)); exists {
				slog.Debug("Direct command matched (empty prefix)", "command", first, "body", body)
				return runCommand(ctx, client, evt, body)
			}
			for _, p := range activePrefixes(ctx, client) {
				if p != "" && strings.HasPrefix(first, p) {
					strippedName := first[len(p):]
					if _, exists := Get(strings.ToLower(strippedName)); exists {
						strippedBody := strings.TrimSpace(body[len(p):])
						slog.Debug("Configured prefix matched", "prefix", p, "command", strippedName, "body", strippedBody)
						return runCommand(ctx, client, evt, strippedBody)
					}
				}
			}
		}
	}

	slog.Debug("No command prefix matched", "text", text)

	if okStore {
		autoAIVal, _ := s.GetSetting(ctx, "autoai:"+chatStr)
		if autoAIVal == "" {
			autoAIVal, _ = s.GetSetting(ctx, "autoai")
		}
		if autoAIVal == "on" && isBotTaggedOrReplied(client, evt, text) {
			slog.Debug("AutoAI triggered by tag/reply/prefix", "chat", chatStr, "sender", senderStr)

			prompt := text
			for _, p := range prefixes {
				if p != "" && matchesPrefix(text, p) {
					prompt = strings.TrimSpace(text[len(p):])
					break
				}
			}

			if client.Store.ID != nil {
				if ourJID := client.Store.ID.ToNonAD(); ourJID.User != "" {
					prompt = strings.TrimSpace(strings.ReplaceAll(prompt, "@"+ourJID.User, ""))
				}
			}
			if ourLID := client.Store.LID.ToNonAD(); ourLID.User != "" {
				prompt = strings.TrimSpace(strings.ReplaceAll(prompt, "@"+ourLID.User, ""))
			}

			botName := GetBotName(ctx, client)
			if botName != "" && strings.HasPrefix(strings.ToLower(prompt), strings.ToLower(botName)) {
				prompt = strings.TrimSpace(prompt[len(botName):])
			}

			if prompt == "" {
				prompt = text
			}

			go func() {
				reqCtx, cancel := context.WithCancel(ctx)
				defer cancel()

				cctx := &Context{
					Ctx:        reqCtx,
					CancelFunc: cancel,
					Client:     client,
					Evt:        evt,
					Command:    "ai",
					Args:       strings.Fields(prompt),
					RawArgs:    prompt,
					Chat:       evt.Info.Chat,
					Sender:     evt.Info.Sender,
				}

				cctx.StartAutoLoader()
				defer cctx.StopAutoLoader()

				if cmd, ok := Get("ai"); ok {
					if err := cmd.Handler(cctx); err != nil {
						slog.Error("AutoAI command handler failed", "err", err)
					}
				}
			}()
			return true
		}
	}

	return false
}

func activePrefixes(ctx context.Context, client *whatsmeow.Client) []string {
	s, ok := getSQLStore(client)
	if !ok {
		return []string{cliutils.DefaultPrefix}
	}
	raw, err := s.GetSetting(ctx, cliutils.PrefixSettingKey)
	if err != nil || raw == "" {
		return []string{cliutils.DefaultPrefix}
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
			out = append(out, "")
		} else {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{cliutils.DefaultPrefix}
	}
	return out
}

func matchesPrefix(text, p string) bool {
	if p == "" {
		return false
	}

	lowerText := strings.ToLower(text)
	lowerP := strings.ToLower(p)

	if !strings.HasPrefix(lowerText, lowerP) {
		return false
	}

	if isWordPrefix(p) {
		rem := text[len(p):]
		if len(rem) == 0 {
			return true
		}
		firstRune, _ := utf8.DecodeRuneInString(rem)
		if unicode.IsLetter(firstRune) || unicode.IsNumber(firstRune) {
			return false
		}
	}

	return true
}

func runCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message, body string) bool {
	if body == "" {
		slog.Debug("Empty command body, skipping execution", "chat", evt.Info.Chat.String())
		return false
	}
	if isSenderBanned(ctx, client, evt.Info.Sender) {
		slog.Warn("Sender is banned, ignoring command", "sender", evt.Info.Sender.String(), "chat", evt.Info.Chat.String())
		return false
	}

	fields := strings.Fields(body)
	name := strings.ToLower(fields[0])
	args := fields[1:]

	cmd, ok := Get(name)
	if !ok {
		if len(fields) > 1 {
			for i := 1; i < len(fields); i++ {
				subName := strings.ToLower(fields[i])
				if subCmd, subOk := Get(subName); subOk {
					name = subName
					cmd = subCmd
					ok = true
					args = fields[i+1:]
					break
				}
			}
		}
	}
	if !ok {
		slog.Debug("Command not found", "name", name, "chat", evt.Info.Chat.String())
		return false
	}

	s, okSetting := getSQLStore(client)
	if okSetting {
		botName := GetBotName(ctx, client)
		if strings.EqualFold(botName, "whatsrook") || strings.EqualFold(botName, "rook") {
			sessionKey := s.JID
			if client.Store != nil && client.Store.ID != nil {
				sessionKey = client.Store.ID.ToNonAD().String()
			}
			cliutils.BotNamePromptDismissedCacheMu.RLock()
			dismissedInMem := cliutils.BotNamePromptDismissedCache[sessionKey] || (s.JID != "" && cliutils.BotNamePromptDismissedCache[s.JID])
			cliutils.BotNamePromptDismissedCacheMu.RUnlock()

			dismissed := dismissedInMem
			if !dismissedInMem {
				val, _ := s.GetSetting(ctx, cliutils.BotNamePromptDismissedKey)
				dismissed = (val == "true")
				if dismissed {
					cliutils.BotNamePromptDismissedCacheMu.Lock()
					cliutils.BotNamePromptDismissedCache[sessionKey] = true
					if s.JID != "" {
						cliutils.BotNamePromptDismissedCache[s.JID] = true
					}
					cliutils.BotNamePromptDismissedCacheMu.Unlock()
				}
			}

			if !dismissed {
				cmdWord := strings.ToLower(name)
				if cmdWord != "botname" && cmdWord != "setbotname" && cmdWord != "setname" && cmdWord != "name" && cmdWord != "setbot" && cmdWord != "reconfigure" && cmdWord != "reconfig" && cmdWord != "setupwizard" {
					cctx := &Context{
						Ctx:    ctx,
						Client: client,
						Evt:    evt,
						Chat:   evt.Info.Chat,
						Sender: evt.Info.Sender,
					}
					p := activePrefixes(ctx, client)[0]
					if p == "" {
						p = cliutils.DefaultPrefix
					}
					bodyText := "BOT NAME CUSTOMIZATION RECOMMENDED\n\nIt's highly recommended to give your own copy of WhatsRook its own name!\nFor example, you can name it something like Fuzzy or Meow.\n\nYou can also run " + p + "reconfigure anytime to open the setup wizard."
					buttons := []struct{ ID, Text string }{
						{ID: p + "setbot setup_customize", Text: "Customize Bot"},
						{ID: p + "setbot setup_continue", Text: "Continue"},
					}
					_ = sendInteractiveButtons(cctx, bodyText, Sprintf("Powered by %s", botName), buttons)
					return true
				}
			}
		}
	}

	rawArgs := ""
	if idx := strings.Index(body, fields[0]); idx == 0 {
		rawArgs = strings.TrimSpace(body[len(fields[0]):])
	}

	go func() {
		reqCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		cctx := &Context{
			Ctx:        reqCtx,
			CancelFunc: cancel,
			Client:     client,
			Evt:        evt,
			Command:    name,
			Args:       args,
			RawArgs:    rawArgs,
			Chat:       evt.Info.Chat,
			Sender:     evt.Info.Sender,
		}
		if cmd.GroupOnly && cctx.Chat.Server != "g.us" {
			slog.Warn("Group-only command executed in non-group chat JID", "command", name, "chat", cctx.Chat.String())
			_ = cctx.Reply("This command can only be used in a group chat.")
			return
		}

		if okSetting {
			botMode, _ := s.GetSetting(ctx, "mode")
			if botMode == "private" && !cctx.IsSudo() {
				slog.Warn("Private mode check failed - silently ignoring non-sudoer", "command", name, "sender", cctx.Sender.String())
				return
			}
		}

		if !cmd.IsPublic && !cctx.IsSudo() {
			slog.Warn("Sudoer command check failed", "command", name, "sender", cctx.Sender.String())
			_ = cctx.Reply("This command is restricted to sudoers/owners only.")
			return
		}

		if okSetting {
			raw, _ := s.GetSetting(ctx, "disabled_commands")
			if raw != "" {
				isDisabled := false
				for disabled := range strings.FieldsSeq(raw) {
					if strings.EqualFold(disabled, name) {
						isDisabled = true
						break
					}
				}
				if isDisabled {
					slog.Warn("Disabled command check failed", "command", name)
					_ = cctx.Replyf(" Command %q is currently disabled.", name)
					return
				}
			}
		}

		slog.Debug("Executing command", "command", name, "chat", cctx.Chat.String(), "sender", cctx.Sender.String(), "args", cctx.Args)
		cctx.StartAutoLoader()
		defer cctx.StopAutoLoader()

		if err := cmd.Handler(cctx); err != nil {
			LogHandlerErrWithContext(cctx, name, err)
		} else {
			slog.Debug("Command completed successfully", "command", name)
		}
	}()

	return true
}

func extractText(evt *events.Message) string {
	if conv := evt.Message.GetConversation(); conv != "" {
		return conv
	}
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if btnResp := evt.Message.GetButtonsResponseMessage(); btnResp != nil {
		if id := btnResp.GetSelectedButtonID(); id != "" {
			return id
		}
		return btnResp.GetSelectedDisplayText()
	}
	if templateResp := evt.Message.GetTemplateButtonReplyMessage(); templateResp != nil {
		if id := templateResp.GetSelectedID(); id != "" {
			return id
		}
		return templateResp.GetSelectedDisplayText()
	}
	if interactiveResp := evt.Message.GetInteractiveResponseMessage(); interactiveResp != nil {
		if nativeFlow := interactiveResp.GetNativeFlowResponseMessage(); nativeFlow != nil {
			if params := nativeFlow.GetParamsJSON(); params != "" {
				var respJSON struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal([]byte(params), &respJSON); err == nil && respJSON.ID != "" {
					return respJSON.ID
				}
				return params
			}
		}
		if body := interactiveResp.GetBody(); body != nil {
			return body.GetText()
		}
	}
	if listResp := evt.Message.GetListResponseMessage(); listResp != nil {
		if singleSelect := listResp.GetSingleSelectReply(); singleSelect != nil {
			return singleSelect.GetSelectedRowID()
		}
	}
	return ""
}

func isSenderBanned(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client.Store.ID == nil {
		return false
	}
	ownerJID := client.Store.ID.ToNonAD()
	senderJID := sender.ToNonAD()
	if senderJID == ownerJID {
		return false
	}

	s, ok := getSQLStore(client)
	if !ok {
		return false
	}

	rawSudo, _ := s.GetSetting(ctx, "sudoers")
	for sudoerStr := range strings.FieldsSeq(rawSudo) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if senderJID == sudoerJID.ToNonAD() {
				return false
			}
		}
	}

	rawBanned, _ := s.GetSetting(ctx, "banned_users")
	for bannedStr := range strings.FieldsSeq(rawBanned) {
		bannedJID, err := types.ParseJID(bannedStr)
		if err == nil {
			if senderJID == bannedJID.ToNonAD() {
				return true
			}
		}
	}

	return false
}

func setReplyContextInfo(msg *waE2E.Message, evt *events.Message) {
	stanzaID := evt.Info.ID
	participant := evt.Info.Sender.ToNonAD().String()
	ci := &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		Participant:   &participant,
		QuotedMessage: evt.Message,
	}

	if msg.ExtendedTextMessage != nil {
		msg.ExtendedTextMessage.ContextInfo = ci
	} else if msg.ImageMessage != nil {
		msg.ImageMessage.ContextInfo = ci
	} else if msg.VideoMessage != nil {
		msg.VideoMessage.ContextInfo = ci
	} else if msg.AudioMessage != nil {
		msg.AudioMessage.ContextInfo = ci
	} else if msg.DocumentMessage != nil {
		msg.DocumentMessage.ContextInfo = ci
	} else if msg.StickerMessage != nil {
		msg.StickerMessage.ContextInfo = ci
	}
}

func handleFiltersAndBGM(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if evt.Info.Chat.Server == "g.us" {
		return false
	}
	if isSenderBanned(ctx, client, evt.Info.Sender) {
		return false
	}
	s, ok := getSQLStore(client)
	if !ok {
		return false
	}
	db := s.GetDB()
	if db == nil {
		return false
	}

	ourJID := client.Store.ID.ToNonAD().String()
	trigger := strings.TrimSpace(strings.ToLower(text))

	var bgmProto string
	err := db.QueryRow(ctx, `SELECT message_proto FROM bot_bgm WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger).Scan(&bgmProto)
	if err == nil && bgmProto != "" {
		if msg, err := utils.DecodeProtoMessage(bgmProto); err == nil {
			setReplyContextInfo(msg, evt)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	var filterProto string
	err = db.QueryRow(ctx, `SELECT message_proto FROM bot_filters WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger).Scan(&filterProto)
	if err == nil && filterProto != "" {
		if msg, err := utils.DecodeProtoMessage(filterProto); err == nil {
			setReplyContextInfo(msg, evt)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	return false
}

func checkSpamLimit(chatStr, senderStr string, maxMsgs int) bool {
	cliutils.SpamTrackMu.Lock()
	defer cliutils.SpamTrackMu.Unlock()

	key := chatStr + ":" + senderStr
	now := time.Now()
	cutoff := now.Add(-5 * time.Second)

	history := cliutils.SpamHistory[key]
	var recent []time.Time
	for _, t := range history {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	cliutils.SpamHistory[key] = recent

	return len(recent) > maxMsgs
}

func handleGroupModeration(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if evt.Info.Chat.Server != "g.us" {
		return false
	}
	if HandlePendingCaptchaReply(ctx, client, evt) {
		return true
	}
	s, ok := getSQLStore(client)
	if !ok {
		return false
	}

	chatStr := evt.Info.Chat.String()
	sender := evt.Info.Sender.ToNonAD()

	if !utils.IsSudoRaw(ctx, client, sender) {
		rawAntiMsgStatus, _ := s.GetSetting(ctx, "antimsg_status:"+chatStr)
		if rawAntiMsgStatus == "on" {
			rawAntiMsgUsers, _ := s.GetSetting(ctx, "antimsg_users:"+chatStr)
			if rawAntiMsgUsers != "" {
				targetUsers := strings.Split(rawAntiMsgUsers, ",")
				senderStr := sender.String()
				for _, uStr := range targetUsers {
					uStr = strings.TrimSpace(uStr)
					if uStr == "" {
						continue
					}
					uJID, err := types.ParseJID(uStr)
					if err != nil {
						continue
					}
					if utils.IsSameUserRaw(ctx, client, uJID, evt.Info.Sender) {
						slog.Debug("antimsg: deleting message from targeted participant", "chat", chatStr, "sender", senderStr)
						_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
						return true
					}
				}
			}
		}
	}

	rawAntiSpamStatus, _ := s.GetSetting(ctx, "antispam_status:"+chatStr)
	if rawAntiSpamStatus == "on" {
		info, err := client.GetGroupInfo(ctx, evt.Info.Chat)
		if err == nil && !utils.IsAdminRaw(ctx, client, info, sender) {
			rawMax, _ := s.GetSetting(ctx, "antispam_max:"+chatStr)
			maxMsgs, _ := strconv.Atoi(rawMax)
			if maxMsgs <= 0 {
				maxMsgs = 5
			}
			if checkSpamLimit(chatStr, sender.String(), maxMsgs) {
				action, _ := s.GetSetting(ctx, "antispam_action:"+chatStr)
				if action == "" {
					action = "delete"
				}
				slog.Debug("antispam: message rate limit exceeded", "chat", chatStr, "sender", sender.String(), "action", action)
				botIsAdmin := false
				if client.Store.ID != nil {
					botIsAdmin = utils.IsAdminRaw(ctx, client, info, *client.Store.ID)
				}
				if botIsAdmin {
					_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
					if action == "kick" {
						_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
					}
					resolvedJID, username := utils.ResolveMentionRaw(ctx, client, evt.Info.Sender)
					textMsg := Sprintf("AntiSpam: @%s message rate limit exceeded (action: %s).", username, action)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
					return true
				}
			}
		}
	}

	antiLinkEnabled := false
	rawLink, _ := s.GetSetting(ctx, "antilink:"+chatStr)
	if rawLink == "on" {
		antiLinkEnabled = true
	}

	var bannedWords []string
	rawWord, _ := s.GetSetting(ctx, "antiword:"+chatStr)
	if rawWord != "" {
		bannedWords = strings.Fields(strings.ToLower(rawWord))
	}

	if !antiLinkEnabled && len(bannedWords) == 0 {
		return false
	}

	info, err := client.GetGroupInfo(ctx, evt.Info.Chat)
	if err != nil {
		return false
	}

	if utils.IsAdminRaw(ctx, client, info, sender) || utils.IsSudoRaw(ctx, client, sender) {
		return false
	}

	violation := false
	reason := ""
	violationType := ""

	if antiLinkEnabled {
		lowerText := strings.ToLower(text)
		mode, _ := s.GetSetting(ctx, "antilink_mode:"+chatStr)
		if mode == "custom" {
			customStr, _ := s.GetSetting(ctx, "antilink_custom:"+chatStr)
			if customStr == "" {
				customStr = "chat.whatsapp.com"
			}
			domains := strings.SplitSeq(customStr, ",")
			for d := range domains {
				d = strings.TrimSpace(strings.ToLower(d))
				if d != "" && strings.Contains(lowerText, d) {
					violation = true
					reason = Sprintf("banned link (%s)", d)
					violationType = "antilink"
					break
				}
			}
		} else {
			if strings.Contains(lowerText, "http://") || strings.Contains(lowerText, "https://") || strings.Contains(lowerText, "www.") || strings.Contains(lowerText, ".com") || strings.Contains(lowerText, ".net") || strings.Contains(lowerText, ".org") {
				violation = true
				reason = "links"
				violationType = "antilink"
			}
		}
	}

	if !violation && len(bannedWords) > 0 {
		lowerText := strings.ToLower(text)
		for _, w := range bannedWords {
			if strings.Contains(lowerText, w) {
				violation = true
				reason = Sprintf("banned word (%s)", w)
				violationType = "antiword"
				break
			}
		}
	}

	if violation {
		botIsAdmin := false
		if client.Store.ID != nil {
			botIsAdmin = utils.IsAdminRaw(ctx, client, info, *client.Store.ID)
		}

		if botIsAdmin {
			_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
			resolvedJID, username := utils.ResolveMentionRaw(ctx, client, evt.Info.Sender)

			actionKey := violationType + "_action:" + chatStr
			action, _ := s.GetSetting(ctx, actionKey)
			action = strings.ToLower(strings.TrimSpace(action))

			switch action {
			case "kick":
				_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
				textMsg := Sprintf("Message from @%s deleted and participant kicked: contains %s.", username, reason)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &textMsg,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})

			case "warn":
				maxWarnKey := violationType + "_maxwarn:" + chatStr
				maxWarnStr, _ := s.GetSetting(ctx, maxWarnKey)
				maxWarn := 3
				if parsed, err := strconv.Atoi(maxWarnStr); err == nil && parsed > 0 {
					maxWarn = parsed
				}

				warnsKey := violationType + "_warns:" + chatStr + ":" + evt.Info.Sender.ToNonAD().String()
				currWarnStr, _ := s.GetSetting(ctx, warnsKey)
				currWarns := 0
				if parsed, err := strconv.Atoi(currWarnStr); err == nil {
					currWarns = parsed
				}
				currWarns++

				if currWarns >= maxWarn {
					_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
					_ = s.PutSetting(ctx, warnsKey, "0")
					textMsg := Sprintf("⚠️ @%s reached maximum warnings (%d/%d) for %s! Message deleted and participant kicked.", username, currWarns, maxWarn, reason)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				} else {
					_ = s.PutSetting(ctx, warnsKey, strconv.Itoa(currWarns))
					textMsg := Sprintf("⚠️ Warning for @%s (%d/%d): Message deleted for %s. Reaching %d warnings will result in a kick!", username, currWarns, maxWarn, reason, maxWarn)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				}

			default:
				textMsg := Sprintf("Message from @%s deleted: contains %s.", username, reason)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &textMsg,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})
			}
			return true
		}
	}

	return false
}

func isBotMentioned(client *whatsmeow.Client, evt *events.Message) bool {
	if client.Store.ID == nil {
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
	if ourJID.Server == types.DefaultUserServer && client.Store.LIDs != nil {
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

func handleStickerCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	stk := evt.Message.StickerMessage
	if stk == nil || len(stk.FileSHA256) == 0 {
		return false
	}

	s, ok := getSQLStore(client)
	if !ok {
		return false
	}
	db := s.GetDB()
	if db == nil {
		return false
	}

	ourJID := client.Store.ID.ToNonAD().String()
	shaHex := hex.EncodeToString(stk.FileSHA256)

	var cmdName string
	err := db.QueryRow(ctx, `SELECT command_name FROM bot_sticker_cmds WHERE our_jid=$1 AND sticker_sha256=$2`, ourJID, shaHex).Scan(&cmdName)
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
			quotedText := extractTextFromProto(ci.QuotedMessage)
			if quotedText != "" {
				args = strings.Fields(quotedText)
				rawArgs = quotedText
			}
		}
	} else if ci := stk.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
		quotedText := extractTextFromProto(ci.QuotedMessage)
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
		botMode, _ := s.GetSetting(ctx, "mode")
		if botMode == "private" && !cctx.IsSudo() {
			_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
			return
		}

		raw, _ := s.GetSetting(ctx, "disabled_commands")
		if raw != "" {
			for disabled := range strings.FieldsSeq(raw) {
				if strings.EqualFold(disabled, cmdName) {
					_ = cctx.Replyf(" Command %q is currently disabled.", cmdName)
					return
				}
			}
		}

		if err := cmd.Handler(cctx); err != nil {
			logHandlerErr(cmdName, err)
		}
	}()

	return true
}

func extractTextFromProto(msg *waE2E.Message) string {
	return utils.ExtractTextFromProto(msg)
}

func getQuotedMessageFromEvent(evt *events.Message) *waE2E.Message {
	if evt == nil || evt.Message == nil {
		return nil
	}
	ci := utils.GetContextInfoFromProto(evt.Message)
	if ci != nil {
		return ci.QuotedMessage
	}
	return nil
}

func isBotTaggedOrReplied(client *whatsmeow.Client, evt *events.Message, text string) bool {
	if client.Store.ID == nil {
		return false
	}
	if evt.Info.Chat.Server != "g.us" {
		return true
	}
	ourJID := client.Store.ID.ToNonAD()
	ourLID := client.Store.LID.ToNonAD()

	lowerText := strings.ToLower(text)
	botName := GetBotName(context.Background(), client)
	lowerBotName := strings.ToLower(botName)

	if (lowerBotName != "" && strings.Contains(lowerText, lowerBotName)) || strings.Contains(lowerText, "whatsrook") || strings.Contains(lowerText, "rook") {
		return true
	}

	if strings.Contains(text, "@"+ourJID.User) || (!ourLID.IsEmpty() && strings.Contains(text, "@"+ourLID.User)) {
		return true
	}

	var ctxInfo *waE2E.ContextInfo
	if evt.Message.GetExtendedTextMessage() != nil {
		ctxInfo = evt.Message.GetExtendedTextMessage().ContextInfo
	} else if evt.Message.GetImageMessage() != nil {
		ctxInfo = evt.Message.GetImageMessage().ContextInfo
	} else if evt.Message.GetVideoMessage() != nil {
		ctxInfo = evt.Message.GetVideoMessage().ContextInfo
	} else if evt.Message.GetAudioMessage() != nil {
		ctxInfo = evt.Message.GetAudioMessage().ContextInfo
	} else if evt.Message.GetDocumentMessage() != nil {
		ctxInfo = evt.Message.GetDocumentMessage().ContextInfo
	}

	if ctxInfo == nil {
		return false
	}

	for _, m := range ctxInfo.MentionedJID {
		if parseJID, err := types.ParseJID(m); err == nil {
			nonAD := parseJID.ToNonAD()
			if nonAD == ourJID || (!ourLID.IsEmpty() && nonAD == ourLID) {
				return true
			}
		}
	}

	if ctxInfo.Participant != nil {
		if parseJID, err := types.ParseJID(*ctxInfo.Participant); err == nil {
			nonAD := parseJID.ToNonAD()
			if nonAD == ourJID || (!ourLID.IsEmpty() && nonAD == ourLID) {
				return true
			}
		}
	}

	return false
}

// extractInteractionDisplayText returns the human-readable label for the
// selected button, list row, or template reply from the incoming event.
// Returns empty string for non-interactive messages.
func extractInteractionDisplayText(evt *events.Message) string {
	if btnResp := evt.Message.GetButtonsResponseMessage(); btnResp != nil {
		return btnResp.GetSelectedDisplayText()
	}
	if listResp := evt.Message.GetListResponseMessage(); listResp != nil {
		return listResp.GetTitle()
	}
	if tmplResp := evt.Message.GetTemplateButtonReplyMessage(); tmplResp != nil {
		return tmplResp.GetSelectedDisplayText()
	}
	if interResp := evt.Message.GetInteractiveResponseMessage(); interResp != nil {
		if body := interResp.GetBody(); body != nil {
			return body.GetText()
		}
	}
	return ""
}
