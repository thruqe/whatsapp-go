package group

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"

	"sync"
	"time"
	"unicode"

	"whatsrook"
	"whatsrook/builder"
	"whatsrook/cmd/dispatch"
	"whatsrook/cmd/store"
	"whatsrook/logger"
	"whatsrook/media"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	dispatch.RegisterPreInterceptor("group_moderation", HandleGroupModeration)
	dispatch.Register(&dispatch.Command{
		Name:        "tag",
		Alias:       "tag, everyone, all, tagmembers",
		Description: "Mention everyone in the group with an optional message",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleTagAll,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "kick",
		Description: "Remove a participant from the group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleKick,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "add",
		Description: "Add a member to the group using their phone number",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAdd,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "promote",
		Description: "Promote a member to group admin",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePromote,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "demote",
		Description: "Demote an admin back to regular member",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleDemote,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "group",
		Description: "Manage group settings (open/close chat, lock/unlock editing)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGroup,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "antilink",
		Description: "Protect the group by automatically moderating unwanted links",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiLink,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "antiword",
		Description: "Filter and delete messages containing banned words",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiWord,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "gstats",
		Description: "View group activity statistics and top active members",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGStats,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "poll",
		Alias:       "lockpoll",
		Description: "Create a single or multiple-choice poll for the group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePoll,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "invite",
		Description: "Get the invite link for this group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleInvite,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "listonline",
		Alias:       "online",
		Description: "Check which members are currently online or active",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListOnline,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "kickall",
		Description: "Remove all participants from the group (Admins & Bot Owners only)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleKickAll,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "leave",
		Alias:       "left",
		Description: "Ask the bot to leave this group with a confirmation prompt",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleLeave,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "join",
		Alias:       "joingroup",
		Description: "Join a group using an invite link or message",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleJoin,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "antimsg",
		Alias:       "antimessage",
		Description: "Automatically delete messages from targeted participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiMsg,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "antispam",
		Alias:       "aspam",
		Description: "Protect the group from rapid message flooding and spam",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiSpam,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "automute",
		Alias:       "autoclose",
		Description: "Set a daily time to automatically close the group chat",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoMute,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "autounmute",
		Alias:       "autoopen",
		Description: "Set a daily time to automatically reopen the group chat",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoUnmute,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "listmute",
		Alias:       "mutestatus",
		Description: "View active daily open and close schedules for this group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListMute,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "events",
		Alias:       "groupevents",
		Description: "Get notified when group settings, subject, or participants change",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleEventsCmd,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "gpp",
		Alias:       "setgpp",
		Description: "Update the group profile picture using an image",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleSetGroupPP,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "warn",
		Alias:       "warning",
		Description: "Issue a warning to a member for breaking group rules",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleWarn,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "unwarn",
		Alias:       "delwarn",
		Description: "Remove a warning previously given to a member",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleUnwarn,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "warns",
		Alias:       "getwarn",
		Description: "Check current warning count for a member or the group limit",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleWarns,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "setwarn",
		Alias:       "warnlimit",
		Description: "Set how many warnings a member gets before being removed",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleSetWarn,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "welcome",
		Alias:       "welc",
		Description: "Set up automatic welcome messages for new group members",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleWelcome,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "goodbye",
		Alias:       "bye",
		Description: "Set up automatic goodbye messages when members leave",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleGoodbye,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "captcha",
		Alias:       "verify",
		Description: "Protect the group from bots by verifying new members with a quick code",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleCaptcha,
	})
}

func handleTagAll(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can mention everyone.")
	}

	msg := "@all"
	if ctx.RawArgs != "" {
		msg += "\n\n *" + ctx.RawArgs + "*"
	}

	return ctx.ReplyWithGroupMention(msg)
}

func handleKick(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can remove members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to remove members. Please promote me to admin first!")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to remove?\n\n• Reply to their message with `%skick`\n• Or type: `%skick @user`\n• Or type: `%skick <phone-number>`", p, p, p)
	}

	var kicked []string
	var kickedJIDs []types.JID
	for _, target := range targets {
		resolvedJID, username := ctx.ResolveMention(target)
		if whatsrook.IsSudoRaw(ctx.Ctx, ctx.Client, target) {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("I can't remove @%s because they are a bot owner/admin.", username), []types.JID{resolvedJID})
			continue
		}
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("Couldn't remove @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			kicked = append(kicked, "@"+username)
			kickedJIDs = append(kickedJIDs, resolvedJID)
		}
	}

	if len(kicked) > 0 {
		return ctx.ReplyWithMentions(dispatch.Sprintf("Removed %s from the group.", strings.Join(kicked, ",")), kickedJIDs)
	}
	return nil
}

func handleAdd(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can add new members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to add members. Please promote me to admin first!")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Please provide the phone number of the person you'd like to add.\n\nExample:\n• `%sadd 1234567890`\n• `%sadd 1234567890 9876543210` (to add multiple)", p, p)
	}

	var added []string
	var addedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeAdd)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("Couldn't add @%s (they might have group invite restrictions turned on): %v", username, err), []types.JID{resolvedJID})
		} else {
			added = append(added, "@"+username)
			addedJIDs = append(addedJIDs, resolvedJID)
		}
	}

	if len(added) > 0 {
		return ctx.ReplyWithMentions(dispatch.Sprintf("Welcome %s to the group!", strings.Join(added, ",")), addedJIDs)
	}
	return nil
}

func handlePromote(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can promote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to promote members. Please promote me to admin first!")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to make an admin?\n\n• Reply to their message with `%spromote`\n• Or type: `%spromote @user`\n• Or type: `%spromote <phone-number>`", p, p, p)
	}

	var promoted []string
	var promotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("Couldn't promote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			promoted = append(promoted, "@"+username)
			promotedJIDs = append(promotedJIDs, resolvedJID)
		}
	}

	if len(promoted) > 0 {
		return ctx.ReplyWithMentions(dispatch.Sprintf("Successfully promoted %s to group admin!", strings.Join(promoted, ",")), promotedJIDs)
	}
	return nil
}

func handleDemote(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can demote other admins.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to demote members. Please promote me to admin first!")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to remove as admin?\n\n• Reply to their message with `%sdemote`\n• Or type: `%sdemote @user`\n• Or type: `%sdemote <phone-number>`", p, p, p)
	}

	var demoted []string
	var demotedJIDs []types.JID
	for _, target := range targets {
		resolvedJID, username := ctx.ResolveMention(target)
		if whatsrook.IsSudoRaw(ctx.Ctx, ctx.Client, target) {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("I can't demote @%s because they are a bot owner/admin.", username), []types.JID{resolvedJID})
			continue
		}
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
		if err != nil {
			_ = ctx.ReplyWithMentions(dispatch.Sprintf("Couldn't demote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			demoted = append(demoted, "@"+username)
			demotedJIDs = append(demotedJIDs, resolvedJID)
		}
	}

	if len(demoted) > 0 {
		return ctx.ReplyWithMentions(dispatch.Sprintf("%s is no longer a group admin.", strings.Join(demoted, ",")), demotedJIDs)
	}
	return nil
}

func handleGroup(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can change group settings.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions to modify group settings. Please promote me to admin first!")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("*Group Settings Commands*\n\n"+
			"• `%sgroup open` — Open chat (everyone can send messages)\n"+
			"• `%sgroup close` — Close chat (only admins can send messages)\n"+
			"• `%sgroup lock` — Lock settings (only admins can edit group info)\n"+
			"• `%sgroup unlock` — Unlock settings (everyone can edit group info)\n"+
			"• `%sgroup linkmode <admin|all>` — Control who can share the invite link\n"+
			"• `%sgroup history <admin|all>` — Control if new members see chat history\n"+
			"• `%sgroup addmode <admin|all>` — Control who can add new members",
			p, p, p, p, p, p, p)
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "open":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Replyf("Failed to open group chat: %v", err)
		}
		return ctx.Reply("Group opened! Everyone can send messages now.")
	case "close":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Replyf("Failed to close group chat: %v", err)
		}
		return ctx.Reply("Group closed! Only admins can send messages now.")
	case "lock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Replyf("Failed to lock group settings: %v", err)
		}
		return ctx.Reply("Group settings locked! Only admins can edit the group name, icon, and description.")
	case "unlock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Replyf("Failed to unlock group settings: %v", err)
		}
		return ctx.Reply("Group settings unlocked! Any member can now edit the group name, icon, and description.")
	case "linkmode", "memberlink":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Please specify who can share the invite link: `%sgroup linkmode admin` or `%sgroup linkmode all`", p, p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberLinkMode
		switch modeArg {
		case "admin", "admin_link", "admins":
			mode = types.GroupMemberLinkModeAdmin
		case "all", "all_member_link", "everyone", "members":
			mode = types.GroupMemberLinkModeAllMember
		default:
			return ctx.Reply("Invalid option. Please choose either `admin` or `all`.")
		}
		err = ctx.Client.SetGroupMemberLinkMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to update invite link setting: %v", err)
		}
		if mode == types.GroupMemberLinkModeAdmin {
			return ctx.Reply("Invite link setting updated: Only admins can manage and share the group link.")
		}
		return ctx.Reply("Invite link setting updated: Any member can now share the group link.")
	case "history", "sharehistory", "historymode":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Please specify chat history visibility for newcomers: `%sgroup history admin` or `%sgroup history all`", p, p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberShareHistoryMode
		switch modeArg {
		case "admin", "admin_share", "admins":
			mode = types.GroupMemberShareHistoryModeAdmin
		case "all", "all_member_share", "everyone", "members":
			mode = types.GroupMemberShareHistoryModeAllMember
		default:
			return ctx.Reply("Invalid option. Please choose either `admin` or `all`.")
		}
		err = ctx.Client.SetGroupMemberShareHistoryMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to update history sharing setting: %v", err)
		}
		if mode == types.GroupMemberShareHistoryModeAdmin {
			return ctx.Reply("History mode updated: New members will not see past chat history.")
		}
		return ctx.Reply("History mode updated: New members can now read recent chat history.")
	case "addmode", "memberadd":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Please specify who can add new members: `%sgroup addmode admin` or `%sgroup addmode all`", p, p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberAddMode
		switch modeArg {
		case "admin", "admin_add", "admins":
			mode = types.GroupMemberAddModeAdmin
		case "all", "all_member_add", "everyone", "members":
			mode = types.GroupMemberAddModeAllMember
		default:
			return ctx.Reply("Invalid option. Please choose either `admin` or `all`.")
		}
		err = ctx.Client.SetGroupMemberAddMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to update member adding setting: %v", err)
		}
		if mode == types.GroupMemberAddModeAdmin {
			return ctx.Reply("Member addition setting updated: Only admins can add members.")
		}
		return ctx.Reply("Member addition setting updated: Any member can now add new participants.")
	default:
		return ctx.Reply("Unknown action. Usage: group <open|close|lock|unlock|linkmode|history|addmode>")
	}
}

