package plugins

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"whatsrook/cli/updater"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"
)

func init() {
	cliutils.InitBootTime()

	Register(&Command{
		Name:        "alive",
		Alias:       "system",
		Description: "Check bot online status, uptime, system stats, and custom alive template or replied media (image/video/audio)",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleAlive,
	})

	Register(&Command{
		Name:        "cpu",
		Description: "Show system CPU information and usage",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleCPU,
	})

	Register(&Command{
		Name:        "memory",
		Description: "Show system and process memory usage",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleMemory,
	})

	Register(&Command{
		Name:        "menu",
		Description: "Show all available commands grouped by category",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleMenu,
	})

	Register(&Command{
		Name:        "ping",
		Description: "Check bot response latency",
		Category:    "info",
		IsPublic:    true,
		Handler:     handlePing,
	})

	Register(&Command{
		Name:        "repo",
		Alias:       "sc",
		Description: "Show the GitHub repository link and project info",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleRepo,
	})

	Register(&Command{
		Name:        "uptime",
		Alias:       "runtime",
		Description: "Show how long the bot has been running",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleUptime,
	})
}

func handleAlive(ctx *Context) error {
	cliutils.InitBootTime()
	s, okStore := getStore(ctx)

	quotedMsg := ctx.GetQuotedMessage()
	if quotedMsg != nil && ctx.IsSudo() {
		var mediaData []byte
		var mediaType string
		var mime string
		var errDl error

		if img := quotedMsg.GetImageMessage(); img != nil {
			mediaData, errDl = ctx.Client.Download(ctx.Ctx, img)
			mediaType = "image"
			mime = img.GetMimetype()
		} else if vid := quotedMsg.GetVideoMessage(); vid != nil {
			mediaData, errDl = ctx.Client.Download(ctx.Ctx, vid)
			mediaType = "video"
			mime = vid.GetMimetype()
		} else if aud := quotedMsg.GetAudioMessage(); aud != nil {
			mediaData, errDl = ctx.Client.Download(ctx.Ctx, aud)
			mediaType = "audio"
			mime = aud.GetMimetype()
		}

		if errDl == nil && len(mediaData) > 0 {
			mediaDir := GetSessionMediaDir(ctx.Client)
			_ = os.MkdirAll(mediaDir, 0755)
			mediaFile := filepath.Join(mediaDir, "alive_media.bin")
			if errWrite := os.WriteFile(mediaFile, mediaData, 0644); errWrite == nil {
				if okStore {
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaTypeKey, mediaType)
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaMimeKey, mime)
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaFileKey, mediaFile)
				}
				caption := extractTextFromProto(quotedMsg)
				if strings.TrimSpace(ctx.RawArgs) != "" && !strings.EqualFold(strings.Fields(ctx.RawArgs)[0], "media") {
					caption = strings.TrimSpace(ctx.RawArgs)
				}
				if caption != "" && okStore {
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveTemplateKey, caption)
				}

				return ctx.Replyf("Saved replied %s as your alive media message!\n\nImage/Video will include the caption, and Audio will include music card context info.\nUse `%salive` to test it.", mediaType, ctx.GetPrefix())
			}
		}
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])

		switch sub {
		case "msg", "set", "template":
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can customize the alive message template.")
			}
			if len(args) < 2 {
				curr := cliutils.DefaultAliveTemplate
				if okStore {
					if val, err := s.GetSetting(ctx.Ctx, cliutils.AliveTemplateKey); err == nil && val != "" {
						curr = val
					}
				}
				return ctx.Reply("Current Alive Message Template:\n\n" + curr)
			}
			newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
			if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") || strings.EqualFold(newTpl, "default") {
				if okStore {
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveTemplateKey, "")
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaTypeKey, "")
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaFileKey, "")
				}
				return renderAliveResponse(ctx, cliutils.DefaultAliveTemplate, "")
			}
			if okStore {
				if err := s.PutSetting(ctx.Ctx, cliutils.AliveTemplateKey, newTpl); err != nil {
					return ctx.Reply("Failed to save alive template: " + err.Error())
				}
			}
			return renderAliveResponse(ctx, newTpl, "")

		case "get", "show", "view":
			curr := cliutils.DefaultAliveTemplate
			if okStore {
				if val, err := s.GetSetting(ctx.Ctx, cliutils.AliveTemplateKey); err == nil && val != "" {
					curr = val
				}
			}
			return ctx.Reply("Current Alive Message Template:\n\n" + curr)

		case "media":
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can set alive media URL.")
			}
			if len(args) < 2 {
				curr := "none"
				if okStore {
					if val, err := s.GetSetting(ctx.Ctx, cliutils.AliveMediaKey); err == nil && val != "" {
						curr = val
					}
				}
				return ctx.Reply("Current Alive Media URL: " + curr)
			}
			urlVal := strings.TrimSpace(args[1])
			if strings.EqualFold(urlVal, "clear") || strings.EqualFold(urlVal, "off") || strings.EqualFold(urlVal, "none") {
				if okStore {
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaKey, "")
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaTypeKey, "")
					_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaFileKey, "")
				}
				return ctx.Reply("Alive media URL cleared.")
			}
			if okStore {
				if err := s.PutSetting(ctx.Ctx, cliutils.AliveMediaKey, urlVal); err != nil {
					return ctx.Reply("Failed to save alive media: " + err.Error())
				}
			}
			return ctx.Reply("Alive media URL updated successfully!")

		case "help", "guide":
			p := ctx.GetPrefix()
			return ctx.Text().
				Header("ALIVE CUSTOMIZATION GUIDE").
				Section("Alive Template Formatting Variables:").
				Bullet("&user / &mention : Tag the command caller").
				Bullet("&pushname : Push name of the caller").
				Bullet("&uptime : Current bot uptime").
				Bullet("&ram : Allocated memory usage").
				Bullet("&goroutines : Active Go routines").
				Bullet("&latency : Response speed").
				Bullet("&owner : Bot owner").
				Blank().
				Section("Media Customization:").
				Bulletf("Reply to any Image, Video, or Audio with `%salive media` to set it as the alive background", p).
				Bulletf("Use `%salive reset` to restore default text-only alive card", p).
				Reply()

		case "reset", "clear":
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can reset alive configuration.")
			}
			if okStore {
				_ = s.PutSetting(ctx.Ctx, cliutils.AliveTemplateKey, "")
				_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaTypeKey, "")
				_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaMimeKey, "")
				_ = s.PutSetting(ctx.Ctx, cliutils.AliveMediaFileKey, "")
			}
			return ctx.Reply("Alive template and custom media reset to default.")
		}
	}

	tpl := cliutils.DefaultAliveTemplate
	if okStore {
		if val, err := s.GetSetting(ctx.Ctx, cliutils.AliveTemplateKey); err == nil && val != "" {
			tpl = val
		}
	}

	return renderAliveResponse(ctx, tpl, "")
}

