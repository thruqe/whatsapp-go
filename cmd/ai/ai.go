package ai

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"strconv"
	"strings"

	utils "whatsrook"
	"whatsrook/cmd/dispatch"
	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func resolveUserPushName(ctx *dispatch.Context, pnjid, rawJID types.JID) string {
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
	}
	if pnjid.User != "" {
		return pnjid.User
	}
	return "User"
}

func NormalizeUserJID(_ any, _ any, jid types.JID) types.JID {
	return jid.ToNonAD()
}

func getQuotedMessageFromEvent(evt *events.Message) *waE2E.Message {
	if evt == nil || evt.Message == nil {
		return nil
	}
	ci := utils.GetContextInfoFromProto(evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		return utils.UnwrapMessageProto(ci.QuotedMessage)
	}
	return nil
}

func extractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
		return *msg.ImageMessage.Caption
	}
	if msg.VideoMessage != nil && msg.VideoMessage.Caption != nil {
		return *msg.VideoMessage.Caption
	}
	if msg.DocumentMessage != nil && msg.DocumentMessage.Caption != nil {
		return *msg.DocumentMessage.Caption
	}
	return ""
}

func sendPollReply(ctx *dispatch.Context, body string, options []string) error {
	return dispatch.SendPollReply(ctx, body, options)
}

func init() {
	dispatch.RegisterFallbackInterceptor("ai_autoai", HandleAutoAIIntercept)
	dispatch.Register(&dispatch.Command{
		Name:        "ai",
		Alias:       "ask",
		Description: "Ask Meta AI a question.",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAI,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "autoai",
		Alias:       "aai",
		Description: "Toggle automatic AI responses when tagged, replied to, or when 'Rook' or 'WhatsRook' is mentioned in this chat (on/off)",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAutoAI,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "csai",
		Alias:       "customai",
		Description: "Configure global AI personality traits and relationship behavior (Sudoers only)",
		Category:    "ai",
		IsPublic:    false,
		Handler:     handleCSAI,
	})

	dispatch.Register(&dispatch.Command{
		Name:         "send",
		Alias:        "say",
		Description:  "Send raw text message to the current chat",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleSend,
	})
	dispatch.Register(&dispatch.Command{
		Name:         "edit",
		Alias:        "editmsg",
		Description:  "Edit a message by message ID or replied message",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleEditMsg,
	})
	dispatch.Register(&dispatch.Command{
		Name:         "ffmpeg",
		Alias:        "ff",
		Description:  "Run raw ffmpeg media command",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     false,
		Handler:      handleFFmpeg,
	})
	dispatch.Register(&dispatch.Command{
		Name:         "dlmsg",
		Alias:        "downloadMessage",
		Description:  "Download media from a message by ID or quoted message",
		Category:     "ai",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleDownloadMessage,
	})
}

func handleAutoAI(ctx *dispatch.Context) error {
	Logger.Debug("handleAutoAI started", "args", ctx.Args)

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

	s, okStore := dispatch.GetStore(ctx)
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
		Logger.Error("failed to update autoai setting", "err", err)
		return ctx.Reply("Failed to update setting: " + err.Error())
	}

	return ctx.Replyf("AutoAI has been set to %s for this chat.", val)
}