func handleAntiLink(ctx *dispatch.Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can change anti-link settings.")
	}

	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}

	groupName := info.Name
	if groupName == "" {
		groupName = ctx.Chat.String()
	}

	chatKey := ctx.Chat.String()
	statusKey := "antilink:" + chatKey
	modeKey := "antilink_mode:" + chatKey
	customKey := "antilink_custom:" + chatKey

	args := ctx.Args
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	p := ctx.GetPrefix()

	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection is now active for this group.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been turned off.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiLinkMenu(ctx, s, "Anti-link protection has been turned off.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection is now active for this group.")

	case "mode", "customize":
		bodyText := dispatch.NewText().
			Header("ANTILINK PROTECTION MODE").
			Field("Group", groupName).
			Blank().
			Section("Choose how links should be filtered:").
			Numbered(1, "All Links: Block any web links (http, https, www, .com, etc.)").
			Numbered(2, "Specific Links Only: Block only specific domains you choose (e.g. chat.whatsapp.com, t.me)").
			Trimmed()
		options := []string{
			"Block All Links",
			"Block Specific Domains",
		}
		return sendPollReply(ctx, bodyText, options)

	case "default":
		_ = s.PutSetting(ctx.Ctx, modeKey, "default")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		bodyText := dispatch.NewText().
			Header("ANTILINK MODE UPDATED").
			Field("Group", groupName).
			Field("Mode", "ALL WEB LINKS").
			Field("Status", "ACTIVE").
			Blank().
			Line("Anti-link is active and will remove all web links posted by non-admins!").
			Trimmed()
		options := []string{
			"Deactivate",
			"Customize Mode",
		}
		return sendPollReply(ctx, bodyText, options)

	case "custom", "set":
		customInput := ""
		if len(args) > 1 {
			customInput = strings.Join(args[1:], "")
		} else if len(args) == 1 && sub != "custom" {
			customInput = args[0]
		}

		customInput = strings.TrimSpace(customInput)
		if customInput == "" || customInput == "custom" {
			_ = s.PutSetting(ctx.Ctx, modeKey, "custom")
			_ = s.PutSetting(ctx.Ctx, statusKey, "on")
			currCustom, _ := s.GetSetting(ctx.Ctx, customKey)
			if currCustom == "" {
				currCustom = "chat.whatsapp.com"
				_ = s.PutSetting(ctx.Ctx, customKey, currCustom)
			}
			bodyText := dispatch.NewText().
				Header("ANTILINK CUSTOM DOMAINS").
				Field("Group", groupName).
				Field("Mode", "CUSTOM DOMAINS").
				Field("Status", "ACTIVE").
				Field("Blocked", currCustom).
				Blank().
				Section("To change the blocked domains, send:").
				Linef("%santilink set domain1, domain2", p).
				Blank().
				Section("Example:").
				Linef("%santilink set chat.whatsapp.com, t.me, instagram.com", p).
				Trimmed()
			options := []string{
				"Block All Links",
				"Deactivate",
			}
			return sendPollReply(ctx, bodyText, options)
		}

		rawParts := strings.Split(customInput, ",")
		var cleaned []string
		for _, part := range rawParts {
			part = strings.TrimSpace(strings.ToLower(part))
			if part != "" {
				cleaned = append(cleaned, part)
			}
		}
		if len(cleaned) == 0 {
			return ctx.Reply("Please specify at least one website domain separated by a comma.\n\nExample: `chat.whatsapp.com, t.me`")
		}

		newCustomStr := strings.Join(cleaned, ",")
		_ = s.PutSetting(ctx.Ctx, customKey, newCustomStr)
		_ = s.PutSetting(ctx.Ctx, modeKey, "custom")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		bodyText := dispatch.NewText().
			Header("ANTILINK CUSTOMIZED").
			Field("Group", groupName).
			Field("Mode", "CUSTOM DOMAINS").
			Field("Status", "ACTIVE").
			Field("Blocked", newCustomStr).
			Blank().
			Line("Anti-link is now active and will moderate messages containing these domain patterns!").
			Trimmed()
		options := []string{
			"Deactivate",
			"Customize Mode",
		}
		return sendPollReply(ctx, bodyText, options)

	case "action", "act":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			if act != "delete" && act != "kick" && act != "warn" {
				return ctx.Reply("Please choose an action: `delete`, `kick`, or `warn`.")
			}
			_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, act)
			return sendAntiLinkMenu(ctx, s, dispatch.Sprintf("Anti-link consequence set to *%s*.", strings.ToUpper(act)))
		}
		bodyText := dispatch.NewText().
			Header("ANTILINK ACTION MODE").
			Field("Group", groupName).
			Blank().
			Section("What should happen when a non-admin sends an unauthorized link?").
			Numbered(1, "Delete: Just delete the message").
			Numbered(2, "Kick: Delete the message and remove the member from the group").
			Numbered(3, "Warn: Warn the member first, then kick them if they reach the limit").
			Trimmed()
		options := []string{
			"Delete Message",
			"Kick Member",
			"Warn Member",
		}
		return sendPollReply(ctx, bodyText, options)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify the maximum warnings allowed. Example: `antilink setwarn 3`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Please enter a valid positive number for the warning limit.")
		}
		_ = s.PutSetting(ctx.Ctx, "antilink_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, "warn")
		return sendAntiLinkMenu(ctx, s, dispatch.Sprintf("Anti-link warning limit set to *%d*. Consequence switched to WARN.", cnt))

	default:
		return sendAntiLinkMenu(ctx, s, "")
	}
}

func sendAntiLinkMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper, note string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}
	status, _ := s.GetSetting(ctx.Ctx, "antilink:"+chatKey)
	if status == "" {
		status = "off"
	}
	mode, _ := s.GetSetting(ctx.Ctx, "antilink_mode:"+chatKey)
	if mode == "" {
		mode = "default"
	}
	action, _ := s.GetSetting(ctx.Ctx, "antilink_action:"+chatKey)
	if action == "" {
		action = "delete"
	}
	actionDisplay := strings.ToUpper(action)
	if action == "warn" {
		maxWarn, _ := s.GetSetting(ctx.Ctx, "antilink_maxwarn:"+chatKey)
		if maxWarn == "" {
			maxWarn = "3"
		}
		actionDisplay = dispatch.Sprintf("WARN (Max: %s)", maxWarn)
	}

	custom, _ := s.GetSetting(ctx.Ctx, "antilink_custom:"+chatKey)

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("ANTILINK CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Field("Mode", strings.ToUpper(mode)).
		Field("Consequence", actionDisplay)

	if mode == "custom" && custom != "" {
		tb.Field("Blocked Domains", custom)
	}
	tb.Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	tb.Section("Available Commands:").
		Bulletf("`%santilink mode` — Choose between blocking all links or custom domains", p).
		Bulletf("`%santilink set <domain1, domain2>` — Set custom blocked domains", p).
		Bulletf("`%santilink action <delete|kick|warn>` — Choose consequence for sending links", p).
		Bulletf("`%santilink setwarn <number>` — Set how many warnings before kick", p)

	toggleText := "Activate"
	if status == "on" {
		toggleText = "Deactivate"
	}

	options := []string{
		toggleText,
		"Action Mode",
		"Customize",
	}

	return sendPollReply(ctx, tb.Trimmed(), options)
}

func handleAntiWord(ctx *dispatch.Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can change anti-word settings.")
	}

	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}

	groupName := info.Name
	if groupName == "" {
		groupName = ctx.Chat.String()
	}

	chatKey := ctx.Chat.String()
	settingKey := "antiword:" + chatKey
	raw, _ := s.GetSetting(ctx.Ctx, settingKey)
	words := strings.Fields(raw)

	args := ctx.Args
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "on")
		return sendAntiWordMenu(ctx, s, "Anti-word filter is now active for this group.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "off")
		return sendAntiWordMenu(ctx, s, "Anti-word filter has been turned off.")

	case "action", "act":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			if act != "delete" && act != "kick" && act != "warn" {
				return ctx.Reply("Please choose an action: `delete`, `kick`, or `warn`.")
			}
			_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, act)
			return sendAntiWordMenu(ctx, s, dispatch.Sprintf("Anti-word consequence set to *%s*.", strings.ToUpper(act)))
		}
		bodyText := dispatch.NewText().
			Header("ANTIWORD ACTION MODE").
			Field("Group", groupName).
			Blank().
			Section("What should happen when someone sends a banned word?").
			Numbered(1, "Delete: Just delete the message").
			Numbered(2, "Kick: Delete the message and remove the member from the group").
			Numbered(3, "Warn: Warn the member first, then kick them if they reach the limit").
			Trimmed()
		options := []string{
			"Delete Message",
			"Kick Member",
			"Warn Member",
		}
		return sendPollReply(ctx, bodyText, options)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify the maximum warnings allowed. Example: `antiword setwarn 3`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Please enter a valid positive number for the warning limit.")
		}
		_ = s.PutSetting(ctx.Ctx, "antiword_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, "warn")
		return sendAntiWordMenu(ctx, s, dispatch.Sprintf("Anti-word warning limit set to *%d*. Consequence switched to WARN.", cnt))

	case "add":
		if len(args) < 2 {
			return ctx.Reply("Which word would you like to ban? Example: `antiword add badword`")
		}
		wordToAdd := strings.ToLower(args[1])
		if slices.Contains(words, wordToAdd) {
			return ctx.Replyf("The word %q is already on the banned words list!", wordToAdd)
		}
		words = append(words, wordToAdd)
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(words, ""))
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "on")
		return sendAntiWordMenu(ctx, s, dispatch.Sprintf("Added %q to the banned words list.", wordToAdd))

	case "del", "remove":
		if len(args) < 2 {
			return ctx.Reply("Which word would you like to unban? Example: `antiword del badword`")
		}
		wordToDel := strings.ToLower(args[1])
		found := false
		var newWords []string
		for _, w := range words {
			if w == wordToDel {
				found = true
			} else {
				newWords = append(newWords, w)
			}
		}
		if !found {
			return ctx.Replyf("The word %q was not found on the banned words list.", wordToDel)
		}
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(newWords, ""))
		return sendAntiWordMenu(ctx, s, dispatch.Sprintf("Removed %q from the banned words list.", wordToDel))

	case "list":
		if len(words) == 0 {
			return ctx.Reply("There are no banned words configured in this group yet. Use `antiword add <word>` to add some!")
		}
		return ctx.Replyf("*Banned Words in %s:*\n\n• %s", groupName, strings.Join(words, "\n•"))

	default:
		return sendAntiWordMenu(ctx, s, "")
	}
}

func sendAntiWordMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper, note string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, "antiword_status:"+chatKey)
	rawWord, _ := s.GetSetting(ctx.Ctx, "antiword:"+chatKey)
	words := strings.Fields(rawWord)
	if status == "" {
		if len(words) > 0 {
			status = "on"
		} else {
			status = "off"
		}
	}

	action, _ := s.GetSetting(ctx.Ctx, "antiword_action:"+chatKey)
	if action == "" {
		action = "delete"
	}
	actionDisplay := strings.ToUpper(action)
	if action == "warn" {
		maxWarn, _ := s.GetSetting(ctx.Ctx, "antiword_maxwarn:"+chatKey)
		if maxWarn == "" {
			maxWarn = "3"
		}
		actionDisplay = dispatch.Sprintf("WARN (Max: %s)", maxWarn)
	}

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("ANTIWORD CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Field("Consequence", actionDisplay).
		Fieldf("Banned Words", "%d word(s)", len(words)).
		Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	if len(words) > 0 {
		tb.Linef("Current Words: %s", strings.Join(words, ",")).Blank()
	}

	tb.Section("Available Commands:").
		Bulletf("`%santiword add <word>` — Add a new banned word", p).
		Bulletf("`%santiword del <word>` — Remove a banned word", p).
		Bulletf("`%santiword action <delete|kick|warn>` — Choose consequence for saying banned words", p).
		Bulletf("`%santiword setwarn <number>` — Set warning limit before kick", p).
		Bulletf("`%santiword list` — View all banned words in this group", p)

	toggleText := "Activate"
	if status == "on" {
		toggleText = "Deactivate"
	}

	options := []string{
		toggleText,
		"Action Mode",
		"List Words",
	}

	return sendPollReply(ctx, tb.Trimmed(), options)
}

func handleGStats(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database is currently unavailable.")
	}

	chatStr := ctx.Chat.String()

	var totalMsgs int
	err := db.QueryRow(ctx.Ctx, `SELECT COUNT(*) FROM whatsmeow_message_secrets WHERE chat_jid=$1`, chatStr).Scan(&totalMsgs)
	if err != nil {
		return err
	}

	if totalMsgs == 0 {
		return ctx.Reply("No recorded message activity for this group yet. Start chatting and check back soon!")
	}

	var activeUsers int
	err = db.QueryRow(ctx.Ctx, `SELECT COUNT(DISTINCT sender_jid) FROM whatsmeow_message_secrets WHERE chat_jid=$1`, chatStr).Scan(&activeUsers)
	if err != nil {
		activeUsers = 0
	}

	rows, err := db.Query(ctx.Ctx, `
		SELECT sender_jid, COUNT(*) as total
		FROM whatsmeow_message_secrets
		WHERE chat_jid=$1
		GROUP BY sender_jid
		ORDER BY total DESC
		LIMIT 10
	`, chatStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	tb := ctx.Text().
		Header("Group Activity Leaderboard").
		Fieldf("Total Messages Tracked", "%d", totalMsgs).
		Fieldf("Active Members", "%d", activeUsers).
		Blank().
		Section("Top Active Members:")

	rank := 1
	for rows.Next() {
		var userStr string
		var count int
		if err := rows.Scan(&userStr, &count); err == nil {
			if uj, err := types.ParseJID(userStr); err == nil {
				uj = uj.ToNonAD()
				resolvedJID, username := ctx.ResolveMention(uj)
				tb.Numbered(rank, dispatch.Sprintf("@%s (%d msgs)", username, count)).
					Mentions(resolvedJID)
				rank++
			}
		}
	}

	return tb.Reply()
}

