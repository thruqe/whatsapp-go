package tools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/httpx"
	"whatsrook/util/logger"

	"go.mau.fi/whatsmeow/types"
)

const TranslateSettingKey = "translate_lang"

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "translate",
		Alias:       "tr,trans",
		Description: "Translate text or a replied message into another language",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleTranslate,
	})
}

func handleTranslate(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	raw := strings.TrimSpace(ctx.RawArgs)

	// 1. Subcommands: set, default, reset, languages
	if raw != "" {
		fields := strings.Fields(raw)
		sub := strings.ToLower(fields[0])

		switch sub {
		case "set", "default", "config":
			if !ctx.IsOwner() && !ctx.IsSudo() {
				return ctx.Reply("Only the bot owner or sudoers can configure the default translation language.")
			}
			if len(fields) < 2 {
				return ctx.Replyf("Usage: `%[1]stranslate set <language_code or name>` (e.g. `%[1]stranslate set es` or `%[1]stranslate set french`)", p)
			}
			langInput := fields[1]
			code, name, ok := NormalizeLanguageCode(langInput)
			if !ok {
				return ctx.Replyf("Unknown language %q. Please use language names (like English, Spanish, French) or short codes (like en, es, fr, de, ar).", langInput)
			}
			if s, okStore := dispatch.GetStore(ctx); okStore {
				_ = s.PutSetting(ctx.Ctx, TranslateSettingKey, code)
			}
			return ctx.Replyf("Default auto-translate language configured to *%s* (`%s`).", name, code)

		case "reset":
			if !ctx.IsOwner() && !ctx.IsSudo() {
				return ctx.Reply("Only the bot owner or sudoers can reset the default translation language.")
			}
			if s, okStore := dispatch.GetStore(ctx); okStore {
				_ = s.DeleteSetting(ctx.Ctx, TranslateSettingKey)
			}
			ownerPhone := getOwnerPhone(ctx)
			info, _ := DetectOwnerCountryLanguage(ownerPhone)
			return ctx.Replyf("Default translation language reset. Auto-detecting owner's country: *%s* (+%s) → *%s* (`%s`).",
				info.CountryName, info.DialCode, info.LangName, info.LangCode)

		case "languages", "langs":
			return ctx.Reply("*Common Translation Languages:*\n" +
				"• `en` (English), `es` (Spanish), `fr` (French), `de` (German)\n" +
				"• `ar` (Arabic), `pt` (Portuguese), `zh` (Chinese), `ja` (Japanese)\n" +
				"• `hi` (Hindi), `ru` (Russian), `it` (Italian), `id` (Indonesian)\n" +
				"• `ko` (Korean), `tr` (Turkish), `nl` (Dutch), `sw` (Swahili)\n" +
				"• `ha` (Hausa), `yo` (Yoruba), `ig` (Igbo), `vi` (Vietnamese)\n\n" +
				"Tip: You can use any 2-letter ISO-639-1 code or language name.")
		}
	}

	// 2. Parse language and text from args
	targetLang, targetLangName, text, explicitlySpecified := parseTranslateInput(raw)

	// 3. Fallback to quoted message if text is empty
	if text == "" {
		if quotedMsg := ctx.GetQuotedMessage(); quotedMsg != nil {
			switch {
			case quotedMsg.GetConversation() != "":
				text = quotedMsg.GetConversation()
			case quotedMsg.GetExtendedTextMessage() != nil && quotedMsg.GetExtendedTextMessage().GetText() != "":
				text = quotedMsg.GetExtendedTextMessage().GetText()
			case quotedMsg.GetImageMessage() != nil && quotedMsg.GetImageMessage().GetCaption() != "":
				text = quotedMsg.GetImageMessage().GetCaption()
			case quotedMsg.GetVideoMessage() != nil && quotedMsg.GetVideoMessage().GetCaption() != "":
				text = quotedMsg.GetVideoMessage().GetCaption()
			case quotedMsg.GetDocumentMessage() != nil && quotedMsg.GetDocumentMessage().GetCaption() != "":
				text = quotedMsg.GetDocumentMessage().GetCaption()
			}
		}
	}

	if strings.TrimSpace(text) == "" {
		return ctx.Replyf("*Translate Command Usage:*\n"+
			"• `%[1]stranslate <text>` (translates to your default/country language)\n"+
			"• `%[1]stranslate <lang> <text>` (e.g. `%[1]stranslate es Hello friend`)\n"+
			"• `%[1]stranslate \"<quoted text>\"`\n"+
			"• Reply to any message with `%[1]stranslate` or `%[1]stranslate <lang>`\n"+
			"• `%[1]stranslate set <lang>` (configure default language)\n"+
			"• `%[1]stranslate reset` (reset back to default language)",
			p)
	}

	// 4. Resolve target language if not explicitly specified
	var targetNote string
	if !explicitlySpecified {
		// Check configured default language in settings
		var configured string
		if s, okStore := dispatch.GetStore(ctx); okStore {
			if val, err := s.GetSetting(ctx.Ctx, TranslateSettingKey); err == nil && strings.TrimSpace(val) != "" {
				configured = strings.TrimSpace(val)
			}
		}

		if configured != "" {
			if code, name, ok := NormalizeLanguageCode(configured); ok {
				targetLang = code
				targetLangName = name
				targetNote = fmt.Sprintf("Default: %s", name)
			} else {
				targetLang = configured
				targetLangName = configured
				targetNote = fmt.Sprintf("Default: %s", configured)
			}
		} else {
			// Auto-detect owner's country and official language
			ownerPhone := getOwnerPhone(ctx)
			info, found := DetectOwnerCountryLanguage(ownerPhone)
			targetLang = info.LangCode
			targetLangName = info.LangName
			if found {
				targetNote = fmt.Sprintf("Auto-detected (%s)", info.CountryName)
			} else {
				targetNote = "Default: English"
			}
		}
	}

	// 5. Execute translation
	translated, sourceLangCode, err := executeTranslation(ctx, text, targetLang)
	if err != nil {
		logger.Error("handleTranslate: translation error", "err", err, "targetLang", targetLang)
		return ctx.Replyf("Translation failed: %v", err)
	}

	// If target language was not explicitly specified and the input text is already in the
	// target/owner language, auto-switch to English (or Spanish if target is English) to provide a useful translation.
	if !explicitlySpecified && isSameLanguage(sourceLangCode, targetLang) {
		altTarget := "en"
		if strings.HasPrefix(strings.ToLower(targetLang), "en") {
			altTarget = "es"
		}
		if altTrans, _, altErr := executeTranslation(ctx, text, altTarget); altErr == nil && altTrans != "" {
			translated = altTrans
			origTargetName := targetLangName
			targetLang = altTarget
			if _, name, ok := NormalizeLanguageCode(altTarget); ok {
				targetLangName = name
			} else {
				targetLangName = altTarget
			}
			targetNote = fmt.Sprintf("Auto-switched to %s (already in %s)", targetLangName, origTargetName)
		}
	}

	// Clean language names for concise header
	srcDisplay := sourceLangCode
	if _, name, ok := NormalizeLanguageCode(sourceLangCode); ok {
		srcDisplay = name
	}
	tgtDisplay := targetLangName
	if tgtDisplay == "" {
		if _, name, ok := NormalizeLanguageCode(targetLang); ok {
			tgtDisplay = name
		} else {
			tgtDisplay = targetLang
		}
	}

	// 6. Build response cleanly and naturally
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌐 *%s* → *%s*\n\n", srcDisplay, tgtDisplay))
	sb.WriteString(translated)

	if targetNote != "" {
		sb.WriteString(fmt.Sprintf("\n\n_%s_", targetNote))
	}

	return ctx.Reply(sb.String())
}

