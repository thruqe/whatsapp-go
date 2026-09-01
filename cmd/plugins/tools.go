package plugins

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"

	utils "whatsrook"
	cliutils "whatsrook/cmd/utils"
	Logger "whatsrook/logger"
)

func init() {
	Register(&Command{
		Name:        "contact",
		Alias:       "savecontact",
		Description: "Save a user to your WhatsApp contact list via AppState sync and send a native vCard",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleSaveContact,
	})
	Register(&Command{
		Name:        "font",
		Description: "Switch default font style used by bot. Usage: font <style_name_or_number>",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFont,
	})
	Register(&Command{
		Name:        "fonts",
		Alias:       "fontlist",
		Description: "List all available font numbers and preview styles",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFontList,
	})
	Register(&Command{
		Name:        "fancy",
		Alias:       "style",
		Description: "Convert text to a fancy font by font number. Usage: fancy <font_number> <text>",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFancy,
	})
	Register(&Command{
		Name:        "ss",
		Alias:       "screenshot",
		Description: "Capture a screenshot of a website",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleScreenshot,
	})
	Register(&Command{
		Name:        "tts",
		Alias:       "say",
		Description: "Convert text into spoken voice audio using Google Speech",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleTTS,
	})
	Register(&Command{
		Name:        "whois",
		Alias:       "userinfo",
		Description: "Display detailed profile information and profile photo of a WhatsApp user",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleUserInfo,
	})
}

func handleSaveContact(ctx *Context) error {
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

	// Update local device contact store cache (correct argument order: firstName, fullName)
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

	vcard := Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:%s;%s;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", firstName, fullName, fullName, targetJID.User, targetJID.User)
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
	return ctx.ReplyWithMentions(Sprintf("Saved @%s (%s) to your WhatsApp contact sync state.", username, fullName), []types.JID{resolvedJID})
}

func handleFont(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Replyf("Current font style: %s\n\nUsage: %sfont <number or style_name>\nType %sfontlist to view all available font numbers.", cliutils.GetFontStyle(), p, p)
	}

	arg := strings.ToLower(ctx.Args[0])
	targetStyle := ""

	if num, err := strconv.Atoi(arg); err == nil && num >= 1 && num <= len(cliutils.IndexedFonts) {
		targetStyle = cliutils.IndexedFonts[num-1].Key
	} else {
		for _, f := range cliutils.IndexedFonts {
			if strings.EqualFold(f.Key, arg) || strings.EqualFold(f.Name, arg) {
				targetStyle = f.Key
				break
			}
		}
		if arg == "normal" || arg == "default" {
			targetStyle = "normal"
		}
	}

	if targetStyle == "" {
		return ctx.Replyf("Invalid font style! Use %sfontlist to view valid numbers (1-%d).", p, len(cliutils.IndexedFonts))
	}

	cliutils.SetFontStyle(targetStyle)
	if s, ok := getStore(ctx); ok {
		_ = s.PutSetting(ctx.Ctx, "font_style", targetStyle)
	}

	return ctx.Replyf("Default font style updated to *%s*.", targetStyle)
}

func handleFontList(ctx *Context) error {
	p := ctx.GetPrefix()
	sampleText := "WhatsRook Bot"

	tb := ctx.Text().Header("AVAILABLE FONT NUMBERS & STYLES")

	for _, f := range cliutils.IndexedFonts {
		curr := cliutils.GetFontStyle()
		cliutils.SetFontStyle(f.Key)
		converted := cliutils.ConvertFontStyle(sampleText)
		cliutils.SetFontStyle(curr)

		tb.Numberedf(f.Number, "%s → %s", f.Name, converted)
	}

	tb.Blank().
		Section("Usage Examples:").
		Bulletf("%sfancy 14 Hello World", p).
		Bulletf("%sfont 14", p)

	return tb.Reply()
}