func handlePoll(ctx *dispatch.Context) error {
	raw := strings.TrimSpace(ctx.RawArgs)
	if raw == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Let's create a poll! Here's how to use it:\n\n• `%spoll What's for lunch? | Pizza | Burger | Tacos`\n• Single choice: `%spoll --single Best day? | Friday | Saturday`\n• Multiple choice: `%spoll --multi Favorite colors? | Blue | Green | Red`", p, p, p)
	}

	selectableCount := -1
	if strings.HasPrefix(raw, "--single") || strings.HasPrefix(raw, "-s") || strings.HasPrefix(raw, "single") {
		selectableCount = 1
		raw = strings.TrimSpace(raw[strings.Index(raw, ""):])
	} else if strings.HasPrefix(raw, "--multi") || strings.HasPrefix(raw, "-m") || strings.HasPrefix(raw, "multi") || strings.HasPrefix(raw, "multiple") {
		selectableCount = 0
		raw = strings.TrimSpace(raw[strings.Index(raw, ""):])
	}

	parts := strings.Split(raw, "|")
	if len(parts) < 3 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Please provide a question and at least 2 options separated by `|`.\n\nExample:\n• `%spoll What's your choice? | Option 1 | Option 2`", p)
	}

	question := strings.TrimSpace(parts[0])
	var options []string
	for _, opt := range parts[1:] {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" {
			options = append(options, trimmed)
		}
	}
	if len(options) < 2 {
		return ctx.Reply("A poll needs at least 2 different options! Please add more choices.")
	}

	if selectableCount >= 0 {
		poll := ctx.Rook().NewPoll(question)
		if selectableCount == 0 {
			poll.MultiChoice()
		} else {
			poll.SingleChoice()
		}
		for _, opt := range options {
			poll.AddOption(opt)
		}
		return poll.Reply(func(req dispatch.PollRequest, res *builder.Response) {
			if len(req.SelectedOptions) > 0 {
				_ = res.Reply(dispatch.Sprintf("Vote recorded for: *%s*", strings.Join(req.SelectedOptions, ",")))
			}
		})
	}

	tb := ctx.Text().
		Line("*Poll Setup*").
		Blank().
		Field("Question", question).
		Blank().
		Section("Options:")

	for i, opt := range options {
		tb.Numbered(i+1, opt)
	}
	tb.Blank().
		Line("Choose whether members can select only one answer or multiple answers below:")

	_ = ctx.Reply(tb.String())

	poll := ctx.Rook().NewPoll("Select Poll Type:")
	poll.AddOption("Single Choice (one answer)").
		AddOption("Multiple Choice (many answers)")

	return poll.Reply(func(req dispatch.PollRequest, res *builder.Response) {
		if len(req.SelectedOptions) > 0 {
			single := strings.HasPrefix(req.SelectedOptions[0], "Single")
			subPoll := res.Rook().NewPoll(question)
			if single {
				subPoll.SingleChoice()
			} else {
				subPoll.MultiChoice()
			}
			for _, opt := range options {
				subPoll.AddOption(opt)
			}
			_ = subPoll.Reply()
		}
	})
}

func handleInvite(ctx *dispatch.Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can retrieve the invite link.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to get the invite link. Please make me an admin first!")
	}

	link, err := ctx.Client.GetGroupInviteLink(ctx.Ctx, ctx.Chat, false)
	if err != nil {
		return ctx.Replyf("Couldn't generate the invite link: %v", err)
	}
	return ctx.Replyf("*Group Invite Link:*\n%s", link)
}

func IsUserOnline(jid types.JID, client *whatsmeow.Client) bool {
	if jid.IsEmpty() {
		return false
	}
	targetKey := jid.ToNonAD().String()

	PresenceMu.RLock()
	info, exists := PresenceMap[targetKey]
	PresenceMu.RUnlock()

	if exists && (info.IsOnline || time.Since(info.LastSeen) < 15*time.Minute) {
		logger.Debug("IsUserOnline check: direct match online", "jid", targetKey, "lastSeen", info.LastSeen)
		return true
	}

	if client != nil && client.Store != nil && client.Store.LIDs != nil {
		ctx := context.Background()
		if jid.Server == types.HiddenUserServer {
			pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
			if err == nil && !pn.IsEmpty() {
				pnKey := pn.ToNonAD().String()
				PresenceMu.RLock()
				pnInfo, pnExists := PresenceMap[pnKey]
				PresenceMu.RUnlock()
				if pnExists && (pnInfo.IsOnline || time.Since(pnInfo.LastSeen) < 15*time.Minute) {
					logger.Debug("IsUserOnline check: PN match online for LID", "lid", targetKey, "pn", pnKey)
					return true
				}
			}
		} else {
			lid, err := client.Store.LIDs.GetLIDForPN(ctx, jid)
			if err == nil && !lid.IsEmpty() {
				lidKey := lid.ToNonAD().String()
				PresenceMu.RLock()
				lidInfo, lidExists := PresenceMap[lidKey]
				PresenceMu.RUnlock()
				if lidExists && (lidInfo.IsOnline || time.Since(lidInfo.LastSeen) < 15*time.Minute) {
					logger.Debug("IsUserOnline check: LID match online for PN", "pn", targetKey, "lid", lidKey)
					return true
				}
			}
		}
	}

	logger.Debug("IsUserOnline check: offline or unknown", "jid", targetKey)
	return false
}

func handleListOnline(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		logger.Debug("handleListOnline: not a group chat", "chat", ctx.Chat.String())
		return ctx.Reply("This command can only be used inside a group chat.")
	}

	logger.Debug("handleListOnline executing", "group", ctx.Chat.String())
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		logger.Error("handleListOnline: failed to get group info", "group", ctx.Chat.String(), "err", err)
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}

	total := len(info.Participants)
	logger.Debug("handleListOnline retrieved group info", "group", ctx.Chat.String(), "participant_count", total)

	if total == 0 {
		return ctx.Reply("No members found in this group.")
	}

	// 1. Send status message to prompt WhatsApp servers to trigger group-wide delivery receipts
	_ = ctx.Reply("Checking who's active right now... Just a second!")

	// Build set of expected participant JID keys (LID & PN formats)
	expectedJIDs := make(map[string]types.JID)
	var mu sync.Mutex
	receivedCount := 0
	doneChan := make(chan struct{})

	var lidJIDs, pnJIDs []types.JID
	for _, p := range info.Participants {
		nonAD := p.JID.ToNonAD()
		expectedJIDs[nonAD.String()] = nonAD
		if nonAD.Server == types.HiddenUserServer {
			lidJIDs = append(lidJIDs, nonAD)
		} else {
			pnJIDs = append(pnJIDs, nonAD)
		}
	}
	if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
		if len(lidJIDs) > 0 {
			for _, lid := range lidJIDs {
				if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, lid); err == nil && !pn.IsEmpty() {
					expectedJIDs[pn.ToNonAD().String()] = expectedJIDs[pn.User+"@"+types.HiddenUserServer]
				}
			}
		}
		if len(pnJIDs) > 0 {
			if lidMap, err := ctx.Client.Store.LIDs.GetManyLIDsForPNs(ctx.Ctx, pnJIDs); err == nil {
				for pn, lid := range lidMap {
					if !lid.IsEmpty() {
						expectedJIDs[lid.ToNonAD().String()] = expectedJIDs[pn.String()]
					}
				}
			}
		}
	}

	// Register temporary event listener for WhatsApp presence and receipt response stanzas
	handlerID := ctx.Client.AddEventHandler(func(evt any) {
		switch pEvt := evt.(type) {
		case *events.Presence:
			fromKey := pEvt.From.ToNonAD().String()
			mu.Lock()
			if targetJID, isExpected := expectedJIDs[fromKey]; isExpected {
				logger.Debug("handleListOnline: received presence stanza from WhatsApp", "from", fromKey, "unavailable", pEvt.Unavailable)
				TrackPresence(targetJID, !pEvt.Unavailable)
				delete(expectedJIDs, fromKey)
				receivedCount++
				if len(expectedJIDs) == 0 {
					select {
					case <-doneChan:
					default:
						close(doneChan)
					}
				}
			}
			mu.Unlock()

		case *events.Receipt:
			senderKey := pEvt.Sender.ToNonAD().String()
			if !pEvt.Sender.IsEmpty() {
				mu.Lock()
				if targetJID, isExpected := expectedJIDs[senderKey]; isExpected {
					logger.Debug("handleListOnline: received delivery receipt from WhatsApp", "sender", senderKey)
					TrackPresence(targetJID, true)
					delete(expectedJIDs, senderKey)
					receivedCount++
					if len(expectedJIDs) == 0 {
						select {
						case <-doneChan:
						default:
							close(doneChan)
						}
					}
				}
				mu.Unlock()
			}
		}
	})
	defer ctx.Client.RemoveEventHandler(handlerID)

	// Dispatch SubscribePresence to WhatsApp for all group participants
	for _, p := range info.Participants {
		_ = ctx.Client.SubscribePresence(ctx.Ctx, p.JID)
	}

	cachedOnlineCount := 0
	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			cachedOnlineCount++
		}
	}

	// If cached online records are small, wait up to 2s for live presence/receipt stanzas
	if cachedOnlineCount < 2 {
		select {
		case <-doneChan:
			logger.Debug("handleListOnline: presence/receipt stanzas collected", "count", receivedCount)
		case <-time.After(2000 * time.Millisecond):
			logger.Debug("handleListOnline: presence wait window ended", "received", receivedCount, "total", total)
		}
	}

	var onlineJIDs []types.JID
	var displayNames []string

	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			resolvedJID, username := ctx.ResolveMention(p.JID)
			onlineJIDs = append(onlineJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
			logger.Debug("handleListOnline: participant online", "participant", p.JID.String(), "username", username)
		} else {
			logger.Debug("handleListOnline: participant offline", "participant", p.JID.String())
		}
	}

	logger.Debug("handleListOnline complete", "group", ctx.Chat.String(), "total_participants", total, "online_count", len(onlineJIDs))

	if len(onlineJIDs) == 0 {
		return ctx.Reply("No active members detected right now. Members may have their online status or read receipts hidden.")
	}

	tb := ctx.Text().
		Headerf("Online & Active Members (%d)", len(onlineJIDs)).
		Mentions(onlineJIDs...)

	for _, name := range displayNames {
		tb.Bullet(name)
	}

	return tb.Reply()
}

func handleKickAll(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}

	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}

	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins or bot owners can use kickall.")
	}

	if !ctx.AmIAdmin(info) {
		return ctx.Reply("I need admin permissions in this group to remove members.")
	}

	var toKick []types.JID
	for _, p := range info.Participants {
		if ctx.Client != nil && ctx.Client.Store != nil {
			if ctx.Client.Store.ID != nil && whatsrook.ParticipantMatchesUser(ctx.Ctx, ctx.Client, p, *ctx.Client.Store.ID) {
				continue
			}
			if !ctx.Client.Store.LID.IsEmpty() && whatsrook.ParticipantMatchesUser(ctx.Ctx, ctx.Client, p, ctx.Client.Store.LID) {
				continue
			}
		}
		if whatsrook.ParticipantMatchesUser(ctx.Ctx, ctx.Client, p, ctx.Sender) {
			continue
		}
		if ctx.IsTargetSudo(p.JID) || (!p.PhoneNumber.IsEmpty() && ctx.IsTargetSudo(p.PhoneNumber)) || (!p.LID.IsEmpty() && ctx.IsTargetSudo(p.LID)) {
			continue
		}
		toKick = append(toKick, p.JID)
	}

	if len(toKick) == 0 {
		return ctx.Reply("There are no eligible members to remove.")
	}

	_ = ctx.Replyf("Removing %d members from the group...", len(toKick))
	_, err = ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, toKick, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		logger.Error("Kickall failed", "err", err)
		return ctx.Replyf("Failed to remove some members: %v", err)
	}

	return ctx.Replyf("Cleanup complete! Successfully removed %d members.", len(toKick))
}

