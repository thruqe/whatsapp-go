package plugins

import (
	"encoding/base64"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	cliutils "whatsrook/cli/utils"
)

func init() {
	Register(&Command{
		Name:        "why",
		Alias:       "whycom",
		Description: "Ask why.com ultimate search for direct answers and deep-dive exploration",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleWhy,
	})
	Register(&Command{
		Name:        "ai",
		Alias:       "ask",
		Description: "Ask Meta AI a question.",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAI,
	})
	Register(&Command{
		Name:        "autoai",
		Alias:       "aai",
		Description: "Toggle automatic AI responses when tagged, replied to, or when 'Rook' or 'WhatsRook' is mentioned in this chat (on/off)",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAutoAI,
	})
	Register(&Command{
		Name:        "csai",
		Alias:       "customai",
		Description: "Configure global AI personality traits and relationship behavior (Sudoers only)",
		Category:    "ai",
		IsPublic:    false,
		Handler:     handleCSAI,
	})

	Register(&Command{
		Name:         "send",
		Alias:        "say",
		Description:  "Send raw text message to the current chat",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleSend,
	})
	Register(&Command{
		Name:         "edit",
		Alias:        "editmsg",
		Description:  "Edit a message by message ID or replied message",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleEditMsg,
	})
	Register(&Command{
		Name:         "ffmpeg",
		Alias:        "ff",
		Description:  "Run raw ffmpeg media command",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     false,
		Handler:      handleFFmpeg,
	})
	Register(&Command{
		Name:         "dlmsg",
		Alias:        "downloadMessage",
		Description:  "Download media from a message by ID or quoted message",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleDownloadMessage,
	})
}

func handleAutoAI(ctx *Context) error {
	slog.Debug("handleAutoAI started", "args", ctx.Args)

	isAuthorized := ctx.IsSudo()
	if !isAuthorized && ctx.Chat.Server == "g.us" {
		info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && info != nil {
			if ctx.IsSenderAdmin(info) {
				isAuthorized = true
			}
		}
	}

	if !isAuthorized {
		return ctx.Reply("Only sudoers or group admins can change the AutoAI setting.")
	}

	s, okStore := getStore(ctx)
	if !okStore {
		return ctx.Reply("Database store is not available.")
	}

	settingKey := "autoai:" + ctx.Chat.String()

	if len(ctx.Args) == 0 {
		current, _ := s.GetSetting(ctx.Ctx, settingKey)
		if current == "" {
			current = "off"
		}
		return ctx.Replyf("AutoAI is currently %s in this chat.", current)
	}

	val := strings.ToLower(ctx.Args[0])
	if val != "on" && val != "off" {
		return ctx.Replyf("Usage: %sautoai [on/off]", ctx.GetPrefix())
	}

	if err := s.PutSetting(ctx.Ctx, settingKey, val); err != nil {
		slog.Error("failed to update autoai setting", "err", err)
		return ctx.Reply("Failed to update setting: " + err.Error())
	}

	return ctx.Replyf("AutoAI has been set to %s for this chat.", val)
}

