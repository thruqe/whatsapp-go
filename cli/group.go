package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"whatsrook/cli/captcha"
	commands "whatsrook/cli/plugins"
	clistore "whatsrook/cli/store"
	cliutils "whatsrook/cli/utils"
	"whatsrook/logger"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Bot) handleGroupGreetings(ctx context.Context, g *events.GroupInfo) {
	cli := b.client.WAClient()
	if cli == nil || g == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.String()

	// Process joins (Welcome)
	if len(g.Join) > 0 {
		status, _ := clistore.GetSetting(ctx, s, "welcome_status:"+chatKey)
		if status == "on" {
			tag, _ := clistore.GetSetting(ctx, s, "welcome_tag:"+chatKey)
			descOpt, _ := clistore.GetSetting(ctx, s, "welcome_desc:"+chatKey)
			customMsg, _ := clistore.GetSetting(ctx, s, "welcome_msg:"+chatKey)

			info, err := cli.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			adminCount := 0
			ownerStr := ""
			ownerJIDStr := ""
			createdAtStr := ""
			groupJIDStr := g.JID.String()

			if err == nil && info != nil {
				if info.Name != "" {
					groupName = info.Name
				}
				groupDesc = info.Topic
				memberCount = len(info.Participants)
				for _, p := range info.Participants {
					if p.IsAdmin || p.IsSuperAdmin {
						adminCount++
					}
				}
				if !info.OwnerJID.IsEmpty() {
					ownerJIDStr = info.OwnerJID.String()
					_, ownerName := utils.ResolveMentionRaw(ctx, cli, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Join {
				resolvedJID, username := utils.ResolveMentionRaw(ctx, cli, participant)
				userTag := "@" + username
				body := customMsg
				if body == "" {
					body = "Welcome " + userTag + " to " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{user_id}", participant.User)
					body = strings.ReplaceAll(body, "{phone}", participant.User)
					body = strings.ReplaceAll(body, "{user_jid}", participant.String())

					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{name}", groupName)
					body = strings.ReplaceAll(body, "{group_jid}", groupJIDStr)
					body = strings.ReplaceAll(body, "{jid}", groupJIDStr)

					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{topic}", groupDesc)

					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{count}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{admins}", strconv.Itoa(adminCount))
					body = strings.ReplaceAll(body, "{admin_count}", strconv.Itoa(adminCount))

					body = strings.ReplaceAll(body, "{owner}", ownerStr)
					body = strings.ReplaceAll(body, "{creator}", ownerStr)

					body = strings.ReplaceAll(body, "{created_at}", createdAtStr)
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") && !strings.Contains(customMsg, "{topic}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := cliutils.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, resolvedJID.String())
				}
				if ownerJIDStr != "" && (strings.Contains(customMsg, "{owner}") || strings.Contains(customMsg, "{creator}")) {
					mentions = append(mentions, ownerJIDStr)
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				_, _ = cli.SendMessage(ctx, g.JID, msg)
			}
		}
	}

	// Process leaves (Goodbye)
	if len(g.Leave) > 0 {
		status, _ := clistore.GetSetting(ctx, s, "goodbye_status:"+chatKey)
		if status == "on" {
			tag, _ := clistore.GetSetting(ctx, s, "goodbye_tag:"+chatKey)
			descOpt, _ := clistore.GetSetting(ctx, s, "goodbye_desc:"+chatKey)
			customMsg, _ := clistore.GetSetting(ctx, s, "goodbye_msg:"+chatKey)

			info, err := cli.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			adminCount := 0
			ownerStr := ""
			ownerJIDStr := ""
			createdAtStr := ""
			groupJIDStr := g.JID.String()

			if err == nil && info != nil {
				if info.Name != "" {
					groupName = info.Name
				}
				groupDesc = info.Topic
				memberCount = len(info.Participants)
				for _, p := range info.Participants {
					if p.IsAdmin || p.IsSuperAdmin {
						adminCount++
					}
				}
				if !info.OwnerJID.IsEmpty() {
					ownerJIDStr = info.OwnerJID.String()
					_, ownerName := utils.ResolveMentionRaw(ctx, cli, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Leave {
				if g.Sender != nil && !g.Sender.IsEmpty() && *g.Sender != participant {
					continue
				}

				resolvedJID, username := utils.ResolveMentionRaw(ctx, cli, participant)
				userTag := "@" + username
				body := customMsg
				if body == "" {
					body = "Goodbye " + userTag + " from " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{user_id}", participant.User)
					body = strings.ReplaceAll(body, "{phone}", participant.User)
					body = strings.ReplaceAll(body, "{user_jid}", participant.String())

					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{name}", groupName)
					body = strings.ReplaceAll(body, "{group_jid}", groupJIDStr)
					body = strings.ReplaceAll(body, "{jid}", groupJIDStr)

					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{topic}", groupDesc)

					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{count}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{admins}", strconv.Itoa(adminCount))
					body = strings.ReplaceAll(body, "{admin_count}", strconv.Itoa(adminCount))

					body = strings.ReplaceAll(body, "{owner}", ownerStr)
					body = strings.ReplaceAll(body, "{creator}", ownerStr)

					body = strings.ReplaceAll(body, "{created_at}", createdAtStr)
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") && !strings.Contains(customMsg, "{topic}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := cliutils.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, resolvedJID.String())
				}
				if ownerJIDStr != "" && (strings.Contains(customMsg, "{owner}") || strings.Contains(customMsg, "{creator}")) {
					mentions = append(mentions, ownerJIDStr)
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				_, _ = cli.SendMessage(ctx, g.JID, msg)
			}
		}
	}
}

func (b *Bot) handleGroupEventsNotification(ctx context.Context, g *events.GroupInfo) {
	cli := b.client.WAClient()
	if cli == nil || g == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.String()
	status, _ := clistore.GetSetting(ctx, s, "events_status:"+chatKey)
	if status != "on" {
		return
	}

	var actorTag string
	var actorJID *types.JID
	if g.Sender != nil && !g.Sender.IsEmpty() {
		actorJID = g.Sender
		_, actorName := utils.ResolveMentionRaw(ctx, cli, *g.Sender)
		actorTag = " by @" + actorName
	}

	// 1. Group Subject / Name Changed
	if g.Name != nil && g.Name.Name != "" {
		msgText := fmt.Sprintf("*Group Event*: Group name changed to *%s*%s.", g.Name.Name, actorTag)
		b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
	}

	// 2. Group Description / Topic Changed
	if g.Topic != nil && g.Topic.Topic != "" {
		msgText := fmt.Sprintf("*Group Event*: Group description updated%s:\n%s", actorTag, g.Topic.Topic)
		b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
	}

	// 3. Announce Mute / Unmute
	if g.Announce != nil {
		if g.Announce.IsAnnounce {
			msgText := fmt.Sprintf("*Group Event*: Group settings updated%s. Only admins can send messages now.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		} else {
			msgText := fmt.Sprintf("*Group Event*: Group settings updated%s. All members can send messages now.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		}
	}

	// 4. Locked / Unlocked
	if g.Locked != nil {
		if g.Locked.IsLocked {
			msgText := fmt.Sprintf("*Group Event*: Group settings locked%s. Only admins can edit group info.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		} else {
			msgText := fmt.Sprintf("*Group Event*: Group settings unlocked%s. All members can edit group info.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		}
	}

	// 5. Admin Promotions
	if len(g.Promote) > 0 {
		for _, userJID := range g.Promote {
			resolvedJID, username := utils.ResolveMentionRaw(ctx, cli, userJID)
			msgText := fmt.Sprintf("*Group Event*: @%s was promoted to Group Admin%s!", username, actorTag)
			b.sendGroupEventMessageWithMentions(ctx, g.JID, msgText, []types.JID{resolvedJID})
		}
	}

	// 6. Admin Demotions
	if len(g.Demote) > 0 {
		for _, userJID := range g.Demote {
			resolvedJID, username := utils.ResolveMentionRaw(ctx, cli, userJID)
			msgText := fmt.Sprintf("*Group Event*: @%s was demoted from Group Admin%s.", username, actorTag)
			b.sendGroupEventMessageWithMentions(ctx, g.JID, msgText, []types.JID{resolvedJID})
		}
	}
}

func (b *Bot) sendGroupEventMessage(ctx context.Context, chatJID types.JID, text string, actor *types.JID) {
	var mentions []types.JID
	if actor != nil && !actor.IsEmpty() {
		mentions = append(mentions, *actor)
	}
	b.sendGroupEventMessageWithMentions(ctx, chatJID, text, mentions)
}

func (b *Bot) sendGroupEventMessageWithMentions(ctx context.Context, chatJID types.JID, text string, targetMentions []types.JID) {
	cli := b.client.WAClient()
	if cli == nil {
		return
	}
	formatted := cliutils.FormatTextResponseRaw(text)
	var mentions []string
	for _, m := range targetMentions {
		if !m.IsEmpty() {
			mentions = append(mentions, m.String())
		}
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentions,
			},
		},
	}
	_, _ = cli.SendMessage(ctx, chatJID, msg)
}

func formatTimeoutStr(sec int) string {
	if sec <= 0 {
		return "2 mins"
	}
	if sec%60 == 0 {
		mins := sec / 60
		if mins == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d mins", mins)
	}
	return fmt.Sprintf("%d seconds", sec)
}

func (b *Bot) handleGroupCaptcha(ctx context.Context, g *events.GroupInfo) {
	if g == nil {
		return
	}
	// Do not process group events that happened before the bot started
	if !g.Timestamp.IsZero() && g.Timestamp.Before(b.startupTime) {
		return
	}

	// Cancel pending captcha if participant left or was removed
	if len(g.Leave) > 0 {
		for _, participant := range g.Leave {
			commands.RemovePendingCaptcha(g.JID, participant)
		}
	}
	// Cancel pending captcha and delete verification message if participant was promoted to admin
	if len(g.Promote) > 0 {
		cli := b.client.WAClient()
		for _, participant := range g.Promote {
			if pending, ok := commands.RemovePendingCaptcha(g.JID, participant); ok && pending != nil {
				if cli != nil && pending.MsgID != "" {
					_, _ = cli.SendMessage(ctx, g.JID, cli.BuildRevoke(g.JID, types.EmptyJID, pending.MsgID))
				}
			}
		}
	}
	if len(g.Join) == 0 {
		return
	}

	go b.processGroupCaptchaJoins(g)
}

func (b *Bot) processGroupCaptchaJoins(g *events.GroupInfo) {
	ctx := context.Background()
	cli := b.client.WAClient()
	if cli == nil {
		return
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.String()
	status, _ := clistore.GetSetting(ctx, s, "captcha_status:"+chatKey)
	if status != "on" {
		return
	}

	info, err := cli.GetGroupInfo(ctx, g.JID)
	if err != nil || info == nil {
		return
	}

	// This plugin should only work if this bot is an admin
	if cli.Store.ID == nil || !utils.IsAdminRaw(ctx, cli, info, *cli.Store.ID) {
		Logger.Warn("handleGroupCaptcha: bot is not an admin in group, skipping captcha verification", "group", chatKey)
		return
	}

	// If group only allows admins to send messages, no need for verification
	if info.IsAnnounce {
		Logger.Debug("handleGroupCaptcha: group is announce-only, skipping captcha verification", "group", chatKey)
		return
	}

	groupName := g.JID.String()
	if info.Name != "" {
		groupName = info.Name
	}

	// Timeout setting (default 120s / 2 mins)
	timeoutSec := 120
	if rawTime, _ := clistore.GetSetting(ctx, s, "captcha_time:"+chatKey); rawTime != "" {
		if t, err := strconv.Atoi(rawTime); err == nil && t >= 10 {
			timeoutSec = t
		}
	}
	timeoutDisplay := formatTimeoutStr(timeoutSec)

	for _, participant := range g.Join {
		// Skip if participant is the bot itself
		if utils.IsSameUserRaw(ctx, cli, participant, *cli.Store.ID) {
			continue
		}
		// Skip sudoers
		if utils.IsSudoRaw(ctx, cli, participant) {
			continue
		}
		// Skip if the joined participant is already an admin
		if utils.IsAdminRaw(ctx, cli, info, participant) {
			continue
		}

		resolvedJID, username := utils.ResolveMentionRaw(ctx, cli, participant)

		// Generate random 4-digit code
		codeInt := rand.Intn(10000)
		code := fmt.Sprintf("%04d", codeInt)

		// Register pending captcha with timeout kick callback
		partCopy := participant
		resolvedCopy := resolvedJID
		userCopy := username
		commands.RegisterPendingCaptcha(
			g.JID,
			partCopy,
			resolvedCopy,
			userCopy,
			code,
			time.Duration(timeoutSec)*time.Second,
			func() {
				// Timeout reached, kick user
				currentInfo, gErr := cli.GetGroupInfo(context.Background(), g.JID)
				if gErr != nil || currentInfo == nil {
					return
				}
				if !utils.IsAdminRaw(context.Background(), cli, currentInfo, *cli.Store.ID) {
					Logger.Warn("handleGroupCaptcha: bot is no longer admin to kick unverified participant", "group", g.JID.String(), "user", partCopy.String())
					return
				}

				_, kErr := cli.UpdateGroupParticipants(context.Background(), g.JID, []types.JID{partCopy}, whatsmeow.ParticipantChangeRemove)
				if kErr != nil {
					Logger.Error("handleGroupCaptcha: failed to kick unverified participant", "user", partCopy.String(), "err", kErr)
					return
				}

				kickTb := utils.NewText()
				kickTb.Linef("@%s was removed from the group for failing to complete the captcha verification within %s.", userCopy, timeoutDisplay)
				b.sendGroupEventMessageWithMentions(context.Background(), g.JID, kickTb.Trimmed(), []types.JID{resolvedCopy})
			},
		)

		// Generate 8-second captcha video using cli/captcha package
		var vidBuf bytes.Buffer
		errGen := captcha.Generate(&vidBuf, code, captcha.Options{
			Seconds: 8.0,
		})

		var mediaUploaded *whatsmeow.UploadResponse
		if errGen == nil && vidBuf.Len() > 0 {
			uploaded, errUp := cli.Upload(ctx, vidBuf.Bytes(), whatsmeow.MediaVideo)
			if errUp == nil {
				mediaUploaded = &uploaded
			} else {
				Logger.Error("handleGroupCaptcha: video upload failed", "err", errUp)
			}
		} else {
			Logger.Error("handleGroupCaptcha: captcha video generation failed", "err", errGen)
		}

		tbVid := utils.NewText()
		tbVid.Linef("Welcome @%s! You are required to complete a verification code to join %s.", username, groupName)
		tbVid.Linef("Please watch the video and reply with the 4-digit verification code within %s, otherwise you will be automatically removed.", timeoutDisplay)
		formattedCaption := tbVid.Trimmed()

		// Send video message as gifplayback if generated successfully
		if mediaUploaded != nil {
			mimetype := "video/mp4"
			vidMsg := &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					URL:           &mediaUploaded.URL,
					DirectPath:    &mediaUploaded.DirectPath,
					MediaKey:      mediaUploaded.MediaKey,
					Mimetype:      &mimetype,
					GifPlayback:   new(bool),
					FileEncSHA256: mediaUploaded.FileEncSHA256,
					FileSHA256:    mediaUploaded.FileSHA256,
					FileLength:    new(uint64(vidBuf.Len())),
					Caption:       &formattedCaption,
					ContextInfo: &waE2E.ContextInfo{
						MentionedJID: []string{resolvedJID.String()},
					},
				},
			}
			*vidMsg.VideoMessage.GifPlayback = true
			if resp, errSend := cli.SendMessage(ctx, g.JID, vidMsg); errSend == nil && resp.ID != "" {
				commands.SetPendingCaptchaMsgID(g.JID, partCopy, resp.ID)
			}
		}
	}
}