func renderAliveResponse(ctx *Context, tpl, fallbackMediaURL string) error {
	startMeasure := time.Now()
	uptime := cliutils.FormatDuration(time.Since(cliutils.BootTime))

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	ramUsage := Sprintf("%.2f MB", float64(memStats.Alloc)/1024/1024)
	goroutines := strconv.Itoa(runtime.NumGoroutine())

	latency := Sprintf("%d ms", time.Since(startMeasure).Milliseconds())

	ownerName := "Thruqe"
	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		ownerName = ctx.Client.Store.ID.ToNonAD().User
	}

	pushName := "User"
	if ctx.Evt != nil && ctx.Evt.Info.PushName != "" {
		pushName = ctx.Evt.Info.PushName
	}

	senderJID := ctx.Sender.ToNonAD()
	userTag := "@" + senderJID.User

	var randomFact, randomQuote, randomJoke, randomRizz string
	if strings.Contains(tpl, "fact") {
		randomFact = cliutils.GetRandomFact(ctx.Ctx)
	}
	if strings.Contains(tpl, "quote") {
		randomQuote = cliutils.GetRandomQuote(ctx.Ctx)
	}
	if strings.Contains(tpl, "joke") {
		randomJoke = cliutils.GetRandomJoke(ctx.Ctx)
	}
	if strings.Contains(tpl, "rizz") {
		randomRizz = cliutils.GetRandomRizz(ctx.Ctx)
	}

	replacer := strings.NewReplacer(
		"{bot}", ctx.GetBotName(),
		"@bot", ctx.GetBotName(),
		"{owner}", ownerName,
		"@owner", ownerName,
		"{user}", userTag,
		"@user", userTag,
		"{name}", pushName,
		"@name", pushName,
		"{uptime}", uptime,
		"@uptime", uptime,
		"{latency}", latency,
		"@latency", latency,
		"{ram}", ramUsage,
		"@ram", ramUsage,
		"{goroutines}", goroutines,
		"@goroutines", goroutines,
		"{version}", "v2.5.0",
		"@version", "v2.5.0",
		"{prefix}", ctx.GetPrefix(),
		"@prefix", ctx.GetPrefix(),
		"{fact}", randomFact,
		"@fact", randomFact,
		"{quote}", randomQuote,
		"@quote", randomQuote,
		"{joke}", randomJoke,
		"@joke", randomJoke,
		"{rizz}", randomRizz,
		"@rizz", randomRizz,
	)

	bodyText := replacer.Replace(tpl)

	s, okStore := getStore(ctx)
	mediaType := ""
	mime := ""
	mediaFile := ""
	mediaURL := fallbackMediaURL

	if okStore {
		mType, _ := s.GetSetting(ctx.Ctx, cliutils.AliveMediaTypeKey)
		mMime, _ := s.GetSetting(ctx.Ctx, cliutils.AliveMediaMimeKey)
		mFile, _ := s.GetSetting(ctx.Ctx, cliutils.AliveMediaFileKey)
		mURL, _ := s.GetSetting(ctx.Ctx, cliutils.AliveMediaKey)

		if mType != "" && mFile != "" {
			mediaType = mType
			mime = mMime
			mediaFile = mFile
		} else if mURL != "" {
			mediaURL = mURL
		}
	}

	if mediaType != "" && mediaFile != "" {
		mediaBytes, errRead := os.ReadFile(mediaFile)
		if errRead == nil && len(mediaBytes) > 0 {
			switch mediaType {
			case "image":
				if mime == "" {
					mime = "image/jpeg"
				}
				return ctx.ReplyWithImage(mediaBytes, mime, bodyText)
			case "video":
				if mime == "" {
					mime = "video/mp4"
				}
				return ctx.ReplyWithVideo(mediaBytes, mime, bodyText)
			case "audio":
				return replyWithAliveAudioCard(ctx, mediaBytes, bodyText, ownerName)
			}
		}
	}

	if mediaURL != "" {
		imgBytes, errDl := utils.FetchURLBytes(ctx.Ctx, mediaURL)
		if errDl == nil && len(imgBytes) > 0 {
			return ctx.ReplyWithImage(imgBytes, "image/jpeg", bodyText)
		}
		bodyText = bodyText + "\n\n" + mediaURL
	}

	if strings.Contains(tpl, "@user") || strings.Contains(tpl, "{user}") {
		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: &bodyText,
				ContextInfo: &waE2E.ContextInfo{
					MentionedJID: []string{senderJID.String()},
				},
			},
		})
		return err
	}

	return ctx.Reply(bodyText)
}