func handleCSAI(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only Sudoers can configure global AI personality traits and custom behavior.")
	}

	s, okStore := getStore(ctx)
	if !okStore {
		return ctx.Reply("Database store is not available.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return renderCSAIPage(ctx, s, 1)
	}

	if len(ctx.Args) >= 1 && strings.HasPrefix(strings.ToLower(ctx.Args[0]), "set_") {
		idxStr := strings.TrimPrefix(strings.ToLower(ctx.Args[0]), "set_")
		idxVal, err := strconv.Atoi(idxStr)
		if err == nil && idxVal >= 1 && idxVal <= len(cliutils.DefaultCSAITraits) {
			trait := cliutils.DefaultCSAITraits[idxVal-1]
			if err := s.PutSetting(ctx.Ctx, "csai_prompt", trait.Instruction); err != nil {
				return ctx.Reply("Failed to save AI personality trait.")
			}
			return ctx.Replyf("Saved AI personality trait to %s!\n\nInstruction: %s", trait.Name, trait.Instruction)
		}
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "custom" {
		customPrompt := strings.TrimSpace(strings.Join(ctx.Args[1:], " "))
		if customPrompt == "" {
			return ctx.Replyf("Usage: `%scsai custom <your prompt / how to refer to you>`\n\nExample: `%scsai custom Always refer to me as Chief and be extremely respectful.`", p, p)
		}
		if err := s.PutSetting(ctx.Ctx, "csai_prompt", customPrompt); err != nil {
			return ctx.Reply("Failed to save custom AI prompt.")
		}
		return ctx.Replyf("Saved custom AI personality prompt!\n\nCustom Prompt: %s", customPrompt)
	}

	if len(ctx.Args) >= 1 && strings.ToLower(ctx.Args[0]) == "reset" {
		_ = s.DeleteSetting(ctx.Ctx, "csai_prompt")
		return ctx.Reply("AI personality prompt has been reset to default.")
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "page" {
		pageNum, _ := strconv.Atoi(ctx.Args[1])
		return renderCSAIPage(ctx, s, pageNum)
	}

	if len(ctx.Args) > 0 {
		subCmd := strings.ToLower(ctx.Args[0])
		if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal >= 1 && idxVal <= len(cliutils.DefaultCSAITraits) {
			trait := cliutils.DefaultCSAITraits[idxVal-1]
			_ = s.PutSetting(ctx.Ctx, "csai_prompt", trait.Instruction)
			return ctx.Replyf("Saved AI personality trait to %s!\n\nInstruction: %s", trait.Name, trait.Instruction)
		}
		if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal == 11 {
			return ctx.Replyf("To set a custom trait/prompt, please type:\n`%scsai custom <your custom prompt / how you want the AI to refer to you>`\n\nExample:\n`%scsai custom Always refer to me as Boss and be concise.`", p, p)
		}

		customPrompt := strings.TrimSpace(ctx.RawArgs)
		if err := s.PutSetting(ctx.Ctx, "csai_prompt", customPrompt); err != nil {
			return ctx.Reply("Failed to save custom AI prompt.")
		}
		return ctx.Replyf("Saved custom AI personality prompt!\n\nCustom Prompt: %s", customPrompt)
	}

	return renderCSAIPage(ctx, s, 1)
}

func renderCSAIPage(ctx *Context, s *StoreWrapper, page int) error {
	currentPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt")
	if currentPrompt == "" {
		currentPrompt = "Standard (Default Meta AI behavior)"
	}

	pageSize := 3
	totalPages := (len(cliutils.DefaultCSAITraits) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := min(startIdx+pageSize, len(cliutils.DefaultCSAITraits))

	pageItems := cliutils.DefaultCSAITraits[startIdx:endIdx]
	p := ctx.GetPrefix()

	tb := ctx.Text().
		Headerf("Custom AI Personality & Trait Configuration (Page %d of %d)", page, totalPages).
		Field("Active AI Trait/Prompt", currentPrompt).
		Blank().
		Line("Select a personality trait for Meta AI below:").
		Blank()

	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		tb.Numberedf(globalIdx, "%s: %s", Bold(trait.Name), trait.Instruction)
	}
	tb.Numbered(11, "Custom Trait / How You Refer To Me: Enter your own custom prompt.")

	var buttons []struct{ ID, Text string }
	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		btnText := Sprintf("%d. %s", globalIdx, trait.Name)
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%scsai set %d", p, globalIdx),
			Text: btnText,
		})
	}

	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%scsai page %d", p, nextPage),
			Text: Sprintf("Next (Page %d)", nextPage),
		})
	} else {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%scsai custom", p),
			Text: "11. Custom Trait",
		})
	}

	tb.Blank().
		Line("To select a personality, tap a button above or type:").
		Bulletf("%scsai <number> (e.g. %scsai 3)", p, p).
		Bulletf("%scsai custom <prompt> (e.g. %scsai custom Refer to me as Sir)", p, p).
		Bulletf("%scsai reset (to restore default AI behavior)", p)

	return sendInteractiveButtons(ctx, tb.Trimmed(), Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func isMediaGenerationPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	return strings.HasPrefix(p, "image of") || strings.HasPrefix(p, "generate an image") ||
		strings.HasPrefix(p, "generate a photo") || strings.HasPrefix(p, "create an image") ||
		strings.HasPrefix(p, "generate image") || strings.HasPrefix(p, "create image") ||
		strings.HasPrefix(p, "draw ") || strings.HasPrefix(p, "picture of") ||
		strings.HasPrefix(p, "photo of") || strings.HasPrefix(p, "make an image") ||
		strings.HasPrefix(p, "make a picture") || strings.HasPrefix(p, "video of") ||
		strings.HasPrefix(p, "generate a video") || strings.HasPrefix(p, "generate video") ||
		strings.HasPrefix(p, "create a video") || strings.HasPrefix(p, "create video") ||
		strings.HasPrefix(p, "animate ") || strings.HasPrefix(p, "make a video") ||
		strings.Contains(p, "generate an image") || strings.Contains(p, "generate a video") ||
		strings.Contains(p, "create an image") || strings.Contains(p, "create a video")
}

