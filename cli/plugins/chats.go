package plugins

import (
	"strconv"
	"strings"
	"time"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	Register(&Command{
		Name:        "archive",
		Description: "Archive the current chat",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleArchive,
	})
	Register(&Command{
		Name:        "unarchive",
		Description: "Unarchive the current chat",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleUnarchive,
	})
	Register(&Command{
		Name:        "pin",
		Description: "Pin the current chat (or pin the replied message)",
		Category:    "chats",
		IsPublic:    true,
		Handler:     handlePin,
	})
	Register(&Command{
		Name:        "unpin",
		Description: "Unpin the current chat (or unpin the replied message)",
		Category:    "chats",
		IsPublic:    true,
		Handler:     handleUnpin,
	})
	Register(&Command{
		Name:        "block",
		Description: "Block the target contact or current private chat JID",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleBlock,
	})
	Register(&Command{
		Name:        "unblock",
		Description: "Unblock a user (must provide phone number, tag, or reply)",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleUnblock,
	})
	Register(&Command{
		Name:        "clear",
		Alias:       "clearchat",
		Description: "Clear all messages in the current chat",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleClear,
	})
	Register(&Command{
		Name:        "delete",
		Alias:       "del",
		Description: "Delete/revoke a message (must reply to the target message)",
		Category:    "chats",
		IsPublic:    true,
		Handler:     handleDelete,
	})
	Register(&Command{
		Name:        "report",
		Alias:       "reportchat",
		Description: "Submit a spam report for the target user or replied message to WhatsApp",
		Category:    "chats",
		IsPublic:    false,
		Handler:     handleReport,
	})
	Register(&Command{
		Name:        "vv",
		Alias:       "viewonce",
		Description: "Unwrap a ViewOnce message and resend it as a normal message (replying to a ViewOnce message)",
		Category:    "chats",
		IsPublic:    true,
		Handler:     handleVV,
	})
}

func handleArchive(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	patch := appstate.BuildArchive(ctx.Chat, true, time.Time{}, nil)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply("Failed to archive chat: " + err.Error())
	}
	return ctx.Reply("Chat archived.")
}

func handleUnarchive(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	patch := appstate.BuildArchive(ctx.Chat, false, time.Time{}, nil)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply("Failed to unarchive chat: " + err.Error())
	}
	return ctx.Reply("Chat unarchived.")
}

func handlePin(ctx *Context) error {
	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil && *ci.StanzaID != "" {
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
			return ctx.Reply("Only sudoers or group admins can pin messages.")
		}

		quotedSender, ok := ctx.GetQuotedSender()
		quotedFromMe := false
		if ok && ctx.Client.Store.ID != nil {
			quotedFromMe = utils.IsSameUserRaw(ctx.Ctx, ctx.Client, quotedSender, *ctx.Client.Store.ID)
		}

		var participantStr *string
		if ctx.Chat.Server == "g.us" && !quotedSender.IsEmpty() {
			participantStr = new(quotedSender.ToNonAD().String())
		}

		pinMsg := &waE2E.Message{
			PinInChatMessage: &waE2E.PinInChatMessage{
				Key: &waCommon.MessageKey{
					FromMe:      new(quotedFromMe),
					ID:          ci.StanzaID,
					RemoteJID:   new(ctx.Chat.String()),
					Participant: participantStr,
				},
				Type:              waE2E.PinInChatMessage_PIN_FOR_ALL.Enum(),
				SenderTimestampMS: new(time.Now().UnixMilli()),
			},
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageAddOnDurationInSecs: new(uint32(604800)), // 7 days (standard WhatsApp pin duration)
			},
		}

		_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, pinMsg)
		if err != nil {
			return ctx.Reply("Failed to pin message: " + err.Error())
		}
		return ctx.Reply("Message pinned.")
	}

	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	patch := appstate.BuildPin(ctx.Chat, true)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply("Failed to pin chat: " + err.Error())
	}
	return ctx.Reply("Chat pinned.")
}

