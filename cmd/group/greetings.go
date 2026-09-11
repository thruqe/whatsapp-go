package group

import (
	"context"
	"strconv"
	"strings"
	"time"

	"whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/util/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// HandleGreetings processes group join and leave events to send welcome and goodbye messages.
func HandleGreetings(ctx context.Context, cli *whatsmeow.Client, g *events.GroupInfo, startupTime time.Time) {
	if cli == nil || g == nil {
		return
	}

	logger.Debug("handleGroupGreetings: received group info event",
		"group", g.JID.String(),
		"joins", len(g.Join),
		"leaves", len(g.Leave),
		"timestamp", g.Timestamp,
	)

	// Do not process group events that happened before the bot started (with 2m clock skew tolerance)
	if !g.Timestamp.IsZero() && g.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		logger.Debug("handleGroupGreetings: skipping stale group event before startup",
			"group", g.JID.String(),
			"timestamp", g.Timestamp,
		)
		return
	}

	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.ToNonAD().String()

	// Process joins (Welcome)
	if len(g.Join) > 0 {
		status, _ := store.GetSetting(ctx, s, "welcome_status:"+chatKey)
		if status == "" {
			status, _ = store.GetSetting(ctx, s, "welcome_status:"+g.JID.String())
		}
		logger.Debug("handleGroupGreetings: evaluating welcome greeting", "group", chatKey, "status", status, "joinCount", len(g.Join))
		if status == "on" {
			tag, _ := store.GetSetting(ctx, s, "welcome_tag:"+chatKey)
			descOpt, _ := store.GetSetting(ctx, s, "welcome_desc:"+chatKey)
			customMsg, _ := store.GetSetting(ctx, s, "welcome_msg:"+chatKey)

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
					_, ownerName := whatsrook.ResolveMentionRaw(ctx, cli, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Join {
				resolvedJIDs, username := whatsrook.ResolveMentionJIDs(ctx, cli, participant)
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

				formatted := whatsrook.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" || tag == "" {
					for _, j := range resolvedJIDs {
						if !j.IsEmpty() {
							mentions = append(mentions, j.String())
						}
					}
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

				logger.Debug("handleGroupGreetings: sending welcome greeting message", "group", g.JID.String(), "participant", participant.String(), "username", username)
				resp, errSend := cli.SendMessage(ctx, g.JID, msg)
				if errSend != nil {
					logger.Error("handleGroupGreetings: failed to send welcome greeting", "group", g.JID.String(), "user", participant.String(), "err", errSend)
				} else {
					logger.Debug("handleGroupGreetings: welcome greeting sent successfully", "group", g.JID.String(), "msg_id", resp.ID, "user", username)
				}
			}
		}
	}

	// Process leaves (Goodbye)
	if len(g.Leave) > 0 {
		status, _ := store.GetSetting(ctx, s, "goodbye_status:"+chatKey)
		if status == "" {
			status, _ = store.GetSetting(ctx, s, "goodbye_status:"+g.JID.String())
		}
		logger.Debug("handleGroupGreetings: evaluating goodbye greeting", "group", chatKey, "status", status, "leaveCount", len(g.Leave))
		if status == "on" {
			tag, _ := store.GetSetting(ctx, s, "goodbye_tag:"+chatKey)
			descOpt, _ := store.GetSetting(ctx, s, "goodbye_desc:"+chatKey)
			customMsg, _ := store.GetSetting(ctx, s, "goodbye_msg:"+chatKey)

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
					_, ownerName := whatsrook.ResolveMentionRaw(ctx, cli, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Leave {
				if g.Sender != nil && !g.Sender.IsEmpty() && *g.Sender != participant {
					logger.Debug("handleGroupGreetings: skipping goodbye for kicked participant", "group", g.JID.String(), "participant", participant.String(), "actor", g.Sender.String())
					continue
				}

				resolvedJIDs, username := whatsrook.ResolveMentionJIDs(ctx, cli, participant)
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

				formatted := whatsrook.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" || tag == "" {
					for _, j := range resolvedJIDs {
						if !j.IsEmpty() {
							mentions = append(mentions, j.String())
						}
					}
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

				logger.Debug("handleGroupGreetings: sending goodbye greeting message", "group", g.JID.String(), "participant", participant.String(), "username", username)
				resp, errSend := cli.SendMessage(ctx, g.JID, msg)
				if errSend != nil {
					logger.Error("handleGroupGreetings: failed to send goodbye greeting", "group", g.JID.String(), "user", participant.String(), "err", errSend)
				} else {
					logger.Debug("handleGroupGreetings: goodbye greeting sent successfully", "group", g.JID.String(), "msg_id", resp.ID, "user", username)
				}
			}
		}
	}
}