func handleAI(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sai <question>\n- %sask <question>\n\nExamples:\n- %sai What is the speed of light?\n- %sask Explain quantum computing in simple terms\n- Reply to an image or message with %sai Analyze this", p, p, p, p, p)
	}

	botName := ctx.GetBotName()
	p := ctx.GetPrefix()
	instruction := cliutils.GetOrBuildInstructionWithNameAndPrefix(botName, p, func() string {
		cmdInfos := ListCommands()
		metaCmds := make([]cliutils.CommandInfo, 0, len(cmdInfos))
		for _, info := range cmdInfos {
			metaCmds = append(metaCmds, cliutils.CommandInfo{
				Name:        info.Name,
				Alias:       info.Alias,
				Description: info.Description,
				IsPublic:    info.IsPublic,
			})
		}
		return cliutils.BuildRunCommandInstructionWithNameAndPrefix(metaCmds, botName, p)
	})

	pushName := ""
	msgID := ""
	if ctx.Evt != nil {
		if ctx.Evt.Info.PushName != "" {
			pushName = ctx.Evt.Info.PushName
		}
		msgID = ctx.Evt.Info.ID
	}
	if pushName == "" {
		senderPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, ctx.Sender)
		pushName = resolveUserPushName(ctx, senderPNJID, ctx.Sender)
	}

	data := cliutils.Data{
		ChatID:    ctx.Chat.String(),
		Question:  ctx.RawArgs,
		MessageID: msgID,
		User:      ctx.Sender,
		PushName:  pushName,
		IsSudo:    ctx.IsSudo(),
	}

	isGroup := ctx.Chat.Server == "g.us"

	if isGroup {
		data.ChatType = "group"
		groupInfo, err := cliutils.GetOrFetchGroupMeta(ctx.Chat.String(), func() (types.GroupInfo, error) {
			info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
			if err != nil || info == nil {
				return types.GroupInfo{}, err
			}
			return *info, nil
		})
		if err == nil {
			data.GroupMetaData = groupInfo
		}
	} else {
		data.ChatType = "direct"
	}

	extractContextFromQuotedMessage(ctx, &data)

	var query string
	isMediaReq := isMediaGenerationPrompt(data.Question)
	if isMediaReq {
		query = data.Question
	} else {
		query = instruction
		if s, okStore := getStore(ctx); okStore {
			if customPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt"); customPrompt != "" {
				query += Sprintf("\n\n[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]\n%s\n\n", customPrompt)
			}
		}
		if isGroup {
			query += cliutils.RenderGroupContext(data.GroupMetaData)
		}
		query += cliutils.RenderUserContext(data)
		query += cliutils.RenderQuotedContext(data)
		query += data.Question
	}

	slog.Debug("handleAI: sending request to Meta AI", "chat", ctx.Chat.String(), "is_media_req", isMediaReq)

	var placeholderMsgID types.MessageID
	onUpdate := func(text string) error {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || cliutils.IsDummyPlaceholderText(trimmed) {
			return nil
		}
		if _, _, ok := cliutils.ParseRunCommand(trimmed); ok {
			return nil
		}
		if isMediaReq {
			return nil
		}
		if placeholderMsgID == "" {
			id, err := ctx.ReplyWithID(text)
			if err == nil {
				placeholderMsgID = id
			}
			return err
		}
		_, err := ctx.Edit(placeholderMsgID, text)
		if err != nil {
			slog.Error("handleAI: failed to send edit", "chat", ctx.Chat.String(), "err", err)
		}
		return err
	}

	res, err := cliutils.QueryMetaAi(ctx.Ctx, ctx.Client, ctx.Chat, query, onUpdate)
	if err != nil {
		slog.Error("handleAI: queryMetaAi failed", "chat", ctx.Chat.String(), "err", err)
		if strings.Contains(err.Error(), "488") {
			errMsg := "Meta AI session initialization required.\n\nPlease make sure you have manually started a direct 1-on-1 chat/conversation with Meta AI on WhatsApp first before WhatsRook can interact with it."
			if placeholderMsgID != "" {
				_, _ = ctx.Edit(placeholderMsgID, errMsg)
			} else {
				_ = ctx.Reply(errMsg)
			}

			metaName := "Meta AI"
			metaVcard := Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:AI;Meta;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", metaName, cliutils.MetaAiBotJID.User, cliutils.MetaAiBotJID.User)
			contactMsg := &waE2E.Message{
				ContactMessage: &waE2E.ContactMessage{
					DisplayName: &metaName,
					Vcard:       &metaVcard,
				},
			}
			_, _ = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, contactMsg)
			return err
		}

		if placeholderMsgID != "" {
			_, _ = ctx.Edit(placeholderMsgID, "Failed to get a response: "+err.Error())
		} else {
			_ = ctx.Reply("Failed to get a response: " + err.Error())
		}
		return err
	}

	reply := res.Text
	mediaBytes := res.GeneratedMedia
	if len(mediaBytes) == 0 {
		mediaBytes = res.GeneratedImg
	}

	if len(mediaBytes) > 0 {
		mType := res.MediaMimeType
		if mType == "" {
			mType = res.ImgMimeType
		}
		if mType == "" {
			mType = "image/jpeg"
		}
		caption := res.MediaCaption
		if caption == "" {
			caption = res.ImgCaption
		}
		if caption == "" {
			caption = reply
		}

		if placeholderMsgID != "" {
			_, _ = ctx.Delete(placeholderMsgID)
			placeholderMsgID = ""
		}

		if strings.HasPrefix(mType, "video/") {
			slog.Debug("handleAI: sending generated video message to chat", "chat", ctx.Chat.String(), "video_len", len(mediaBytes), "mime", mType)
			_ = ctx.ReplyWithVideo(mediaBytes, mType, caption)
		} else {
			slog.Debug("handleAI: sending generated image message to chat", "chat", ctx.Chat.String(), "img_len", len(mediaBytes), "mime", mType)
			_ = ctx.ReplyWithImage(mediaBytes, mType, caption)
		}
	} else if placeholderMsgID == "" && reply != "" {
		if _, _, ok := cliutils.ParseRunCommand(reply); !ok {
			_ = ctx.Reply(reply)
		}
	}

	if cmdName, rawArgs, ok := cliutils.ParseRunCommand(reply); ok {
		if cmdName == "sh" || cmdName == "exec" || cmdName == "run" || cmdName == "shell" {
			if !ctx.IsSudo() {
				slog.Warn("handleAI: blocked unauthorized shell execution request", "sender", ctx.Sender.String())
				_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run shell commands.")
				return nil
			}

			output, err := cliutils.RunCmd(rawArgs)
			if err != nil && output == "" {
				output = err.Error()
			}
			if output == "" {
				output = "(no output)"
			}

			resText := Sprintf("Output:\n```\n%s\n```", output)
			_, err = ctx.Edit(placeholderMsgID, resText)
			return err
		}

		if cmdName == "ai" || cmdName == "autoai" || cmdName == "gpt" || cmdName == "ask" {
			slog.Warn("handleAI: blocked recursive AI command execution", "command", cmdName)
			_, err := ctx.Edit(placeholderMsgID, "Recursive AI command execution is not allowed.")
			return err
		}

		targetCmd, exists := Get(cmdName)
		if !exists {
			slog.Warn("handleAI: RUN_COMMAND referenced unknown command", "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "Sorry, I don't have a command called \""+cmdName+"\".")
			return nil
		}

		if !targetCmd.IsPublic && !ctx.IsSudo() {
			slog.Warn("handleAI: blocked unauthorized RUN_COMMAND", "sender", ctx.Sender.String(), "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run this command.")
			return nil
		}

		if placeholderMsgID != "" {
			_, _ = ctx.Delete(placeholderMsgID)
			placeholderMsgID = ""
		}

		cctx := &Context{
			Ctx:     ctx.Ctx,
			Client:  ctx.Client,
			Evt:     ctx.Evt,
			Command: cmdName,
			Args:    strings.Fields(rawArgs),
			RawArgs: rawArgs,
			Chat:    ctx.Chat,
			Sender:  ctx.Sender,
		}
		slog.Debug("handleAI: executing command on behalf of AI", "command", cmdName, "args", ctx.Args)
		return targetCmd.Handler(cctx)
	}

	slog.Debug("handleAI: completed successfully", "chat", ctx.Chat.String())
	return nil
}