func handleLeave(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}

	senderUser := ctx.Sender.ToNonAD().User

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if strings.HasPrefix(arg0, "confirm") {
		parts := strings.Split(arg0, "_")
		if len(parts) >= 2 {
			callerUser := parts[1]
			if senderUser != callerUser && !ctx.IsSudo() {
				callerMention, _ := ctx.ResolveMention(types.NewJID(callerUser, "s.whatsapp.net"))
				return ctx.ReplyWithMentions(dispatch.Sprintf("Only the person who asked me to leave (%s) can confirm this action.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}

		_ = ctx.Reply("Goodbye everyone! Leaving the group now.")
		err := ctx.Client.LeaveGroup(ctx.Ctx, ctx.Chat)
		if err != nil {
			logger.Error("Failed to leave group", "err", err)
			return ctx.Replyf("Failed to leave the group: %v", err)
		}
		return nil
	}

	if strings.HasPrefix(arg0, "cancel") {
		parts := strings.Split(arg0, "_")
		if len(parts) >= 2 {
			callerUser := parts[1]
			if senderUser != callerUser && !ctx.IsSudo() {
				callerMention, _ := ctx.ResolveMention(types.NewJID(callerUser, "s.whatsapp.net"))
				return ctx.ReplyWithMentions(dispatch.Sprintf("Only the person who asked me to leave (%s) can cancel this action.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}
		return ctx.Reply("Leave request cancelled. I'm staying in the group!")
	}

	bodyText := "*Are you sure you want me to leave this group?*\n\nPlease confirm or cancel your request using the options below:"
	options := []string{
		"Confirm Leave",
		"Cancel",
	}

	return sendPollReply(ctx, bodyText, options)
}

func handleJoin(ctx *dispatch.Context) error {
	var inviteMsg *waE2E.GroupInviteMessage
	var isQuoted bool

	if quoted := ctx.GetQuotedMessage(); quoted != nil && quoted.GetGroupInviteMessage() != nil {
		inviteMsg = quoted.GetGroupInviteMessage()
		isQuoted = true
	} else if ctx.Evt != nil && ctx.Evt.Message != nil && ctx.Evt.Message.GetGroupInviteMessage() != nil {
		inviteMsg = ctx.Evt.Message.GetGroupInviteMessage()
	}

	if inviteMsg != nil {
		return handleJoinV4(ctx, inviteMsg, isQuoted)
	}

	code := extractGroupInviteCode(ctx)
	if code == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Please provide a WhatsApp group invite link!\n\nExample:\n• `%sjoin https://chat.whatsapp.com/XXXXX`", p)
	}

	jid, err := ctx.Client.JoinGroupWithLink(ctx.GetSendContext(), code)
	if err != nil {
		return ctx.Replyf("Couldn't join the group: %v", err)
	}

	groupName := ""
	if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), jid); errInfo == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}

	if groupName != "" {
		return ctx.Replyf("Successfully joined *%s*!", groupName)
	}
	return ctx.Reply("Successfully joined the group!")
}

func handleJoinV4(ctx *dispatch.Context, inviteMsg *waE2E.GroupInviteMessage, isQuoted bool) error {
	groupJIDStr := inviteMsg.GetGroupJID()
	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil || groupJID.IsEmpty() {
		return ctx.Reply("Invalid group information found in this invite.")
	}

	code := inviteMsg.GetInviteCode()
	if code == "" {
		return ctx.Reply("This invite message is missing a valid join code.")
	}

	expiration := inviteMsg.GetInviteExpiration()

	var inviterJID types.JID
	if isQuoted {
		if sender, ok := ctx.GetQuotedSender(); ok && !sender.IsEmpty() {
			inviterJID = sender
		}
	}
	if inviterJID.IsEmpty() {
		if ci := inviteMsg.GetContextInfo(); ci != nil && ci.GetParticipant() != "" {
			if parsed, errP := types.ParseJID(ci.GetParticipant()); errP == nil && !parsed.IsEmpty() {
				inviterJID = parsed
			}
		}
	}
	if inviterJID.IsEmpty() {
		inviterJID = ctx.Sender
	}

	err = ctx.Client.JoinGroupWithInvite(ctx.GetSendContext(), groupJID, inviterJID, code, expiration)
	if err != nil {
		return ctx.Replyf("Couldn't join the group from this invite: %v", err)
	}

	groupName := inviteMsg.GetGroupName()
	if groupName == "" {
		if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), groupJID); errInfo == nil && info != nil && info.Name != "" {
			groupName = info.Name
		}
	}

	if groupName != "" {
		return ctx.Replyf("Successfully joined *%s*!", groupName)
	}
	return ctx.Reply("Successfully joined the group!")
}

func extractGroupInviteCode(ctx *dispatch.Context) string {
	if ctx == nil {
		return ""
	}
	if ctx.RawArgs != "" {
		if match := GroupInviteLinkRegex.FindStringSubmatch(ctx.RawArgs); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(ctx.RawArgs)
		if !strings.ContainsAny(trimmed, "\t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		text := extractMessageText(quoted)
		if match := GroupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(text)
		if !strings.ContainsAny(trimmed, "\t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if ctx.Evt != nil && ctx.Evt.Message != nil {
		text := extractMessageText(ctx.Evt.Message)
		if match := GroupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

func extractMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetConversation(); text != "" {
		return text
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	if inv := msg.GetGroupInviteMessage(); inv != nil && inv.GetCaption() != "" {
		return inv.GetCaption()
	}
	return ""
}

func handleAntiMsg(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "antimsg_status:" + chatKey
	usersKey := "antimsg_users:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	// 1. Explicit global state commands that do not target participants
	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiMsgMenu(ctx, s, "Anti-Message is now ON! Messages from targeted members will be automatically deleted.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiMsgMenu(ctx, s, "Anti-Message has been turned off.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiMsgMenu(ctx, s, "Anti-Message has been turned off.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiMsgMenu(ctx, s, "Anti-Message is now ON! Messages from targeted members will be automatically deleted.")

	case "clear":
		_ = s.PutSetting(ctx.Ctx, usersKey, "")
		return sendAntiMsgMenu(ctx, s, "All targeted members have been removed from the Anti-Message list.")

	case "list", "status":
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		if status == "" {
			status = "off"
		}

		p := ctx.GetPrefix()
		if len(users) == 0 {
			bodyText := dispatch.NewText().
				Header("ANTIMSG TARGETS").
				Field("Status", strings.ToUpper(status)).
				Field("Targets", "None").
				Blank().
				Line("No members are currently targeted in this group.").
				Linef("Reply to anyone's message or tag them with `%santimsg @user` to add them.", p).
				Trimmed()
			options := []string{"Activate"}
			return sendPollReply(ctx, bodyText, options)
		}

		var mentions []types.JID
		var displayUsers []types.JID

		for _, u := range users {
			uj, err := types.ParseJID(u)
			if err != nil || uj.IsEmpty() {
				continue
			}
			if !slices.ContainsFunc(displayUsers, func(existing types.JID) bool {
				return whatsrook.IsSameUserRaw(ctx.Ctx, ctx.Client, existing, uj)
			}) {
				displayUsers = append(displayUsers, uj)
			}
		}

		tb := ctx.Text().
			Header("ANTIMSG TARGETS").
			Field("Status", strings.ToUpper(status)).
			Fieldf("Total Targeted", "%d member(s)", len(displayUsers)).
			Blank().
			Section("Targeted Members:")

		for _, uj := range displayUsers {
			resolvedJID, username := ctx.ResolveMention(uj)
			tb.Bullet("@" + username)
			mentions = append(mentions, resolvedJID)
		}

		toggleText := "Activate"
		if status == "on" {
			toggleText = "Deactivate"
		}

		options := []string{
			toggleText,
			"Clear Targets",
		}

		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, mentions)

	case "del", "remove", "delete":
		targets := extractTargetParticipants(ctx, args)
		if len(targets) == 0 {
			return ctx.Reply("Please reply to a member's message or tag them (@user) to remove them from Anti-Message.")
		}
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)

		var removedMentions []types.JID
		var removedUsernames []string

		for _, t := range targets {
			newUsers := make([]string, 0, len(users))
			for _, uStr := range users {
				uJID, err := types.ParseJID(uStr)
				if err == nil && whatsrook.IsSameUserRaw(ctx.Ctx, ctx.Client, uJID, t) {
					continue
				}
				newUsers = append(newUsers, uStr)
			}
			users = newUsers
			resolvedJID, username := ctx.ResolveMention(t)
			removedMentions = append(removedMentions, resolvedJID)
			removedUsernames = append(removedUsernames, "@"+username)
		}

		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		bodyText := dispatch.NewText().
			Header("ANTIMSG UPDATED").
			Field("Removed", strings.Join(removedUsernames, ",")).
			Fieldf("Remaining Targets", "%d member(s)", len(users)).
			Trimmed()

		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		toggleText := "Activate"
		if status == "on" {
			toggleText = "Deactivate"
		}

		options := []string{
			toggleText,
			"Target List",
		}

		return sendPollReplyWithMentions(ctx, bodyText, options, removedMentions)
	}

	// 2. Target addition: when sub is "add", or user simply runs .antimsg @user or replies to user
	targets := extractTargetParticipants(ctx, args)
	if len(targets) > 0 {
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)

		var addedMentions []types.JID
		var addedUsernames []string
		var sudoRejected []string

		for _, t := range targets {
			resolvedTarget := ctx.ResolvePN(t)
			tStr := resolvedTarget.ToNonAD().String()

			if ctx.IsTargetSudo(resolvedTarget) {
				_, username := ctx.ResolveMention(resolvedTarget)
				sudoRejected = append(sudoRejected, "@"+username)
				continue
			}

			isAlreadyTargeted := false
			for _, uStr := range users {
				uJID, err := types.ParseJID(uStr)
				if err == nil && whatsrook.IsSameUserRaw(ctx.Ctx, ctx.Client, uJID, resolvedTarget) {
					isAlreadyTargeted = true
					break
				}
			}
			if !isAlreadyTargeted {
				users = append(users, tStr)
				resolvedJID, username := ctx.ResolveMention(resolvedTarget)
				addedMentions = append(addedMentions, resolvedJID)
				addedUsernames = append(addedUsernames, "@"+username)
			}
		}

		if len(addedUsernames) == 0 && len(sudoRejected) > 0 {
			return ctx.Replyf("Cannot target bot owners or sudo users (%s).", strings.Join(sudoRejected, ","))
		}

		if len(addedUsernames) == 0 {
			return ctx.Reply("The specified member(s) are already on the Anti-Message target list.")
		}

		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		tb := dispatch.NewText().
			Header("ANTIMSG ACTIVATED").
			Field("Status", "ACTIVE").
			Field("Added", strings.Join(addedUsernames, ",")).
			Fieldf("Total Targets", "%d member(s)", len(users)).
			Blank().
			Line("Anti-Message is active! Any new messages sent by these members will be automatically deleted.")
		if len(sudoRejected) > 0 {
			tb.Blank().Linef("Skipped bot owner/sudo users: %s", strings.Join(sudoRejected, ","))
		}
		bodyText := tb.Trimmed()

		options := []string{
			"Deactivate",
			"Target List",
			"Clear Targets",
		}

		return sendPollReplyWithMentions(ctx, bodyText, options, addedMentions)
	}

	if sub == "add" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to target?\n\n• Reply to their message with `%santimsg`\n• Or tag them: `%santimsg @user`", p, p)
	}

	// 3. Default menu display
	return sendAntiMsgMenu(ctx, s, "")
}

func sendAntiMsgMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper, note string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}
	status, _ := s.GetSetting(ctx.Ctx, "antimsg_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	rawUsers, _ := s.GetSetting(ctx.Ctx, "antimsg_users:"+chatKey)
	users := splitCSV(rawUsers)

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("ANTIMSG CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Fieldf("Targeted Members", "%d member(s)", len(users)).
		Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	tb.Section("How to use Anti-Message:").
		Bulletf("Reply to someone's message with `%santimsg` to target them", p).
		Bulletf("Or tag them: `%santimsg @user`", p).
		Bulletf("Remove a member: `%santimsg del @user`", p).
		Bulletf("View targeted members: `%santimsg list`", p).
		Bulletf("Clear all targets: `%santimsg clear`", p)

	toggleText := "Activate"
	if status == "on" {
		toggleText = "Deactivate"
	}

	options := []string{
		toggleText,
		"Target List",
		"Clear Targets",
	}

	return sendPollReply(ctx, tb.Trimmed(), options)
}

func isSubcommand(s string) bool {
	s = strings.ToLower(s)
	return s == "add" || s == "del" || s == "remove" || s == "delete" ||
		s == "on" || s == "off" || s == "toggle" || s == "list" ||
		s == "clear" || s == "enable" || s == "disable" || s == "status" ||
		s == "activate" || s == "deactivate"
}

func extractTargetParticipants(ctx *dispatch.Context, args []string) []types.JID {
	var targets []types.JID

	addJID := func(j types.JID) {
		if j.IsEmpty() {
			return
		}
		resolved := ctx.ResolvePN(j)
		if !slices.ContainsFunc(targets, func(existing types.JID) bool {
			return whatsrook.IsSameUserRaw(ctx.Ctx, ctx.Client, existing, resolved)
		}) {
			targets = append(targets, resolved)
		}
	}

	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
			botJID := ctx.Client.Store.ID.ToNonAD()
			if !whatsrook.IsSameUserRaw(ctx.Ctx, ctx.Client, quotedSender, botJID) {
				addJID(quotedSender)
			}
		} else {
			addJID(quotedSender)
		}
	}

	if ci := ctx.GetContextInfo(); ci != nil {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := parseUserJID(m); err == nil && !parsed.IsEmpty() {
				addJID(parsed)
			}
		}
	}

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || isSubcommand(arg) {
			continue
		}
		if parsed, err := parseUserJID(arg); err == nil && !parsed.IsEmpty() {
			addJID(parsed)
		}
	}

	return targets
}