func handleFancy(ctx *Context) error {
	p := ctx.GetPrefix()

	var fontNum int
	var textToConvert string

	quoted := ctx.GetQuotedMessage()
	var quotedText string
	if quoted != nil {
		quotedText = strings.TrimSpace(extractTextFromProto(quoted))
	}

	if len(ctx.Args) == 0 {
		if quotedText != "" {
			sample := quotedText
			if len([]rune(sample)) > 30 {
				sample = string([]rune(sample)[:30]) + "..."
			}
			tb := ctx.Text().Header("Select a font style below to convert your replied message:")
			for _, fn := range []int{14, 2, 8, 4, 18, 22} {
				if fn <= len(cliutils.IndexedFonts) {
					f := cliutils.IndexedFonts[fn-1]
					curr := cliutils.GetFontStyle()
					cliutils.SetFontStyle(f.Key)
					preview := cliutils.ConvertFontStyle(sample)
					cliutils.SetFontStyle(curr)
					tb.Linef("#%d (%s): %s", fn, f.Name, preview)
				}
			}
			options := []string{
				"Small Caps (#14)",
				"Bold (#2)",
				"Fraktur (#8)",
			}
			return sendPollReply(ctx, tb.String(), options)
		}

		tb := ctx.Text().
			Linef("Please provide a font number and text to convert, or reply to a message with *%sfancy <font_number>*.", p).
			Blank().
			Linef("Use *%sfontlist* to view all available font numbers.", p).
			Blank().
			Section("Usage Examples:").
			Bulletf("`%sfancy 14 Hello World`", p).
			Bulletf("`%sfancy 14` (as reply to a message)", p).
			Bulletf("`%sfancy 2 WhatsRook AI`", p).
			Blank().
			Line("Select a font preset from the poll below to convert default sample text:")

		options := []string{
			"Small Caps (#14)",
			"Bold (#2)",
			"Fraktur (#8)",
		}

		return sendPollReply(ctx, tb.String(), options)
	}

	num, err := strconv.Atoi(ctx.Args[0])
	if err == nil {
		if num < 1 || num > len(cliutils.IndexedFonts) {
			return ctx.Replyf("Invalid font number %q. Please choose a number between 1 and %d.\nType %sfontlist to view all font numbers.", ctx.Args[0], len(cliutils.IndexedFonts), p)
		}
		fontNum = num

		if len(ctx.Args) >= 2 {
			textToConvert = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
		} else if quotedText != "" {
			textToConvert = quotedText
		} else {
			return ctx.Replyf("Please provide text to convert or reply to a message with text.\nExample: `%sfancy %d Hello World` or reply to a message with `%sfancy %d`", p, fontNum, p, fontNum)
		}
	} else {
		fontNum = 14
		textToConvert = strings.TrimSpace(ctx.RawArgs)
	}

	if textToConvert == "" {
		if quotedText != "" {
			textToConvert = quotedText
		} else {
			return ctx.Replyf("Please provide text to convert. Example: `%sfancy %d Hello World`", p, fontNum)
		}
	}

	targetFont := cliutils.IndexedFonts[fontNum-1]

	curr := cliutils.GetFontStyle()
	cliutils.SetFontStyle(targetFont.Key)
	convertedText := cliutils.ConvertFontStyle(textToConvert)
	cliutils.SetFontStyle(curr)

	return ctx.Reply(convertedText)
}

func handleScreenshot(ctx *Context) error {
	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = strings.TrimSpace(extractTextFromProto(quoted))
		}
	}

	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: `%sss <URL>`\n\nExample:\n- `%sss https://google.com`\n- Reply to a message containing a URL with `%sss`", p, p, p)
	}

	fields := strings.Fields(query)
	targetURL := ""
	for _, f := range fields {
		if strings.Contains(f, ".") && !strings.HasPrefix(f, "@") {
			targetURL = f
			break
		}
	}
	if targetURL == "" {
		targetURL = fields[0]
	}

	targetURL = strings.TrimPrefix(targetURL, "<")
	targetURL = strings.TrimSuffix(targetURL, ">")

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Host == "" || !strings.Contains(parsed.Host, ".") {
		return ctx.Reply("Invalid URL. Please specify a valid web address (e.g. `https://google.com`).")
	}

	imgData, mimeType, err := fetchWebsiteScreenshot(ctx.Ctx, targetURL)
	if err != nil {
		Logger.Error("handleScreenshot failed", "url", targetURL, "err", err)
		return ctx.Replyf("Failed to capture screenshot for `%s`.\nPlease verify the website URL and try again.", targetURL)
	}

	caption := ""
	return ctx.ReplyWithImage(imgData, mimeType, caption)
}