func extractContextFromQuotedMessage(ctx *Context, data *cliutils.Data) {
	if ctx.Evt == nil {
		return
	}
	quotedMsg := getQuotedMessageFromEvent(ctx.Evt)
	if quotedMsg == nil {
		return
	}

	var quotedParticipant string
	msg := ctx.Evt.Message
	var ci *waE2E.ContextInfo
	switch {
	case msg.GetExtendedTextMessage() != nil:
		ci = msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		ci = msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		ci = msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		ci = msg.GetAudioMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		ci = msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		ci = msg.GetStickerMessage().GetContextInfo()
	}
	if ci != nil {
		quotedParticipant = ci.GetParticipant()
		if ci.StanzaID != nil {
			data.QuotedMessageID = *ci.StanzaID
		}
	}

	if quotedParticipant != "" {
		if quotedJID, err := types.ParseJID(quotedParticipant); err == nil {
			quotedPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, quotedJID)
			quotedPushName := resolveUserPushName(ctx, quotedPNJID, quotedJID)
			data.UserOfQuotedMessage = quotedPushName
			if data.ChatType == "group" {
				for _, p := range data.GroupMetaData.Participants {
					if p.JID.User == quotedJID.User || p.JID.User == quotedPNJID.User {
						switch {
						case p.IsSuperAdmin:
							data.QuotedMessageParticipantRole = "Super Admin"
						case p.IsAdmin:
							data.QuotedMessageParticipantRole = "Admin"
						default:
							data.QuotedMessageParticipantRole = "Member"
						}
						break
					}
				}
			}
		}
	}

	switch {
	case quotedMsg.GetConversation() != "":
		data.QuotedMessageType = "Text"
		data.QuotedMessageOfQuestion = quotedMsg.GetConversation()

	case quotedMsg.GetExtendedTextMessage() != nil:
		data.QuotedMessageType = "Text"
		data.QuotedMessageOfQuestion = quotedMsg.GetExtendedTextMessage().GetText()

	case quotedMsg.GetImageMessage() != nil:
		imgMsg := quotedMsg.GetImageMessage()
		data.QuotedMessageType = "Image"
		data.QuotedMessageOfQuestion = imgMsg.GetCaption()
		mimetype := imgMsg.GetMimetype()
		if mimetype == "" {
			mimetype = "image/jpeg"
		}
		data.QuotedImageMimeType = mimetype

		imgData, err := ctx.Client.Download(ctx.Ctx, imgMsg)
		if err == nil && len(imgData) > 0 {
			data.QuotedImageBase64 = base64.StdEncoding.EncodeToString(imgData)
			slog.Debug("extractContextFromQuotedMessage: extracted image base64", "len", len(data.QuotedImageBase64))
		} else {
			slog.Warn("extractContextFromQuotedMessage: failed to download quoted image", "err", err)
		}

	case quotedMsg.GetVideoMessage() != nil:
		vidMsg := quotedMsg.GetVideoMessage()
		data.QuotedMessageType = "Video"
		caption := vidMsg.GetCaption()
		if caption != "" {
			data.QuotedMessageOfQuestion = Sprintf("[Video message. Note: Video file reading is not supported yet. Caption: %s]", caption)
		} else {
			data.QuotedMessageOfQuestion = "[Video message. Note: Video file reading is not supported yet.]"
		}

	case quotedMsg.GetAudioMessage() != nil:
		data.QuotedMessageType = "Audio"
		data.QuotedMessageOfQuestion = "[Voice/Audio message]"

	case quotedMsg.GetDocumentMessage() != nil:
		docMsg := quotedMsg.GetDocumentMessage()
		data.QuotedMessageType = "Document"
		caption := docMsg.GetCaption()
		filename := docMsg.GetFileName()
		if filename != "" {
			data.QuotedMessageOfQuestion = Sprintf("File: %s. Caption: %s", filename, caption)
		} else {
			data.QuotedMessageOfQuestion = caption
		}

	case quotedMsg.GetStickerMessage() != nil:
		stkMsg := quotedMsg.GetStickerMessage()
		if stkMsg.GetIsAnimated() {
			data.QuotedMessageType = "Animated Sticker"
			data.QuotedMessageOfQuestion = "[Animated/Video sticker message. Note: Animated or video stickers are not supported yet.]"
		} else {
			data.QuotedMessageType = "Sticker"
			data.QuotedMessageOfQuestion = "[Sticker image]"
			mimetype := stkMsg.GetMimetype()
			if mimetype == "" {
				mimetype = "image/webp"
			}
			data.QuotedImageMimeType = mimetype

			stkData, err := ctx.Client.Download(ctx.Ctx, stkMsg)
			if err == nil && len(stkData) > 0 {
				data.QuotedImageBase64 = base64.StdEncoding.EncodeToString(stkData)
				slog.Debug("extractContextFromQuotedMessage: extracted sticker image base64", "len", len(data.QuotedImageBase64))
			} else {
				slog.Warn("extractContextFromQuotedMessage: failed to download quoted sticker image", "err", err)
			}
		}

	case quotedMsg.GetPollCreationMessage() != nil || quotedMsg.GetPollCreationMessageV2() != nil || quotedMsg.GetPollCreationMessageV3() != nil:
		data.QuotedMessageType = "Poll"
		var pollName string
		var options []string
		if p := quotedMsg.GetPollCreationMessage(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		} else if p := quotedMsg.GetPollCreationMessageV2(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		} else if p := quotedMsg.GetPollCreationMessageV3(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		}
		data.QuotedMessageOfQuestion = Sprintf("Poll Question: %s. Options: %s", pollName, strings.Join(options, ", "))

	case quotedMsg.GetLocationMessage() != nil:
		locMsg := quotedMsg.GetLocationMessage()
		data.QuotedMessageType = "Location"
		data.QuotedMessageOfQuestion = Sprintf("Location: %f, %f (%s)", locMsg.GetDegreesLatitude(), locMsg.GetDegreesLongitude(), locMsg.GetName())

	case quotedMsg.GetContactMessage() != nil:
		contMsg := quotedMsg.GetContactMessage()
		data.QuotedMessageType = "Contact"
		data.QuotedMessageOfQuestion = Sprintf("Contact: %s", contMsg.GetDisplayName())

	default:
		if txt := extractTextFromProto(quotedMsg); txt != "" {
			data.QuotedMessageType = "Other"
			data.QuotedMessageOfQuestion = txt
		}
	}
}