func replyWithAliveAudioCard(ctx *Context, data []byte, bodyText, ownerName string) error {
	meta, errMeta := utils.EnsureOpusPTT(ctx.Ctx, data)
	mimetype := "audio/ogg; codecs=opus"
	if errMeta == nil && meta != nil && meta.Converted && len(meta.Data) > 0 {
		data = meta.Data
	} else {
		mimetype = "audio/mp4"
	}

	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return errors.New("audio upload failed: " + err.Error())
	}

	botName := ctx.GetBotName()
	title := Sprintf("%s IS ALIVE", strings.ToUpper(botName))
	body := "Owner: " + ownerName
	sourceURL := "https://github.com/Thruqe/whatsrook"

	adInfo := &waE2E.ContextInfo_ExternalAdReplyInfo{
		Title:                 new(title),
		Body:                  new(body),
		SourceURL:             new(sourceURL),
		MediaType:             waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum(),
		RenderLargerThumbnail: new(true),
		ShowAdAttribution:     new(false),
	}

	cinfo := &waE2E.ContextInfo{
		ExternalAdReply: adInfo,
	}

	senderJID := ctx.Sender.ToNonAD()
	if strings.Contains(bodyText, "@user") || strings.Contains(bodyText, "{user}") {
		cinfo.MentionedJID = []string{senderJID.String()}
	}

	fileLength := uint64(len(data))
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      new(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &fileLength,
			PTT:           new(true),
			ContextInfo:   cinfo,
		},
	}

	if meta != nil {
		if meta.Seconds > 0 {
			msg.AudioMessage.Seconds = new(meta.Seconds)
		}
		if len(meta.Waveform) > 0 {
			msg.AudioMessage.Waveform = meta.Waveform
		}
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

func sendAliveCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("ALIVE CUSTOMIZATION GUIDE").
		Section("Usage").
		Bulletf("Check Alive Status : %salive", p).
		Bulletf("Custom Message     : %salive msg <your custom template> or %salive <template>", p, p).
		Bulletf("Reply with Media   : Reply to an image, video, or audio message with %salive to set it as your alive media!", p).
		Indent(4, "- Image/Video : Sent with your custom text template as the caption.").NewLine().
		Indent(4, "- Audio       : Sent as an audio voice note with a music card (context info).").NewLine().
		Bulletf("Custom Media URL   : %salive media <url | clear>", p).
		Bulletf("Reset Template     : %salive msg reset", p).
		Blank().
		Section("Available Placeholders").
		Bullet("@user / {user}     : Sender mention tag").
		Bullet("@name / {name}     : Sender pushname").
		Bullet("@uptime / {uptime} : Active system uptime").
		Bullet("@bot / {bot}       : Bot display name").
		Bullet("@owner / {owner}   : Bot owner user ID").
		Bullet("@latency / {latency}: Response latency").
		Bullet("@ram / {ram}       : Allocated RAM usage").
		Bullet("@goroutines / {goroutines}: Active Go routines").
		Bullet("@version / {version} : Engine version").
		Bullet("@prefix / {prefix} : Active command prefix").
		Bullet("@fact / {fact}     : Random fact from API").
		Bullet("@quote / {quote}   : Random quote from API").
		Bullet("@joke / {joke}     : Random joke from API").
		Bullet("@rizz / {rizz}     : Random rizz from API").
		Blank().
		Section("Example Custom Templates").
		Linef("%salive msg Hello @name! @bot is active. Uptime: @uptime", p).
		Linef("%salive @user I am alive @uptime", p).
		Reply()
}

