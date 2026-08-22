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
	fmt.Fprintf(&sb, "WhatsApp Business Profile\n\n")
	fmt.Fprintf(&sb, "Target: @%s\n", rawTarget.User)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "Categories: %s\n", strings.Join(cats, ", "))
	}
	if profile.Description != "" {
		fmt.Fprintf(&sb, "Bio: %s\n", strings.TrimSpace(profile.Description))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "Email: %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "Address: %s\n", profile.Address)
	}
	if len(profile.Websites) > 0 {
		fmt.Fprintf(&sb, "Websites: %s\n", strings.Join(profile.Websites, ", "))
	}
	if len(profile.BusinessHours) > 0 {
		fmt.Fprintf(&sb, "Operating Hours: %d schedule entries\n", len(profile.BusinessHours))
		if profile.BusinessHoursTimeZone != "" {
			fmt.Fprintf(&sb, "TimeZone: %s\n", profile.BusinessHoursTimeZone)
		}
		for _, bh := range profile.BusinessHours {
			day := bh.DayOfWeek
			if day == "" {
				day = "Schedule"
			}
			if bh.OpenTime != "" && bh.CloseTime != "" {
				fmt.Fprintf(&sb, "• %s: %s - %s (%s)\n", day, bh.OpenTime, bh.CloseTime, bh.Mode)
			} else {
				fmt.Fprintf(&sb, "• %s: %s\n", day, bh.Mode)
			}
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
	fmt.Fprintf(&sb, "Business Short Link Target\n\n")
	if target.VerifiedName != "" {
		fmt.Fprintf(&sb, "Verified Name: %s\n", target.VerifiedName)
	}
	if target.PushName != "" {
		fmt.Fprintf(&sb, "Push Name: %s\n", target.PushName)
	}
	if !target.JID.IsEmpty() {
		fmt.Fprintf(&sb, "Target Account: @%s\n", target.JID.User)
	}
	if target.VerifiedLevel != "" {
		fmt.Fprintf(&sb, "Verification Level: %s\n", target.VerifiedLevel)
	}
	if target.Message != "" {
		fmt.Fprintf(&sb, "Pre-filled Message: %s\n", target.Message)
	}

	if !target.JID.IsEmpty() {
		return ctx.ReplyWithMentions(sb.String(), []types.JID{target.JID})
	}
	return ctx.Reply(sb.String())
}