func handleAntiSpam(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "antispam_status:" + chatKey
	actionKey := "antispam_action:" + chatKey
	maxKey := "antispam_max:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAntiSpamMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate":
		if err := s.PutSetting(ctx.Ctx, statusKey, "on"); err != nil {
			return ctx.Reply("Failed to enable Anti-Spam.")
		}
		return ctx.Reply("Anti-Spam protection is now ON! Rapid messages will be automatically moderated.")

	case "off", "disable", "deactivate":
		if err := s.PutSetting(ctx.Ctx, statusKey, "off"); err != nil {
			return ctx.Reply("Failed to disable Anti-Spam.")
		}
		return ctx.Reply("Anti-Spam protection has been turned off.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		nextState := "on"
		if curr == "on" {
			nextState = "off"
		}
		if err := s.PutSetting(ctx.Ctx, statusKey, nextState); err != nil {
			return ctx.Reply("Failed to toggle Anti-Spam.")
		}
		if nextState == "on" {
			return ctx.Reply("Anti-Spam protection is now ON!")
		}
		return ctx.Reply("Anti-Spam protection has been turned off.")

	case "customize", "custom", "help":
		return sendAntiSpamCustomizeGuide(ctx)

	case "action":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, actionKey)
			if curr == "" {
				curr = "delete"
			}
			return ctx.Replyf("Current Anti-Spam action: *%s*\n\nUsage: `%santispam action [delete|warn|kick]`", curr, ctx.GetPrefix())
		}
		act := strings.ToLower(args[1])
		if act != "delete" && act != "warn" && act != "kick" {
			return ctx.Reply("Please choose an action: `delete` (delete message), `warn` (give warning), or `kick` (remove member).")
		}
		if err := s.PutSetting(ctx.Ctx, actionKey, act); err != nil {
			return ctx.Reply("Failed to update Anti-Spam action.")
		}
		return ctx.Replyf("Anti-Spam consequence updated to *%s*.", strings.ToUpper(act))

	case "max", "threshold", "limit":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, maxKey)
			if curr == "" {
				curr = "5"
			}
			return ctx.Replyf("Current rate limit: *%s messages per 5 seconds*\n\nUsage: `%santispam max <number>` (e.g. `%santispam max 5`)", curr, ctx.GetPrefix(), ctx.GetPrefix())
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 2 || num > 30 {
			return ctx.Reply("Please specify a limit between 2 and 30 messages per 5 seconds.")
		}
		if err := s.PutSetting(ctx.Ctx, maxKey, strconv.Itoa(num)); err != nil {
			return ctx.Reply("Failed to update Anti-Spam limit.")
		}
		return ctx.Replyf("Anti-Spam rate limit set to *%d messages per 5 seconds*.", num)

	default:
		return ctx.Replyf("Usage: `%santispam [on|off|toggle|customize|action|max|help]`", ctx.GetPrefix())
	}
}

func sendAntiSpamMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, "antispam_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := dispatch.NewText().
		Header("ANTISPAM CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Anti-Spam prevents members from flooding the group with rapid messages.").
		Blank().
		Line("Select an option below to toggle or customize settings:").
		Trimmed()

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendAntiSpamCustomizeGuide(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("ANTISPAM CUSTOMIZATION GUIDE").
		Blank().
		Section("Available Settings:").
		Bulletf("Consequence Mode : `%santispam action delete | warn | kick`", p).
		Bulletf("Message Rate Limit : `%santispam max <count>` (allowed messages per 5s)", p).
		Blank().
		Section("Examples:").
		Numberedf(1, "`%santispam action kick` — Automatically remove spammers", p).
		Numberedf(2, "`%santispam action warn` — Issue warnings before kicking", p).
		Numberedf(3, "`%santispam max 4` — Allow at most 4 messages every 5 seconds", p).
		Reply()
}

var (
	autoMuteMu     sync.Mutex
	autoMuteCancel context.CancelFunc
)

func StartAutoMuteScheduler(ctx context.Context, client *whatsmeow.Client) {
	autoMuteMu.Lock()
	defer autoMuteMu.Unlock()

	if autoMuteCancel != nil {
		autoMuteCancel()
		autoMuteCancel = nil
	}

	schedCtx, cancel := context.WithCancel(ctx)
	autoMuteCancel = cancel

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-schedCtx.Done():
				return
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("automute: PANIC in scheduler tick", "recover", r)
						}
					}()
					checkAndExecuteMuteSchedules(schedCtx, client)
				}()
			}
		}
	}()
}

func StopAutoMuteScheduler() {
	autoMuteMu.Lock()
	defer autoMuteMu.Unlock()
	if autoMuteCancel != nil {
		autoMuteCancel()
		autoMuteCancel = nil
	}
}

func handleAutoMute(ctx *dispatch.Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can configure daily auto-close schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Replyf("Please specify the daily closing time.\n\nExample:\n• `%sautomute 22:00` (closes chat at 10:00 PM every day)\n• `%sautomute off` (turns off the schedule)", p, p)
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}

	settingKey := "automute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Daily group closing schedule has been turned off.")
	}

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Please enter the time as HH:MM in 24-hour format (e.g. `22:00`) or 12-hour format (e.g. `10:00 PM`).")
	}
	arg = normalized

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save auto-close schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Replyf("Daily group close scheduled for *%s* (%s timezone).\n\nThe chat will automatically close at this time each day so only admins can send messages.", arg, tz)
}

func handleAutoUnmute(ctx *dispatch.Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used inside a group chat.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can configure daily auto-open schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Replyf("Please specify the daily reopening time.\n\nExample:\n• `%sautounmute 07:00` (reopens chat at 7:00 AM every day)\n• `%sautounmute off` (turns off the schedule)", p, p)
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}

	settingKey := "autounmute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Daily group reopening schedule has been turned off.")
	}

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Please enter the time as HH:MM in 24-hour format (e.g. `07:00`) or 12-hour format (e.g. `7:00 AM`).")
	}
	arg = normalized

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save auto-reopen schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Replyf("Daily group reopen scheduled for *%s* (%s timezone).\n\nThe chat will automatically reopen at this time each day so everyone can chat.", arg, tz)
}

func handleListMute(ctx *dispatch.Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used inside a group chat.")
	}

	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Settings store is currently unavailable.")
	}

	muteTime, _ := s.GetSetting(ctx.Ctx, "automute:"+ctx.Chat.String())
	unmuteTime, _ := s.GetSetting(ctx.Ctx, "autounmute:"+ctx.Chat.String())
	tz := getUserTimezone(ctx.Ctx, s)

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("Group Mute & Unmute Schedule").
		Field("Timezone", tz).
		Blank()

	if muteTime != "" {
		tb.Linef("Auto-Close (Mute) : %s daily", muteTime)
	} else {
		tb.Line("Auto-Close (Mute) : Disabled")
	}

	if unmuteTime != "" {
		tb.Linef("Auto-Open (Unmute) : %s daily", unmuteTime)
	} else {
		tb.Line("Auto-Open (Unmute) : Disabled")
	}

	tb.Blank().
		Section("Available Commands:").
		Bulletf("`%sautomute <HH:MM>` (e.g. `%sautomute 22:00`) — Set close time", p, p).
		Bulletf("`%sautounmute <HH:MM>` (e.g. `%sautounmute 07:00`) — Set open time", p, p).
		Bulletf("`%stimezone` — View or change bot timezone", p)

	return tb.Reply()
}

func normalizeTimeInput(s string) (string, bool) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	if strings.HasSuffix(upper, "AM") || strings.HasSuffix(upper, "PM") {
		isPM := strings.HasSuffix(upper, "PM")
		timePart := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(upper, "AM"), "PM"))

		parts := strings.Split(timePart, ":")
		if len(parts) != 2 {
			return "", false
		}
		hour, err1 := strconv.Atoi(parts[0])
		minute, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return "", false
		}
		if hour < 1 || hour > 12 || minute < 0 || minute > 59 {
			return "", false
		}

		if isPM && hour != 12 {
			hour += 12
		}
		if !isPM && hour == 12 {
			hour = 0
		}
		return dispatch.Sprintf("%02d:%02d", hour, minute), true
	}

	if len(s) != 5 || s[2] != ':' {
		return "", false
	}
	hours, err1 := strconv.Atoi(s[:2])
	mins, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil {
		return "", false
	}
	if hours < 0 || hours > 23 || mins < 0 || mins > 59 {
		return "", false
	}
	return s, true
}