// isSameLanguage checks whether two language codes represent the same language (e.g. "pt" and "pt-BR").
func isSameLanguage(lang1, lang2 string) bool {
	l1 := strings.ToLower(strings.TrimSpace(lang1))
	l2 := strings.ToLower(strings.TrimSpace(lang2))
	if l1 == "" || l2 == "" {
		return false
	}
	if l1 == l2 {
		return true
	}
	if idx := strings.IndexByte(l1, '-'); idx > 0 {
		l1 = l1[:idx]
	}
	if idx := strings.IndexByte(l2, '-'); idx > 0 {
		l2 = l2[:idx]
	}
	return l1 == l2
}

// parseTranslateInput extracts target language (if explicitly specified) and cleans quotes from the text.
func parseTranslateInput(raw string) (targetLang, targetLangName, text string, explicitlySpecified bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}

	fields := strings.Fields(raw)
	if len(fields) > 0 {
		firstToken := fields[0]
		if code, name, ok := NormalizeLanguageCode(firstToken); ok {
			targetLang = code
			targetLangName = name
			explicitlySpecified = true
			text = strings.TrimSpace(raw[len(firstToken):])
		}
	}

	if !explicitlySpecified {
		text = raw
	}

	text = unquoteText(text)
	return targetLang, targetLangName, text, explicitlySpecified
}