func handleSend(ctx *Context) error {
	if ctx.RawArgs == "" {
		return ctx.Reply("Usage: send <text>")
	}
	return ctx.SendText(ctx.RawArgs)
}

func handleEditMsg(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: edit <msg_id> <new_text> or reply to a message with edit <new_text>")
	}

	var targetID types.MessageID
	var newText string

	ci := ctx.GetContextInfo()
	quotedSender, hasQuoted := ctx.GetQuotedSender()

	if len(ctx.Args) >= 2 && len(ctx.Args[0]) > 6 {
		targetID = types.MessageID(ctx.Args[0])
		newText = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	} else if ci != nil && ci.StanzaID != nil {
		targetID = *ci.StanzaID
		newText = ctx.RawArgs
	} else {
		targetID = types.MessageID(ctx.Args[0])
		newText = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	}

	if hasQuoted {
		if ctx.Client.Store.ID != nil && !ctx.IsSameUser(quotedSender, *ctx.Client.Store.ID) {
			return ctx.Reply("You can only edit messages sent by the bot (fromMe=true).")
		}
	}

	if newText == "" {
		return ctx.Reply("Please provide the new text for the message.")
	}

	_, err := ctx.Edit(targetID, newText)
	if err != nil {
		return ctx.Reply("Failed to edit message: " + err.Error())
	}
	return nil
}