func checkAndExecuteMuteSchedules(ctx context.Context, client *whatsmeow.Client) {
	if ctx.Err() != nil {
		return
	}
	if client == nil || client.Store == nil || client.Store.ID == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return
	}
	s, ok := dispatch.GetSQLStore(client)
	if !ok || s == nil {
		logger.Error("automute is not okay in the database, it will not run!, status: v%", ok)
		return
	}

	tzName := getUserTimezone(ctx, s)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		logger.Error("automute: failed to load timezone, falling back to UTC", "tz", tzName, "err", err)
		loc = time.UTC
	}

	now := time.Now().In(loc)
	currentTimeStr := dispatch.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	settings, err := store.ListSettingsWithPrefixes(ctx, s.SQLStore, "automute:", "autounmute:")
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "database is closed") || ctx.Err() != nil {
			return
		}
		logger.Error("automute: query failed", "err", err)
		return
	}

	for _, setting := range settings {
		key := setting.Key
		targetTime := setting.Value
		if targetTime != currentTimeStr {
			continue
		}

		if after, ok0 := strings.CutPrefix(key, "automute:"); ok0 {
			groupJIDStr := after
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				continue
			}

			execKey := "last_exec_automute:" + groupJIDStr
			sCtx, sCancel := context.WithTimeout(ctx, 5*time.Second)
			lastExec, sErr := s.GetSetting(sCtx, execKey)
			sCancel()
			if sErr != nil {
				logger.Error("automute: GetSetting execKey failed or timed out", "group", groupJIDStr, "err", sErr)
				continue
			}
			dateMinuteKey := dispatch.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				logger.Error("automute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				logger.Warn("automute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			isAdmin := whatsrook.IsBotAdminRaw(ctx, client, info)
			logger.Debug("automute: admin check", "group", groupJIDStr, "is_admin", isAdmin)

			if !isAdmin {
				logger.Warn("automute: bot is not admin in group, cannot mute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, true); err != nil {
				logger.Error("automute: SetGroupAnnounce(true) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				logger.Error("automute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}
			logger.Debug("automute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)

			unmuteTime, _ := s.GetSetting(ctx, "autounmute:"+groupJIDStr)
			groupName := info.GroupName.Name
			var noticeText string
			if unmuteTime != "" {
				noticeText = dispatch.Sprintf("*%s* is now closed for the night! Only admins can send messages. It will reopen at %s (%s).", groupName, unmuteTime, tzName)
			} else {
				noticeText = dispatch.Sprintf("*%s* is now closed. Only admins can send messages.", groupName)
			}
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				logger.Error("automute: failed to send close notice", "group", groupJIDStr, "err", sendErr)
			}

		} else if after, ok0 := strings.CutPrefix(key, "autounmute:"); ok0 {
			groupJIDStr := after
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				logger.Warn("autounmute: bad group JID, skipping", "raw", groupJIDStr, "err", err)
				continue
			}

			execKey := "last_exec_autounmute:" + groupJIDStr
			lastExec, _ := s.GetSetting(ctx, execKey)
			dateMinuteKey := dispatch.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				logger.Error("autounmute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				logger.Warn("autounmute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			isAdmin := whatsrook.IsBotAdminRaw(ctx, client, info)
			logger.Debug("autounmute: admin check", "group", groupJIDStr, "is_admin", isAdmin)

			if !isAdmin {
				logger.Warn("autounmute: bot is not admin in group, cannot unmute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, false); err != nil {
				logger.Error("autounmute: SetGroupAnnounce(false) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				logger.Error("autounmute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}

			logger.Debug("autounmute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)
			groupName := info.GroupName.Name
			noticeText := dispatch.Sprintf("Good morning! *%s* is now open. Everyone can send messages!", groupName)
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				logger.Error("autounmute: failed to send open notice", "group", groupJIDStr, "err", sendErr)
			}
		}
	}
}

func handleEventsCmd(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "events_status:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendEventsMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Group event notifications are now ON! You'll receive alerts when group settings, names, or members change.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("Group event notifications are now OFF.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("Group event notifications are now OFF.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Group event notifications are now ON!")

	case "customize", "custom", "help":
		return sendEventsCustomizeGuide(ctx)

	default:
		return ctx.Replyf("Usage: `%sevents [on|off|toggle|help]`", ctx.GetPrefix())
	}
}

func sendEventsMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper) error {
	chatKey := ctx.Chat.String()
	status, _ := s.GetSetting(ctx.Ctx, "events_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := dispatch.NewText().
		Header("GROUP EVENTS NOTIFICATIONS").
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Get real-time updates when group settings change or members join and leave.").
		Blank().
		Line("Choose an option below to toggle notifications or view details:").
		Trimmed()

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendEventsCustomizeGuide(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("GROUP EVENTS NOTIFICATIONS GUIDE").
		Blank().
		Section("Notifications You'll Receive:").
		Bullet("Group name changes").
		Bullet("Group description updates").
		Bullet("Group settings lock/unlock (who can edit info)").
		Bullet("Group chat open/close (who can send messages)").
		Bullet("Admin promotions and demotions").
		Bullet("New members joining or leaving").
		Blank().
		Section("Commands:").
		Bulletf("Turn On : `%sevents on`", p).
		Bulletf("Turn Off : `%sevents off`", p).
		Bulletf("Toggle : `%sevents toggle`", p).
		Reply()
}

func handleSetGroupPP(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used inside a group chat.")
	}

	groupInfo, errGroup := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if errGroup != nil || groupInfo == nil {
		return ctx.Replyf("Couldn't retrieve group details right now: %v", errGroup)
	}

	if !ctx.IsSenderAdmin(groupInfo) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can change the group profile photo.")
	}
	if groupInfo.IsLocked && !ctx.AmIAdmin(groupInfo) {
		return ctx.Reply("Group settings are restricted to admins only. Please make me an admin so I can update the group picture!")
	}

	downloadable, _, mime := ExtractMediaFromEvent(ctx.Evt)
	if downloadable == nil {
		return ctx.Replyf("Please reply to an image or attach a photo with `%sgpp` to set it as the new group picture.", ctx.GetPrefix())
	}

	rawBytes, err := ctx.Client.Download(ctx.Ctx, downloadable)

	if err != nil || len(rawBytes) == 0 {
		return ctx.Replyf("Couldn't download the photo: %v", err)
	}

	jpegData, errConv := media.EnsureJPEG(ctx.Ctx, rawBytes)
	if errConv != nil || len(jpegData) == 0 {
		return ctx.Replyf("Couldn't process the photo format: %v", errConv)
	}

	logger.Debug("handleSetGroupPP: Setting group profile picture", "group", ctx.Chat.String(), "mime", mime, "rawBytes", len(rawBytes), "jpegBytes", len(jpegData))
	picID, errSet := ctx.Client.SetGroupPhoto(ctx.Ctx, ctx.Chat, jpegData)
	if errSet != nil {
		logger.Error("handleSetGroupPP failed", "err", errSet)
		return ctx.Replyf("Failed to update group photo: %v", errSet)
	}

	return ctx.Replyf("Group profile picture updated successfully! (ID: %s)", picID)
}

func handleWarn(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	ci := ctx.GetContextInfo()
	hasMention := ci != nil && len(ci.GetMentionedJID()) > 0

	if len(args) == 0 && ctx.GetQuotedMessage() == nil && !hasMention {
		return sendWarnMenu(ctx, s)
	}

	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "customize" || sub == "custom" || sub == "help" {
			return sendWarnCustomizeGuide(ctx)
		}
		if sub == "limit" || sub == "max" || sub == "set" {
			if len(args) > 1 {
				ctx.RawArgs = strings.Join(args[1:], "")
			} else {
				ctx.RawArgs = ""
			}
			return handleSetWarn(ctx)
		}
	}

	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to warn?\n\n• Reply to their message with `%swarn`\n• Or type: `%swarn @user [reason]`\n• Or type: `%swarn <phone-number> [reason]`", p, p, p)
	}

	if isJIDOwnerOrSudo(ctx, targetJID) {
		return ctx.Reply("You cannot issue warnings to bot owners or sudo users.")
	}

	isGroup := ctx.Chat.Server == "g.us"
	var groupInfo *types.GroupInfo
	if isGroup {
		var err error
		groupInfo, err = ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err != nil {
			return ctx.Replyf("Couldn't retrieve group details right now: %v", err)
		}

		if isParticipantAdmin(groupInfo, targetJID) && !ctx.IsSudo() {
			return ctx.Reply("Group admins cannot be warned by the bot.")
		}
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := dispatch.Sprintf("warn_count:%s:%s", chatKey, userKey)
	limitKey := dispatch.Sprintf("warn_limit:%s", chatKey)

	rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
	currentWarns, _ := strconv.Atoi(rawCount)
	currentWarns++
	_ = s.PutSetting(ctx.Ctx, warnKey, strconv.Itoa(currentWarns))

	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	reason := ""
	if len(args) > 1 && !strings.HasPrefix(args[0], "@") {
		reason = strings.Join(args[1:], "")
	} else if len(args) > 1 && strings.HasPrefix(args[0], "@") {
		reason = strings.Join(args[1:], "")
	}

	if currentWarns < maxLimit {
		msg := dispatch.Sprintf("Warning (%d/%d) issued to @%s.", currentWarns, maxLimit, username)
		if reason != "" {
			msg += "\nReason:" + reason
		}
		return ctx.ReplyWithMentions(msg, []types.JID{resolvedJID})
	}

	if isGroup {
		targetIsAdmin := isParticipantAdmin(groupInfo, targetJID)
		botIsOwner := isBotGroupOwner(ctx, groupInfo)

		if targetIsAdmin && !botIsOwner {
			return ctx.ReplyWithMentions(dispatch.Sprintf("@%s has reached the maximum warning limit (%d/%d), but cannot be removed because they are a group admin.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		if !isBotAdmin(ctx, groupInfo) {
			return ctx.ReplyWithMentions(dispatch.Sprintf("@%s has reached the maximum warning limit (%d/%d), but I need admin permissions to remove them from the group.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		_, _ = ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock)
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{targetJID}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			return ctx.Replyf("Failed to remove @%s from the group: %v", username, err)
		}

		_ = s.PutSetting(ctx.Ctx, warnKey, "0")
		return ctx.ReplyWithMentions(dispatch.Sprintf("@%s reached the maximum warning limit (%d/%d) and has been removed from the group.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	_ = ctx.ReplyWithMentions(dispatch.Sprintf("@%s reached the maximum warning limit (%d/%d) and has been blocked.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	_ = s.PutSetting(ctx.Ctx, warnKey, "0")
	if _, err := ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock); err != nil {
		logger.Error("handleWarn: failed to block user in DM", "target", targetJID.String(), "err", err)
	}
	return nil
}

func handleUnwarn(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		p := ctx.GetPrefix()
		return ctx.Replyf("Who would you like to remove a warning from?\n\n• Reply to their message with `%sunwarn`\n• Or tag them: `%sunwarn @user`", p, p)
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := dispatch.Sprintf("warn_count:%s:%s", chatKey, userKey)

	rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
	currentWarns, _ := strconv.Atoi(rawCount)
	if currentWarns <= 0 {
		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(dispatch.Sprintf("@%s has a clean record with 0 active warnings.", username), []types.JID{resolvedJID})
	}

	currentWarns--
	_ = s.PutSetting(ctx.Ctx, warnKey, strconv.Itoa(currentWarns))
	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(dispatch.Sprintf("Removed 1 warning from @%s. Remaining warnings: %d.", username, currentWarns), []types.JID{resolvedJID})
}

func handleWarns(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	chatKey := ctx.Chat.String()
	limitKey := dispatch.Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	if !targetJID.IsEmpty() {
		userKey := targetJID.ToNonAD().User
		warnKey := dispatch.Sprintf("warn_count:%s:%s", chatKey, userKey)
		rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
		currentWarns, _ := strconv.Atoi(rawCount)

		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(dispatch.Sprintf("@%s currently has %d out of %d allowed warnings.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	p := ctx.GetPrefix()
	return ctx.Replyf("The warning limit for this chat is *%d warnings* before automatic removal.\n\nTo check a specific member's warnings, type: `%swarns @user`", maxLimit, p)
}

func handleSetWarn(ctx *dispatch.Context) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is currently unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Please specify the maximum warnings allowed.\n\nExample: `%ssetwarn 3`", p)
	}

	num, err := strconv.Atoi(args[0])
	if err != nil || num < 1 || num > 20 {
		return ctx.Reply("Please specify a valid warning limit between 1 and 20. Example: `setwarn 3`")
	}

	chatKey := ctx.Chat.String()
	limitKey := dispatch.Sprintf("warn_limit:%s", chatKey)
	if err := s.PutSetting(ctx.Ctx, limitKey, strconv.Itoa(num)); err != nil {
		return ctx.Reply("Failed to update warning threshold.")
	}

	return ctx.Replyf("Warning limit for this chat set to *%d warnings*. Members who exceed this will be removed.", num)
}

func sendWarnMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper) error {
	chatKey := ctx.Chat.String()
	limitKey := dispatch.Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	bodyText := dispatch.NewText().
		Header("WARN CONFIGURATION").
		Fieldf("Max Warnings Allowed", "%d Warnings", maxLimit).
		Blank().
		Line("Choose an option below to set the warning threshold or view the guide:").
		Trimmed()

	options := []string{
		"Set Limit (3)",
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendWarnCustomizeGuide(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("WARN SYSTEM GUIDE").
		Blank().
		Section("Available Commands:").
		Bulletf("Issue Warning : `%swarn @user [reason]`", p).
		Bulletf("Remove Warning : `%sunwarn @user`", p).
		Bulletf("Check Warnings : `%swarns [@user]`", p).
		Bulletf("Set Max Threshold : `%ssetwarn <number>`", p).
		Blank().
		Section("How It Works:").
		Numbered(1, "When a member reaches the maximum warnings in a group, they are removed (requires bot admin).").
		Numbered(2, "In private chat, reaching the limit blocks messaging.").
		Numbered(3, "Bot Owners and sudoers are immune to warnings.").
		Numbered(4, "Group Admins cannot be removed unless the bot is the group creator.").
		Blank().
		Section("Examples:").
		Numberedf(1, "`%swarn @user Posting unauthorized links`", p).
		Numberedf(2, "`%sunwarn @user`", p).
		Numberedf(3, "`%ssetwarn 3`", p).
		Reply()
}

func extractWarnTarget(ctx *dispatch.Context, args []string) types.JID {
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		return NormalizeUserJID(ctx.Ctx, ctx.Client, quotedSender)
	}
	if ci := ctx.GetContextInfo(); ci != nil && len(ci.GetMentionedJID()) > 0 {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := parseUserJID(m); err == nil && !parsed.IsEmpty() {
				return NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
			}
		}
	}
	for _, arg := range args {
		sub := strings.ToLower(arg)
		if sub == "customize" || sub == "custom" || sub == "help" || sub == "limit" || sub == "max" || sub == "set" {
			continue
		}
		if _, err := strconv.Atoi(arg); err == nil {
			continue
		}
		if parsed, err := parseUserJID(arg); err == nil && !parsed.IsEmpty() {
			return NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
		}
	}
	return types.EmptyJID
}

func isJIDOwnerOrSudo(ctx *dispatch.Context, target types.JID) bool {
	return whatsrook.IsSudoRaw(ctx.Ctx, ctx.Client, target)
}

func isParticipantAdmin(info *types.GroupInfo, target types.JID) bool {
	if info == nil || target.IsEmpty() {
		return false
	}
	return whatsrook.IsAdminRaw(context.Background(), nil, info, target)
}

func isBotAdmin(ctx *dispatch.Context, info *types.GroupInfo) bool {
	if ctx == nil || info == nil {
		return false
	}
	return ctx.AmIAdmin(info)
}

func isBotGroupOwner(ctx *dispatch.Context, info *types.GroupInfo) bool {
	if ctx == nil || info == nil || ctx.Client == nil || ctx.Client.Store == nil {
		return false
	}
	if info.OwnerJID.IsEmpty() && info.OwnerPN.IsEmpty() {
		return false
	}
	if ctx.Client.Store.ID != nil && !ctx.Client.Store.ID.IsEmpty() {
		if !info.OwnerJID.IsEmpty() && ctx.IsSameUser(info.OwnerJID, *ctx.Client.Store.ID) {
			return true
		}
		if !info.OwnerPN.IsEmpty() && ctx.IsSameUser(info.OwnerPN, *ctx.Client.Store.ID) {
			return true
		}
	}
	if !ctx.Client.Store.LID.IsEmpty() {
		if !info.OwnerJID.IsEmpty() && ctx.IsSameUser(info.OwnerJID, ctx.Client.Store.LID) {
			return true
		}
		if !info.OwnerPN.IsEmpty() && ctx.IsSameUser(info.OwnerPN, ctx.Client.Store.LID) {
			return true
		}
	}
	return false
}

func handleWelcome(ctx *dispatch.Context) error {
	return handleGroupGreetingConfig(ctx, "welcome")
}

func handleGoodbye(ctx *dispatch.Context) error {
	return handleGroupGreetingConfig(ctx, "goodbye")
}

func handleGroupGreetingConfig(ctx *dispatch.Context, kind string) error {
	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database service is currently unavailable. Please try again later.")
	}

	chatKey := ctx.Chat.String()
	key := func(suffix string) string {
		return kind + "_" + suffix + ":" + chatKey
	}
	statusKey := key("status")
	tagKey := key("tag")
	descKey := key("desc")
	msgKey := key("msg")
	mediaKey := key("media")

	label := titleCase(kind)

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendGreetingMenu(ctx, s, kind)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate":
		return applyToggle(ctx, s, statusKey, "on", label+"messages")

	case "off", "disable", "deactivate":
		return applyToggle(ctx, s, statusKey, "off", label+"messages")

	case "toggle":
		return applyToggle(ctx, s, statusKey, "toggle", label+"messages")

	case "customize", "custom", "help":
		return sendGreetingCustomizeGuide(ctx, kind)

	case "tag":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, tagKey)
			statusText := "disabled"
			if curr == "on" {
				statusText = "enabled"
			}
			return ctx.Replyf("*%s Mentions:* Currently *%s*.\n\nUse `%s%s tag [on|off]` to toggle.", label, statusText, ctx.GetPrefix(), kind)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "enable" && mode != "activate" && mode != "off" && mode != "false" && mode != "disable" && mode != "deactivate" && mode != "toggle" {
			return ctx.Replyf("*Usage:* `%s%s tag <on|off|toggle>`\n\nChoose whether the bot should @mention the member in the %s message.", ctx.GetPrefix(), kind, strings.ToLower(kind))
		}
		return applyToggle(ctx, s, tagKey, mode, label+"member tagging")

	case "desc":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, descKey)
			statusText := "disabled"
			if curr == "on" {
				statusText = "enabled"
			}
			return ctx.Replyf("*%s Group Description:* Currently *%s*.\n\nUse `%s%s desc [on|off]` to toggle.", label, statusText, ctx.GetPrefix(), kind)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "enable" && mode != "activate" && mode != "off" && mode != "false" && mode != "disable" && mode != "deactivate" && mode != "toggle" {
			return ctx.Replyf("*Usage:* `%s%s desc <on|off|toggle>`\n\nChoose whether to include the group description in the %s message.", ctx.GetPrefix(), kind, strings.ToLower(kind))
		}
		return applyToggle(ctx, s, descKey, mode, label+"group description inclusion")

	case "msg", "message", "text":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, msgKey)
			if curr == "" {
				curr = "Default template"
			}
			return ctx.Replyf("*Current %s Message Template:*\n\n%s\n\nTo update it, use:\n`%s%s msg <your custom message>`\n\nType `%s%s customize` to view available template placeholders.", label, curr, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind)
		}
		text := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if err := s.PutSetting(ctx.Ctx, msgKey, text); err != nil {
			return ctx.Reply("Failed to save custom message template:" + err.Error())
		}
		return ctx.Replyf("*%s message template updated!*\n\nThe bot will now use this customized message.", label)

	case "media", "video":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, mediaKey)
			if curr == "" {
				curr = "None (text only)"
			}
			return ctx.Replyf("*Current %s Media URL:*\n%s\n\nTo set a greeting image or video, use:\n`%s%s media <direct-url>`\nOr remove it with `%s%s media clear`.", label, curr, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind)
		}
		url := strings.TrimSpace(args[1])
		if url == "none" || url == "clear" {
			if err := s.PutSetting(ctx.Ctx, mediaKey, ""); err != nil {
				return ctx.Reply("Failed to clear media:" + err.Error())
			}
			return ctx.Replyf("*%s media removed.* %s messages will now be sent as text only.", label, label)
		}
		if err := s.PutSetting(ctx.Ctx, mediaKey, url); err != nil {
			return ctx.Reply("Failed to save media URL:" + err.Error())
		}
		return ctx.Replyf("*%s media URL updated!* The bot will attach this media when greeting members.", label)

	default:
		return ctx.Replyf("*%s Greeting Options:*\n\n• `%s%s activate` - Enable greetings\n• `%s%s deactivate` - Disable greetings\n• `%s%s customize` - View placeholders & formatting guide\n• `%s%s msg <text>` - Set a custom message\n• `%s%s media <url>` - Attach an image/video\n• `%s%s tag <on|off>` - Toggle member @mention\n• `%s%s desc <on|off>` - Toggle group description", label, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind, ctx.GetPrefix(), kind)
	}
}