func handleUnpin(ctx *Context) error {
	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil && *ci.StanzaID != "" {
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
			return ctx.Reply("Only sudoers or group admins can unpin messages.")
		}

		quotedSender, ok := ctx.GetQuotedSender()
		quotedFromMe := false
		if ok && ctx.Client.Store.ID != nil {
			quotedFromMe = utils.IsSameUserRaw(ctx.Ctx, ctx.Client, quotedSender, *ctx.Client.Store.ID)
		}

		var participantStr *string
		if ctx.Chat.Server == "g.us" && !quotedSender.IsEmpty() {
			participantStr = new(quotedSender.ToNonAD().String())
		}

		unpinMsg := &waE2E.Message{
			PinInChatMessage: &waE2E.PinInChatMessage{
				Key: &waCommon.MessageKey{
					FromMe:      new(quotedFromMe),
					ID:          ci.StanzaID,
					RemoteJID:   new(ctx.Chat.String()),
					Participant: participantStr,
				},
				Type:              waE2E.PinInChatMessage_UNPIN_FOR_ALL.Enum(),
				SenderTimestampMS: new(time.Now().UnixMilli()),
			},
		}

		_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, unpinMsg)
		if err != nil {
			return ctx.Reply("Failed to unpin message: " + err.Error())
		}
		return ctx.Reply("Message unpinned.")
	}

	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	patch := appstate.BuildPin(ctx.Chat, false)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply("Failed to unpin chat: " + err.Error())
	}
	return ctx.Reply("Chat unpinned.")
}

func handleBlock(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	target := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		target = targets[0]
	}

	if target.Server == "g.us" {
		return ctx.Reply("Cannot block a group JID. Block commands only apply to users.")
	}

	if utils.IsSudoRaw(ctx.Ctx, ctx.Client, target) {
		return ctx.Reply("⚠️ Cannot block the bot owner or sudo users.")
	}

	bare := target.ToNonAD()
	jidsToBlock := []types.JID{bare}

	if uMap, err := ctx.Client.GetUserInfo(ctx.Ctx, []types.JID{bare}); err == nil && uMap != nil {
		if uInfo, ok := uMap[bare]; ok {
			if !uInfo.LID.IsEmpty() && uInfo.LID != bare {
				jidsToBlock = append(jidsToBlock, uInfo.LID.ToNonAD())
			}
		}
	}

	var lastErr error
	blockedAny := false
	for _, j := range jidsToBlock {
		_, err := ctx.Client.UpdateBlocklist(ctx.Ctx, j, events.BlocklistChangeActionBlock)
		if err == nil {
			blockedAny = true
		} else {
			lastErr = err
		}
	}

	if !blockedAny && lastErr != nil {
		return ctx.Reply("Failed to block user: " + lastErr.Error())
	}

	resolvedJID, username := ctx.ResolveMention(target)
	return ctx.ReplyWithMentions(Sprintf("Blocked @%s.", username), []types.JID{resolvedJID})
}

func handleUnblock(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	target := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		target = targets[0]
	}

	if target.Server == "g.us" {
		return ctx.Reply("Cannot unblock a group. Unblock commands only apply to users.")
	}

	bare := target.ToNonAD()
	jidsToUnblock := []types.JID{bare}

	if uMap, err := ctx.Client.GetUserInfo(ctx.Ctx, []types.JID{bare}); err == nil && uMap != nil {
		if uInfo, ok := uMap[bare]; ok {
			if !uInfo.LID.IsEmpty() && uInfo.LID != bare {
				jidsToUnblock = append(jidsToUnblock, uInfo.LID.ToNonAD())
			}
		}
	}

	var lastErr error
	unblockedAny := false
	for _, j := range jidsToUnblock {
		_, err := ctx.Client.UpdateBlocklist(ctx.Ctx, j, events.BlocklistChangeActionUnblock)
		if err == nil {
			unblockedAny = true
		} else {
			lastErr = err
		}
	}

	if !unblockedAny && lastErr != nil {
		return ctx.Reply("Failed to unblock user: " + lastErr.Error())
	}

	resolvedJID, username := ctx.ResolveMention(target)
	return ctx.ReplyWithMentions(Sprintf("Unblocked @%s.", username), []types.JID{resolvedJID})
}

func handleClear(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}
	patch := appstate.BuildDeleteChat(ctx.Chat, time.Now(), nil, true)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply("Failed to clear chat: " + err.Error())
	}
	return ctx.Reply("Chat messages cleared.")
}

func handleDelete(ctx *Context) error {
	var targetID types.MessageID
	if len(ctx.Args) > 0 && strings.TrimSpace(ctx.Args[0]) != "" {
		targetID = types.MessageID(strings.TrimSpace(ctx.Args[0]))
	} else {
		ci := ctx.GetContextInfo()
		if ci == nil || ci.StanzaID == nil {
			return ctx.Reply("Reply to the message you want to delete, or specify a message ID.")
		}
		targetID = *ci.StanzaID
	}

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
		return ctx.Reply("Only sudoers or group admins can delete messages.")
	}

	quotedSender, ok := ctx.GetQuotedSender()
	var revokeSender types.JID
	if ok {
		revokeSender = quotedSender
	} else {
		revokeSender = types.EmptyJID
	}

	revokeMsg := ctx.Client.BuildRevoke(ctx.Chat, revokeSender, targetID)
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, revokeMsg)
	if err != nil {
		return ctx.Reply("Failed to delete message: " + err.Error())
	}
	return nil
}