func handleFFmpeg(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers/owner only.")
	}
	if ctx.RawArgs == "" {
		return ctx.Reply("Usage: ffmpeg <args...>")
	}

	mediaData, mimetype, err := ctx.GetMedia()
	if err == nil && len(mediaData) > 0 {
		ext := ".tmp"
		if strings.Contains(mimetype, "image/") {
			ext = "." + strings.TrimPrefix(mimetype, "image/")
		} else if strings.Contains(mimetype, "video/") {
			ext = "." + strings.TrimPrefix(mimetype, "video/")
		} else if strings.Contains(mimetype, "audio/") {
			ext = "." + strings.TrimPrefix(mimetype, "audio/")
		}

		tmpFile, err := os.CreateTemp("", "ffmpeg_input_*"+ext)
		if err != nil {
			return ctx.Reply("Failed to create temporary file: " + err.Error())
		}
		defer os.Remove(tmpFile.Name())
		_, _ = tmpFile.Write(mediaData)
		_ = tmpFile.Close()

		outFile := tmpFile.Name() + ".out.mp4"
		if strings.Contains(ctx.RawArgs, ".mp3") {
			outFile = tmpFile.Name() + ".out.mp3"
		} else if strings.Contains(ctx.RawArgs, ".webp") {
			outFile = tmpFile.Name() + ".out.webp"
		} else if strings.Contains(ctx.RawArgs, ".png") {
			outFile = tmpFile.Name() + ".out.png"
		} else if strings.Contains(ctx.RawArgs, ".jpg") || strings.Contains(ctx.RawArgs, ".jpeg") {
			outFile = tmpFile.Name() + ".out.jpg"
		}
		defer os.Remove(outFile)

		rawCmd := ctx.RawArgs
		if strings.Contains(rawCmd, "{input}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{input}", tmpFile.Name())
		} else if strings.Contains(rawCmd, "{in}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{in}", tmpFile.Name())
		}
		if strings.Contains(rawCmd, "{output}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{output}", outFile)
		} else if strings.Contains(rawCmd, "{out}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{out}", outFile)
		}

		parts := strings.Fields(rawCmd)
		cmd := exec.Command("ffmpeg", parts...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ctx.Replyf("FFmpeg execution error: %v\nOutput: %s", err, string(out))
		}

		if outBytes, err := os.ReadFile(outFile); err == nil && len(outBytes) > 0 {
			if strings.HasSuffix(outFile, ".mp3") || strings.HasSuffix(outFile, ".ogg") {
				return ctx.SendAudio(outBytes, "audio/ogg; codecs=opus")
			} else if strings.HasSuffix(outFile, ".webp") {
				return ctx.SendSticker(outBytes)
			} else if strings.HasSuffix(outFile, ".png") || strings.HasSuffix(outFile, ".jpg") {
				return ctx.SendImage(outBytes, "image/jpeg", "FFmpeg processed image")
			} else {
				return ctx.SendVideo(outBytes, "video/mp4", "FFmpeg processed video")
			}
		}

		resStr := string(out)
		if len(resStr) > 1500 {
			resStr = resStr[:1500] + "\n... (truncated)"
		}
		if resStr == "" {
			resStr = "FFmpeg command completed successfully."
		}
		return ctx.Reply(resStr)
	}

	parts := strings.Fields(ctx.RawArgs)
	cmd := exec.Command("ffmpeg", parts...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ctx.Replyf("FFmpeg execution error: %v\nOutput: %s", err, string(out))
	}
	resStr := string(out)
	if len(resStr) > 1500 {
		resStr = resStr[:1500] + "\n... (truncated)"
	}
	if resStr == "" {
		resStr = "FFmpeg command completed successfully."
	}
	return ctx.Reply(resStr)
}