// unquoteText strips surrounding single, double, or typographical quotes.
func unquoteText(s string) string {
	s = strings.TrimSpace(s)
	quotes := [][2]string{
		{`"`, `"`},
		{`'`, `'`},
		{"“", "”"},
		{"‘", "’"},
		{"```", "```"},
		{"`", "`"},
	}

	for _, pair := range quotes {
		if strings.HasPrefix(s, pair[0]) && strings.HasSuffix(s, pair[1]) && len(s) >= len(pair[0])+len(pair[1]) {
			s = s[len(pair[0]) : len(s)-len(pair[1])]
			s = strings.TrimSpace(s)
			break
		}
	}
	return s
}

// resolvePhoneNumber extracts the true E.164 phone number from a JID, ensuring LIDs are properly mapped to PNs.
func resolvePhoneNumber(ctx *dispatch.Context, jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}

	// 1. If the JID is already a standard phone number JID (@s.whatsapp.net), use it directly.
	if jid.Server == types.DefaultUserServer && jid.User != "" {
		return jid.User
	}

	// 2. If it's an LID (@lid), NEVER use the LID's User directly as a phone number!
	if jid.Server == types.HiddenUserServer {
		// 2a. Check if message event already has SenderAlt with phone number JID
		if ctx != nil && ctx.Evt != nil && !ctx.Evt.Info.SenderAlt.IsEmpty() {
			if ctx.Evt.Info.SenderAlt.Server == types.DefaultUserServer && ctx.Evt.Info.SenderAlt.User != "" {
				return ctx.Evt.Info.SenderAlt.User
			}
		}

		// 2b. Check local cached LID -> PN mapping in store
		if ctx != nil && ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			reqCtx := ctx.GetSendContext()
			if pn, err := ctx.Client.Store.LIDs.GetPNForLID(reqCtx, jid.ToNonAD()); err == nil && !pn.IsEmpty() {
				if pn.Server == types.DefaultUserServer && pn.User != "" {
					return pn.User
				}
			}
		}

		// 2c. Query WhatsApp GetUserInfo to resolve LID to PN device JID
		if ctx != nil && ctx.Client != nil && ctx.Client.IsConnected() {
			reqCtx := ctx.GetSendContext()
			if uMap, err := ctx.Client.GetUserInfo(reqCtx, []types.JID{jid.ToNonAD()}); err == nil && uMap != nil {
				if uInfo, ok := uMap[jid.ToNonAD()]; ok && len(uInfo.Devices) > 0 {
					for _, dev := range uInfo.Devices {
						if dev.Server == types.DefaultUserServer && dev.User != "" {
							pnJID := types.NewJID(dev.User, types.DefaultUserServer)
							if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
								_ = ctx.Client.Store.LIDs.PutLIDMapping(reqCtx, jid.ToNonAD(), pnJID)
							}
							return dev.User
						}
					}
				}
			}
		}
	}

	return ""
}

