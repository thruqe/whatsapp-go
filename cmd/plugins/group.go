package plugins

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

	utils "whatsrook"
	"whatsrook/cmd/store"
	cliutils "whatsrook/cmd/utils"
	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	Register(&Command{
		Name:        "tagall",
		Alias:       "tag, everyone, all, tagmembers",
		Description: "Mention everyone in the group. Usage: tagall [message] or tag [message]",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleTagAll,
	})
	Register(&Command{
		Name:        "kick",
		Description: "Remove a member from the group (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleKick,
	})
	Register(&Command{
		Name:        "add",
		Description: "Add a member to the group (phone number/JID)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAdd,
	})
	Register(&Command{
		Name:        "promote",
		Description: "Promote a member to admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePromote,
	})
	Register(&Command{
		Name:        "demote",
		Description: "Demote a member from admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleDemote,
	})
	Register(&Command{
		Name:        "group",
		Description: "Manage group settings (open, close, lock, unlock)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGroup,
	})
	Register(&Command{
		Name:        "antilink",
		Description: "Enable or disable anti-link protection (on/off)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiLink,
	})
	Register(&Command{
		Name:        "antiword",
		Description: "Manage banned words (add [word], del [word], list)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiWord,
	})
	Register(&Command{
		Name:        "gstats",
		Description: "Provide statistics on the most active group participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGStats,
	})
	Register(&Command{
		Name:        "poll",
		Alias:       "lockpoll",
		Description: "Create a poll with single or multiple choice options",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePoll,
	})
	Register(&Command{
		Name:        "invite",
		Description: "Get the group invite link",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleInvite,
	})
	Register(&Command{
		Name:        "listonline",
		Alias:       "online",
		Description: "List online participants in the current group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListOnline,
	})
	Register(&Command{
		Name:        "kickall",
		Description: "Remove all participants from the group except the bot and sudoers",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleKickAll,
	})
	Register(&Command{
		Name:        "community",
		Alias:       "groups",
		Description: "List all joined groups, communities, and subgroups with invite links",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleCommunity,
	})
	Register(&Command{
		Name:        "channels",
		Alias:       "newsletters",
		Description: "List all subscribed WhatsApp channels and newsletters with subscriber counts",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleChannels,
	})
	Register(&Command{
		Name:        "leave",
		Alias:       "left",
		Description: "Leave the current group with interactive confirmation",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleLeave,
	})
	Register(&Command{
		Name:        "join",
		Alias:       "joingroup",
		Description: "Join a group using a group URL or group invite message",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleJoin,
	})
	Register(&Command{
		Name:        "antimsg",
		Alias:       "antimessage",
		Description: "Automatically delete messages sent by specified group participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiMsg,
	})
	Register(&Command{
		Name:        "antispam",
		Alias:       "aspam",
		Description: "Configure group anti-spam rate limits, warning thresholds, and automated actions",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiSpam,
	})
	Register(&Command{
		Name:        "automute",
		Alias:       "autoclose",
		Description: "Configure automatic daily group mute (close) time in HH:MM (24h format)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoMute,
	})
	Register(&Command{
		Name:        "autounmute",
		Alias:       "autoopen",
		Description: "Configure automatic daily group unmute (open) time in HH:MM (24h format)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoUnmute,
	})
	Register(&Command{
		Name:        "listmute",
		Alias:       "mutestatus",
		Description: "List active automute & autounmute schedules for this group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListMute,
	})
	Register(&Command{
		Name:        "events",
		Alias:       "groupevents",
		Description: "Toggle real-time notifications for group subject, description, settings, and participant changes",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleEventsCmd,
	})
	Register(&Command{
		Name:        "gpp",
		Alias:       "setgpp",
		Description: "Update the group's profile picture (replying to an image or image upload in a group)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleSetGroupPP,
	})
	Register(&Command{
		Name:        "warn",
		Alias:       "warning",
		Description: "Issue a warning to a participant. Blocks and kicks when max warning threshold is reached.",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleWarn,
	})
	Register(&Command{
		Name:        "unwarn",
		Alias:       "delwarn",
		Description: "Remove warnings from a participant",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleUnwarn,
	})
	Register(&Command{
		Name:        "warns",
		Alias:       "getwarn",
		Description: "Check current warning count for a participant or group",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleWarns,
	})
	Register(&Command{
		Name:        "setwarn",
		Alias:       "warnlimit",
		Description: "Set max warning threshold before taking automated block/kick action",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleSetWarn,
	})
	Register(&Command{
		Name:        "welcome",
		Alias:       "welc",
		Description: "Configure group welcome messages, tagging, description headers, custom templates, and media",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleWelcome,
	})
	Register(&Command{
		Name:        "goodbye",
		Alias:       "bye",
		Description: "Configure group goodbye messages, tagging, description headers, custom templates, and media",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleGoodbye,
	})
	Register(&Command{
		Name:        "captcha",
		Alias:       "verify",
		Description: "Configure captcha verification for newly joined group participants (on, off, toggle, time)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleCaptcha,
	})
}

func handleTagAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can tag everyone.")
	}

	msg := "@all"
	if ctx.RawArgs != "" {
		msg += "\nMessage: *" + ctx.RawArgs + "*"
	}

	return ctx.ReplyWithGroupMention(msg)
}

func handleKick(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf(" Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can kick members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to kick members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %skick @user\n- %skick 1234567890\n- Reply to a user's message with %skick", p, p, p)
	}

	var kicked []string
	var kickedJIDs []types.JID
	for _, target := range targets {
		resolvedJID, username := ctx.ResolveMention(target)
		if utils.IsSudoRaw(ctx.Ctx, ctx.Client, target) {
			_ = ctx.ReplyWithMentions(Sprintf("⚠️ Cannot kick bot owner or sudo user @%s.", username), []types.JID{resolvedJID})
			continue
		}
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			_ = ctx.ReplyWithMentions(Sprintf("Failed to kick @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			kicked = append(kicked, "@"+username)
			kickedJIDs = append(kickedJIDs, resolvedJID)
		}
	}

	if len(kicked) > 0 {
		return ctx.ReplyWithMentions(Sprintf("Kicked: %s", strings.Join(kicked, ", ")), kickedJIDs)
	}
	return nil
}

func handleAdd(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf(" Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can add members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to add members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sadd 1234567890\n- %sadd 1234567890 9876543210", p, p)
	}

	var added []string
	var addedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeAdd)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(Sprintf("Failed to add @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			added = append(added, "@"+username)
			addedJIDs = append(addedJIDs, resolvedJID)
		}
	}

	if len(added) > 0 {
		return ctx.ReplyWithMentions(Sprintf("Added: %s", strings.Join(added, ", ")), addedJIDs)
	}
	return nil
}

func handlePromote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf(" Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can promote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to promote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %spromote @user\n- %spromote 1234567890\n- Reply to a user's message with %spromote", p, p, p)
	}

	var promoted []string
	var promotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(Sprintf("Failed to promote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			promoted = append(promoted, "@"+username)
			promotedJIDs = append(promotedJIDs, resolvedJID)
		}
	}

	if len(promoted) > 0 {
		return ctx.ReplyWithMentions(Sprintf("Promoted: %s", strings.Join(promoted, ", ")), promotedJIDs)
	}
	return nil
}

