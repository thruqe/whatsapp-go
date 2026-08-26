package plugins

import (
	"strings"

	"go.mau.fi/whatsmeow"
	cliutils "whatsrook/cmd/utils"
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
		return ctx.Text().
			Linef("User @%s is not an actual WhatsApp Business account or profile is unavailable.", rawTarget.User).
			Mentions(rawTarget).
			Reply()
	}

	tb := ctx.Text().
		Header("WhatsApp Business Profile").
		Field("Target", "@"+rawTarget.User, rawTarget)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		tb.Field("Categories", strings.Join(cats, ", "))
	}
	tb.FieldIf(profile.Description != "", "Bio", strings.TrimSpace(profile.Description)).
		FieldIf(profile.Email != "", "Email", profile.Email).
		FieldIf(profile.Address != "", "Address", profile.Address).
		FieldIf(len(profile.Websites) > 0, "Websites", strings.Join(profile.Websites, ", "))

	if len(profile.BusinessHours) > 0 {
		tb.Fieldf("Operating Hours", "%d schedule entries", len(profile.BusinessHours)).
			FieldIf(profile.BusinessHoursTimeZone != "", "TimeZone", profile.BusinessHoursTimeZone)
		for _, bh := range profile.BusinessHours {
			day := bh.DayOfWeek
			if day == "" {
				day = "Schedule"
			}
			if bh.OpenTime != "" && bh.CloseTime != "" {
				tb.Bulletf("%s: %s - %s (%s)", day, bh.OpenTime, bh.CloseTime, bh.Mode)
			} else {
				tb.Bulletf("%s: %s", day, bh.Mode)
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
		return tb.ReplyWithImage(pfpData, "image/jpeg")
	}

	return tb.Reply()
}

func handleBusinessLink(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sbizlink <code>\n- %sbizlink https://wa.me/message/<code>", p, p)
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
		return ctx.Replyf("Could not resolve business link code %q: %v", code, err)
	}

	return ctx.Text().
		Header("Business Short Link Target").
		FieldIf(target.VerifiedName != "", "Verified Name", target.VerifiedName).
		FieldIf(target.PushName != "", "Push Name", target.PushName).
		FieldIf(!target.JID.IsEmpty(), "Target Account", "@"+target.JID.User, target.JID).
		FieldIf(target.VerifiedLevel != "", "Verification Level", target.VerifiedLevel).
		FieldIf(target.Message != "", "Pre-filled Message", target.Message).
		Reply()
}