func handleCPU(ctx *Context) error {
	model := cliutils.GetCPUModel()
	cores := runtime.NumCPU()
	loadAvg := cliutils.GetLoadAvg()

	usageStr := "Unknown"
	if u, err := cliutils.GetCPUUsage(); err == nil {
		usageStr = Sprintf("%.2f%%", u)
	}

	return ctx.Text().
		Header("CPU Information").
		Field("Model", model).
		Fieldf("Cores/Threads", "%d", cores).
		Field("Load Average", loadAvg).
		Field("Current Usage", usageStr).
		Reply()
}

func handleMemory(ctx *Context) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	procAlloc := float64(m.Alloc) / 1024 / 1024
	procSys := float64(m.Sys) / 1024 / 1024

	tb := ctx.Text().Header("Memory Information")
	if sysMem, err := cliutils.GetSystemMemory(); err != nil {
		tb.Bulletf("System Memory: Error reading (%v)", err)
	} else {
		totalGB := float64(sysMem.Total) / 1024 / 1024
		availableGB := float64(sysMem.Available) / 1024 / 1024
		usedGB := totalGB - availableGB
		percent := (usedGB / totalGB) * 100

		tb.Fieldf("Total System Memory", "%.2f GB", totalGB).
			Fieldf("Used System Memory", "%.2f GB (%.1f%%)", usedGB, percent).
			Fieldf("Available System Memory", "%.2f GB", availableGB)
	}

	return tb.
		Fieldf("Process Allocated", "%.2f MB", procAlloc).
		Fieldf("Process System Reserved", "%.2f MB", procSys).
		Reply()
}

func HandlePendingMenuMediaReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	key := evt.Info.Chat.ToNonAD().String() + ":" + evt.Info.Sender.ToNonAD().String()
	cliutils.MenuThumbPromptsMu.RLock()
	promptTime, active := cliutils.PendingMenuThumbPrompts[key]
	cliutils.MenuThumbPromptsMu.RUnlock()

	if active && time.Since(promptTime) > 5*time.Minute {
		cliutils.MenuThumbPromptsMu.Lock()
		delete(cliutils.PendingMenuThumbPrompts, key)
		cliutils.MenuThumbPromptsMu.Unlock()
		active = false
	}

	if !active {
		return false
	}

	text := utils.ExtractMessageText(evt)
	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Evt:    evt,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
	}

	var prefixes []string
	if client != nil {
		prefixes = activePrefixes(ctx, client)
	} else {
		prefixes = []string{cliutils.DefaultPrefix}
	}
	if text != "" {
		for _, pref := range prefixes {
			if pref != "" && strings.HasPrefix(text, pref) {
				cliutils.MenuThumbPromptsMu.Lock()
				delete(cliutils.PendingMenuThumbPrompts, key)
				cliutils.MenuThumbPromptsMu.Unlock()
				return false
			}
		}
	}

	downloadable, isVideo, mime := ExtractMediaFromEvent(evt)
	if downloadable == nil {
		return false
	}

	cliutils.MenuThumbPromptsMu.Lock()
	delete(cliutils.PendingMenuThumbPrompts, key)
	cliutils.MenuThumbPromptsMu.Unlock()

	slog.Info("HandlePendingMenuMediaReply: Downloading custom menu media", "chat", key, "mime", mime, "isVideo", isVideo)
	data, err := client.Download(ctx, downloadable)

	if err != nil || len(data) == 0 {
		slog.Error("HandlePendingMenuMediaReply: Download failed", "chat", key, "err", err)
		_ = fakeCtx.Reply("Failed to download media for menu thumbnail.")
		return true
	}

	authDir := GetSessionAuthDir(client)
	targetPath, errProc := ProcessAndSaveThumbnail(ctx, authDir, data, isVideo)
	if errProc != nil {
		_ = fakeCtx.Replyf("Failed to process menu thumbnail: %v", errProc)
		return true
	}

	if s, ok := getSQLStore(client); ok {
		_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
	}

	_ = fakeCtx.Replyf("Bot menu thumbnail updated successfully! Type %smenu to view your custom thumbnail.", fakeCtx.GetPrefix())
	return true
}