func fetchWebsiteScreenshot(ctx context.Context, targetURL string) ([]byte, string, error) {
	thumURL := "https://image.thum.io/get/width/1280/crop/800/noanimate/" + targetURL
	data, mime, err := fetchImageBytes(ctx, thumURL)
	if err == nil && len(data) > 5000 {
		return data, mime, nil
	}

	microApiURL := "https://api.microlink.io/?url=" + url.QueryEscape(targetURL) + "&screenshot=true"
	var res struct {
		Status string `json:"status"`
		Data   struct {
			Screenshot struct {
				URL string `json:"url"`
			} `json:"screenshot"`
		} `json:"data"`
	}
	if err := utils.FetchJSON(ctx, microApiURL, &res); err == nil && res.Data.Screenshot.URL != "" {
		sData, sMime, sErr := fetchImageBytes(ctx, res.Data.Screenshot.URL)
		if sErr == nil && len(sData) > 5000 {
			return sData, sMime, nil
		}
	}

	thumAltURL := "https://image.thum.io/get/width/1280/noanimate/" + targetURL
	dataAlt, mimeAlt, errAlt := fetchImageBytes(ctx, thumAltURL)
	if errAlt == nil && len(dataAlt) > 5000 {
		return dataAlt, mimeAlt, nil
	}

	return nil, "", errors.New("failed to capture screenshot from available services")
}

func fetchImageBytes(ctx context.Context, imgURL string) ([]byte, string, error) {
	data, err := utils.FetchURLBytes(ctx, imgURL)
	if err != nil {
		return nil, "", err
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", errors.New("invalid image content type: " + mimeType)
	}

	return data, mimeType, nil
}

func handleTTS(ctx *Context) error {
	p := ctx.GetPrefix()
	textToSpeak := strings.TrimSpace(ctx.RawArgs)
	if textToSpeak == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			textToSpeak = utils.ExtractTextFromProto(quoted)
		}
	}
	if textToSpeak == "" {
		return ctx.Replyf("Usage: %stts <text> or %stts <lang_code> <text>\n\nExamples:\n• %stts Hello world!\n• %stts es Hola, ¿cómo estás?\n• %stts fr Bonjour tout le monde", p, p, p, p, p)
	}

	lang := "en"
	if len(ctx.Args) > 0 {
		firstWord := strings.ToLower(ctx.Args[0])
		if len(firstWord) >= 2 && len(firstWord) <= 5 && utils.IsKnownLanguageCode(firstWord) && len(ctx.Args) > 1 {
			lang = firstWord
			textToSpeak = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
		}
	}

	if strings.TrimSpace(textToSpeak) == "" {
		return ctx.Reply("Please provide text to convert to speech.")
	}

	if len(textToSpeak) > 500 {
		textToSpeak = textToSpeak[:500]
	}

	mp3Data, err := fetchGoogleTTS(ctx.Ctx, textToSpeak, lang)
	if err != nil {
		Logger.Error("handleTTS: Google TTS fetch failed", "err", err, "lang", lang)
		return ctx.Replyf("Failed to generate speech audio: %v", err)
	}

	opusData, errConv := convertMP3ToOpus(ctx.Ctx, mp3Data)
	if errConv == nil && len(opusData) > 0 {
		return ctx.ReplyWithAudio(opusData, "audio/ogg; codecs=opus")
	}

	Logger.Warn("handleTTS: ffmpeg OPUS conversion failed, falling back to automatic conversion", "err", errConv)
	return ctx.ReplyWithAudio(mp3Data, "audio/ogg; codecs=opus")
}

func fetchGoogleTTS(ctx context.Context, text string, lang string) ([]byte, error) {
	apiURL := Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=%s&client=tw-ob", url.QueryEscape(text), url.QueryEscape(lang))
	return utils.FetchURLBytes(ctx, apiURL, utils.WithHeader("Referer", "https://translate.google.com/"))
}

func convertMP3ToOpus(ctx context.Context, mp3Bytes []byte) ([]byte, error) {
	tempDir := os.TempDir()
	tempMP3 := filepath.Join(tempDir, Sprintf("tts_%d.mp3", time.Now().UnixNano()))
	tempOpus := filepath.Join(tempDir, Sprintf("tts_%d.opus", time.Now().UnixNano()))

	if err := os.WriteFile(tempMP3, mp3Bytes, 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tempMP3)
	defer os.Remove(tempOpus)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tempMP3, "-c:a", "libopus", "-b:a", "32k", "-application", "voip", tempOpus)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return os.ReadFile(tempOpus)
}

func handleUserInfo(ctx *Context) error {
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

	// Resolve target JID from LID to Phone Number JID and clean it up (ToNonAD)
	targetJID := NormalizeUserJID(ctx.Ctx, ctx.Client, rawTarget)

	// Fetch status bio & device list to double check PN resolution if still LID
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
				// If targetJID is still LID, check devices for phone number JID
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
		Logger.Debug("handleUserInfo: Downloading profile photo", "url", ppInfo.URL)
		imgData, errDownload := utils.FetchURLBytes(ctx.Ctx, ppInfo.URL)
		if errDownload == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, "image/jpeg", infoText)
		}
	}

	return ctx.Reply(infoText)
}
