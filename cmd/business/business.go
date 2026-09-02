package business

import (
	"strings"

	"whatsrook/cmd/dispatch"
	"whatsrook/httpx"

	"go.mau.fi/whatsmeow"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "business",
		Alias:       "biz",
		Description: "View business profile details",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessProfile,
	})
}

func handleBusinessProfile(ctx *dispatch.Context) error {
	rawTarget, queryJID, err := ResolveBusinessTarget(ctx.Ctx, ctx.Client, ctx.GetTargets(), ctx.Chat, ctx.GetPrefix(), "business")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	profile, errFetch := FetchBusinessProfileAndValidate(ctx.Ctx, ctx.Client, rawTarget, queryJID)
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
			pfpData, _ = httpx.FetchBytes(ctx.Ctx, picInfo.URL)
		}
		if len(pfpData) == 0 && rawTarget != queryJID {
			if picInfo, errPic := ctx.Client.GetProfilePictureInfo(ctx.Ctx, rawTarget, &whatsmeow.GetProfilePictureParams{}); errPic == nil && picInfo != nil && picInfo.URL != "" {
				pfpData, _ = httpx.FetchBytes(ctx.Ctx, picInfo.URL)
			}
		}
	}
	if len(pfpData) > 0 {
		return tb.ReplyWithImage(pfpData, "image/jpeg")
	}

	return tb.Reply()
}