func handleMenu(ctx *Context) error {
	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "reconfigure" || sub == "reconfig" || sub == "wizard" || sub == "setup" {
			return handleReconfigure(ctx)
		}
		if sub == "customize" || sub == "custom" {
			key := ctx.Chat.ToNonAD().String() + ":" + ctx.Sender.ToNonAD().String()
			cliutils.MenuThumbPromptsMu.Lock()
			cliutils.PendingMenuThumbPrompts[key] = time.Now()
			cliutils.MenuThumbPromptsMu.Unlock()
			return ctx.Reply("Upload or reply with an image (.jpg/.png) or video (.mp4) to set it as the custom bot menu thumbnail.\n\nTo restore default: " + ctx.GetPrefix() + "menu reset")
		}
		if sub == "reset" {
			key := ctx.Chat.ToNonAD().String() + ":" + ctx.Sender.ToNonAD().String()
			cliutils.MenuThumbPromptsMu.Lock()
			delete(cliutils.PendingMenuThumbPrompts, key)
			cliutils.MenuThumbPromptsMu.Unlock()

			authDir := GetSessionAuthDir(ctx.Client)
			if s, ok := getStore(ctx); ok {
				_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			}
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
			return ctx.Reply("Bot menu thumbnail reset to default.")
		}
	}

	type entry struct{ name, desc string }
	categoryOrder := []string{}
	categories := map[string][]entry{}
	seenCat := map[string]bool{}

	hiddenCmds := map[string]bool{
		"menu": true,
	}

	displayedCount := 0
	for _, cmd := range Visible() {
		if hiddenCmds[strings.ToLower(cmd.Name)] {
			continue
		}
		cat := cmd.Category
		if cat == "" {
			cat = "misc"
		}
		if !seenCat[cat] {
			seenCat[cat] = true
			categoryOrder = append(categoryOrder, cat)
		}
		categories[cat] = append(categories[cat], entry{name: cmd.Name, desc: cmd.Description})
		displayedCount++
	}

	uptime := utils.FormatUptime(time.Since(cliutils.StartTime).Seconds())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	usedRAM := ms.Alloc
	platform := runtime.GOOS

	user := ctx.Evt.Info.PushName
	if user == "" {
		user = ctx.Sender.User
	}

	botMode := "public"
	s, ok := getStore(ctx)
	if ok {
		if rawMode, err := s.GetSetting(ctx.Ctx, "mode"); err == nil && rawMode != "" {
			botMode = rawMode
		}
	}

	tb := ctx.Text().
		Header(toFancy(ctx.GetBotName())).
		Field("User", toFancy(user)).
		Field("OS", toFancy(platform)).
		Field("Mem", toFancy(utils.FormatBytes(usedRAM))).
		Field("Plugins", toFancy(strconv.Itoa(displayedCount))).
		Field("Mode", toFancy(botMode)).
		Field("Uptime", toFancy(uptime)).
		Field("Version", toFancy(updater.GetAppVersion())).
		Blank()

	for _, cat := range categoryOrder {
		cmds := categories[cat]
		tb.Section(toFancy(strings.ToUpper(cat)))
		for _, e := range cmds {
			tb.Bullet(toFancy(e.name))
		}
		tb.Blank()
	}

	menuText := tb.Trimmed()

	authDir := GetSessionAuthDir(ctx.Client)
	videoPath := filepath.Join(authDir, "custom_menu_thumbnail.mp4")
	if ok {
		if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
			videoPath = custom
		}
	}
	if _, err := os.Stat(videoPath); err != nil {
		jpgPath := filepath.Join(authDir, "custom_menu_thumbnail.jpg")
		if _, errJpg := os.Stat(jpgPath); errJpg == nil {
			videoPath = jpgPath
		} else {
			videoPath = ""
		}
	}

	if videoPath != "" {
		if videoData, err := os.ReadFile(videoPath); err == nil && len(videoData) > 0 {
			mType := "video/mp4"
			if strings.HasSuffix(videoPath, ".jpg") || strings.HasSuffix(videoPath, ".jpeg") {
				return ctx.ReplyWithImage(videoData, "image/jpeg", menuText)
			}
			errSend := ctx.ReplyWithVideoGif(videoData, mType, menuText)
			if errSend != nil {
				return sendText(ctx, menuText)
			}
			return nil
		}
	}

	return sendText(ctx, menuText)
}

func toFancy(s string) string {
	return cliutils.ConvertFontStyle(s)
}

func handlePing(ctx *Context) error {
	start := time.Now()
	msgID, err := ctx.ReplyWithID("Ping...")
	if err != nil {
		return err
	}

	elapsed := time.Since(start)
	respText := Sprintf("%d ms", elapsed.Milliseconds())

	if _, editErr := ctx.Edit(msgID, respText); editErr != nil {
		_ = ctx.Reply(respText)
	}
	return nil
}

func handleRepo(ctx *Context) error {
	repoURL := "https://github.com/Thruqe/whatsrook"
	text := Sprintf("WhatsRook Repository\n\n%s\n\nPlease star the repository if you like the project, it helps support and motivate me.\n\nPowered by %s", repoURL, ctx.GetBotName())

	return ctx.Reply(text)
}

func handleUptime(ctx *Context) error {
	out := cliutils.FormatDuration(time.Since(cliutils.StartTime))
	return sendText(ctx, out)
}