func handleCSAI(ctx *dispatch.Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only Sudoers can configure global AI personality traits and custom behavior.")
	}

	s, okStore := dispatch.GetStore(ctx)
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
		if err == nil && idxVal >= 1 && idxVal <= len(DefaultCSAITraits) {
			trait := DefaultCSAITraits[idxVal-1]
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
		if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal >= 1 && idxVal <= len(DefaultCSAITraits) {
			trait := DefaultCSAITraits[idxVal-1]
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

func renderCSAIPage(ctx *dispatch.Context, s *dispatch.StoreWrapper, page int) error {
	currentPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt")
	if currentPrompt == "" {
		currentPrompt = "Standard (Default Meta AI behavior)"
	}

	pageSize := 3
	totalPages := (len(DefaultCSAITraits) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := min(startIdx+pageSize, len(DefaultCSAITraits))

	pageItems := DefaultCSAITraits[startIdx:endIdx]
	p := ctx.GetPrefix()

	tb := ctx.Text().
		Headerf("Custom AI Personality & Trait Configuration (Page %d of %d)", page, totalPages).
		Field("Active AI Trait/Prompt", currentPrompt).
		Blank().
		Line("Select a personality trait for Meta AI below:").
		Blank()

	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		tb.Numberedf(globalIdx, "%s: %s", dispatch.Bold(trait.Name), trait.Instruction)
	}
	tb.Numbered(11, "Custom Trait / How You Refer To Me: Enter your own custom prompt.")

	var options []string
	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		optText := dispatch.Sprintf("%d. %s", globalIdx, trait.Name)
		if len(optText) > 40 {
			optText = optText[:37] + "..."
		}
		options = append(options, optText)
	}

	if page < totalPages {
		nextPage := page + 1
		options = append(options, dispatch.Sprintf("Next (Page %d)", nextPage))
	} else {
		options = append(options, "11. Custom Trait")
	}

	tb.Blank().
		Line("To select a personality, select an option from the poll below or type:").
		Bulletf("%scsai <number> (e.g. %scsai 3)", p, p).
		Bulletf("%scsai custom <prompt> (e.g. %scsai custom Refer to me as Sir)", p, p).
		Bulletf("%scsai reset (to restore default AI behavior)", p)

	return sendPollReply(ctx, tb.Trimmed(), options)
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

func handleAI(ctx *dispatch.Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sai <question>\n- %sask <question>\n\nExamples:\n- %sai What is the speed of light?\n- %sask Explain quantum computing in simple terms\n- Reply to an image or message with %sai Analyze this", p, p, p, p, p)
	}

	botName := ctx.GetBotName()
	p := ctx.GetPrefix()
	instruction := GetOrBuildInstructionWithNameAndPrefix(botName, p, func() string {
		cmdInfos := dispatch.All()
		metaCmds := make([]CommandInfo, 0, len(cmdInfos))
		for _, info := range cmdInfos {
			metaCmds = append(metaCmds, CommandInfo{
				Name:        info.Name,
				Alias:       info.Alias,
				Description: info.Description,
				IsPublic:    info.IsPublic,
			})
		}
		return BuildRunCommandInstructionWithNameAndPrefix(metaCmds, botName, p)
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

	data := Data{
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
		groupInfo, err := GetOrFetchGroupMeta(ctx.Chat.String(), func() (types.GroupInfo, error) {
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
		if s, okStore := dispatch.GetStore(ctx); okStore {
			if customPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt"); customPrompt != "" {
				query += dispatch.Sprintf("\n\n[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]\n%s\n\n", customPrompt)
			}
		}
		if isGroup {
			query += RenderGroupContext(data.GroupMetaData)
		}
		query += RenderUserContext(data)
		query += RenderQuotedContext(data)
		query += data.Question
	}

	Logger.Debug("handleAI: sending request to Meta AI", "chat", ctx.Chat.String(), "is_media_req", isMediaReq)

	var placeholderMsgID types.MessageID
	onUpdate := func(text string) error {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || IsDummyPlaceholderText(trimmed) {
			return nil
		}
		if _, _, ok := ParseRunCommand(trimmed); ok {
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
			Logger.Error("handleAI: failed to send edit", "chat", ctx.Chat.String(), "err", err)
		}
		return err
	}

	res, err := QueryMetaAi(ctx.Ctx, ctx.Client, ctx.Chat, query, onUpdate)
	if err != nil {
		Logger.Error("handleAI: queryMetaAi failed", "chat", ctx.Chat.String(), "err", err)
		if strings.Contains(err.Error(), "488") {
			errMsg := "This WA Account cannot use MetaAI"
			if placeholderMsgID != "" {
				_, _ = ctx.Edit(placeholderMsgID, errMsg)
			} else {
				_ = ctx.Reply(errMsg)
			}
			return nil
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
			Logger.Debug("handleAI: sending generated video message to chat", "chat", ctx.Chat.String(), "video_len", len(mediaBytes), "mime", mType)
			_ = ctx.ReplyWithVideo(mediaBytes, mType, caption)
		} else {
			Logger.Debug("handleAI: sending generated image message to chat", "chat", ctx.Chat.String(), "img_len", len(mediaBytes), "mime", mType)
			_ = ctx.ReplyWithImage(mediaBytes, mType, caption)
		}
	} else if placeholderMsgID == "" && reply != "" {
		if _, _, ok := ParseRunCommand(reply); !ok {
			_ = ctx.Reply(reply)
		}
	}

	if cmdName, rawArgs, ok := ParseRunCommand(reply); ok {
		if cmdName == "sh" || cmdName == "exec" || cmdName == "run" || cmdName == "shell" {
			if !ctx.IsSudo() {
				Logger.Warn("handleAI: blocked unauthorized shell execution request", "sender", ctx.Sender.String())
				_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run shell commands.")
				return nil
			}

			output, err := RunCmd(rawArgs)
			if err != nil && output == "" {
				output = err.Error()
			}
			if output == "" {
				output = "(no output)"
			}

			resText := dispatch.Sprintf("Output:\n```\n%s\n```", output)
			_, err = ctx.Edit(placeholderMsgID, resText)
			return err
		}

		if cmdName == "ai" || cmdName == "autoai" || cmdName == "gpt" || cmdName == "ask" {
			Logger.Warn("handleAI: blocked recursive AI command execution", "command", cmdName)
			_, err := ctx.Edit(placeholderMsgID, "Recursive AI command execution is not allowed.")
			return err
		}

		targetCmd, exists := dispatch.Get(cmdName)
		if !exists {
			Logger.Warn("handleAI: RUN_COMMAND referenced unknown command", "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "Sorry, I don't have a command called \""+cmdName+"\".")
			return nil
		}

		if !targetCmd.IsPublic && !ctx.IsSudo() {
			Logger.Warn("handleAI: blocked unauthorized RUN_COMMAND", "sender", ctx.Sender.String(), "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run this command.")
			return nil
		}

		if placeholderMsgID != "" {
			_, _ = ctx.Delete(placeholderMsgID)
			placeholderMsgID = ""
		}

		cctx := &dispatch.Context{
			Ctx:     ctx.Ctx,
			Client:  ctx.Client,
			Evt:     ctx.Evt,
			Command: cmdName,
			Args:    strings.Fields(rawArgs),
			RawArgs: rawArgs,
			Chat:    ctx.Chat,
			Sender:  ctx.Sender,
		}
		Logger.Debug("handleAI: executing command on behalf of AI", "command", cmdName, "args", ctx.Args)
		return targetCmd.Handler(cctx)
	}

	Logger.Debug("handleAI: completed successfully", "chat", ctx.Chat.String())
	return nil
}

func extractContextFromQuotedMessage(ctx *dispatch.Context, data *Data) {
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
			Logger.Debug("extractContextFromQuotedMessage: extracted image base64", "len", len(data.QuotedImageBase64))
		} else {
			Logger.Warn("extractContextFromQuotedMessage: failed to download quoted image", "err", err)
		}

	case quotedMsg.GetVideoMessage() != nil:
		vidMsg := quotedMsg.GetVideoMessage()
		data.QuotedMessageType = "Video"
		caption := vidMsg.GetCaption()
		if caption != "" {
			data.QuotedMessageOfQuestion = dispatch.Sprintf("[Video message. Note: Video file reading is not supported yet. Caption: %s]", caption)
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
			data.QuotedMessageOfQuestion = dispatch.Sprintf("File: %s. Caption: %s", filename, caption)
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
				Logger.Debug("extractContextFromQuotedMessage: extracted sticker image base64", "len", len(data.QuotedImageBase64))
			} else {
				Logger.Warn("extractContextFromQuotedMessage: failed to download quoted sticker image", "err", err)
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
		data.QuotedMessageOfQuestion = dispatch.Sprintf("Poll Question: %s. Options: %s", pollName, strings.Join(options, ", "))

	case quotedMsg.GetLocationMessage() != nil:
		locMsg := quotedMsg.GetLocationMessage()
		data.QuotedMessageType = "Location"
		data.QuotedMessageOfQuestion = dispatch.Sprintf("Location: %f, %f (%s)", locMsg.GetDegreesLatitude(), locMsg.GetDegreesLongitude(), locMsg.GetName())

	case quotedMsg.GetContactMessage() != nil:
		contMsg := quotedMsg.GetContactMessage()
		data.QuotedMessageType = "Contact"
		data.QuotedMessageOfQuestion = dispatch.Sprintf("Contact: %s", contMsg.GetDisplayName())

	default:
		if txt := extractTextFromProto(quotedMsg); txt != "" {
			data.QuotedMessageType = "Other"
			data.QuotedMessageOfQuestion = txt
		}
	}
}

func handleSend(ctx *dispatch.Context) error {
	if ctx.RawArgs == "" {
		return ctx.Reply("Usage: send <text>")
	}
	return ctx.SendText(ctx.RawArgs)
}

func handleEditMsg(ctx *dispatch.Context) error {
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

func handleFFmpeg(ctx *dispatch.Context) error {
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

func handleDownloadMessage(ctx *dispatch.Context) error {
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

// HandleAutoAIIntercept checks if AutoAI is enabled and the bot was tagged, replied to, or called by name.
func HandleAutoAIIntercept(c *dispatch.Context, text string) bool {
	s, ok := dispatch.GetStore(c)
	if !ok {
		return false
	}
	ctx := c.Ctx
	chatStr := c.Chat.String()

	autoAIVal, _ := s.GetSetting(ctx, "autoai:"+chatStr)
	if autoAIVal == "" {
		autoAIVal, _ = s.GetSetting(ctx, "autoai")
	}
	if autoAIVal != "on" {
		return false
	}

	if !isBotTaggedOrReplied(c, text) {
		return false
	}

	prompt := text
	if c.Client.Store.ID != nil {
		ourJID := c.Client.Store.ID.ToNonAD()
		if ourJID.User != "" {
			prompt = strings.TrimSpace(strings.ReplaceAll(prompt, "@"+ourJID.User, ""))
		}
	}
	if !c.Client.Store.LID.IsEmpty() {
		ourLID := c.Client.Store.LID.ToNonAD()
		if ourLID.User != "" {
			prompt = strings.TrimSpace(strings.ReplaceAll(prompt, "@"+ourLID.User, ""))
		}
	}
	botName := c.GetBotName()
	if botName != "" && strings.HasPrefix(strings.ToLower(prompt), strings.ToLower(botName)) {
		prompt = strings.TrimSpace(prompt[len(botName):])
	}
	if prompt == "" {
		prompt = text
	}

	go func() {
		reqCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		aiCtx := &dispatch.Context{
			Ctx:        reqCtx,
			CancelFunc: cancel,
			Client:     c.Client,
			Evt:        c.Evt,
			Command:    "ai",
			Args:       strings.Fields(prompt),
			RawArgs:    prompt,
			Chat:       c.Chat,
			Sender:     c.Sender,
		}
		aiCtx.StartAutoLoader()
		defer aiCtx.StopAutoLoader()

		if cmd, ok := dispatch.Get("ai"); ok {
			_ = cmd.Handler(aiCtx)
		}
	}()

	return true
}

func isBotTaggedOrReplied(c *dispatch.Context, text string) bool {
	client := c.Client
	evt := c.Evt
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return false
	}
	if evt.Info.Chat.Server != "g.us" {
		return true
	}
	ourJID := client.Store.ID.ToNonAD()
	ourLID := ourJID
	if !client.Store.LID.IsEmpty() {
		ourLID = client.Store.LID.ToNonAD()
	}

	lowerText := strings.ToLower(text)
	botName := c.GetBotName()
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