func handleDemote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf(" Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can demote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to demote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sdemote @user\n- %sdemote 1234567890\n- Reply to a user's message with %sdemote", p, p, p)
	}

	var demoted []string
	var demotedJIDs []types.JID
	for _, target := range targets {
		resolvedJID, username := ctx.ResolveMention(target)
		if utils.IsSudoRaw(ctx.Ctx, ctx.Client, target) {
			_ = ctx.ReplyWithMentions(Sprintf("⚠️ Cannot demote bot owner or sudo user @%s.", username), []types.JID{resolvedJID})
			continue
		}
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
		if err != nil {
			_ = ctx.ReplyWithMentions(Sprintf("Failed to demote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			demoted = append(demoted, "@"+username)
			demotedJIDs = append(demotedJIDs, resolvedJID)
		}
	}

	if len(demoted) > 0 {
		return ctx.ReplyWithMentions(Sprintf("Demoted: %s", strings.Join(demoted, ", ")), demotedJIDs)
	}
	return nil
}

func handleGroup(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf(" Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change group settings.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to change group settings.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sgroup open\n- %sgroup close\n- %sgroup lock\n- %sgroup unlock\n- %sgroup linkmode <admin|all>\n- %sgroup history <admin|all>\n- %sgroup addmode <admin|all>", p, p, p, p, p, p, p)
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "open":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Replyf("Failed to open group: %v", err)
		}
		return ctx.Reply("Group opened. Everyone can send messages.")
	case "close":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Replyf("Failed to close group: %v", err)
		}
		return ctx.Reply("Group closed. Only admins can send messages.")
	case "lock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Replyf("Failed to lock group: %v", err)
		}
		return ctx.Reply("Group locked. Only admins can edit group settings.")
	case "unlock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Replyf("Failed to unlock group: %v", err)
		}
		return ctx.Reply("Group unlocked. Everyone can edit group settings.")
	case "linkmode", "memberlink":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Usage: %sgroup linkmode <admin|all>", p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberLinkMode
		switch modeArg {
		case "admin", "admin_link", "admins":
			mode = types.GroupMemberLinkModeAdmin
		case "all", "all_member_link", "everyone", "members":
			mode = types.GroupMemberLinkModeAllMember
		default:
			return ctx.Reply("Invalid mode. Options: admin, all")
		}
		err = ctx.Client.SetGroupMemberLinkMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to set group member link mode: %v", err)
		}
		if mode == types.GroupMemberLinkModeAdmin {
			return ctx.Reply("Member link mode updated: Only admins can manage/share invite links.")
		}
		return ctx.Reply("Member link mode updated: All members can manage/share invite links.")
	case "history", "sharehistory", "historymode":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Usage: %sgroup history <admin|all>", p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberShareHistoryMode
		switch modeArg {
		case "admin", "admin_share", "admins":
			mode = types.GroupMemberShareHistoryModeAdmin
		case "all", "all_member_share", "everyone", "members":
			mode = types.GroupMemberShareHistoryModeAllMember
		default:
			return ctx.Reply("Invalid mode. Options: admin, all")
		}
		err = ctx.Client.SetGroupMemberShareHistoryMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to set group history mode: %v", err)
		}
		if mode == types.GroupMemberShareHistoryModeAdmin {
			return ctx.Reply("Group history sharing mode set to ADMIN only.")
		}
		return ctx.Reply("Group history sharing mode set to ALL members (new members can read recent history).")
	case "addmode", "memberadd":
		if len(ctx.Args) < 2 {
			p := ctx.GetPrefix()
			return ctx.Replyf("Usage: %sgroup addmode <admin|all>", p)
		}
		modeArg := strings.ToLower(ctx.Args[1])
		var mode types.GroupMemberAddMode
		switch modeArg {
		case "admin", "admin_add", "admins":
			mode = types.GroupMemberAddModeAdmin
		case "all", "all_member_add", "everyone", "members":
			mode = types.GroupMemberAddModeAllMember
		default:
			return ctx.Reply("Invalid mode. Options: admin, all")
		}
		err = ctx.Client.SetGroupMemberAddMode(ctx.Ctx, ctx.Chat, mode)
		if err != nil {
			return ctx.Replyf("Failed to set group member add mode: %v", err)
		}
		if mode == types.GroupMemberAddModeAdmin {
			return ctx.Reply("Member add mode updated: Only admins can add members.")
		}
		return ctx.Reply("Member add mode updated: All members can add members.")
	default:
		return ctx.Reply("Invalid action. Usage: group <open|close|lock|unlock|linkmode|history|addmode>")
	}
}

func handleAntiLink(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-link settings.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
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
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been activated for this group.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been deactivated for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiLinkMenu(ctx, s, "Anti-link protection has been deactivated for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been activated for this group.")

	case "mode", "customize":
		bodyText := NewText().
			Header("ANTILINK CUSTOMIZE").
			Field("Group", groupName).
			Blank().
			Section("Choose Anti-Link Protection Mode:").
			Numbered(1, "Default Links: Block all web links (http://, https://, www, .com, etc.)").
			Numbered(2, "Custom URLs: Block specific domain patterns separated by comma (e.g. chat.whatsapp.com, t.me)").
			Trimmed()
		options := []string{
			"Default Links",
			"Custom URLs",
		}
		return sendPollReply(ctx, bodyText, options)

	case "default":
		_ = s.PutSetting(ctx.Ctx, modeKey, "default")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		bodyText := NewText().
			Header("ANTILINK MODE SET").
			Field("Group", groupName).
			Field("Mode", "DEFAULT (ALL LINKS)").
			Field("Status", "ACTIVE").
			Blank().
			Line("Anti-link will now block all web links sent in this group!").
			Trimmed()
		options := []string{
			"Deactivate",
			"Customize Mode",
		}
		return sendPollReply(ctx, bodyText, options)

	case "custom", "set":
		customInput := ""
		if len(args) > 1 {
			customInput = strings.Join(args[1:], " ")
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
			bodyText := NewText().
				Header("ANTILINK CUSTOM MODE").
				Field("Group", groupName).
				Field("Mode", "CUSTOM DOMAINS").
				Field("Status", "ACTIVE").
				Field("Blocked", currCustom).
				Blank().
				Section("To update custom domains, send:").
				Linef("%santilink set domain1, domain2", p).
				Blank().
				Section("Example:").
				Linef("%santilink set chat.whatsapp.com, t.me, instagram.com", p).
				Trimmed()
			options := []string{
				"Default Links",
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
			return ctx.Reply("Please specify at least one valid domain pattern separated by comma. Example: `chat.whatsapp.com, t.me`")
		}

		newCustomStr := strings.Join(cleaned, ", ")
		_ = s.PutSetting(ctx.Ctx, customKey, newCustomStr)
		_ = s.PutSetting(ctx.Ctx, modeKey, "custom")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		bodyText := NewText().
			Header("ANTILINK CUSTOMIZED").
			Field("Group", groupName).
			Field("Mode", "CUSTOM DOMAINS").
			Field("Status", "ACTIVE").
			Field("Blocked", newCustomStr).
			Blank().
			Line("Anti-link will now block messages containing these custom domain patterns!").
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
				return ctx.Reply("Invalid action. Options: delete, kick, warn")
			}
			_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, act)
			return sendAntiLinkMenu(ctx, s, Sprintf("Anti-link action mode updated to *%s*.", strings.ToUpper(act)))
		}
		bodyText := NewText().
			Header("ANTILINK ACTION MODE").
			Field("Group", groupName).
			Blank().
			Section("Choose what happens when a non-admin participant sends a link:").
			Numbered(1, "Delete: Delete message only").
			Numbered(2, "Kick: Delete message & kick participant").
			Numbered(3, "Warn: Issue warning (default 3 max). Kick upon reaching threshold").
			Trimmed()
		options := []string{
			"Delete Only",
			"Kick User",
			"Warn User",
		}
		return sendPollReply(ctx, bodyText, options)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify warning limit. Example: `antilink setwarn 5`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Invalid warning limit. Must be a positive integer.")
		}
		_ = s.PutSetting(ctx.Ctx, "antilink_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, "warn")
		return sendAntiLinkMenu(ctx, s, Sprintf("Anti-link warning limit set to *%d*. Action mode switched to WARN.", cnt))

	default:
		return sendAntiLinkMenu(ctx, s, "")
	}
}

func sendAntiLinkMenu(ctx *Context, s *StoreWrapper, note string) error {
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
		actionDisplay = Sprintf("WARN (Max: %s)", maxWarn)
	}

	custom, _ := s.GetSetting(ctx.Ctx, "antilink_custom:"+chatKey)

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("ANTILINK CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Field("Mode", strings.ToUpper(mode)).
		Field("Action", actionDisplay)

	if mode == "custom" && custom != "" {
		tb.Field("Blocked", custom)
	}
	tb.Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	tb.Section("Options:").
		Bulletf("`%santilink mode` - Switch between Default Links & Custom URLs", p).
		Bulletf("`%santilink action <delete|kick|warn>` - Set action mode", p).
		Bulletf("`%santilink setwarn 3` - Customize max warnings", p)

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

func handleAntiWord(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-word settings.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
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
		return sendAntiWordMenu(ctx, s, "Anti-word protection activated.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "off")
		return sendAntiWordMenu(ctx, s, "Anti-word protection deactivated.")

	case "action", "act":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			if act != "delete" && act != "kick" && act != "warn" {
				return ctx.Reply("Invalid action. Options: delete, kick, warn")
			}
			_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, act)
			return sendAntiWordMenu(ctx, s, Sprintf("Anti-word action mode set to *%s*.", strings.ToUpper(act)))
		}
		bodyText := NewText().
			Header("ANTIWORD ACTION MODE").
			Field("Group", groupName).
			Blank().
			Section("Choose what happens when a non-admin participant sends a banned word:").
			Numbered(1, "Delete: Delete message only").
			Numbered(2, "Kick: Delete message & kick participant").
			Numbered(3, "Warn: Issue warning (default 3 max). Kick upon reaching threshold").
			Trimmed()
		options := []string{
			"Delete Only",
			"Kick User",
			"Warn User",
		}
		return sendPollReply(ctx, bodyText, options)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify warning limit. Example: `antiword setwarn 5`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Invalid warning limit. Must be a positive integer.")
		}
		_ = s.PutSetting(ctx.Ctx, "antiword_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, "warn")
		return sendAntiWordMenu(ctx, s, Sprintf("Anti-word warning limit set to *%d*. Action mode switched to WARN.", cnt))

	case "add":
		if len(args) < 2 {
			return ctx.Reply("Please specify the word to add.")
		}
		wordToAdd := strings.ToLower(args[1])
		if slices.Contains(words, wordToAdd) {
			return ctx.Replyf("Word %q is already banned.", wordToAdd)
		}
		words = append(words, wordToAdd)
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(words, " "))
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "on")
		return sendAntiWordMenu(ctx, s, Sprintf("Banned word %q added.", wordToAdd))

	case "del", "remove":
		if len(args) < 2 {
			return ctx.Reply("Please specify the word to remove.")
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
			return ctx.Replyf("Word %q was not banned.", wordToDel)
		}
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(newWords, " "))
		return sendAntiWordMenu(ctx, s, Sprintf("Banned word %q removed.", wordToDel))

	case "list":
		if len(words) == 0 {
			return ctx.Reply("No banned words configured in this group.")
		}
		return ctx.Replyf("Banned Words list for %s:\n- %s", groupName, strings.Join(words, "\n- "))

	default:
		return sendAntiWordMenu(ctx, s, "")
	}
}

