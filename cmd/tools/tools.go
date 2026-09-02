// tools package provides utilities for contact vCard syncing, user info inspection,
// timezone lookups, and decorative Unicode font transformations.
package tools

import (
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"

	"whatsrook/cmd/dispatch"
	"whatsrook/httpx"
	Logger "whatsrook/logger"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "contact",
		Alias:       "savecontact",
		Description: "Save a user to your WhatsApp contact list via AppState sync and send a native vCard",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleSaveContact,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "whois",
		Alias:       "userinfo",
		Description: "Display detailed profile information and profile photo of a WhatsApp user",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleUserInfo,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "timezone",
		Alias:       "tz",
		Description: "Search and configure your active timezone offset",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleTimezone,
	})
	dispatch.Register(&dispatch.Command{
		Name:        "font",
		Alias:       "fonts,fancy",
		Description: "Convert text into styled Unicode decorative fonts",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFont,
	})
}

func handleSaveContact(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	isGroup := ctx.Chat.Server == types.GroupServer

	targets := ctx.GetTargets()
	var targetJID types.JID
	if len(targets) > 0 {
		targetJID = targets[0]
	} else if !isGroup {
		targetJID = ctx.Chat
	}

	args := strings.Fields(ctx.RawArgs)
	var nameParts []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "@") || strings.HasPrefix(arg, "+") {
			continue
		}
		if targetJID.User != "" && strings.Contains(arg, targetJID.User) {
			continue
		}
		nameParts = append(nameParts, arg)
	}

	fullName := strings.Join(nameParts, " ")

	if targetJID.IsEmpty() {
		return ctx.Replyf("Please specify a user to save. Usage:\n- %ssavecontact <Name> @user\n- Reply to a message with %ssavecontact <Name>", p, p)
	}

	if fullName == "" {
		if ctx.Evt != nil && ctx.Evt.Info.Sender.User == targetJID.User && ctx.Evt.Info.PushName != "" {
			fullName = ctx.Evt.Info.PushName
		}

		if fullName == "" && ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
			if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, targetJID.ToNonAD()); err == nil && contact.Found {
				if contact.PushName != "" {
					fullName = contact.PushName
				} else if contact.BusinessName != "" {
					fullName = contact.BusinessName
				} else if contact.FullName != "" {
					fullName = contact.FullName
				}
			}
		}

		if fullName == "" {
			if isGroup {
				return ctx.Replyf("Could not auto-detect contact pushname. Please specify a name:\n- %ssavecontact <Name> @user", p)
			}
			return ctx.Replyf("Could not auto-detect contact pushname. Please specify a name:\n- %ssavecontact <Name>", p)
		}
	}

	firstName := fullName
	if len(nameParts) > 0 {
		firstName = nameParts[0]
	} else if fields := strings.Fields(fullName); len(fields) > 0 {
		firstName = fields[0]
	}

	Logger.Debug("handleSaveContact: processing contact save", "target", targetJID.String(), "fullName", fullName, "firstName", firstName)

	var pnStr string
	var lidStr string
	var pnJID types.JID

	if targetJID.Server == types.HiddenUserServer {
		lidStr = targetJID.String()
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, targetJID); err == nil && !pn.IsEmpty() {
				pnJID = pn
				pnStr = pn.ToNonAD().String()
			}
		}
	} else {
		pnStr = targetJID.ToNonAD().String()
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if lid, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, targetJID); err == nil && !lid.IsEmpty() {
				lidStr = lid.String()
			}
		}
	}

	contactAction := &waSyncAction.ContactAction{
		FullName:                 new(string),
		FirstName:                new(string),
		SaveOnPrimaryAddressbook: new(bool),
	}
	*contactAction.FullName = fullName
	*contactAction.FirstName = firstName
	*contactAction.SaveOnPrimaryAddressbook = true
	if pnStr != "" {
		contactAction.PnJID = new(string)
		*contactAction.PnJID = pnStr
	}
	if lidStr != "" {
		contactAction.LidJID = new(string)
		*contactAction.LidJID = lidStr
	}

	indexJID := pnStr
	if indexJID == "" {
		indexJID = lidStr
	}

	patch := appstate.PatchInfo{
		Type: appstate.WAPatchCriticalUnblockLow,
		Mutations: []appstate.MutationInfo{
			{
				Index:   []string{appstate.IndexContact, indexJID},
				Version: 2,
				Value: &waSyncAction.SyncActionValue{
					ContactAction: contactAction,
				},
			},
		},
	}

	Logger.Debug("handleSaveContact: sending AppState patch", "type", patch.Type, "indexJID", indexJID, "target", targetJID.String())
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		Logger.Error("handleSaveContact: failed to send AppState patch", "err", err, "target", targetJID.String())
	} else {
		Logger.Debug("handleSaveContact: AppState patch sent successfully", "target", targetJID.String())
	}

	if ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		if err := ctx.Client.Store.Contacts.PutContactName(ctx.Ctx, targetJID.ToNonAD(), firstName, fullName); err != nil {
			Logger.Error("handleSaveContact: failed to update local contact store", "err", err, "target", targetJID.String())
		} else {
			Logger.Debug("handleSaveContact: updated local contact store cache", "target", targetJID.String())
		}
		if !pnJID.IsEmpty() {
			_ = ctx.Client.Store.Contacts.PutContactName(ctx.Ctx, pnJID.ToNonAD(), firstName, fullName)
		}
	}

	vcard := dispatch.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:%s;%s;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", firstName, fullName, fullName, targetJID.User, targetJID.User)
	vcardMsg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: &fullName,
			Vcard:       &vcard,
		},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, vcardMsg)
	if err != nil {
		Logger.Error("handleSaveContact: failed to send vCard message", "err", err, "chat", ctx.Chat.String())
	} else {
		Logger.Debug("handleSaveContact: sent native vCard contact message", "chat", ctx.Chat.String())
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(dispatch.Sprintf("Saved @%s (%s) to your WhatsApp contact sync state.", username, fullName), []types.JID{resolvedJID})
}