func handleDownloadMessage(ctx *Context) error {
	mediaData, mimetype, err := ctx.GetMedia()
	if err != nil || len(mediaData) == 0 {
		return ctx.Reply("No downloadable media found in the target or quoted message.")
	}

	switch {
	case strings.HasPrefix(mimetype, "image/webp"):
		return ctx.SendSticker(mediaData)
	case strings.HasPrefix(mimetype, "image/"):
		return ctx.SendImage(mediaData, mimetype, "Downloaded image")
	case strings.HasPrefix(mimetype, "video/"):
		return ctx.SendVideo(mediaData, mimetype, "Downloaded video")
	case strings.HasPrefix(mimetype, "audio/"):
		return ctx.SendAudio(mediaData, mimetype)
	default:
		return ctx.SendDocument(mediaData, mimetype, "downloaded_media", "")
	}
}

func handleWhy(ctx *Context) error {
	rawQuery := strings.TrimSpace(ctx.RawArgs)
	if rawQuery == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			rawQuery = strings.TrimSpace(extractTextFromProto(quoted))
		}
	}

	if rawQuery == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %swhy <question>\n- Reply to a message with %swhy\n\nExample:\n- %swhy what makes aging impossible to reverse", p, p, p)
	}

	page := 1
	query := rawQuery
	fields := strings.Fields(rawQuery)
	if len(fields) >= 2 && strings.EqualFold(fields[0], "page") {
		if pNum, err := strconv.Atoi(fields[1]); err == nil && pNum >= 1 {
			page = pNum
			query = strings.TrimSpace(strings.TrimPrefix(rawQuery, fields[0]+" "+fields[1]))
		}
	}
	if query == "" {
		query = rawQuery
	}

	ctx.StartAutoLoader()
	defer ctx.StopAutoLoader()

	res, err := cliutils.QueryWhy(ctx.Ctx, query)
	if err != nil {
		slog.Error("handleWhy failed", "query", query, "err", err)
		return ctx.Replyf("Error querying why.com: %v", err)
	}

	if res == nil || res.Answer == "" {
		return ctx.Reply("No answer received from why.com.")
	}

	const pageSize = 3
	totalPulls := len(res.Pulls)
	totalPages := 1
	if totalPulls > 0 {
		totalPages = (totalPulls + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}

	startIdx := (page - 1) * pageSize
	endIdx := min(startIdx+pageSize, totalPulls)
	var pagePulls []cliutils.WhyPull
	if totalPulls > 0 && startIdx < totalPulls {
		pagePulls = res.Pulls[startIdx:endIdx]
	}

	p := ctx.GetPrefix()
	tb := ctx.Text(res.Answer)

	if totalPulls > 0 {
		tb.Blank()
		if totalPages > 1 {
			tb.Linef("Related Questions (Page %d of %d):", page, totalPages)
		} else {
			tb.Line("Related Questions:")
		}
		for i, pull := range res.Pulls {
			label := strings.TrimSpace(pull.Label)
			if label == "" {
				label = strings.TrimSpace(pull.Query)
			}
			if label != "" {
				tb.Numbered(i+1, label)
			}
		}
	}

	var buttons []struct{ ID, Text string }
	for i, pull := range pagePulls {
		globalIdx := startIdx + i + 1
		btnQuery := strings.TrimSpace(pull.Query)
		if btnQuery == "" {
			btnQuery = strings.TrimSpace(pull.Label)
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%swhy %s", p, btnQuery),
			Text: Sprintf("Question %d", globalIdx),
		})
	}

	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%swhy page %d %s", p, nextPage, query),
			Text: "Next",
		})
	} else if totalPages > 1 && page == totalPages {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%swhy page 1 %s", p, query),
			Text: "First Page",
		})
	}

	if len(buttons) > 0 {
		footer := Sprintf("Powered by why.com • %s", ctx.GetBotName())
		if err := sendInteractiveButtons(ctx, tb.String(), footer, buttons); err == nil {
			return nil
		}
	}

	return ctx.Reply(tb.String())
}