func sendAntiWordMenu(ctx *Context, s *StoreWrapper, note string) error {
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
		actionDisplay = Sprintf("WARN (Max: %s)", maxWarn)
	}

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("ANTIWORD CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Field("Action", actionDisplay).
		Fieldf("Banned", "%d word(s)", len(words)).
		Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	if len(words) > 0 {
		tb.Linef("Banned Words: %s", strings.Join(words, ", ")).Blank()
	}

	tb.Section("Options:").
		Bulletf("`%santiword add <word>` - Add banned word", p).
		Bulletf("`%santiword del <word>` - Remove banned word", p).
		Bulletf("`%santiword action <delete|kick|warn>` - Set action mode", p).
		Bulletf("`%santiword setwarn 3` - Set warning limit", p)

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

func handleGStats(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	chatStr := ctx.Chat.String()

	var totalMsgs int
	err := db.QueryRow(ctx.Ctx, `SELECT COUNT(*) FROM whatsmeow_message_secrets WHERE chat_jid=$1`, chatStr).Scan(&totalMsgs)
	if err != nil {
		return err
	}

	if totalMsgs == 0 {
		return ctx.Reply("No message activity found in database for this group.")
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
		Header("Group Activity Statistics").
		Fieldf("Total messages tracked", "%d", totalMsgs).
		Fieldf("Unique active senders", "%d", activeUsers).
		Blank().
		Section("Top Active Participants")

	rank := 1
	for rows.Next() {
		var userStr string
		var count int
		if err := rows.Scan(&userStr, &count); err == nil {
			if uj, err := types.ParseJID(userStr); err == nil {
				uj = uj.ToNonAD()
				resolvedJID, username := ctx.ResolveMention(uj)
				tb.Numbered(rank, Sprintf("@%s (%d msgs)", username, count)).
					Mentions(resolvedJID)
				rank++
			}
		}
	}

	return tb.Reply()
}

func handlePoll(ctx *Context) error {
	raw := strings.TrimSpace(ctx.RawArgs)
	if raw == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %spoll Question | Option 1 | Option 2 | ...", p)
	}

	selectableCount := -1
	if strings.HasPrefix(raw, "--single ") || strings.HasPrefix(raw, "-s ") || strings.HasPrefix(raw, "single ") {
		selectableCount = 1
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	} else if strings.HasPrefix(raw, "--multi ") || strings.HasPrefix(raw, "-m ") || strings.HasPrefix(raw, "multi ") || strings.HasPrefix(raw, "multiple ") {
		selectableCount = 0
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	}

	parts := strings.Split(raw, "|")
	if len(parts) < 3 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %spoll Question | Option 1 | Option 2 | ...", p)
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
		return ctx.Reply("Please provide at least 2 options.")
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
		return poll.Reply(func(req utils.PollRequest, res *utils.Response) {
			if len(req.SelectedOptions) > 0 {
				_ = res.Reply(Sprintf("🗳️ Vote recorded for: *%s*", strings.Join(req.SelectedOptions, ", ")))
			}
		})
	}

	tb := ctx.Text().
		Line("Poll Creation").
		Blank().
		Field("Question", question).
		Blank().
		Section("Options:")

	for i, opt := range options {
		tb.Numbered(i+1, opt)
	}
	tb.Blank().
		Line("Select poll type from the poll below to create your poll.")

	_ = ctx.Reply(tb.String())

	poll := ctx.Rook().NewPoll("Select Poll Type:")
	poll.AddOption("Single Choice").
		AddOption("Multiple Choice")

	return poll.Reply(func(req utils.PollRequest, res *utils.Response) {
		if len(req.SelectedOptions) > 0 {
			single := req.SelectedOptions[0] == "Single Choice"
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

func handleInvite(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can retrieve the invite link.")
	}

	link, err := ctx.Client.GetGroupInviteLink(ctx.Ctx, ctx.Chat, false)
	if err != nil {
		return ctx.Replyf("Failed to get invite link: %v", err)
	}
	return ctx.Reply(link)
}

func TrackPresence(jid types.JID, isOnline bool) {
	cliutils.TrackPresence(jid, isOnline)
}

func IsUserOnline(jid types.JID, client *whatsmeow.Client) bool {
	if jid.IsEmpty() {
		return false
	}
	targetKey := jid.ToNonAD().String()

	cliutils.PresenceMu.RLock()
	info, exists := cliutils.PresenceMap[targetKey]
	cliutils.PresenceMu.RUnlock()

	if exists && (info.IsOnline || time.Since(info.LastSeen) < 15*time.Minute) {
		Logger.Debug("IsUserOnline check: direct match online", "jid", targetKey, "lastSeen", info.LastSeen)
		return true
	}

	if client != nil && client.Store != nil && client.Store.LIDs != nil {
		ctx := context.Background()
		if jid.Server == types.HiddenUserServer {
			pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
			if err == nil && !pn.IsEmpty() {
				pnKey := pn.ToNonAD().String()
				cliutils.PresenceMu.RLock()
				pnInfo, pnExists := cliutils.PresenceMap[pnKey]
				cliutils.PresenceMu.RUnlock()
				if pnExists && (pnInfo.IsOnline || time.Since(pnInfo.LastSeen) < 15*time.Minute) {
					Logger.Debug("IsUserOnline check: PN match online for LID", "lid", targetKey, "pn", pnKey)
					return true
				}
			}
		} else {
			lid, err := client.Store.LIDs.GetLIDForPN(ctx, jid)
			if err == nil && !lid.IsEmpty() {
				lidKey := lid.ToNonAD().String()
				cliutils.PresenceMu.RLock()
				lidInfo, lidExists := cliutils.PresenceMap[lidKey]
				cliutils.PresenceMu.RUnlock()
				if lidExists && (lidInfo.IsOnline || time.Since(lidInfo.LastSeen) < 15*time.Minute) {
					Logger.Debug("IsUserOnline check: LID match online for PN", "pn", targetKey, "lid", lidKey)
					return true
				}
			}
		}
	}

	Logger.Debug("IsUserOnline check: offline or unknown", "jid", targetKey)
	return false
}

func handleListOnline(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		Logger.Debug("handleListOnline: not a group chat", "chat", ctx.Chat.String())
		return ctx.Reply("This command can only be used in a group.")
	}

	Logger.Debug("handleListOnline executing", "group", ctx.Chat.String())
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		Logger.Error("handleListOnline: failed to get group info", "group", ctx.Chat.String(), "err", err)
		return ctx.Replyf("Failed to get group info: %v", err)
	}

	total := len(info.Participants)
	Logger.Debug("handleListOnline retrieved group info", "group", ctx.Chat.String(), "participant_count", total)

	if total == 0 {
		return ctx.Reply("No participants found in this group.")
	}

	// 1. Send status message to prompt WhatsApp servers to trigger group-wide delivery receipts
	_ = ctx.Reply("Fetching online participants...")

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
				Logger.Debug("handleListOnline: received presence stanza from WhatsApp", "from", fromKey, "unavailable", pEvt.Unavailable)
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
					Logger.Debug("handleListOnline: received delivery receipt from WhatsApp", "sender", senderKey)
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
			Logger.Debug("handleListOnline: presence/receipt stanzas collected", "count", receivedCount)
		case <-time.After(2000 * time.Millisecond):
			Logger.Debug("handleListOnline: presence wait window ended", "received", receivedCount, "total", total)
		}
	}

	var onlineJIDs []types.JID
	var displayNames []string

	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			resolvedJID, username := ctx.ResolveMention(p.JID)
			onlineJIDs = append(onlineJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
			Logger.Debug("handleListOnline: participant online", "participant", p.JID.String(), "username", username)
		} else {
			Logger.Debug("handleListOnline: participant offline", "participant", p.JID.String())
		}
	}

	Logger.Debug("handleListOnline complete", "group", ctx.Chat.String(), "total_participants", total, "online_count", len(onlineJIDs))

	if len(onlineJIDs) == 0 {
		return ctx.Reply("No online participants detected in this group.")
	}

	tb := ctx.Text().
		Headerf("Online Participants (%d)", len(onlineJIDs)).
		Mentions(onlineJIDs...)

	for _, name := range displayNames {
		tb.Bullet(name)
	}

	return tb.Reply()
}

func handleKickAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}

	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}

	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins or bot owners can use kickall.")
	}

	botJID := ctx.Client.Store.ID.ToNonAD()
	botLID := ctx.Client.Store.LID.ToNonAD()

	botIsAdmin := false
	for _, p := range info.Participants {
		if (p.JID.User == botJID.User || (!botLID.IsEmpty() && p.JID.User == botLID.User)) && p.IsAdmin {
			botIsAdmin = true
			break
		}
	}

	if !botIsAdmin {
		return ctx.Reply("I need admin privileges to kick participants.")
	}

	var toKick []types.JID
	for _, p := range info.Participants {
		if p.JID.User == botJID.User || (!botLID.IsEmpty() && p.JID.User == botLID.User) {
			continue
		}
		if p.JID.User == ctx.Sender.ToNonAD().User {
			continue
		}
		if isJIDSudo(ctx, p.JID) {
			continue
		}
		toKick = append(toKick, p.JID)
	}

	if len(toKick) == 0 {
		return ctx.Reply("No participants to kick.")
	}

	_ = ctx.Replyf("Kicking %d participants...", len(toKick))
	_, err = ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, toKick, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		Logger.Error("Kickall failed", "err", err)
		return ctx.Replyf("Failed to kick participants: %v", err)
	}

	return ctx.Replyf("Kickall complete! Removed %d participants.", len(toKick))
}