func applyToggle(ctx *dispatch.Context, s *dispatch.StoreWrapper, key, mode, label string) error {
	next := "on"
	switch mode {
	case "on", "true", "enable", "activate":
		next = "on"
	case "off", "false", "disable", "deactivate":
		next = "off"
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, key)
		next = "on"
		if curr == "on" {
			next = "off"
		}
	}

	if err := s.PutSetting(ctx.Ctx, key, next); err != nil {
		return ctx.Reply("Failed to update setting:" + err.Error())
	}

	verb := "enabled"
	if next == "off" {
		verb = "disabled"
	}
	return ctx.Replyf("*%s %s.*", label, verb)
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func sendGreetingMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper, kind string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, kind+"_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := dispatch.NewText().
		Header(dispatch.Sprintf("%s GREETING SETTINGS", strings.ToUpper(kind))).
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Choose an option below to activate or deactivate greetings, or choose Customize to personalize your greeting message.").
		Trimmed()

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendGreetingCustomizeGuide(ctx *dispatch.Context, kind string) error {
	p := ctx.GetPrefix()
	kUpper := strings.ToUpper(kind)

	tb := ctx.Text().
		Header(dispatch.Sprintf("%s CUSTOMIZATION GUIDE", kUpper)).
		Blank().
		Section("How to Configure:").
		Bulletf("Custom Message : `%s%s msg <your message text>`", p, kind).
		Bulletf("Member Tagging : `%s%s tag on | off`", p, kind).
		Bulletf("Group Description : `%s%s desc on | off`", p, kind).
		Bulletf("Greeting Media : `%s%s media <url | clear>`", p, kind).
		Blank().
		Section("Available Placeholders:").
		Bullet("`{user}` : Mentions the member (@username)").
		Bullet("`{user_id}` : Member's phone number / user ID").
		Bullet("`{user_jid}` : Member's full WhatsApp JID").
		Bullet("`{group}` : Group name").
		Bullet("`{group_jid}` : Group chat JID").
		Bullet("`{desc}` : Group description / topic").
		Bullet("`{members}` : Total participant count").
		Bullet("`{admins}` : Total admin count").
		Bullet("`{owner}` : Mentions group creator / owner").
		Bullet("`{created_at}` : Group creation date").
		Blank().
		Section("Examples:")

	if kind == "welcome" {
		tb.Numberedf(1, "`%swelcome msg Welcome {user} to {group}! We now have {members} members. Please be respectful and enjoy your stay!`", p).
			Numberedf(2, "`%swelcome tag on`", p).
			Numberedf(3, "`%swelcome media https://example.com/welcome.mp4`", p)
	} else {
		tb.Numberedf(1, "`%sgoodbye msg Farewell {user}! Thanks for spending time with us in {group}.`", p).
			Numberedf(2, "`%sgoodbye tag off`", p).
			Numberedf(3, "`%sgoodbye media https://example.com/goodbye.gif`", p)
	}

	return tb.Reply()
}

type PendingCaptcha struct {
	GroupJID    types.JID
	UserJID     types.JID
	ResolvedJID types.JID
	Username    string
	Code        string
	MsgID       types.MessageID
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Timer       *time.Timer
}

var (
	pendingCaptchaMu sync.RWMutex
	pendingCaptchas  = make(map[string]*PendingCaptcha)
)

func captchaKey(groupJID, userJID types.JID) string {
	return groupJID.ToNonAD().String() + ":" + userJID.ToNonAD().String()
}

// ClearPendingCaptchasForGroup cancels all timers and removes pending captchas for a specific group.
func ClearPendingCaptchasForGroup(groupJID types.JID) int {
	pendingCaptchaMu.Lock()
	defer pendingCaptchaMu.Unlock()

	chatStr := groupJID.ToNonAD().String()
	cleared := 0
	for k, p := range pendingCaptchas {
		if strings.HasPrefix(k, chatStr+":") {
			if p.Timer != nil {
				p.Timer.Stop()
			}
			delete(pendingCaptchas, k)
			cleared++
		}
	}
	if cleared > 0 {
		logger.Debug("ClearPendingCaptchasForGroup: cleared pending captchas", "group", chatStr, "count", cleared)
	}
	return cleared
}

// RegisterPendingCaptcha registers a new pending captcha verification for a participant.
func RegisterPendingCaptcha(groupJID, userJID, resolvedJID types.JID, username, code string, duration time.Duration, onTimeout func()) {
	pendingCaptchaMu.Lock()
	defer pendingCaptchaMu.Unlock()

	key := captchaKey(groupJID, userJID)
	if existing, ok := pendingCaptchas[key]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}

	timer := time.AfterFunc(duration, func() {
		pendingCaptchaMu.Lock()
		_, ok := pendingCaptchas[key]
		if ok {
			delete(pendingCaptchas, key)
		}
		pendingCaptchaMu.Unlock()

		if ok && onTimeout != nil {
			logger.Debug("RegisterPendingCaptcha: timeout triggered", "group", groupJID.String(), "user", userJID.String(), "code", code)
			onTimeout()
		}
	})

	now := time.Now()
	pendingCaptchas[key] = &PendingCaptcha{
		GroupJID:    groupJID,
		UserJID:     userJID,
		ResolvedJID: resolvedJID,
		Username:    username,
		Code:        code,
		CreatedAt:   now,
		ExpiresAt:   now.Add(duration),
		Timer:       timer,
	}
	logger.Debug("RegisterPendingCaptcha: registered new pending captcha", "group", groupJID.String(), "user", userJID.String(), "username", username, "code", code, "duration", duration)
}

// SetPendingCaptchaMsgID stores the verification message ID for revocation if cancelled.
func SetPendingCaptchaMsgID(groupJID, userJID types.JID, msgID types.MessageID) {
	pendingCaptchaMu.Lock()
	defer pendingCaptchaMu.Unlock()

	key := captchaKey(groupJID, userJID)
	if p, ok := pendingCaptchas[key]; ok {
		p.MsgID = msgID
		logger.Debug("SetPendingCaptchaMsgID: updated msgID for pending captcha", "group", groupJID.String(), "user", userJID.String(), "msgID", msgID)
		return
	}

	chatStr := groupJID.ToNonAD().String()
	userNonAD := userJID.ToNonAD()
	for _, p := range pendingCaptchas {
		if p.GroupJID.ToNonAD().String() == chatStr {
			if p.UserJID.ToNonAD() == userNonAD || p.ResolvedJID.ToNonAD() == userNonAD || p.UserJID.User == userNonAD.User || p.ResolvedJID.User == userNonAD.User {
				p.MsgID = msgID
				logger.Debug("SetPendingCaptchaMsgID: updated msgID via fallback matching", "group", groupJID.String(), "user", userJID.String(), "msgID", msgID)
				return
			}
		}
	}
}

// RemovePendingCaptcha cancels and removes any pending captcha for a group participant.
func RemovePendingCaptcha(groupJID, userJID types.JID) (*PendingCaptcha, bool) {
	pendingCaptchaMu.Lock()
	defer pendingCaptchaMu.Unlock()

	key := captchaKey(groupJID, userJID)
	p, ok := pendingCaptchas[key]
	if ok {
		if p.Timer != nil {
			p.Timer.Stop()
		}
		delete(pendingCaptchas, key)
		logger.Debug("RemovePendingCaptcha: removed pending captcha", "group", groupJID.String(), "user", userJID.String())
		return p, true
	}

	chatStr := groupJID.ToNonAD().String()
	userNonAD := userJID.ToNonAD()
	for k, entry := range pendingCaptchas {
		if strings.HasPrefix(k, chatStr+":") {
			if entry.UserJID.ToNonAD() == userNonAD || entry.ResolvedJID.ToNonAD() == userNonAD || entry.UserJID.User == userNonAD.User || entry.ResolvedJID.User == userNonAD.User {
				if entry.Timer != nil {
					entry.Timer.Stop()
				}
				delete(pendingCaptchas, k)
				logger.Debug("RemovePendingCaptcha: removed pending captcha via fallback matching", "group", groupJID.String(), "user", userJID.String(), "key", k)
				return entry, true
			}
		}
	}
	return nil, false
}

func parseCaptchaTime(raw string) (int, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(raw); err == nil && sec >= 10 && sec <= 600 {
		return sec, true
	}
	fields := strings.Fields(raw)
	if len(fields) == 2 {
		num, err := strconv.Atoi(fields[0])
		if err == nil {
			unit := strings.TrimRight(fields[1], "s")
			switch unit {
			case "min", "minute", "m":
				sec := num * 60
				if sec >= 10 && sec <= 600 {
					return sec, true
				}
			case "sec", "second", "s":
				if num >= 10 && num <= 600 {
					return num, true
				}
			}
		}
	} else if len(fields) == 1 {
		val := fields[0]
		if strings.HasSuffix(val, "m") || strings.HasSuffix(val, "min") {
			numStr := strings.TrimRight(strings.TrimRight(val, "in"), "m")
			if num, err := strconv.Atoi(numStr); err == nil {
				sec := num * 60
				if sec >= 10 && sec <= 600 {
					return sec, true
				}
			}
		} else if before, ok := strings.CutSuffix(val, "s"); ok {
			numStr := before
			if num, err := strconv.Atoi(numStr); err == nil {
				if num >= 10 && num <= 600 {
					return num, true
				}
			}
		}
	}
	return 0, false
}

func formatCaptchaTimeout(sec int) string {
	if sec <= 0 {
		return "2 mins"
	}
	if sec%60 == 0 {
		mins := sec / 60
		if mins == 1 {
			return "1 min"
		}
		return dispatch.Sprintf("%d mins", mins)
	}
	return dispatch.Sprintf("%d seconds", sec)
}

func findPendingCaptcha(client *whatsmeow.Client, chat, sender types.JID) (*PendingCaptcha, bool) {
	pendingCaptchaMu.RLock()
	defer pendingCaptchaMu.RUnlock()

	chatStr := chat.ToNonAD().String()
	senderNonAD := sender.ToNonAD()

	for key, p := range pendingCaptchas {
		if !strings.HasPrefix(key, chatStr+":") {
			continue
		}
		if p.UserJID.ToNonAD() == senderNonAD || p.ResolvedJID.ToNonAD() == senderNonAD || p.UserJID.User == senderNonAD.User || p.ResolvedJID.User == senderNonAD.User {
			return p, true
		}
		if client != nil && whatsrook.IsSameUserRaw(context.Background(), client, p.UserJID, sender) {
			return p, true
		}
	}
	return nil, false
}