func handleUserInfo(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()

	targets := ctx.GetTargets()
	var rawTarget types.JID
	if len(targets) > 0 {
		rawTarget = targets[0]
	} else if len(ctx.Args) == 0 {
		rawTarget = ctx.Sender
	} else {
		return ctx.Replyf("Could not find user. Usage:\n- %suserinfo @user\n- %suserinfo 1234567890\n- Reply to a user's message with %suserinfo", p, p, p)
	}

	targetJID := rawTarget.ToNonAD()

	bioText := ""
	queryJIDs := []types.JID{targetJID}
	if rawTarget != targetJID {
		queryJIDs = append(queryJIDs, rawTarget)
	}

	if uMap, err := ctx.Client.GetUserInfo(ctx.Ctx, queryJIDs); err == nil && uMap != nil {
		for _, qJID := range queryJIDs {
			if uInfo, ok := uMap[qJID]; ok {
				if uInfo.Status != "" && bioText == "" {
					bioText = strings.TrimSpace(uInfo.Status)
				}
				if targetJID.Server == types.HiddenUserServer && len(uInfo.Devices) > 0 {
					for _, dev := range uInfo.Devices {
						if dev.Server == types.DefaultUserServer && dev.User != "" {
							pnJID := types.NewJID(dev.User, types.DefaultUserServer)
							targetJID = pnJID
							if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
								_ = ctx.Client.Store.LIDs.PutLIDMapping(ctx.Ctx, rawTarget.ToNonAD(), pnJID)
							}
							break
						}
					}
				}
			}
		}
	}

	pushName := targetJID.User
	if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, targetJID); err == nil && contact.Found {
		if contact.PushName != "" {
			pushName = contact.PushName
		} else if contact.FullName != "" {
			pushName = contact.FullName
		} else if contact.BusinessName != "" {
			pushName = contact.BusinessName
		}
	}

	phoneOrLid := targetJID.User
	if targetJID.Server == types.DefaultUserServer {
		phoneOrLid = "+" + targetJID.User
	}

	tb := ctx.Text().
		Header("USER INFO").
		Field("Name", pushName).
		Field("Phone/ID", phoneOrLid)

	if bioText != "" && bioText != "N/A" {
		tb.Field("Status", bioText)
	}

	tb.Blank().
		Linef("Tip: Use %suserinfo @user to check another contact.", p)

	infoText := tb.String()

	ppInfo, errPP := ctx.Client.GetProfilePictureInfo(ctx.Ctx, targetJID, &whatsmeow.GetProfilePictureParams{})
	if (errPP != nil || ppInfo == nil || ppInfo.URL == "") && rawTarget != targetJID {
		ppInfo, errPP = ctx.Client.GetProfilePictureInfo(ctx.Ctx, rawTarget, &whatsmeow.GetProfilePictureParams{})
	}

	if errPP == nil && ppInfo != nil && ppInfo.URL != "" {
		imgData, errDownload := httpx.FetchBytes(ctx.Ctx, ppInfo.URL)
		if errDownload == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, "image/jpeg", infoText)
		}
	}

	return ctx.Reply(infoText)
}

func handleTimezone(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	query := strings.TrimSpace(ctx.RawArgs)

	if query == "" {
		return ctx.Replyf("Usage:\n- `%stimezone <city/country>` (e.g. `%stimezone Lagos` or `%stimezone New York`)\n- `%stimezone set <city/country>` to configure default bot timezone", p, p, p, p)
	}

	if strings.HasPrefix(strings.ToLower(query), "set ") {
		tzQuery := strings.TrimSpace(query[4:])
		tz, err := SearchTimezone(tzQuery)
		if err != nil {
			return ctx.Replyf("Could not find timezone matching %q: %v", tzQuery, err)
		}
		if s, ok := dispatch.GetStore(ctx); ok {
			_ = s.PutSetting(ctx.Ctx, "timezone", tz.Location)
		}
		return ctx.Replyf("Bot timezone updated to *%s* (UTC%s).", tz.Location, tz.UTCOffset)
	}

	tz, err := SearchTimezone(query)
	if err != nil {
		return ctx.Replyf("Could not find timezone matching %q: %v", query, err)
	}

	return ctx.Replyf("*Timezone Result*\n• Location: %s\n• Current Time: %s\n• UTC Offset: UTC%s\n• Timezone ID: %s",
		tz.Location, tz.CurrentTime, tz.UTCOffset, tz.ID)
}

func handleFont(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	if ctx.RawArgs == "" {
		return ctx.Replyf("Usage: `%sfont <style_number or text>` or `%sfonts` to preview all styles.", p, p)
	}

	text := ctx.RawArgs
	styled := ConvertFontStyle(text)
	return ctx.Reply(styled)
}