func handleCommunity(ctx *Context) error {
	groups, err := ctx.Client.GetJoinedGroups(ctx.Ctx)
	if err != nil || len(groups) == 0 {
		return ctx.Reply("Failed to fetch joined groups or no groups joined.")
	}

	tb := ctx.Text().Header("GROUPS & COMMUNITIES")

	for i, g := range groups {
		groupName := g.Name
		if groupName == "" && g.GroupName.Name != "" {
			groupName = g.GroupName.Name
		}
		if groupName == "" {
			groupName = Sprintf("Group %d", i+1)
		}

		typeTag := "Group"
		if g.GroupParent.IsParent {
			typeTag = "Community (Parent)"
		} else if !g.GroupLinkedParent.LinkedParentJID.IsEmpty() {
			typeTag = "Community Subgroup"
		}

		memberCount := len(g.Participants)
		link := "Invite link unavailable"
		if code, errL := ctx.Client.GetGroupInviteLink(ctx.Ctx, g.JID, false); errL == nil && code != "" {
			link = "https://chat.whatsapp.com/" + code
		}

		tb.Numbered(i+1, Sprintf("%s [%s]", Bold(groupName), typeTag)).
			Indent(3, Sprintf("Members: %d", memberCount)).NewLine().
			Indent(3, Sprintf("Link: %s", link)).NewLine().
			Blank()
	}

	return tb.Reply()
}

func handleChannels(ctx *Context) error {
	newsletters, err := ctx.Client.GetSubscribedNewsletters(ctx.Ctx)
	if err != nil || len(newsletters) == 0 {
		return ctx.Reply("No subscribed channels/newsletters found.")
	}

	tb := ctx.Text().Header("SUBSCRIBED CHANNELS")

	for i, n := range newsletters {
		name := n.ThreadMeta.Name.Text
		if name == "" {
			name = Sprintf("Channel %d", i+1)
		}
		subs := n.ThreadMeta.SubscriberCount
		role := "SUBSCRIBER"
		if n.ViewerMeta != nil && n.ViewerMeta.Role != "" {
			role = string(n.ViewerMeta.Role)
		}
		link := "None"
		if n.ThreadMeta.InviteCode != "" {
			link = "https://whatsapp.com/channel/" + n.ThreadMeta.InviteCode
		}

		tb.Numbered(i+1, Bold(name)).
			Indent(3, Sprintf("Role: %s | Followers: %d", role, subs)).NewLine().
			Indent(3, Sprintf("Link: %s", link)).NewLine().
			Blank()
	}

	return tb.Reply()
}

func handleLeave(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
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
				return ctx.ReplyWithMentions(Sprintf("Only the command caller (%s) can confirm leaving this group.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}

		_ = ctx.Reply("Leaving group... Goodbye!")
		err := ctx.Client.LeaveGroup(ctx.Ctx, ctx.Chat)
		if err != nil {
			Logger.Error("Failed to leave group", "err", err)
			return ctx.Replyf("Failed to leave group: %v", err)
		}
		return nil
	}

	if strings.HasPrefix(arg0, "cancel") {
		parts := strings.Split(arg0, "_")
		if len(parts) >= 2 {
			callerUser := parts[1]
			if senderUser != callerUser && !ctx.IsSudo() {
				callerMention, _ := ctx.ResolveMention(types.NewJID(callerUser, "s.whatsapp.net"))
				return ctx.ReplyWithMentions(Sprintf("Only the command caller (%s) can cancel leaving.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}
		return ctx.Reply("Leave group cancelled.")
	}

	bodyText := "⚠️ ARE YOU SURE YOU WANT ME TO LEAVE THIS GROUP?\n\nSelect an option from the poll below to confirm or cancel."
	options := []string{
		"Confirm Leave",
		"Cancel",
	}

	return sendPollReply(ctx, bodyText, options)
}

func handleJoin(ctx *Context) error {
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
		return ErrUsage("join <group_url>")
	}

	jid, err := ctx.Client.JoinGroupWithLink(ctx.GetSendContext(), code)
	if err != nil {
		return ctx.Replyf("Failed to join group: %v", err)
	}

	groupName := ""
	if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), jid); errInfo == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}

	if groupName != "" {
		return ctx.Replyf("Successfully joined group: *%s*", groupName)
	}
	return ctx.Reply("Successfully joined the group!")
}

func handleJoinV4(ctx *Context, inviteMsg *waE2E.GroupInviteMessage, isQuoted bool) error {
	groupJIDStr := inviteMsg.GetGroupJID()
	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil || groupJID.IsEmpty() {
		return ctx.Reply("Invalid group JID in invite message.")
	}

	code := inviteMsg.GetInviteCode()
	if code == "" {
		return ctx.Reply("Invalid invite code in invite message.")
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
		return ctx.Replyf("Failed to join group via invite: %v", err)
	}

	groupName := inviteMsg.GetGroupName()
	if groupName == "" {
		if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), groupJID); errInfo == nil && info != nil && info.Name != "" {
			groupName = info.Name
		}
	}

	if groupName != "" {
		return ctx.Replyf("Successfully joined group: *%s*", groupName)
	}
	return ctx.Reply("Successfully joined the group!")
}

func extractGroupInviteCode(ctx *Context) string {
	if ctx == nil {
		return ""
	}
	if ctx.RawArgs != "" {
		if match := cliutils.GroupInviteLinkRegex.FindStringSubmatch(ctx.RawArgs); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(ctx.RawArgs)
		if !strings.ContainsAny(trimmed, " \t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		text := extractMessageText(quoted)
		if match := cliutils.GroupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(text)
		if !strings.ContainsAny(trimmed, " \t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if ctx.Evt != nil && ctx.Evt.Message != nil {
		text := extractMessageText(ctx.Evt.Message)
		if match := cliutils.GroupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
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

func handleAntiMsg(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been activated for this group.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been deactivated for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiMsgMenu(ctx, s, "AntiMsg has been deactivated for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been activated for this group.")

	case "clear":
		_ = s.PutSetting(ctx.Ctx, usersKey, "")
		return sendAntiMsgMenu(ctx, s, "AntiMsg target list has been cleared.")

	case "list", "status":
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		if status == "" {
			status = "off"
		}

		p := ctx.GetPrefix()
		if len(users) == 0 {
			bodyText := NewText().
				Header("ANTIMSG TARGETS").
				Field("Status", strings.ToUpper(status)).
				Field("Targets", "None").
				Blank().
				Line("No participants are currently targeted in this group.").
				Linef("Reply to or mention (@user) anyone with %santimsg to add them.", p).
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
				return utils.IsSameUserRaw(ctx.Ctx, ctx.Client, existing, uj)
			}) {
				displayUsers = append(displayUsers, uj)
			}
		}

		tb := ctx.Text().
			Header("ANTIMSG TARGETS").
			Field("Status", strings.ToUpper(status)).
			Fieldf("Total", "%d targeted user(s)", len(displayUsers)).
			Blank().
			Section("Targeted Participants:")

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
			return ctx.Reply("Please reply to a user's message or mention (@user) to remove them from AntiMsg.")
		}
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)

		var removedMentions []types.JID
		var removedUsernames []string

		for _, t := range targets {
			newUsers := make([]string, 0, len(users))
			for _, uStr := range users {
				uJID, err := types.ParseJID(uStr)
				if err == nil && utils.IsSameUserRaw(ctx.Ctx, ctx.Client, uJID, t) {
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

		bodyText := NewText().
			Header("ANTIMSG UPDATED").
			Field("Removed", strings.Join(removedUsernames, ", ")).
			Fieldf("Total", "%d targeted user(s)", len(users)).
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
				if err == nil && utils.IsSameUserRaw(ctx.Ctx, ctx.Client, uJID, resolvedTarget) {
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
			return ctx.Replyf("⚠️ Cannot add bot owner or sudo user(s) (%s) to AntiMsg.", strings.Join(sudoRejected, ", "))
		}

		if len(addedUsernames) == 0 {
			return ctx.Reply("Specified user(s) are already in the AntiMsg target list.")
		}

		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		tb := NewText().
			Header("ANTIMSG ACTIVATED").
			Field("Status", "ON").
			Field("Added", strings.Join(addedUsernames, ", ")).
			Fieldf("Total", "%d targeted user(s)", len(users)).
			Blank().
			Line("AntiMsg is active! Messages from targeted participants will be automatically deleted.")
		if len(sudoRejected) > 0 {
			tb.Blank().Linef("⚠️ Skipped bot owner/sudoers: %s", strings.Join(sudoRejected, ", "))
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
		return ctx.Replyf("Please reply to a user's message or mention (@user) to add them to AntiMsg.\n\nExample:\n- Reply to message with `%santimsg`\n- `%santimsg @user`", p, p)
	}

	// 3. Default menu display
	return sendAntiMsgMenu(ctx, s, "")
}

func sendAntiMsgMenu(ctx *Context, s *StoreWrapper, note string) error {
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
		Fieldf("Targets", "%d user(s)", len(users)).
		Blank()

	if note != "" {
		tb.Line(note).Blank()
	}

	tb.Section("How to use AntiMsg:").
		Bulletf("Reply to any message with `%santimsg` to add user", p).
		Bulletf("Mention `@user` with `%santimsg` to add user", p).
		Bulletf("Remove user: `%santimsg del @user`", p).
		Bulletf("View list: `%santimsg list`", p)

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

func extractTargetParticipants(ctx *Context, args []string) []types.JID {
	var targets []types.JID

	addJID := func(j types.JID) {
		if j.IsEmpty() {
			return
		}
		resolved := ctx.ResolvePN(j)
		if !slices.ContainsFunc(targets, func(existing types.JID) bool {
			return utils.IsSameUserRaw(ctx.Ctx, ctx.Client, existing, resolved)
		}) {
			targets = append(targets, resolved)
		}
	}

	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
			botJID := ctx.Client.Store.ID.ToNonAD()
			if !utils.IsSameUserRaw(ctx.Ctx, ctx.Client, quotedSender, botJID) {
				addJID(quotedSender)
			}
		} else {
			addJID(quotedSender)
		}
	}

	if ci := ctx.GetContextInfo(); ci != nil {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := utils.ParseUserJID(m); err == nil && !parsed.IsEmpty() {
				addJID(parsed)
			}
		}
	}

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || isSubcommand(arg) {
			continue
		}
		if parsed, err := utils.ParseUserJID(arg); err == nil && !parsed.IsEmpty() {
			addJID(parsed)
		}
	}

	return targets
}

