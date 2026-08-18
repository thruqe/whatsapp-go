package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"

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
		status, _ := s.GetSetting(ctx, "welcome_status:"+chatKey)
		if status == "on" {
			tag, _ := s.GetSetting(ctx, "welcome_tag:"+chatKey)
			descOpt, _ := s.GetSetting(ctx, "welcome_desc:"+chatKey)
			customMsg, _ := s.GetSetting(ctx, "welcome_msg:"+chatKey)

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
		status, _ := s.GetSetting(ctx, "goodbye_status:"+chatKey)
		if status == "on" {
			tag, _ := s.GetSetting(ctx, "goodbye_tag:"+chatKey)
			descOpt, _ := s.GetSetting(ctx, "goodbye_desc:"+chatKey)
			customMsg, _ := s.GetSetting(ctx, "goodbye_msg:"+chatKey)

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
	status, _ := s.GetSetting(ctx, "events_status:"+chatKey)
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