// getOwnerPhone resolves the user or bot owner's phone number string.
func getOwnerPhone(ctx *dispatch.Context) string {
	if ctx == nil {
		return ""
	}

	// 1. If sender is owner, resolve their phone number (handling LID -> PN mapping)
	if ctx.IsOwner() {
		if pn := resolvePhoneNumber(ctx, ctx.Sender); pn != "" {
			return pn
		}
	}

	// 2. Check environment variables
	for _, envKey := range []string{"OWNER", "SUDO", "SUDOERS"} {
		if val := strings.TrimSpace(os.Getenv(envKey)); val != "" {
			parts := strings.FieldsSeq(val)
			for part := range parts {
				clean := strings.TrimPrefix(part, "+")
				if idx := strings.Index(clean, "@"); idx != -1 {
					clean = clean[:idx]
				}
				clean = strings.TrimSpace(clean)
				if clean != "" {
					return clean
				}
			}
		}
	}

	// 3. Check client Store ID (bot account phone number)
	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil && !ctx.Client.Store.ID.IsEmpty() {
		if ctx.Client.Store.ID.Server == types.DefaultUserServer && ctx.Client.Store.ID.User != "" {
			return ctx.Client.Store.ID.User
		}
	}

	// 4. Resolve sender's phone number even if not flagged as owner
	if pn := resolvePhoneNumber(ctx, ctx.Sender); pn != "" {
		return pn
	}

	return ""
}

// executeTranslation performs the translation via Google Translate GTX endpoint with fallback.
func executeTranslation(ctx *dispatch.Context, text, targetLang string) (translatedText string, sourceLang string, err error) {
	gtxURL := fmt.Sprintf("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s",
		url.QueryEscape(targetLang), url.QueryEscape(text))

	body, err := httpx.FetchBytes(ctx.Ctx, gtxURL)
	if err == nil && len(body) > 0 {
		var rawResp []any
		if jsonErr := json.Unmarshal(body, &rawResp); jsonErr == nil && len(rawResp) > 0 {
			var sb strings.Builder
			if sentences, ok := rawResp[0].([]any); ok {
				for _, s := range sentences {
					if parts, okParts := s.([]any); okParts && len(parts) > 0 {
						if transPart, okStr := parts[0].(string); okStr {
							sb.WriteString(transPart)
						}
					}
				}
			}

			if len(rawResp) > 2 {
				if srcStr, okSrc := rawResp[2].(string); okSrc {
					sourceLang = srcStr
				}
			}

			if sb.Len() > 0 {
				return sb.String(), sourceLang, nil
			}
		}
	}

	// Fallback to MyMemory translation API
	fallbackURL := fmt.Sprintf("https://api.mymemory.translated.net/get?q=%s&langpair=auto|%s",
		url.QueryEscape(text), url.QueryEscape(targetLang))

	type myMemoryResp struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		Matches []struct {
			Source string `json:"source"`
		} `json:"matches"`
	}

	var mm myMemoryResp
	if fetchErr := httpx.FetchJSON(ctx.Ctx, fallbackURL, &mm); fetchErr == nil && mm.ResponseData.TranslatedText != "" {
		src := "auto"
		if len(mm.Matches) > 0 && mm.Matches[0].Source != "" {
			src = mm.Matches[0].Source
		}
		return mm.ResponseData.TranslatedText, src, nil
	}

	return "", "", fmt.Errorf("all translation providers failed or rate-limited: %w", err)
}