func handleAntiSpam(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
			return ctx.Reply("Failed to enable AntiSpam.")
		}
		return ctx.Reply("AntiSpam feature enabled for this group.")

	case "off", "disable", "deactivate":
		if err := s.PutSetting(ctx.Ctx, statusKey, "off"); err != nil {
			return ctx.Reply("Failed to disable AntiSpam.")
		}
		return ctx.Reply("AntiSpam feature disabled for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		nextState := "on"
		if curr == "on" {
			nextState = "off"
		}
		if err := s.PutSetting(ctx.Ctx, statusKey, nextState); err != nil {
			return ctx.Reply("Failed to toggle AntiSpam.")
		}
		if nextState == "on" {
			return ctx.Reply("AntiSpam feature enabled for this group.")
		}
		return ctx.Reply("AntiSpam feature disabled for this group.")

	case "customize", "custom", "help":
		return sendAntiSpamCustomizeGuide(ctx)

	case "action":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, actionKey)
			if curr == "" {
				curr = "delete"
			}
			return ctx.Replyf("Current AntiSpam action: %s\nUsage: %santispam action [delete|warn|kick]", curr, ctx.GetPrefix())
		}
		act := strings.ToLower(args[1])
		if act != "delete" && act != "warn" && act != "kick" {
			return ctx.Replyf("Invalid action. Usage: %santispam action [delete|warn|kick]", ctx.GetPrefix())
		}
		if err := s.PutSetting(ctx.Ctx, actionKey, act); err != nil {
			return ctx.Reply("Failed to update AntiSpam action.")
		}
		return ctx.Reply("AntiSpam action updated to " + act + ".")

	case "max", "threshold", "limit":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, maxKey)
			if curr == "" {
				curr = "5"
			}
			return ctx.Replyf("Current AntiSpam message limit: %s msgs/5s\nUsage: %santispam max [number]", curr, ctx.GetPrefix())
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 2 || num > 30 {
			return ctx.Reply("Please specify a valid message limit between 2 and 30.")
		}
		if err := s.PutSetting(ctx.Ctx, maxKey, strconv.Itoa(num)); err != nil {
			return ctx.Reply("Failed to update AntiSpam threshold.")
		}
		return ctx.Reply("AntiSpam message limit set to " + strconv.Itoa(num) + " messages per 5 seconds.")

	default:
		return ctx.Replyf("Usage: %santispam [on|off|toggle|customize|action|max]", ctx.GetPrefix())
	}
}

func sendAntiSpamMenu(ctx *Context, s *StoreWrapper) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, "antispam_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := NewText().
		Header("ANTISPAM CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Choose an option below to change status or view customization options.").
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

func sendAntiSpamCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("ANTISPAM CUSTOMIZATION GUIDE").
		Blank().
		Section("Available Customizations:").
		Bulletf("Automated Action : `%santispam action delete | warn | kick`", p).
		Bulletf("Rate Limit Max   : `%santispam max <number>` (messages per 5 seconds)", p).
		Blank().
		Section("Examples:").
		Numberedf(1, "`%santispam action kick` (Automatically kick spammers)", p).
		Numberedf(2, "`%santispam action warn` (Issue warnings to spammers)", p).
		Numberedf(3, "`%santispam max 3` (Set limit to 3 msgs / 5s)", p).
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
							Logger.Error("automute: PANIC in scheduler tick", "recover", r)
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

func handleAutoMute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can set automute schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Replyf("Usage:\n- `%sautomute 22:00` (Sets daily automute at 10:00 PM)\n- `%sautomute off` (Disables automute)", p, p)
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	settingKey := "automute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Automute schedule disabled for this group.")
	}

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Invalid time format. Please specify time as HH:MM (24h, e.g. `22:00`) or H:MM AM/PM (12h, e.g. `10:00 PM`).")
	}
	arg = normalized

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save automute schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Replyf("Automute schedule set to *%s* daily (Timezone: *%s*).\nThe group will close automatically at %s every day.", arg, tz, arg)
}

func handleAutoUnmute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can set autounmute schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Replyf("Usage:\n- `%sautounmute 06:00` (Sets daily autounmute at 06:00 AM)\n- `%sautounmute off` (Disables autounmute)", p, p)
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	settingKey := "autounmute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Autounmute schedule disabled for this group.")
	}

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Invalid time format. Please specify time as HH:MM (24h, e.g. `22:00`) or H:MM AM/PM (12h, e.g. `10:00 PM`).")
	}
	arg = normalized

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save autounmute schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Replyf("Autounmute schedule set to *%s* daily (Timezone: *%s*).\nThe group will open automatically at %s every day.", arg, tz, arg)
}

