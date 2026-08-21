package plugins

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "business",
		Alias:       "biz",
		Description: "View WhatsApp Business profile details",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessProfile,
	})
	Register(&Command{
		Name:        "bizhours",
		Alias:       "hours",
		Description: "View operating schedule and timezone of a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessHours,
	})
	Register(&Command{
		Name:        "isbiz",
		Alias:       "checkbiz",
		Description: "Check if a contact or user is a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleIsBusiness,
	})
	Register(&Command{
		Name:        "bizcard",
		Alias:       "vcardbiz",
		Description: "Display a digital Business Card summary for a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessCard,
	})
	Register(&Command{
		Name:        "bizlink",
		Alias:       "resolvelink",
		Description: "Resolve a WhatsApp Business short link code (wa.me/message/<code>)",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessLink,
	})
}

func handleBusinessProfile(ctx *Context) error {
	rawTarget, queryJID, err := cliutils.ResolveBusinessTarget(ctx.Ctx, ctx.Client, ctx.GetTargets(), ctx.Chat, ctx.GetPrefix(), "business")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	profile, errFetch := cliutils.FetchBusinessProfileAndValidate(ctx.Ctx, ctx.Client, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		_ = ctx.ReplyWithMentions(fmt.Sprintf("User @%s is not an actual WhatsApp Business account or profile is unavailable.", rawTarget.User), []types.JID{rawTarget})
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*WhatsApp Business Profile*\n\n")
	fmt.Fprintf(&sb, "*Target:* @%s\n", rawTarget.User)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "*Categories:* %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "*Address:* %s\n", profile.Address)
	}
	if len(profile.BusinessHours) > 0 {
		fmt.Fprintf(&sb, "*Operating Hours:* %d schedule entries\n", len(profile.BusinessHours))
		if profile.BusinessHoursTimeZone != "" {
			fmt.Fprintf(&sb, "*TimeZone:* %s\n", profile.BusinessHoursTimeZone)
		}
	}

	var pfpData []byte
	if ctx.Client != nil {
		if picInfo, errPic := ctx.Client.GetProfilePictureInfo(ctx.Ctx, queryJID, &whatsmeow.GetProfilePictureParams{}); errPic == nil && picInfo != nil && picInfo.URL != "" {
			pfpData, _ = utils.FetchURLBytes(ctx.Ctx, picInfo.URL)
		}
		if len(pfpData) == 0 && rawTarget != queryJID {
			if picInfo, errPic := ctx.Client.GetProfilePictureInfo(ctx.Ctx, rawTarget, &whatsmeow.GetProfilePictureParams{}); errPic == nil && picInfo != nil && picInfo.URL != "" {
				pfpData, _ = utils.FetchURLBytes(ctx.Ctx, picInfo.URL)
			}
		}
	}
	if len(pfpData) > 0 {
		return ctx.ReplyWithImageWithMentions(pfpData, "image/jpeg", sb.String(), []types.JID{rawTarget})
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessHours(ctx *Context) error {
	rawTarget, queryJID, err := cliutils.ResolveBusinessTarget(ctx.Ctx, ctx.Client, ctx.GetTargets(), ctx.Chat, ctx.GetPrefix(), "bizhours")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	profile, errFetch := cliutils.FetchBusinessProfileAndValidate(ctx.Ctx, ctx.Client, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		_ = ctx.ReplyWithMentions(fmt.Sprintf("User @%s is not an actual WhatsApp Business account or profile is unavailable.", rawTarget.User), []types.JID{rawTarget})
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Operating Hours*\n\n")
	fmt.Fprintf(&sb, "*Business:* @%s\n", rawTarget.User)
	if profile.BusinessHoursTimeZone != "" {
		fmt.Fprintf(&sb, "*Timezone:* %s\n", profile.BusinessHoursTimeZone)
	}
	fmt.Fprintf(&sb, "\n")

	if len(profile.BusinessHours) == 0 {
		fmt.Fprintf(&sb, "No specific business hours configured for this account.")
	} else {
		for _, bh := range profile.BusinessHours {
			day := bh.DayOfWeek
			if day == "" {
				day = "Schedule"
			}
			if bh.OpenTime != "" && bh.CloseTime != "" {
				fmt.Fprintf(&sb, "• *%s*: %s - %s (%s)\n", day, bh.OpenTime, bh.CloseTime, bh.Mode)
			} else {
				fmt.Fprintf(&sb, "• *%s*: %s\n", day, bh.Mode)
			}
		}
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleIsBusiness(ctx *Context) error {
	rawTarget, queryJID, err := cliutils.ResolveBusinessTarget(ctx.Ctx, ctx.Client, ctx.GetTargets(), ctx.Chat, ctx.GetPrefix(), "isbiz")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	profile, errProfile := ctx.Client.GetBusinessProfile(ctx.Ctx, queryJID)
	if (errProfile != nil || profile == nil) && rawTarget != queryJID {
		profile, errProfile = ctx.Client.GetBusinessProfile(ctx.Ctx, rawTarget)
	}

	if errProfile != nil || profile == nil {
		return ctx.ReplyWithMentions(fmt.Sprintf("❌ @%s is NOT an actual WhatsApp Business account.", rawTarget.User), []types.JID{rawTarget})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ @%s IS an actual WhatsApp Business account.\n", rawTarget.User)
	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "*Category:* %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessCard(ctx *Context) error {
	rawTarget, queryJID, err := cliutils.ResolveBusinessTarget(ctx.Ctx, ctx.Client, ctx.GetTargets(), ctx.Chat, ctx.GetPrefix(), "bizcard")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	profile, errFetch := cliutils.FetchBusinessProfileAndValidate(ctx.Ctx, ctx.Client, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		_ = ctx.ReplyWithMentions(fmt.Sprintf("User @%s is not an actual WhatsApp Business account or profile is unavailable.", rawTarget.User), []types.JID{rawTarget})
		return nil
	}

	pushName := rawTarget.User
	if contact, cErr := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, queryJID); cErr == nil && contact.Found {
		if contact.BusinessName != "" {
			pushName = contact.BusinessName
		} else if contact.PushName != "" {
			pushName = contact.PushName
		} else if contact.FullName != "" {
			pushName = contact.FullName
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 BUSINESS CARD 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name     : %s\n", pushName)
	fmt.Fprintf(&sb, "│ Account  : @%s\n", rawTarget.User)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "│ Category : %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "│ Email    : %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "│ Address  : %s\n", profile.Address)
	}
	if len(profile.BusinessHours) > 0 {
		fmt.Fprintf(&sb, "│ Hours    : %d schedules (%s)\n", len(profile.BusinessHours), profile.BusinessHoursTimeZone)
	}
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━━━━━━")

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessLink(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sbizlink <code>\n- %sbizlink https://wa.me/message/<code>", p, p))
	}

	rawArg := strings.TrimSpace(ctx.Args[0])
	code := rawArg

	if strings.Contains(rawArg, "wa.me/message/") {
		parts := strings.Split(rawArg, "wa.me/message/")
		if len(parts) > 1 {
			code = parts[1]
		}
	} else if strings.Contains(rawArg, "whatsapp.com/") {
		parts := strings.Split(rawArg, "/")
		code = parts[len(parts)-1]
	}
	code = strings.TrimSpace(strings.Split(code, "?")[0])

	if code == "" {
		return ctx.Reply("Invalid business short link code.")
	}

	target, err := ctx.Client.ResolveBusinessMessageLink(ctx.Ctx, code)
	if err != nil || target == nil {
		return ctx.Reply(fmt.Sprintf("Could not resolve business link code %q: %v", code, err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Short Link Target*\n\n")
	if target.VerifiedName != "" {
		fmt.Fprintf(&sb, "*Verified Name:* %s\n", target.VerifiedName)
	}
	if target.PushName != "" {
		fmt.Fprintf(&sb, "*Push Name:* %s\n", target.PushName)
	}
	if !target.JID.IsEmpty() {
		fmt.Fprintf(&sb, "*Target Account:* @%s\n", target.JID.User)
	}
	if target.VerifiedLevel != "" {
		fmt.Fprintf(&sb, "*Verification Level:* %s\n", target.VerifiedLevel)
	}
	if target.Message != "" {
		fmt.Fprintf(&sb, "*Pre-filled Message:* %s\n", target.Message)
	}

	if !target.JID.IsEmpty() {
		return ctx.ReplyWithMentions(sb.String(), []types.JID{target.JID})
	}
	return ctx.Reply(sb.String())
}