// HandlePendingCaptchaReply checks if an incoming message is a captcha verification answer.
func HandlePendingCaptchaReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil || evt.Info.Chat.Server != "g.us" {
		return false
	}

	sender := evt.Info.Sender
	chat := evt.Info.Chat

	pending, ok := findPendingCaptcha(client, chat, sender)
	if !ok {
		// Fallback: match via the quoted message ID so that native WhatsApp
		// "reply to video" messages are correctly attributed to the pending user.
		pending, ok = findPendingCaptchaByQuotedMsgID(chat, evt)
	}
	if !ok {
		return false
	}

	logger.Debug("HandlePendingCaptchaReply: found pending captcha for participant",
		"group", chat.String(),
		"sender", sender.String(),
		"code", pending.Code,
		"createdAt", pending.CreatedAt,
		"expiresAt", pending.ExpiresAt,
	)

	// Ignore messages sent before the pending captcha was created
	if !evt.Info.Timestamp.IsZero() && evt.Info.Timestamp.Before(pending.CreatedAt.Add(-2*time.Second)) {
		logger.Debug("HandlePendingCaptchaReply: ignoring message sent before captcha creation",
			"group", chat.String(),
			"sender", sender.String(),
			"msgTimestamp", evt.Info.Timestamp,
			"captchaCreatedAt", pending.CreatedAt,
		)
		return false
	}

	text := strings.TrimSpace(whatsrook.ExtractMessageText(evt))
	if text == "" {
		return false
	}

	cleanText := strings.TrimSpace(strings.Trim(text, "*_`~#"))

	logger.Debug("HandlePendingCaptchaReply: verifying message text against captcha code",
		"group", chat.String(),
		"sender", sender.String(),
		"input", cleanText,
		"expected", pending.Code,
	)

	// Check if user submitted the correct 4-digit code
	if cleanText == pending.Code {
		logger.Debug("HandlePendingCaptchaReply: correct code submitted, verification successful", "group", chat.String(), "sender", sender.String())
		if pending.MsgID != "" && client != nil {
			_, _ = client.SendMessage(ctx, chat, client.BuildRevoke(chat, types.EmptyJID, pending.MsgID))
		}
		RemovePendingCaptcha(chat, pending.UserJID)
		RemovePendingCaptcha(chat, sender)
		// Use the registered captcha user identity for the welcome message, not the
		// raw sender JID, since they may differ (e.g. LID vs phone JID).
		username := pending.Username
		resolvedJID := pending.ResolvedJID
		if username == "" {
			resolvedJID, username = whatsrook.ResolveMentionRaw(ctx, client, pending.UserJID)
		}

		pctx := &whatsrook.PluginContext{Ctx: ctx, Client: client, Chat: chat, Sender: sender}
		tb := pctx.Text()
		tb.Header("Verification Confirmed")
		tb.Linef("Welcome to the group, @%s! You have successfully completed the verification check.", username)
		tb.Mentions(resolvedJID)
		_ = tb.Send()
		return true
	}

	// Check if user sent a numeric verification attempt that was wrong
	allDigits := true
	for _, r := range cleanText {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits && (len(cleanText) == 4 || len(cleanText) == len(pending.Code)) {
		logger.Debug("HandlePendingCaptchaReply: incorrect numeric attempt", "group", chat.String(), "sender", sender.String(), "attempt", cleanText, "expected", pending.Code)
		resolvedJID, username := whatsrook.ResolveMentionRaw(ctx, client, sender)

		pctx := &whatsrook.PluginContext{Ctx: ctx, Client: client, Chat: chat, Sender: sender}
		tb := pctx.Text()
		tb.Header("Incorrect Code")
		tb.Linef("That code is incorrect, @%s. Please check the 4-digit code in the video clip and try again.", username)
		tb.Mentions(resolvedJID)
		_ = tb.Send()
		return true
	}

	return false
}

// findPendingCaptchaByQuotedMsgID searches pending captchas for the group by matching the
// quoted message ID in the incoming event's ContextInfo against any registered pending.MsgID.
// This handles the native WhatsApp "reply to message" flow where the sender JID lookup may
// fail due to LID/phone JID mismatches, but the quoted stanza ID uniquely identifies the
// captcha challenge message.
func findPendingCaptchaByQuotedMsgID(chat types.JID, evt *events.Message) (*PendingCaptcha, bool) {
	if evt == nil || evt.Message == nil {
		return nil, false
	}
	ctx := whatsrook.GetContextInfoFromProto(evt.Message)
	if ctx == nil || ctx.GetStanzaID() == "" {
		return nil, false
	}
	quotedID := ctx.GetStanzaID()

	chatStr := chat.ToNonAD().String()

	pendingCaptchaMu.RLock()
	defer pendingCaptchaMu.RUnlock()

	for key, p := range pendingCaptchas {
		if !strings.HasPrefix(key, chatStr+":") {
			continue
		}
		if p.MsgID != "" && string(p.MsgID) == quotedID {
			return p, true
		}
	}
	return nil, false
}

func handleCaptcha(ctx *dispatch.Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("Captcha verification can only be configured in groups.")
	}

	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Could not retrieve group information: %v", err)
	}

	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can configure captcha verification.")
	}

	// This plugin should only work if this bot is an admin
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("*Admin Rights Required*\n\nThe bot must be promoted to an admin so it can remove members who fail the captcha challenge.")
	}

	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database service is currently unavailable. Please try again later.")
	}

	chatKey := ctx.Chat.ToNonAD().String()
	statusKey := "captcha_status:" + chatKey
	timeKey := "captcha_time:" + chatKey

	groupName := ctx.Chat.String()
	if info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	p := ctx.GetPrefix()
	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendCaptchaMenu(ctx, s, groupName)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate", "start":
		if err := s.PutSetting(ctx.Ctx, statusKey, "on"); err != nil {
			return ctx.Reply("Failed to activate captcha:" + err.Error())
		}
		tb := ctx.Text()
		tb.Header("CAPTCHA VERIFICATION ACTIVATED")
		tb.Field("Group", groupName)
		tb.Field("Status", "ACTIVE (ON)")
		tb.Blank()
		tb.Line("Captcha protection is now enabled! New participants must enter the 4-digit visual code within the time limit or they will be automatically removed.")

		options := []string{"Deactivate", "Set Timeout"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "off", "disable", "deactivate", "stop":
		if err := s.PutSetting(ctx.Ctx, statusKey, "off"); err != nil {
			return ctx.Reply("Failed to deactivate captcha:" + err.Error())
		}
		ClearPendingCaptchasForGroup(ctx.Chat)
		tb := ctx.Text()
		tb.Header("CAPTCHA VERIFICATION DEACTIVATED")
		tb.Field("Group", groupName)
		tb.Field("Status", "DISABLED (OFF)")
		tb.Blank()
		tb.Line("Captcha protection has been turned off. New members can now join without verification.")

		options := []string{"Activate"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		next := "on"
		if curr == "on" {
			next = "off"
		}
		if err := s.PutSetting(ctx.Ctx, statusKey, next); err != nil {
			return ctx.Reply("Failed to toggle captcha:" + err.Error())
		}
		if next == "off" {
			ClearPendingCaptchasForGroup(ctx.Chat)
		}
		verb := "activated"
		if next == "off" {
			verb = "deactivated"
		}
		tb := ctx.Text()
		tb.Header("CAPTCHA TOGGLED")
		tb.Field("Group", groupName)
		tb.Field("Status", strings.ToUpper(next))
		tb.Blank()
		tb.Linef("Captcha verification has been %s for this group.", verb)

		actionText := "Activate"
		if next == "on" {
			actionText = "Deactivate"
		}
		options := []string{actionText, "Set Timeout"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "time", "timeout", "duration":
		if len(args) < 2 {
			currTime, _ := s.GetSetting(ctx.Ctx, timeKey)
			secVal := 120
			if currTime != "" {
				if t, err := strconv.Atoi(currTime); err == nil && t > 0 {
					secVal = t
				}
			}
			tb := ctx.Text()
			tb.Header("CAPTCHA TIMEOUT SETTINGS")
			tb.Field("Group", groupName)
			tb.Field("Current Timeout", formatCaptchaTimeout(secVal))
			tb.Blank()
			tb.Linef("To change the verification time limit, reply with:\n`%scaptcha time <seconds|minutes>`", p)
			tb.Linef("Example: `%scaptcha time 120` or `%scaptcha time 2 mins`", p, p)
			tb.Blank()
			tb.Line("Or pick a quick duration below:")

			options := []string{"1 Min", "2 Mins", "3 Mins"}
			return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})
		}
		timeArg := strings.Join(args[1:], " ")
		sec, okTime := parseCaptchaTime(timeArg)
		if !okTime {
			return ctx.Reply("Invalid duration. Please provide a time between 10 and 600 seconds (e.g. `60`, `120`, or `2 mins`).")
		}
		if err := s.PutSetting(ctx.Ctx, timeKey, strconv.Itoa(sec)); err != nil {
			return ctx.Reply("Failed to update timeout: " + err.Error())
		}
		tb := ctx.Text()
		tb.Header("CAPTCHA TIMEOUT SET")
		tb.Field("Group", groupName)
		tb.Field("New Timeout", formatCaptchaTimeout(sec))
		tb.Blank()
		tb.Linef("Newly joined members will have %s to verify before being removed.", formatCaptchaTimeout(sec))

		options := []string{"Activate", "Deactivate"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "help", "guide", "info":
		return sendCaptchaGuide(ctx)

	default:
		// Check if user passed duration directly (e.g. `captcha 120` or `captcha 2 mins`)
		if sec, okTime := parseCaptchaTime(ctx.RawArgs); okTime {
			if err := s.PutSetting(ctx.Ctx, timeKey, strconv.Itoa(sec)); err != nil {
				return ctx.Reply("Failed to update timeout: " + err.Error())
			}
			tb := ctx.Text()
			tb.Header("CAPTCHA TIMEOUT SET")
			tb.Field("Group", groupName)
			tb.Field("New Timeout", formatCaptchaTimeout(sec))
			tb.Blank()
			tb.Linef("Newly joined members will have %s to verify before being removed.", formatCaptchaTimeout(sec))

			options := []string{"Activate", "Deactivate"}
			return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})
		}
		return ctx.Replyf("*Usage:* `%scaptcha <activate|deactivate|toggle|time <duration>|help>`\n\nType `%scaptcha help` for a complete setup guide.", p, p)
	}
}

func sendCaptchaMenu(ctx *dispatch.Context, s *dispatch.StoreWrapper, groupName string) error {
	chatKey := ctx.Chat.ToNonAD().String()
	status, _ := s.GetSetting(ctx.Ctx, "captcha_status:"+chatKey)
	if status == "" {
		status, _ = s.GetSetting(ctx.Ctx, "captcha_status:"+ctx.Chat.String())
	}
	if status == "" {
		status = "off"
	}
	timeoutStr, _ := s.GetSetting(ctx.Ctx, "captcha_time:"+chatKey)
	if timeoutStr == "" {
		timeoutStr, _ = s.GetSetting(ctx.Ctx, "captcha_time:"+ctx.Chat.String())
	}
	secVal := 120
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			secVal = t
		}
	}

	tb := ctx.Text()
	tb.Header("CAPTCHA SECURITY CONFIGURATION")
	tb.Field("Group", groupName)
	tb.Field("Status", strings.ToUpper(status))
	tb.Field("Timeout Window", formatCaptchaTimeout(secVal))
	tb.Blank()
	tb.Line("Protect your group from raid bots and spam accounts. When active, new arrivals must solve a quick 4-digit video challenge within the timeout window or be automatically removed.")
	tb.Blank()
	tb.Line("Note: The bot must be an admin to remove unverified users.")

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}
	options := []string{
		actionText,
		"Set Timeout",
		"Help / Guide",
	}

	return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})
}

func sendCaptchaGuide(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	tb := ctx.Text()
	tb.Header("CAPTCHA VERIFICATION GUIDE")
	tb.Line("Protect your community from raid bots and spam accounts with automated newcomer verification.")
	tb.Blank()
	tb.Section("How It Works:")
	tb.Numbered(1, "When someone joins the group, the bot sends an animated clip with a unique 4-digit code.")
	tb.Numbered(2, "The bot mentions the new member and asks them to reply with the code.")
	tb.Numbered(3, "The newcomer types the 4-digit code into the chat before the timer expires.")
	tb.Numbered(4, "If the code is correct, their membership is confirmed and they can chat normally.")
	tb.Numbered(5, "If the time runs out without verification, the bot automatically removes them.")
	tb.Blank()
	tb.Section("Available Commands:")
	tb.Bulletf("`%scaptcha activate`- Turn on captcha protection", p)
	tb.Bulletf("`%scaptcha deactivate`- Turn off captcha protection", p)
	tb.Bulletf("`%scaptcha toggle`- Switch captcha on or off", p)
	tb.Bulletf("`%scaptcha time <duration>`- Set response window (e.g.`60`,`120`,`2 mins`)", p)
	tb.Bulletf("`%scaptcha help` - Show this guide", p)
	tb.Blank()
	tb.Section("Admin Requirement:")
	tb.Bullet("The bot must be promoted to a group admin in order to remove unverified accounts.")

	return ctx.Reply(tb.Trimmed())
}