func handleListMute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	muteTime, _ := s.GetSetting(ctx.Ctx, "automute:"+ctx.Chat.String())
	unmuteTime, _ := s.GetSetting(ctx.Ctx, "autounmute:"+ctx.Chat.String())
	tz := getUserTimezone(ctx.Ctx, s)

	p := ctx.GetPrefix()
	tb := ctx.Text().
		Header("Group Mute/Unmute Schedule Status").
		Field("Configured Timezone", tz).
		Blank()

	if muteTime != "" {
		tb.Linef("Automute (Group Close): %s daily", muteTime)
	} else {
		tb.Line("Automute (Group Close): Disabled")
	}

	if unmuteTime != "" {
		tb.Linef("Autounmute (Group Open): %s daily", unmuteTime)
	} else {
		tb.Line("Autounmute (Group Open): Disabled")
	}

	tb.Blank().
		Section("Commands:").
		Bulletf("`%sautomute <HH:MM>` (e.g. `%sautomute 22:00`)", p, p).
		Bulletf("`%sautounmute <HH:MM>` (e.g. `%sautounmute 06:00`)", p, p).
		Bulletf("`%stimezone` (to configure bot timezone)", p)

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
		return Sprintf("%02d:%02d", hour, minute), true
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
	s, ok := getSQLStore(client)
	if !ok || s == nil {
		return
	}

	tzName := getUserTimezone(ctx, s)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		Logger.Warn("automute: failed to load timezone, falling back to UTC", "tz", tzName, "err", err)
		loc = time.UTC
	}

	now := time.Now().In(loc)
	currentTimeStr := Sprintf("%02d:%02d", now.Hour(), now.Minute())

	settings, err := store.ListSettingsWithPrefixes(ctx, s.SQLStore, "automute:", "autounmute:")
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "database is closed") || ctx.Err() != nil {
			return
		}
		Logger.Error("automute: query failed", "err", err)
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
				Logger.Error("automute: GetSetting execKey failed or timed out", "group", groupJIDStr, "err", sErr)
				continue
			}
			dateMinuteKey := Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				Logger.Error("automute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				Logger.Warn("automute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			botJID := client.Store.ID.ToNonAD()
			botLID := client.Store.GetLID().ToNonAD()
			isAdmin := false
			for _, p := range info.Participants {
				matchesBot := (!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == botJID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == botLID) ||
					(p.JID.ToNonAD() == botJID)
				if matchesBot && (p.IsAdmin || p.IsSuperAdmin) {
					isAdmin = true
					break
				}
			}
			Logger.Debug("automute: admin check", "group", groupJIDStr, "bot_jid", botJID.String(), "is_admin", isAdmin)

			if !isAdmin {
				Logger.Warn("automute: bot is not admin in group, cannot mute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, true); err != nil {
				Logger.Error("automute: SetGroupAnnounce(true) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				Logger.Error("automute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}
			Logger.Info("automute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)

			unmuteTime, _ := s.GetSetting(ctx, "autounmute:"+groupJIDStr)
			groupName := info.GroupName.Name
			var noticeText string
			if unmuteTime != "" {
				noticeText = Sprintf("%s has been closed, and will be opened by %s at %s.", groupName, unmuteTime, tzName)
			} else {
				noticeText = Sprintf("%s has been closed.", groupName)
			}
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				Logger.Error("automute: failed to send close notice", "group", groupJIDStr, "err", sendErr)
			}

		} else if after, ok0 := strings.CutPrefix(key, "autounmute:"); ok0 {
			groupJIDStr := after
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				Logger.Warn("autounmute: bad group JID, skipping", "raw", groupJIDStr, "err", err)
				continue
			}

			execKey := "last_exec_autounmute:" + groupJIDStr
			lastExec, _ := s.GetSetting(ctx, execKey)
			dateMinuteKey := Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				Logger.Error("autounmute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				Logger.Warn("autounmute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			botJID := client.Store.ID.ToNonAD()
			botLID := client.Store.GetLID().ToNonAD()
			isAdmin := false
			for _, p := range info.Participants {
				matchesBot := (!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == botJID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == botLID) ||
					(p.JID.ToNonAD() == botJID) // fallback for older/PN-addressed groups
				if matchesBot && (p.IsAdmin || p.IsSuperAdmin) {
					isAdmin = true
					break
				}
			}
			Logger.Debug("autounmute: admin check", "group", groupJIDStr, "bot_jid", botJID.String(), "is_admin", isAdmin)

			if !isAdmin {
				Logger.Warn("autounmute: bot is not admin in group, cannot unmute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, false); err != nil {
				Logger.Error("autounmute: SetGroupAnnounce(false) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				Logger.Error("autounmute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}

			Logger.Info("autounmute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)
			groupName := info.GroupName.Name
			noticeText := Sprintf("%s has been opened.", groupName)
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				Logger.Error("autounmute: failed to send open notice", "group", groupJIDStr, "err", sendErr)
			}
		}
	}
}

func handleEventsCmd(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
		return ctx.Reply("Group Events notifications ENABLED.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("Group Events notifications DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("Group Events notifications DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Group Events notifications ENABLED.")

	case "customize", "custom", "help":
		return sendEventsCustomizeGuide(ctx)

	default:
		return ctx.Replyf("Usage: %sevents [on|off|toggle|customize]", ctx.GetPrefix())
	}
}

func sendEventsMenu(ctx *Context, s *StoreWrapper) error {
	chatKey := ctx.Chat.String()
	status, _ := s.GetSetting(ctx.Ctx, "events_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := NewText().
		Header("GROUP EVENTS NOTIFICATIONS").
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Choose an option below to toggle notifications or view customization options.").
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

func sendEventsCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("GROUP EVENTS NOTIFICATIONS GUIDE").
		Blank().
		Section("Supported Event Notifications:").
		Bullet("Group Name / Subject Changes").
		Bullet("Group Description / Topic Updates").
		Bullet("Group Settings Lock (Admins vs All Members)").
		Bullet("Group Announce Mute (Admins vs All Members)").
		Bullet("Admin Promotions & Demotions").
		Bullet("Member Joins & Leaves").
		Blank().
		Section("Commands:").
		Bulletf("Enable Notifications  : `%sevents on`", p).
		Bulletf("Disable Notifications : `%sevents off`", p).
		Bulletf("Toggle Status        : `%sevents toggle`", p).
		Reply()
}

func handleSetGroupPP(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group chat.")
	}

	groupInfo, errGroup := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if errGroup == nil && groupInfo != nil {
		isBotAdmin := utils.IsAdminRaw(ctx.Ctx, ctx.Client, groupInfo, ctx.Sender)
		if groupInfo.IsAnnounce && !isBotAdmin {
			return ctx.Reply("Only group admins are allowed to edit group info.")
		}
	}

	downloadable, _, mime := ExtractMediaFromEvent(ctx.Evt)
	if downloadable == nil {
		return ctx.Replyf("Please upload or reply to an image to set as group profile picture. Usage: %sgpp", ctx.GetPrefix())
	}

	rawBytes, err := ctx.Client.Download(ctx.Ctx, downloadable)

	if err != nil || len(rawBytes) == 0 {
		return ctx.Replyf("Failed to download image: %v", err)
	}

	jpegData, errConv := utils.EnsureJPEG(ctx.Ctx, rawBytes)
	if errConv != nil || len(jpegData) == 0 {
		return ctx.Replyf("Failed to process group photo format: %v", errConv)
	}

	Logger.Info("handleSetGroupPP: Setting group profile picture", "group", ctx.Chat.String(), "mime", mime, "rawBytes", len(rawBytes), "jpegBytes", len(jpegData))
	picID, errSet := ctx.Client.SetGroupPhoto(ctx.Ctx, ctx.Chat, jpegData)
	if errSet != nil {
		Logger.Error("handleSetGroupPP failed", "err", errSet)
		return ctx.Replyf("Failed to update group photo: %v", errSet)
	}

	return ctx.Replyf("Group profile photo updated successfully! (Picture ID: %s)", picID)
}

func handleWarn(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
				ctx.RawArgs = strings.Join(args[1:], " ")
			} else {
				ctx.RawArgs = ""
			}
			return handleSetWarn(ctx)
		}
	}

	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %swarn @user [reason]\n- Reply to a message with %swarn\n- %swarn 1234567890", p, p, p)
	}

	if isJIDOwnerOrSudo(ctx, targetJID) {
		return ctx.Reply("You cannot issue warnings to the bot owner or sudoers.")
	}

	isGroup := ctx.Chat.Server == "g.us"
	var groupInfo *types.GroupInfo
	if isGroup {
		var err error
		groupInfo, err = ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err != nil {
			return ctx.Replyf("Failed to fetch group info: %v", err)
		}

		if isParticipantAdmin(groupInfo, targetJID) && !ctx.IsSudo() {
			return ctx.Reply("You cannot issue warnings to group admins.")
		}
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := Sprintf("warn_count:%s:%s", chatKey, userKey)
	limitKey := Sprintf("warn_limit:%s", chatKey)

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
		reason = strings.Join(args[1:], " ")
	} else if len(args) > 1 && strings.HasPrefix(args[0], "@") {
		reason = strings.Join(args[1:], " ")
	}

	if currentWarns < maxLimit {
		msg := Sprintf("⚠️ Warning issued to @%s (%d/%d).", username, currentWarns, maxLimit)
		if reason != "" {
			msg += "\nReason: " + reason
		}
		return ctx.ReplyWithMentions(msg, []types.JID{resolvedJID})
	}

	if isGroup {
		targetIsAdmin := isParticipantAdmin(groupInfo, targetJID)
		botIsOwner := isBotGroupOwner(ctx, groupInfo)

		if targetIsAdmin && !botIsOwner {
			return ctx.ReplyWithMentions(Sprintf("⚠️ @%s reached the maximum warning limit (%d/%d), but cannot be kicked/blocked because they are a group admin and I am not the group owner.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		if !isBotAdmin(ctx, groupInfo) {
			return ctx.ReplyWithMentions(Sprintf("⚠️ @%s reached the maximum warning limit (%d/%d), but I require admin privileges to block and kick them.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		_, _ = ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock)
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{targetJID}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			return ctx.Replyf("Failed to kick @%s from group: %v", username, err)
		}

		_ = s.PutSetting(ctx.Ctx, warnKey, "0")
		return ctx.ReplyWithMentions(Sprintf("🚨 @%s reached maximum warnings (%d/%d) and has been blocked and kicked from the group.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	_, _ = ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock)
	_ = s.PutSetting(ctx.Ctx, warnKey, "0")
	return ctx.ReplyWithMentions(Sprintf("🚨 User @%s reached maximum warnings (%d/%d) and has been blocked.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
}

func handleUnwarn(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		return ctx.Reply("Please mention a participant or quote their message to remove warnings.")
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := Sprintf("warn_count:%s:%s", chatKey, userKey)

	rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
	currentWarns, _ := strconv.Atoi(rawCount)
	if currentWarns <= 0 {
		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(Sprintf("@%s has 0 active warnings.", username), []types.JID{resolvedJID})
	}

	currentWarns--
	_ = s.PutSetting(ctx.Ctx, warnKey, strconv.Itoa(currentWarns))
	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(Sprintf(" Removed 1 warning from @%s. Remaining warnings: %d.", username, currentWarns), []types.JID{resolvedJID})
}

func handleWarns(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	chatKey := ctx.Chat.String()
	limitKey := Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	if !targetJID.IsEmpty() {
		userKey := targetJID.ToNonAD().User
		warnKey := Sprintf("warn_count:%s:%s", chatKey, userKey)
		rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
		currentWarns, _ := strconv.Atoi(rawCount)

		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(Sprintf("Participant @%s has %d/%d warnings.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	p := ctx.GetPrefix()
	return ctx.Replyf("Max Warning Threshold for this chat: %d warnings.\nUsage: %swarns @user to check specific participant warnings.", maxLimit, p)
}

func handleSetWarn(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %ssetwarn <count> (e.g. %ssetwarn 3)", p, p)
	}

	num, err := strconv.Atoi(args[0])
	if err != nil || num < 1 || num > 20 {
		return ctx.Reply("Please specify a valid warning limit count between 1 and 20.")
	}

	chatKey := ctx.Chat.String()
	limitKey := Sprintf("warn_limit:%s", chatKey)
	if err := s.PutSetting(ctx.Ctx, limitKey, strconv.Itoa(num)); err != nil {
		return ctx.Reply("Failed to update warning threshold.")
	}

	return ctx.Replyf("Warning threshold for this chat set to %d warnings.", num)
}

func sendWarnMenu(ctx *Context, s *StoreWrapper) error {
	chatKey := ctx.Chat.String()
	limitKey := Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	bodyText := NewText().
		Header("WARN CONFIGURATION").
		Fieldf("Max Warn Threshold", "%d Warnings", maxLimit).
		Blank().
		Line("Choose an option below to set threshold or view customization guide.").
		Trimmed()

	options := []string{
		"Set Limit (3)",
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendWarnCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("WARN CUSTOMIZATION GUIDE").
		Blank().
		Section("Available Commands:").
		Bulletf("Issue Warning     : `%swarn @user [reason]`", p).
		Bulletf("Remove Warning    : `%sunwarn @user`", p).
		Bulletf("Check Warnings    : `%swarns [@user]`", p).
		Bulletf("Set Max Threshold : `%ssetwarn <number>`", p).
		Blank().
		Section("Automated Enforcement Rules:").
		Numbered(1, "Reaching max threshold in Group -> Blocks user & Kicks from group (requires bot admin).").
		Numbered(2, "Reaching max threshold in Private Chat -> Blocks messaging.").
		Numbered(3, "Bot Owner & Sudoers are immune to warnings.").
		Numbered(4, "Group Admins cannot be kicked unless bot is group owner.").
		Blank().
		Section("Examples:").
		Numberedf(1, "`%swarn @user Spamming links in group`", p).
		Numberedf(2, "`%sunwarn @user`", p).
		Numberedf(3, "`%ssetwarn 3`", p).
		Reply()
}

func extractWarnTarget(ctx *Context, args []string) types.JID {
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		return NormalizeUserJID(ctx.Ctx, ctx.Client, quotedSender)
	}
	if ci := ctx.GetContextInfo(); ci != nil && len(ci.GetMentionedJID()) > 0 {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := utils.ParseUserJID(m); err == nil && !parsed.IsEmpty() {
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
		if parsed, err := utils.ParseUserJID(arg); err == nil && !parsed.IsEmpty() {
			return NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
		}
	}
	return types.EmptyJID
}

func isJIDOwnerOrSudo(ctx *Context, target types.JID) bool {
	return utils.IsSudoRaw(ctx.Ctx, ctx.Client, target)
}

func isParticipantAdmin(info *types.GroupInfo, target types.JID) bool {
	if info == nil {
		return false
	}
	targetUser := target.ToNonAD().User
	for _, p := range info.Participants {
		if (p.JID.User == targetUser) && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

func isBotAdmin(ctx *Context, info *types.GroupInfo) bool {
	if info == nil || ctx.Client.Store.ID == nil {
		return false
	}
	botUser := ctx.Client.Store.ID.ToNonAD().User
	botLIDUser := ""
	if !ctx.Client.Store.LID.IsEmpty() {
		botLIDUser = ctx.Client.Store.LID.ToNonAD().User
	}

	for _, p := range info.Participants {
		if (p.JID.User == botUser || (botLIDUser != "" && p.JID.User == botLIDUser)) && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

func isBotGroupOwner(ctx *Context, info *types.GroupInfo) bool {
	if info == nil || info.OwnerJID.IsEmpty() || ctx.Client.Store.ID == nil {
		return false
	}
	return ctx.IsSameUser(info.OwnerJID, *ctx.Client.Store.ID)
}

func handleWelcome(ctx *Context) error {
	return handleGroupGreetingConfig(ctx, "welcome")
}

func handleGoodbye(ctx *Context) error {
	return handleGroupGreetingConfig(ctx, "goodbye")
}

func handleGroupGreetingConfig(ctx *Context, kind string) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
		return applyToggle(ctx, s, statusKey, "on", label+" message")

	case "off", "disable", "deactivate":
		return applyToggle(ctx, s, statusKey, "off", label+" message")

	case "toggle":
		return applyToggle(ctx, s, statusKey, "toggle", label+" message")

	case "customize", "custom", "help":
		return sendGreetingCustomizeGuide(ctx, kind)

	case "tag":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, tagKey)
			return ctx.Reply(label + " participant tag setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "enable" && mode != "activate" && mode != "off" && mode != "false" && mode != "disable" && mode != "deactivate" && mode != "toggle" {
			return ctx.Reply("Usage: " + ctx.GetPrefix() + kind + " tag [on|off|toggle]")
		}
		return applyToggle(ctx, s, tagKey, mode, label+" participant tagging")

	case "desc":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, descKey)
			return ctx.Reply(label + " group description setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "enable" && mode != "activate" && mode != "off" && mode != "false" && mode != "disable" && mode != "deactivate" && mode != "toggle" {
			return ctx.Reply("Usage: " + ctx.GetPrefix() + kind + " desc [on|off|toggle]")
		}
		return applyToggle(ctx, s, descKey, mode, label+" group description inclusion")

	case "msg", "message", "text":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, msgKey)
			if curr == "" {
				curr = "none (using default template)"
			}
			return ctx.Reply(label + " custom message template: " + curr)
		}
		text := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if err := s.PutSetting(ctx.Ctx, msgKey, text); err != nil {
			return ctx.Reply("Failed to update message template: " + err.Error())
		}
		return ctx.Reply(label + " custom message template updated.")

	case "media", "video":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, mediaKey)
			if curr == "" {
				curr = "none"
			}
			return ctx.Reply(label + " media URL: " + curr)
		}
		url := strings.TrimSpace(args[1])
		if url == "none" || url == "clear" {
			if err := s.PutSetting(ctx.Ctx, mediaKey, ""); err != nil {
				return ctx.Reply("Failed to clear media: " + err.Error())
			}
			return ctx.Reply(label + " media cleared.")
		}
		if err := s.PutSetting(ctx.Ctx, mediaKey, url); err != nil {
			return ctx.Reply("Failed to update media: " + err.Error())
		}
		return ctx.Reply(label + " media URL saved.")

	default:
		return ctx.Reply("Usage: " + ctx.GetPrefix() + kind + " [activate|deactivate|toggle|customize|tag|desc|msg|media]")
	}
}

func applyToggle(ctx *Context, s *StoreWrapper, key, mode, label string) error {
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
		return ctx.Reply("Failed to update setting: " + err.Error())
	}

	verb := "enabled"
	if next == "off" {
		verb = "disabled"
	}
	return ctx.Reply(label + " " + verb + ".")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func sendGreetingMenu(ctx *Context, s *StoreWrapper, kind string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, kind+"_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	bodyText := NewText().
		Header(strings.ToUpper(kind)+" CONFIGURATION").
		Field("Group", groupName).
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Choose an option below to change status or view customization options.").
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

func sendGreetingCustomizeGuide(ctx *Context, kind string) error {
	p := ctx.GetPrefix()
	kUpper := strings.ToUpper(kind)

	tb := ctx.Text().
		Header(Sprintf("%s CUSTOMIZATION GUIDE", kUpper)).
		Blank().
		Section("Available Customizations:").
		Bulletf("Custom Message : `%s%s msg <your message text>`", p, kind).
		Bulletf("Participant Tagging : `%s%s tag on | off`", p, kind).
		Bulletf("Group Description   : `%s%s desc on | off`", p, kind).
		Bulletf("Greeting Media URL  : `%s%s media <url | clear>`", p, kind).
		Blank().
		Section("Available GroupInfo Placeholders:").
		Bullet("`{user}`       : Participant mention tag (@username)").
		Bullet("`{user_id}`    : Participant's phone number / user ID").
		Bullet("`{user_jid}`   : Participant's full WhatsApp JID").
		Bullet("`{group}`      : Group Name").
		Bullet("`{group_jid}`  : Group JID").
		Bullet("`{desc}`       : Group Description / Topic").
		Bullet("`{members}`    : Total group participant count").
		Bullet("`{admins}`     : Total group admin count").
		Bullet("`{owner}`      : Mentions group creator / owner").
		Bullet("`{created_at}` : Group creation date").
		Blank().
		Section("Examples:")

	if kind == "welcome" {
		tb.Numberedf(1, "`%swelcome msg Welcome {user} to {group}! We now have {members} members (Admins: {admins}). Created by {owner} on {created_at}.`", p).
			Numberedf(2, "`%swelcome tag on`", p).
			Numberedf(3, "`%swelcome media https://example.com/welcome.mp4`", p)
	} else {
		tb.Numberedf(1, "`%sgoodbye msg Goodbye {user}! {group} now has {members} members remaining.`", p).
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
		Logger.Debug("ClearPendingCaptchasForGroup: cleared pending captchas", "group", chatStr, "count", cleared)
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
			Logger.Debug("RegisterPendingCaptcha: timeout triggered", "group", groupJID.String(), "user", userJID.String(), "code", code)
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
	Logger.Debug("RegisterPendingCaptcha: registered new pending captcha", "group", groupJID.String(), "user", userJID.String(), "username", username, "code", code, "duration", duration)
}

// SetPendingCaptchaMsgID stores the verification message ID for revocation if cancelled.
func SetPendingCaptchaMsgID(groupJID, userJID types.JID, msgID types.MessageID) {
	pendingCaptchaMu.Lock()
	defer pendingCaptchaMu.Unlock()

	key := captchaKey(groupJID, userJID)
	if p, ok := pendingCaptchas[key]; ok {
		p.MsgID = msgID
		Logger.Debug("SetPendingCaptchaMsgID: updated msgID for pending captcha", "group", groupJID.String(), "user", userJID.String(), "msgID", msgID)
		return
	}

	chatStr := groupJID.ToNonAD().String()
	userNonAD := userJID.ToNonAD()
	for _, p := range pendingCaptchas {
		if p.GroupJID.ToNonAD().String() == chatStr {
			if p.UserJID.ToNonAD() == userNonAD || p.ResolvedJID.ToNonAD() == userNonAD || p.UserJID.User == userNonAD.User || p.ResolvedJID.User == userNonAD.User {
				p.MsgID = msgID
				Logger.Debug("SetPendingCaptchaMsgID: updated msgID via fallback matching", "group", groupJID.String(), "user", userJID.String(), "msgID", msgID)
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
		Logger.Debug("RemovePendingCaptcha: removed pending captcha", "group", groupJID.String(), "user", userJID.String())
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
				Logger.Debug("RemovePendingCaptcha: removed pending captcha via fallback matching", "group", groupJID.String(), "user", userJID.String(), "key", k)
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
		return Sprintf("%d mins", mins)
	}
	return Sprintf("%d seconds", sec)
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
		if client != nil && utils.IsSameUserRaw(context.Background(), client, p.UserJID, sender) {
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
		return false
	}

	Logger.Debug("HandlePendingCaptchaReply: found pending captcha for participant",
		"group", chat.String(),
		"sender", sender.String(),
		"code", pending.Code,
		"createdAt", pending.CreatedAt,
		"expiresAt", pending.ExpiresAt,
	)

	// Ignore messages sent before the pending captcha was created
	if !evt.Info.Timestamp.IsZero() && evt.Info.Timestamp.Before(pending.CreatedAt.Add(-2*time.Second)) {
		Logger.Debug("HandlePendingCaptchaReply: ignoring message sent before captcha creation",
			"group", chat.String(),
			"sender", sender.String(),
			"msgTimestamp", evt.Info.Timestamp,
			"captchaCreatedAt", pending.CreatedAt,
		)
		return false
	}

	text := strings.TrimSpace(utils.ExtractMessageText(evt))
	if text == "" {
		return false
	}

	cleanText := strings.TrimSpace(strings.Trim(text, "*_`~#"))

	Logger.Debug("HandlePendingCaptchaReply: verifying message text against captcha code",
		"group", chat.String(),
		"sender", sender.String(),
		"input", cleanText,
		"expected", pending.Code,
	)

	// Check if user submitted the correct 4-digit code
	if cleanText == pending.Code {
		Logger.Debug("HandlePendingCaptchaReply: correct code submitted, verification successful", "group", chat.String(), "sender", sender.String())
		if pending.MsgID != "" && client != nil {
			_, _ = client.SendMessage(ctx, chat, client.BuildRevoke(chat, types.EmptyJID, pending.MsgID))
		}
		RemovePendingCaptcha(chat, pending.UserJID)
		RemovePendingCaptcha(chat, sender)
		resolvedJID, username := utils.ResolveMentionRaw(ctx, client, sender)

		pctx := &utils.PluginContext{Ctx: ctx, Client: client, Chat: chat, Sender: sender}
		tb := utils.NewTextWithContext(pctx)
		tb.Header("Verification Successful")
		tb.Linef("@%s has successfully confirmed their participant status. Welcome to the group!", username)
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
		Logger.Debug("HandlePendingCaptchaReply: incorrect numeric attempt", "group", chat.String(), "sender", sender.String(), "attempt", cleanText, "expected", pending.Code)
		resolvedJID, username := utils.ResolveMentionRaw(ctx, client, sender)

		pctx := &utils.PluginContext{Ctx: ctx, Client: client, Chat: chat, Sender: sender}
		tb := utils.NewTextWithContext(pctx)
		tb.Header("Incorrect Code")
		tb.Linef("Please watch the verification video carefully and reply with the correct 4-digit code, @%s.", username)
		tb.Mentions(resolvedJID)
		_ = tb.Send()
		return true
	}

	return false
}

func handleCaptcha(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}

	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Replyf("Failed to get group info: %v", err)
	}

	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins can configure captcha verification.")
	}

	// This plugin should only work if this bot is an admin
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("Captcha verification requires this bot to be a group admin in order to remove unverified members.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
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
			return ctx.Reply("Failed to enable captcha: " + err.Error())
		}
		tb := ctx.Rook().NewText()
		tb.Header("CAPTCHA ACTIVATED")
		tb.Field("Group", groupName)
		tb.Field("Status", "ACTIVE (ON)")
		tb.Blank()
		tb.Line("Captcha verification is now active. Newly joined participants are required to complete verification within the specified time limit or they will be kicked out.")

		options := []string{"Deactivate", "Set Timeout"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "off", "disable", "deactivate", "stop":
		if err := s.PutSetting(ctx.Ctx, statusKey, "off"); err != nil {
			return ctx.Reply("Failed to disable captcha: " + err.Error())
		}
		ClearPendingCaptchasForGroup(ctx.Chat)
		tb := ctx.Rook().NewText()
		tb.Header("CAPTCHA DEACTIVATED")
		tb.Field("Group", groupName)
		tb.Field("Status", "DISABLED (OFF)")
		tb.Blank()
		tb.Line("Captcha verification has been turned off for this group.")

		options := []string{"Activate"}
		return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		next := "on"
		if curr == "on" {
			next = "off"
		}
		if err := s.PutSetting(ctx.Ctx, statusKey, next); err != nil {
			return ctx.Reply("Failed to toggle captcha: " + err.Error())
		}
		if next == "off" {
			ClearPendingCaptchasForGroup(ctx.Chat)
		}
		verb := "activated"
		if next == "off" {
			verb = "deactivated"
		}
		tb := ctx.Rook().NewText()
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
			tb := ctx.Rook().NewText()
			tb.Header("CAPTCHA TIMEOUT")
			tb.Field("Group", groupName)
			tb.Field("Timeout", formatCaptchaTimeout(secVal))
			tb.Blank()
			tb.Linef("To change verification time limit, use: %scaptcha time <seconds>", p)
			tb.Linef("Example: %scaptcha time 120 (2 mins)", p)

			options := []string{"1 Min", "2 Mins", "3 Mins"}
			return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})
		}
		timeArg := strings.Join(args[1:], " ")
		sec, okTime := parseCaptchaTime(timeArg)
		if !okTime {
			return ctx.Reply("Invalid timeout duration. Please specify a time between 10 and 600 seconds (e.g. `60`, `120`, `2 mins`).")
		}
		if err := s.PutSetting(ctx.Ctx, timeKey, strconv.Itoa(sec)); err != nil {
			return ctx.Reply("Failed to update timeout: " + err.Error())
		}
		tb := ctx.Rook().NewText()
		tb.Header("CAPTCHA TIMEOUT SET")
		tb.Field("Group", groupName)
		tb.Field("Timeout", formatCaptchaTimeout(sec))
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
			tb := ctx.Rook().NewText()
			tb.Header("CAPTCHA TIMEOUT SET")
			tb.Field("Group", groupName)
			tb.Field("Timeout", formatCaptchaTimeout(sec))
			tb.Blank()
			tb.Linef("Newly joined members will have %s to verify before being removed.", formatCaptchaTimeout(sec))

			options := []string{"Activate", "Deactivate"}
			return sendPollReplyWithMentions(ctx, tb.Trimmed(), options, []types.JID{ctx.Sender})
		}
		return ctx.Reply("Usage: " + p + "captcha [activate|deactivate|toggle|time <sec>|help]")
	}
}

func sendCaptchaMenu(ctx *Context, s *StoreWrapper, groupName string) error {
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

	tb := ctx.Rook().NewText()
	tb.Header("CAPTCHA CONFIGURATION")
	tb.Field("Group", groupName)
	tb.Field("Status", strings.ToUpper(status))
	tb.Field("Timeout", formatCaptchaTimeout(secVal))
	tb.Blank()
	tb.Line("Description:")
	tb.Line("Requires a newly joined participant to complete a verification when they join the group within a certain amount of time else it kicks them out for failing to verify their participant status.")
	tb.Blank()
	tb.Line("Note: This plugin only works if this bot is an admin.")

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

func sendCaptchaGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	tb := ctx.Rook().NewText()
	tb.Header("CAPTCHA VERIFICATION GUIDE")
	tb.Line("Description:")
	tb.Line("Requires a newly joined participant to complete a verification when they join the group within a certain amount of time else it kicks them out for failing to verify their participant status.")
	tb.Blank()
	tb.Section("How It Works:")
	tb.Numbered(1, "When a new participant joins, a 4-digit code animation video is generated using the captcha package.")
	tb.Numbered(2, "The bot sends the video and an interactive button prompt tagging the newly joined participant.")
	tb.Numbered(3, "The participant is expected to input the 4-digit code to confirm their status within the time limit.")
	tb.Numbered(4, "If the code matches, their participant status is confirmed.")
	tb.Numbered(5, "If they fail to verify within the time limit, they will be kicked out.")
	tb.Blank()
	tb.Section("Commands:")
	tb.Bulletf("`%scaptcha activate`     : Enable captcha verification", p)
	tb.Bulletf("`%scaptcha deactivate`   : Disable captcha verification", p)
	tb.Bulletf("`%scaptcha toggle`       : Toggle captcha on/off", p)
	tb.Bulletf("`%scaptcha time <sec>`   : Set verification timeout (10-600s or e.g. 2 mins)", p)
	tb.Bulletf("`%scaptcha help`         : Show this guide", p)
	tb.Blank()
	tb.Section("Requirements:")
	tb.Bullet("This plugin only works if this bot is an admin.")

	return ctx.Reply(tb.Trimmed())
}