func isJIDSudo(ctx *Context, jid types.JID) bool {
	return utils.IsSudoRaw(ctx.Ctx, ctx.Client, jid)
}

func handleReport(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers only.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 && ctx.GetQuotedMessage() == nil {
		return ctx.Replyf("⚠️ *WARNING*: The %sreport command reports a target user or chat directly to WhatsApp for spam and terms violations.\n\nUsage:\n- Reply to a message with %sreport\n- %sreport @user\n- %sreport <count>x", p, p, p, p)
	}

	targetJID := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		targetJID = targets[0]
	}

	var messageChild []waBinary.Node
	var spamFlow = "ContactInfo"

	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil {
		quotedSender, _ := ctx.GetQuotedSender()
		if !quotedSender.IsEmpty() {
			targetJID = quotedSender
		}

		spamFlow = "MessageMenu"
		messageChild = []waBinary.Node{
			{
				Tag: "message",
				Attrs: waBinary.Attrs{
					"id": *ci.StanzaID,
					"t":  strconv.FormatInt(time.Now().Unix(), 10),
				},
			},
		}
	}

	if isJIDSudo(ctx, targetJID) {
		return ctx.Reply("Cannot report the bot or any of its sudo users.")
	}

	count := 1
	for _, arg := range ctx.Args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if before, ok := strings.CutSuffix(trimmed, "x"); ok {
			numPart := before
			if val, err := strconv.Atoi(numPart); err == nil && val > 0 {
				count = val
				break
			}
		} else {
			if val, err := strconv.Atoi(trimmed); err == nil && val > 0 {
				count = val
				break
			}
		}
	}
	if count > 20 {
		count = 20
	}

	spamListAttrs := waBinary.Attrs{
		"spam_flow": spamFlow,
	}
	if targetJID.Server == "g.us" {
		spamListAttrs["jid"] = targetJID.String()
		spamListAttrs["subject"] = "Group Spam"
		if spamFlow == "ContactInfo" {
			spamListAttrs["spam_flow"] = "GroupInfoReport"
		}
	} else {
		spamListAttrs["jid"] = targetJID.String()
	}

	for i := 0; i < count; i++ {
		//lint:ignore SA1019 intentional use of internal API for spam reporting
		reqID := ctx.Client.DangerousInternals().GenerateRequestID()

		iqNode := waBinary.Node{
			Tag: "iq",
			Attrs: waBinary.Attrs{
				"id":    reqID,
				"to":    types.ServerJID.String(),
				"type":  "set",
				"xmlns": "spam",
			},
			Content: []waBinary.Node{
				{
					Tag:     "spam_list",
					Attrs:   spamListAttrs,
					Content: messageChild,
				},
			},
		}

		//lint:ignore SA1019 intentional use of internal API for spam reporting
		_, err := ctx.Client.DangerousInternals().SendNodeAndGetData(ctx.Ctx, iqNode)
		if err != nil {
			return ctx.Replyf("Failed to submit spam report on iteration %d: %s", i+1, err.Error())
		}

		if count > 1 && i < count-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	if targetJID.Server == "g.us" {
		groupName := targetJID.String()
		info, err := ctx.Client.GetGroupInfo(ctx.Ctx, targetJID)
		if err == nil && info != nil {
			groupName = info.GroupName.Name
		}
		if count > 1 {
			return ctx.Replyf("Reported %s for spam to whatsapp %dx.", groupName, count)
		}
		return ctx.Replyf("Reported %s for spam to whatsapp.", groupName)
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	if count > 1 {
		return ctx.ReplyWithMentions(Sprintf("Reported @%s for spam to whatsapp %dx.", username, count), []types.JID{resolvedJID})
	}
	return ctx.ReplyWithMentions(Sprintf("Reported @%s for spam to whatsapp.", username), []types.JID{resolvedJID})
}

func handleVV(ctx *Context) error {
	s, ok := getStore(ctx)

	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "customize" || sub == "custom" || sub == "help" {
			return sendVVCustomizeGuide(ctx, s)
		}
		if sub == "dest" || sub == "destination" || sub == "mode" || sub == "set" {
			if ok && len(args) > 1 {
				val := strings.TrimSpace(args[1])
				_ = s.PutSetting(ctx.Ctx, "vv_destination", val)
				return ctx.Replyf("ViewOnce media destination updated to: %s", val)
			}
			return ctx.Replyf("Usage: %svv dest chat | owner | <phone_number> | <group_jid>", ctx.GetPrefix())
		}
	}

	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return sendVVMenu(ctx, s)
	}

	if !utils.IsViewOnceMessage(quoted) {
		if quoted.GetImageMessage() == nil && quoted.GetVideoMessage() == nil && quoted.GetAudioMessage() == nil && quoted.GetDocumentWithCaptionMessage() == nil {
			return ctx.Reply("The replied message is not a ViewOnce or media message.")
		}
	}

	targetJID := ctx.Chat
	if ok {
		dest, _ := s.GetSetting(ctx.Ctx, "vv_destination")
		dest = strings.TrimSpace(strings.ToLower(dest))
		switch {
		case dest == "" || dest == "chat":
			targetJID = ctx.Chat
		case dest == "owner" || dest == "me" || dest == "pm" || dest == "dm":
			if ctx.Client.Store.ID != nil {
				targetJID = ctx.Client.Store.ID.ToNonAD()
			}
		default:
			if parsed, err := types.ParseJID(dest); err == nil && !parsed.IsEmpty() {
				targetJID = parsed
			} else if !strings.Contains(dest, "@") {
				targetJID = types.NewJID(dest, types.DefaultUserServer)
			}
		}
	}

	var senderJID types.JID
	var pushName string
	if sender, okQ := ctx.GetQuotedSender(); okQ && !sender.IsEmpty() {
		senderJID = sender
	} else if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		if part := ext.GetContextInfo().Participant; part != nil {
			senderJID, _ = types.ParseJID(*part)
		}
	}
	if senderJID.IsEmpty() {
		senderJID = ctx.Sender
	}

	if senderJID == ctx.Sender && ctx.Evt != nil && ctx.Evt.Info.PushName != "" {
		pushName = ctx.Evt.Info.PushName
	}

	var quoteID string
	if ci := ctx.GetContextInfo(); ci != nil && ci.StanzaID != nil && *ci.StanzaID != "" {
		quoteID = *ci.StanzaID
	} else if ctx.Evt != nil {
		quoteID = ctx.Evt.Info.ID
	}
	err := utils.UnwrapAndSendViewOnceMessage(ctx.Ctx, ctx.Client, quoted, senderJID, pushName, targetJID, quoteID, ctx.Chat)
	if err != nil {
		return ctx.Reply("Failed to unwrap ViewOnce message: " + err.Error())
	}
	return nil
}

