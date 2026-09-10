package group

import (
	"context"
	"time"

	"whatsrook"
	"whatsrook/cmd/store"
	"whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// HandleEventsNotification processes group metadata events to send administrative change notifications.
func HandleEventsNotification(ctx context.Context, cli *whatsmeow.Client, g *events.GroupInfo, startupTime time.Time) {
	if cli == nil || g == nil {
		return
	}

	logger.Debug("handleGroupEventsNotification: received group info event",
		"group", g.JID.String(),
		"timestamp", g.Timestamp,
	)

	// Do not process group events that happened before the bot started (with 2m clock skew tolerance)
	if !g.Timestamp.IsZero() && g.Timestamp.Before(startupTime.Add(-2*time.Minute)) {
		logger.Debug("handleGroupEventsNotification: skipping stale group event notification before startup",
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
	status, _ := store.GetSetting(ctx, s, "events_status:"+chatKey)
	if status == "" {
		status, _ = store.GetSetting(ctx, s, "events_status:"+g.JID.String())
	}
	if status != "on" {
		logger.Debug("handleGroupEventsNotification: event notifications disabled for group", "group", chatKey, "status", status)
		return
	}

	var actorTag string
	var actorJID *types.JID
	if g.Sender != nil && !g.Sender.IsEmpty() {
		actorJID = g.Sender
		_, actorName := whatsrook.ResolveMentionRaw(ctx, cli, *g.Sender)
		actorTag = " by @" + actorName
	}

	// 1. Group Subject / Name Changed
	if g.Name != nil && g.Name.Name != "" {
		logger.Debug("handleGroupEventsNotification: group name changed", "group", chatKey, "newName", g.Name.Name, "actor", actorTag)
		msgText := whatsrook.Sprintf("*Group Event*: Group name changed to *%s*%s.", g.Name.Name, actorTag)
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}

	// 2. Group Description / Topic Changed
	if g.Topic != nil && g.Topic.Topic != "" {
		logger.Debug("handleGroupEventsNotification: group topic changed", "group", chatKey, "actor", actorTag)
		msgText := whatsrook.Sprintf("*Group Event*: Group description updated%s:\n%s", actorTag, g.Topic.Topic)
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}

	// 3. Announce Mute / Unmute
	if g.Announce != nil {
		logger.Debug("handleGroupEventsNotification: group announce changed", "group", chatKey, "isAnnounce", g.Announce.IsAnnounce, "actor", actorTag)
		if g.Announce.IsAnnounce {
			msgText := whatsrook.Sprintf("*Group Event*: Group settings updated%s. Only admins can send messages now.", actorTag)
			sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
		} else {
			msgText := whatsrook.Sprintf("*Group Event*: Group settings updated%s. All members can send messages now.", actorTag)
			sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
		}
	}

	// 4. Locked / Unlocked
	if g.Locked != nil {
		logger.Debug("handleGroupEventsNotification: group lock changed", "group", chatKey, "isLocked", g.Locked.IsLocked, "actor", actorTag)
		if g.Locked.IsLocked {
			msgText := whatsrook.Sprintf("*Group Event*: Group settings locked%s. Only admins can edit group info.", actorTag)
			sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
		} else {
			msgText := whatsrook.Sprintf("*Group Event*: Group settings unlocked%s. All members can edit group info.", actorTag)
			sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
		}
	}

	// 5. Admin Promotions
	if len(g.Promote) > 0 {
		for _, userJID := range g.Promote {
			resolvedJIDs, username := whatsrook.ResolveMentionJIDs(ctx, cli, userJID)
			logger.Debug("handleGroupEventsNotification: participant promoted to admin", "group", chatKey, "user", username, "actor", actorTag)
			msgText := whatsrook.Sprintf("*Group Event*: @%s was promoted to Group Admin%s!", username, actorTag)
			mentions := resolvedJIDs
			if actorJID != nil && !actorJID.IsEmpty() {
				mentions = append(mentions, *actorJID)
			}
			sendGroupEventMessageWithMentions(ctx, cli, g.JID, msgText, mentions)
		}
	}

	// 6. Admin Demotions
	if len(g.Demote) > 0 {
		for _, userJID := range g.Demote {
			resolvedJIDs, username := whatsrook.ResolveMentionJIDs(ctx, cli, userJID)
			logger.Debug("handleGroupEventsNotification: admin demoted to member", "group", chatKey, "user", username, "actor", actorTag)
			msgText := whatsrook.Sprintf("*Group Event*: @%s was demoted from Group Admin%s.", username, actorTag)
			mentions := resolvedJIDs
			if actorJID != nil && !actorJID.IsEmpty() {
				mentions = append(mentions, *actorJID)
			}
			sendGroupEventMessageWithMentions(ctx, cli, g.JID, msgText, mentions)
		}
	}

	// 7. Member Add Mode
	if g.MemberAddMode != nil {
		logger.Debug("handleGroupEventsNotification: member add mode changed", "group", chatKey, "mode", *g.MemberAddMode, "actor", actorTag)
		var msgText string
		if *g.MemberAddMode == types.GroupMemberAddModeAdmin {
			msgText = whatsrook.Sprintf("*Group Event*: Group settings updated%s. Only admins can add members.", actorTag)
		} else {
			msgText = whatsrook.Sprintf("*Group Event*: Group settings updated%s. All members can add members.", actorTag)
		}
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}

	// 8. Member Link Mode
	if g.MemberLinkMode != nil {
		logger.Debug("handleGroupEventsNotification: member link mode changed", "group", chatKey, "mode", *g.MemberLinkMode, "actor", actorTag)
		var msgText string
		if *g.MemberLinkMode == types.GroupMemberLinkModeAdmin {
			msgText = whatsrook.Sprintf("*Group Event*: Group settings updated%s. Only admins can manage invite links.", actorTag)
		} else {
			msgText = whatsrook.Sprintf("*Group Event*: Group settings updated%s. All members can manage invite links.", actorTag)
		}
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}

	// 9. Member Share History Mode
	if g.MemberShareHistoryMode != nil {
		logger.Debug("handleGroupEventsNotification: member share history mode changed", "group", chatKey, "mode", *g.MemberShareHistoryMode, "actor", actorTag)
		var msgText string
		if *g.MemberShareHistoryMode == types.GroupMemberShareHistoryModeAdmin {
			msgText = whatsrook.Sprintf("*Group Event*: Group history sharing updated%s. Only admins can share group history.", actorTag)
		} else {
			msgText = whatsrook.Sprintf("*Group Event*: Group history sharing updated%s. Recent history is shared with new members.", actorTag)
		}
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}

	// 10. Allow Non-Admin Subgroup Creation (Community)
	if g.AllowNonAdminSubGroupCreation != nil {
		logger.Debug("handleGroupEventsNotification: allow non-admin subgroup creation changed", "group", chatKey, "allow", *g.AllowNonAdminSubGroupCreation, "actor", actorTag)
		var msgText string
		if *g.AllowNonAdminSubGroupCreation {
			msgText = whatsrook.Sprintf("*Group Event*: Community settings updated%s. All members can create sub-groups now.", actorTag)
		} else {
			msgText = whatsrook.Sprintf("*Group Event*: Community settings updated%s. Only admins can create sub-groups now.", actorTag)
		}
		sendGroupEventMessage(ctx, cli, g.JID, msgText, actorJID)
	}
}

func sendGroupEventMessage(ctx context.Context, cli *whatsmeow.Client, chatJID types.JID, text string, actor *types.JID) {
	var mentions []types.JID
	if actor != nil && !actor.IsEmpty() {
		mentions = append(mentions, *actor)
	}
	sendGroupEventMessageWithMentions(ctx, cli, chatJID, text, mentions)
}

func sendGroupEventMessageWithMentions(ctx context.Context, cli *whatsmeow.Client, chatJID types.JID, text string, targetMentions []types.JID) {
	if cli == nil {
		return
	}
	logger.Debug("sendGroupEventMessageWithMentions: sending event message", "group", chatJID.String(), "mentionsCount", len(targetMentions))
	formatted := whatsrook.FormatTextResponseRaw(text)
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
	if resp, err := cli.SendMessage(ctx, chatJID, msg); err != nil {
		logger.Error("sendGroupEventMessageWithMentions: failed to send message", "group", chatJID.String(), "err", err)
	} else {
		logger.Debug("sendGroupEventMessageWithMentions: message sent successfully", "group", chatJID.String(), "msg_id", resp.ID)
	}
}