func sendVVMenu(ctx *Context, s *StoreWrapper) error {
	dest := "chat"
	if s != nil {
		if val, err := s.GetSetting(ctx.Ctx, "vv_destination"); err == nil && val != "" {
			dest = val
		}
	}

	p := ctx.GetPrefix()
	bodyText := Sprintf("╭━━━〔 VIEWONCE UNWRAPPER 〕━━━\n│ Destination : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nReply to any ViewOnce image, video, or audio message with %svv to unwrap it.", dest, p)

	buttons := []struct{ ID, Text string }{
		{ID: p + "vv dest owner", Text: "Set Owner DM"},
		{ID: p + "vv customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, Sprintf("%s VV Unwrapper", ctx.GetBotName()), buttons)
}

func sendVVCustomizeGuide(ctx *Context, s *StoreWrapper) error {
	p := ctx.GetPrefix()
	dest := "chat"
	if s != nil {
		if val, err := s.GetSetting(ctx.Ctx, "vv_destination"); err == nil && val != "" {
			dest = val
		}
	}

	return ctx.Text().
		Header("VIEWONCE CUSTOMIZATION GUIDE").
		Section("Choose where unwrapped ViewOnce media is sent").
		Bulletf("Current Chat : %svv dest chat", p).
		Bulletf("Bot Owner DM : %svv dest owner", p).
		Bulletf("Specific JID : %svv dest 1234567890 or %svv dest <group_jid>", p, p).
		Blank().
		Section("Examples").
		Numberedf(1, "%svv dest chat (Resends media in the active chat)", p).
		Numberedf(2, "%svv dest owner (Sends unwrapped media directly to owner DM)", p).
		Numberedf(3, "%svv dest 1234567890 (Sends to specified phone number)", p).
		Blank().
		Linef("Current Destination: %s", dest).
		Reply()
}
