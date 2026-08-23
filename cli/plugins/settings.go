package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"
)

func resetAFKUserTracker() {
	cliutils.AfkUserTrackerLock.Lock()
	cliutils.AfkUserTracker = make(map[string]*cliutils.UserAFKState)
	cliutils.AfkUserTrackerLock.Unlock()
}

func init() {
	sort.Strings(cliutils.SupportedTimezones)

	Register(&Command{
		Name:        "afk",
		Alias:       "away",
		Description: "Set or customize your Away-From-Keyboard (AFK) status with customizable templates, @ placeholders, and last active tracking",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleAFK,
	})
	Register(&Command{
		Name:        "autobio",
		Alias:       "bioauto",
		Description: "Auto-update WhatsApp status bio every minute with time & inspirational quotes",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleAutoBio,
	})
	Register(&Command{
		Name:        "timezone",
		Alias:       "tz",
		Description: "View or configure timezone for automute schedules via interactive buttons",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleTimezone,
	})
	Register(&Command{
		Name:        "setbot",
		Description: "Unified Bot Customization Wizard (Bot Name, Menu Thumbnail, Prefix, Bio)",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleSetBot,
	})
	Register(&Command{
		Name:        "reconfig",
		Alias:       "reconfigure",
		Description: "Reconfigure bot settings and re-bring the setup wizard",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleReconfigure,
	})
	Register(&Command{
		Name:        "autolike",
		Alias:       "likestatus",
		Description: "Automatically react with love emojis to incoming status broadcasts",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleLikeStatusCmd,
	})
	Register(&Command{
		Name:        "prefix",
		Description: "View or change the bot command prefix(es). Use 'none' for no prefix.",
		Category:    "settings",
		Handler:     handlePrefix,
	})
	Register(&Command{
		Name:        "privacy",
		Alias:       "myprivacy",
		Description: "View and update WhatsApp privacy settings (Last Seen, Profile Photo, Status, Read Receipts) via interactive buttons",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handlePrivacy,
	})
	Register(&Command{
		Name:        "setcmd",
		Description: "Link a sticker to a command trigger. Usage: setcmd [command_name] (replying to a sticker)",
		Category:    "settings",
		Handler:     handleSetCmd,
	})
	Register(&Command{
		Name:        "delcmd",
		Description: "Unlink a sticker from a command trigger. Usage: delcmd [command_name] or reply to a mapped sticker",
		Category:    "settings",
		Handler:     handleDelCmd,
	})
	Register(&Command{
		Name:        "getcmd",
		Description: "List all mapped sticker commands",
		Category:    "settings",
		Handler:     handleGetCmd,
	})
	Register(&Command{
		Name:        "discmd",
		Alias:       "disablecmd",
		Description: "Disable a command globally for normal users",
		Category:    "settings",
		Handler:     handleDisableCmd,
	})
	Register(&Command{
		Name:        "encmd",
		Alias:       "enablecmd",
		Description: "Enable a previously disabled command",
		Category:    "settings",
		Handler:     handleEnableCmd,
	})
	Register(&Command{
		Name:        "autoview",
		Alias:       "autovv",
		Description: "Toggle automatic ViewOnce message forwarding to DM",
		Category:    "settings",
		Handler:     handleAutoVV,
	})
	Register(&Command{
		Name:        "savestatus",
		Alias:       "autostatus",
		Description: "Toggle automatic status updates saving to DM",
		Category:    "settings",
		Handler:     handleAutoStatusSave,
	})
}

func UpdateOwnerLastActive(ctx context.Context, s *StoreWrapper) {
	now := time.Now()
	cliutils.AFKMu.Lock()
	cliutils.LastActiveCache = now
	cliutils.AFKMu.Unlock()

	if s != nil {
		_ = s.PutSetting(ctx, cliutils.AFKLastActiveKey, now.Format(time.RFC3339))
	}
}

func GetOwnerLastActive(ctx context.Context, s *StoreWrapper) time.Time {
	cliutils.AFKMu.RLock()
	cached := cliutils.LastActiveCache
	cliutils.AFKMu.RUnlock()
	if !cached.IsZero() {
		return cached
	}

	if s != nil {
		if val, err := s.GetSetting(ctx, cliutils.AFKLastActiveKey); err == nil && val != "" {
			if t, errParse := time.Parse(time.RFC3339, val); errParse == nil {
				return t
			}
		}
	}
	return time.Now()
}

func handleAFK(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK status.")
		}
		return setAFKStatus(ctx, s, "AFK (No reason specified)")
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "off", "disable", "back", "done":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can turn off AFK status.")
		}
		_ = s.PutSetting(ctx.Ctx, cliutils.AFKStatusKey, "off")
		_ = s.PutSetting(ctx.Ctx, cliutils.AFKReasonKey, "")
		_ = s.PutSetting(ctx.Ctx, cliutils.AFKTimeKey, "")
		resetAFKUserTracker()
		UpdateOwnerLastActive(ctx.Ctx, s)
		return ctx.Reply("Welcome back! AFK mode has been turned off.")

	case "customize", "custom", "help":
		return sendAFKCustomizeGuide(ctx)

	case "msg", "template", "text":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can customize the AFK template.")
		}
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, cliutils.AFKTemplateKey)
			if curr == "" {
				curr = cliutils.DefaultAFKTemplate
			}
			return ctx.Reply("Current AFK Message Template:\n\n" + curr)
		}
		newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") {
			_ = s.PutSetting(ctx.Ctx, cliutils.AFKTemplateKey, "")
			return ctx.Reply("AFK message template reset to default.")
		}
		if err := s.PutSetting(ctx.Ctx, cliutils.AFKTemplateKey, newTpl); err != nil {
			return ctx.Reply("Failed to save AFK template: " + err.Error())
		}
		return ctx.Reply("Custom AFK message template updated successfully!\n\nUse `" + ctx.GetPrefix() + "afk msg reset` to restore default.")

	case "media":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK media.")
		}
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, cliutils.AFKMediaKey)
			if curr == "" {
				return ctx.Reply("No custom AFK media URL set.")
			}
			return ctx.Reply("Current AFK Media URL: " + curr)
		}
		urlVal := strings.TrimSpace(args[1])
		if strings.EqualFold(urlVal, "clear") || strings.EqualFold(urlVal, "off") || strings.EqualFold(urlVal, "none") {
			_ = s.PutSetting(ctx.Ctx, cliutils.AFKMediaKey, "")
			return ctx.Reply("AFK media URL cleared.")
		}
		if err := s.PutSetting(ctx.Ctx, cliutils.AFKMediaKey, urlVal); err != nil {
			return ctx.Reply("Failed to save AFK media URL: " + err.Error())
		}
		return ctx.Reply("AFK media URL updated successfully!")

	default:
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK status.")
		}
		reason := strings.TrimSpace(ctx.RawArgs)
		return setAFKStatus(ctx, s, reason)
	}
}

func setAFKStatus(ctx *Context, s *StoreWrapper, reason string) error {
	lastActive := GetOwnerLastActive(ctx.Ctx, s)
	nowStr := time.Now().Format("2006-01-02 15:04:05 MST")
	lastActiveStr := lastActive.Format("2006-01-02 15:04:05 MST")

	_ = s.PutSetting(ctx.Ctx, cliutils.AFKStatusKey, "on")
	_ = s.PutSetting(ctx.Ctx, cliutils.AFKReasonKey, reason)
	_ = s.PutSetting(ctx.Ctx, cliutils.AFKTimeKey, nowStr)
	_ = s.PutSetting(ctx.Ctx, cliutils.AFKLastActiveKey, lastActiveStr)
	resetAFKUserTracker()

	p := ctx.GetPrefix()
	return ctx.Replyf("AFK mode activated.\n\nReason: %s\nTime: %s\nLast Available: %s\n\nTurn off anytime using `%safk back` or `%safk off`.", reason, nowStr, lastActiveStr, p, p)
}

func sendAFKCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("AFK CUSTOMIZATION GUIDE").
		Section("Usage").
		Bulletf("Activate AFK   : %safk <reason>", p).
		Bulletf("Deactivate AFK : %safk off or %safk back", p, p).
		Bulletf("Custom Message : %safk msg <your custom template>", p).
		Bulletf("Custom Media   : %safk media <url | clear>", p).
		Bulletf("Reset Template : %safk msg reset", p).
		Blank().
		Section("Available Placeholders & Tags").
		Bullet("{reason} / @reason : Reason for being AFK").
		Bullet("{time} / @time : Time AFK mode was set").
		Bullet("{last_available} / @last_available : Owner's last available active timestamp").
		Bullet("{fact} / @fact : Random interesting fact").
		Bullet("{quote} / @quote : Random inspirational quote").
		Bullet("{joke} / @joke : Random funny joke").
		Bullet("{rizz} / @rizz : Random smooth pickup line / rizz").
		Bullet("{user} / @user : Mention sender tag (@username)").
		Bullet("{group} / @group : Group name (if in group)").
		Blank().
		Section("Example Custom Template").
		Linef("%safk msg Hello {user}! Owner has been AFK since {time} (Last active: {last_available}). Reason: {reason}. Here is a joke for you: {joke}", p).
		Reply()
}

func HandleAFKAutoResponse(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if client == nil || client.Store == nil || client.Store.ID == nil || evt == nil {
		return false
	}
	s, ok := getSQLStore(client)
	if !ok {
		return false
	}

	ownerJID := client.Store.ID.ToNonAD()
	senderJID := evt.Info.Sender.ToNonAD()

	if senderJID.User == ownerJID.User {
		UpdateOwnerLastActive(ctx, s)
		status, _ := s.GetSetting(ctx, cliutils.AFKStatusKey)
		if status == "on" {
			if !strings.HasPrefix(strings.TrimSpace(text), ".") && !strings.HasPrefix(strings.TrimSpace(text), "/") && !strings.HasPrefix(strings.TrimSpace(text), "!") && !strings.HasPrefix(strings.TrimSpace(text), "#") {
				_ = s.PutSetting(ctx, cliutils.AFKStatusKey, "off")
				resetAFKUserTracker()
				cctx := &Context{
					Ctx:    ctx,
					Client: client,
					Evt:    evt,
					Chat:   evt.Info.Chat,
					Sender: evt.Info.Sender,
				}
				_ = cctx.Reply("Welcome back! You sent a message, so AFK mode has been automatically turned off.")
			}
		}
		return false
	}

	status, _ := s.GetSetting(ctx, cliutils.AFKStatusKey)
	if status != "on" {
		return false
	}

	isGroup := evt.Info.Chat.Server == "g.us"
	if isGroup {
		isMentionedOrReplied := false
		if strings.Contains(text, "@"+ownerJID.User) {
			isMentionedOrReplied = true
		}
		if ci := getContextInfoFromProto(evt.Message); ci != nil {
			for _, m := range ci.GetMentionedJID() {
				if parsed, err := types.ParseJID(m); err == nil && parsed.ToNonAD().User == ownerJID.User {
					isMentionedOrReplied = true
					break
				}
			}
			if ci.Participant != nil && *ci.Participant != "" {
				if parsed, err := types.ParseJID(*ci.Participant); err == nil && parsed.ToNonAD().User == ownerJID.User {
					isMentionedOrReplied = true
				}
			}
		}
		if !isMentionedOrReplied {
			return false
		}
	}

	cliutils.AfkUserTrackerLock.Lock()
	uState, okState := cliutils.AfkUserTracker[senderJID.User]
	if !okState {
		uState = &cliutils.UserAFKState{}
		cliutils.AfkUserTracker[senderJID.User] = uState
	}
	if time.Since(uState.LastSent) < 1*time.Minute {
		cliutils.AfkUserTrackerLock.Unlock()
		return false
	}
	alreadySent := uState.HasSent
	uState.LastSent = time.Now()
	uState.HasSent = true
	cliutils.AfkUserTrackerLock.Unlock()

	reason, _ := s.GetSetting(ctx, cliutils.AFKReasonKey)
	if reason == "" {
		reason = "AFK (No reason specified)"
	}
	afkTime, _ := s.GetSetting(ctx, cliutils.AFKTimeKey)
	if afkTime == "" {
		afkTime = time.Now().Format("2006-01-02 15:04:05 MST")
	}
	lastActiveStr, _ := s.GetSetting(ctx, cliutils.AFKLastActiveKey)
	if lastActiveStr == "" {
		lastActiveStr = GetOwnerLastActive(ctx, s).Format("2006-01-02 15:04:05 MST")
	}

	template, _ := s.GetSetting(ctx, cliutils.AFKTemplateKey)
	if template == "" {
		template = cliutils.DefaultAFKTemplate
	}

	userTag := "@" + senderJID.User
	groupName := evt.Info.Chat.String()

	randomFact := cliutils.GetRandomFact(ctx)
	randomQuote := cliutils.GetRandomQuote(ctx)
	randomJoke := cliutils.GetRandomJoke(ctx)
	randomRizz := cliutils.GetRandomRizz(ctx)

	replacer := strings.NewReplacer(
		"{reason}", reason,
		"@reason", reason,
		"{time}", afkTime,
		"{last_available}", lastActiveStr,
		"@time", lastActiveStr,
		"{fact}", randomFact,
		"@fact", randomFact,
		"{quote}", randomQuote,
		"@quote", randomQuote,
		"{joke}", randomJoke,
		"@joke", randomJoke,
		"{rizz}", randomRizz,
		"@rizz", randomRizz,
		"{user}", userTag,
		"@user", userTag,
		"{group}", groupName,
		"@group", groupName,
	)

	body := replacer.Replace(template)

	if alreadySent {
		body = "Still on afk\n\n" + body
	}

	cctx := &Context{
		Ctx:    ctx,
		Client: client,
		Evt:    evt,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
	}

	mediaURL, _ := s.GetSetting(ctx, cliutils.AFKMediaKey)
	if mediaURL != "" {
		body = body + "\n\n" + mediaURL
	}
	_ = cctx.Reply(body)

	return true
}

func getContextInfoFromProto(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo()
	}
	if stk := msg.GetStickerMessage(); stk != nil {
		return stk.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	return nil
}

var (
	autoBioMu     sync.Mutex
	autoBioCancel context.CancelFunc
)

func StartAutoBioScheduler(ctx context.Context, client *whatsmeow.Client) {
	autoBioMu.Lock()
	defer autoBioMu.Unlock()

	if autoBioCancel != nil {
		autoBioCancel()
		autoBioCancel = nil
	}

	schedCtx, cancel := context.WithCancel(ctx)
	autoBioCancel = cancel

	go func() {
		select {
		case <-schedCtx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		_, _ = updateAutoBio(schedCtx, client)

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-schedCtx.Done():
				return
			case <-ticker.C:
				_, _ = updateAutoBio(schedCtx, client)
			}
		}
	}()
}

func StopAutoBioScheduler() {
	autoBioMu.Lock()
	defer autoBioMu.Unlock()
	if autoBioCancel != nil {
		autoBioCancel()
		autoBioCancel = nil
	}
}

func handleAutoBio(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	switch arg0 {
	case "on", "enable", "true", "start":
		if err := s.PutSetting(ctx.Ctx, "autobio_enabled", "true"); err != nil {
			return ctx.Reply("Failed to enable AutoBio.")
		}
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply("AutoBio ENABLED! Status bio will update every minute with local time and quotes.")

	case "off", "disable", "false", "stop":
		if err := s.PutSetting(ctx.Ctx, "autobio_enabled", "false"); err != nil {
			return ctx.Reply("Failed to disable AutoBio.")
		}
		return ctx.Reply("AutoBio DISABLED.")

	case "toggle":
		enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
		if enabled == "true" {
			_ = s.PutSetting(ctx.Ctx, "autobio_enabled", "false")
			return ctx.Reply("AutoBio DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autobio_enabled", "true")
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply("AutoBio ENABLED!")

	case "customize", "custom", "help":
		return sendAutoBioCustomizeGuide(ctx, s)

	case "tz", "timezone":
		p := ctx.GetPrefix()
		if len(ctx.Args) < 2 {
			tz := getAutoBioTimezone(ctx.Ctx, s)
			return ctx.Replyf("Current AutoBio timezone: %s\n\nTo change timezone:\n- %sautobio tz Africa/Lagos\n- %sautobio tz America/New_York\n- %sautobio tz UTC", tz, p, p, p)
		}
		newTZ := ctx.Args[1]
		if _, err := time.LoadLocation(newTZ); err != nil {
			return ctx.Replyf("Invalid timezone: %q. Please use valid IANA format (e.g. Africa/Lagos, UTC, America/New_York).", newTZ)
		}
		if err := s.PutSetting(ctx.Ctx, "autobio_timezone", newTZ); err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Replyf("AutoBio timezone updated to: %s!", newTZ)

	case "now", "update":
		bioText, err := updateAutoBio(ctx.Ctx, ctx.Client)
		if err != nil {
			return ctx.Replyf("Failed to update status bio: %v", err)
		}
		return ctx.Replyf("Status bio updated!\n\nNew Bio:\n\"%s\"", bioText)

	case "status":
		enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
		statusStr := "DISABLED"
		if enabled == "true" {
			statusStr = "ENABLED"
		}
		tzStr := getAutoBioTimezone(ctx.Ctx, s)
		previewBio := generateBioText(tzStr)
		return ctx.Replyf("AutoBio Status: %s\nTimezone: %s\n\nLive Preview:\n\"%s\"", statusStr, tzStr, previewBio)
	}

	return sendAutoBioMenu(ctx, s)
}

func sendAutoBioMenu(ctx *Context, s *StoreWrapper) error {
	enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
	statusStr := "DISABLED"
	if enabled == "true" {
		statusStr = "ENABLED"
	}
	tzStr := getAutoBioTimezone(ctx.Ctx, s)

	p := ctx.GetPrefix()
	bodyText := Sprintf("╭━━━〔 AUTOBIO CONFIGURATION 〕━━━\n│ Status   : %s\n│ Timezone : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", statusStr, tzStr)

	var actionButton struct{ ID, Text string }
	if enabled == "true" {
		actionButton = struct{ ID, Text string }{ID: p + "autobio off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "autobio on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "autobio customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, Sprintf("%s AutoBio Updater", ctx.GetBotName()), buttons)
}

func sendAutoBioCustomizeGuide(ctx *Context, s *StoreWrapper) error {
	p := ctx.GetPrefix()
	tzStr := getAutoBioTimezone(ctx.Ctx, s)
	previewBio := generateBioText(tzStr)

	return ctx.Text().
		Header("AUTOBIO CUSTOMIZATION GUIDE").
		Section("Available Customizations").
		Bulletf("Set Timezone : %sautobio tz <IANA Timezone>", p).
		Bulletf("Force Update : %sautobio now", p).
		Blank().
		Section("Examples").
		Numberedf(1, "%sautobio tz Africa/Lagos", p).
		Numberedf(2, "%sautobio tz America/New_York", p).
		Numberedf(3, "%sautobio now (Force status bio refresh right now)", p).
		Blank().
		Section("Current Live Bio Preview").
		Linef("%q", previewBio).
		Reply()
}

func getAutoBioTimezone(ctx context.Context, s *StoreWrapper) string {
	tz, err := s.GetSetting(ctx, "autobio_timezone")
	if err == nil && tz != "" {
		return tz
	}
	tzGen, errGen := s.GetSetting(ctx, "timezone")
	if errGen == nil && tzGen != "" {
		return tzGen
	}
	return "UTC"
}

func generateBioText(tzStr string) string {
	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	timeFormatted := now.Format("03:04 AM")

	cliutils.AutoBioRngMutex.Lock()
	quote := cliutils.BioQuotes[cliutils.AutoBioRng.Intn(len(cliutils.BioQuotes))]
	cliutils.AutoBioRngMutex.Unlock()

	return Sprintf("⏰ %s | %s", timeFormatted, quote)
}

func updateAutoBio(ctx context.Context, client *whatsmeow.Client) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if client == nil || client.Store == nil || client.Store.ID == nil || !client.IsConnected() {
		return "", nil
	}

	s, ok := getSQLStore(client)
	if !ok || s == nil || s.GetDB() == nil {
		return "", nil
	}

	enabled, err := s.GetSetting(ctx, "autobio_enabled")
	if err != nil || enabled != "true" {
		return "", nil
	}

	tzStr := getAutoBioTimezone(ctx, s)
	bioText := generateBioText(tzStr)

	err = client.SetStatusMessage(ctx, types.SetStatusInput{Text: &bioText})
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "database is closed") || ctx.Err() != nil {
			return "", nil
		}
		slog.Error("[AutoBio] Failed to update WhatsApp status message", "err", err)
		return "", err
	}

	slog.Debug("[AutoBio] Updated WhatsApp status bio", "bio", bioText, "timezone", tzStr)
	return bioText, nil
}

func getUserTimezone(ctx context.Context, s *StoreWrapper) string {
	tz, err := s.GetSetting(ctx, "timezone")
	if err != nil || tz == "" {
		return "UTC"
	}
	return tz
}

func handleTimezone(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "set" {
		tzName := ctx.Args[1]
		if decoded, err := url.QueryUnescape(tzName); err == nil {
			tzName = decoded
		}

		if _, err := time.LoadLocation(tzName); err != nil {
			if resolved, okLoc := cliutils.ResolveTimezoneAlias(tzName); okLoc {
				tzName = resolved
			} else {
				return ctx.Replyf("Invalid timezone: %q. Please select a valid IANA timezone, Windows timezone name, or abbreviation.", tzName)
			}
		}

		err := s.PutSetting(ctx.Ctx, "timezone", tzName)
		if err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		return ctx.Replyf("Bot timezone successfully set to *%s*.", tzName)
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "page" {
		pageNum, _ := strconv.Atoi(ctx.Args[1])
		return renderTimezonePage(ctx, s, pageNum)
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "setidx" {
		idx, err := strconv.Atoi(ctx.Args[1])
		if err != nil || idx < 1 || idx > len(cliutils.SupportedTimezones) {
			return ctx.Reply("Invalid timezone selection.")
		}
		tzName := cliutils.SupportedTimezones[idx-1]
		if err := s.PutSetting(ctx.Ctx, "timezone", tzName); err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		return ctx.Replyf("Bot timezone successfully set to *%s*.", tzName)
	}

	if len(ctx.Args) == 1 {
		tzName := ctx.Args[0]
		if _, err := time.LoadLocation(tzName); err == nil {
			_ = s.PutSetting(ctx.Ctx, "timezone", tzName)
			return ctx.Replyf("Bot timezone successfully set to *%s*.", tzName)
		}
	}

	return renderTimezonePage(ctx, s, 1)
}

func renderTimezonePage(ctx *Context, s *StoreWrapper, page int) error {
	currentTZ := getUserTimezone(ctx.Ctx, s)

	pageSize := 3
	totalPages := (len(cliutils.SupportedTimezones) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := min(startIdx+pageSize, len(cliutils.SupportedTimezones))

	pageItems := cliutils.SupportedTimezones[startIdx:endIdx]
	p := ctx.GetPrefix()

	tb := ctx.Text().
		Headerf("Timezone Configuration (Page %d of %d, Total: %d)", page, totalPages, len(cliutils.SupportedTimezones)).
		Field("Current Timezone", currentTZ).
		Blank().
		Line("Select your local timezone below so automute & autounmute execute at your exact local time:").
		Blank()

	for idx, tz := range pageItems {
		globalIdx := startIdx + idx + 1
		loc, err := time.LoadLocation(tz)
		offsetStr := ""
		if err == nil {
			now := time.Now().In(loc)
			_, offset := now.Zone()
			hours := offset / 3600
			mins := (offset % 3600) / 60
			if mins < 0 {
				mins = -mins
			}
			offsetStr = Sprintf(" (UTC%+03d:%02d)", hours, mins)
		}
		tb.Numbered(globalIdx, Bold(tz)+offsetStr)
	}

	var buttons []struct{ ID, Text string }
	for idx, tz := range pageItems {
		globalIdx := startIdx + idx + 1
		btnText := tz
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%stimezone setidx %d", p, globalIdx),
			Text: btnText,
		})
	}

	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%stimezone page %d", p, nextPage),
			Text: Sprintf("Next (Page %d)", nextPage),
		})
	} else if page > 1 {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   Sprintf("%stimezone page 1", p),
			Text: "First Page",
		})
	}

	tb.Blank().
		Line("Tap a button above to select your timezone, or type:").
		Linef("%stimezone <Name> (e.g. %stimezone Africa/Lagos)", p, p)

	return sendInteractiveButtons(ctx, tb.Trimmed(), Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func handleReconfigure(ctx *Context) error {
	key := ctx.Chat.ToNonAD().String() + ":" + ctx.Sender.ToNonAD().String()
	cliutils.BotWizardMu.Lock()
	cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "name", UpdatedAt: time.Now()}
	cliutils.BotWizardMu.Unlock()

	p := ctx.GetPrefix()
	bodyText := "Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):"
	return ctx.Replyf("%s\n\n(Tip: Type %sreconfigure anytime to restart this wizard)", bodyText, p)
}

func GetSessionAuthDir(client *whatsmeow.Client) string {
	baseAuth := whatsrook.DefaultAuthDir()
	if client != nil && client.Store != nil {
		if client.Store.ID != nil && client.Store.ID.User != "" {
			return filepath.Join(baseAuth, client.Store.ID.User)
		}
	}
	return filepath.Join(baseAuth, "default")
}

func GetSessionMediaDir(client *whatsmeow.Client, subdirs ...string) string {
	base := filepath.Join(GetSessionAuthDir(client), "media")
	if len(subdirs) > 0 {
		elem := append([]string{base}, subdirs...)
		return filepath.Join(elem...)
	}
	return base
}

func ProcessAndSaveThumbnail(ctx context.Context, authDir string, data []byte, isVideo bool) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty media data")
	}

	if authDir == "" {
		authDir = filepath.Join("auth", "default")
	}
	_ = os.MkdirAll(authDir, 0755)
	targetPath := filepath.Join(authDir, "custom_menu_thumbnail.mp4")

	if isVideo {
		tempInput := filepath.Join(authDir, Sprintf("input_%d.mp4", time.Now().UnixNano()))
		if err := os.WriteFile(tempInput, data, 0644); err != nil {
			return "", errors.New("failed to save temp video: " + err.Error())
		}
		defer os.Remove(tempInput)

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tempInput,
			"-t", "5",
			"-vf", "scale='min(480,iw)':-2,fps=15",
			"-c:v", "libx264", "-preset", "fast", "-crf", "28",
			"-an", "-pix_fmt", "yuv420p",
			targetPath)

		if err := cmd.Run(); err != nil {
			slog.Warn("ffmpeg video processing failed, checking raw video fallback", "err", err)
			if len(data) <= 10*1024*1024 {
				if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
					return "", errors.New("failed to write raw video fallback: " + errWrite.Error())
				}
			} else {
				return "", errors.New("video file too large (>10MB) and ffmpeg processing failed: " + err.Error())
			}
		}
	} else {
		tempImg := filepath.Join(authDir, Sprintf("thumb_%d.jpg", time.Now().UnixNano()))
		if err := os.WriteFile(tempImg, data, 0644); err != nil {
			return "", errors.New("failed to save temp image: " + err.Error())
		}
		defer os.Remove(tempImg)

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loop", "1", "-i", tempImg,
			"-c:v", "libx264", "-t", "2", "-pix_fmt", "yuv420p",
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", targetPath)
		if err := cmd.Run(); err != nil {
			targetPath = filepath.Join(authDir, "custom_menu_thumbnail.jpg")
			if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
				return "", errors.New("failed to save raw image fallback: " + errWrite.Error())
			}
		}
	}

	return targetPath, nil
}

func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}

	msg := evt.Message
	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		msg = msg.EphemeralMessage.Message
	}
	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		msg = msg.ViewOnceMessage.Message
	} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		msg = msg.ViewOnceMessageV2.Message
	} else if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		msg = msg.ViewOnceMessageV2Extension.Message
	}

	if img := msg.GetImageMessage(); img != nil {
		return img, false, img.GetMimetype()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, true, vid.GetMimetype()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		mime := doc.GetMimetype()
		filename := strings.ToLower(doc.GetFileName())
		if strings.HasPrefix(mime, "video/") || strings.HasSuffix(filename, ".mp4") || strings.HasSuffix(filename, ".mkv") {
			return doc, true, mime
		}
		if strings.HasPrefix(mime, "image/") || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") || strings.HasSuffix(filename, ".jpeg") {
			return doc, false, mime
		}
	}

	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil && ext.GetContextInfo().QuotedMessage != nil {
		q := ext.GetContextInfo().QuotedMessage
		if q.EphemeralMessage != nil && q.EphemeralMessage.Message != nil {
			q = q.EphemeralMessage.Message
		}
		if img := q.GetImageMessage(); img != nil {
			return img, false, img.GetMimetype()
		}
		if vid := q.GetVideoMessage(); vid != nil {
			return vid, true, vid.GetMimetype()
		}
		if doc := q.GetDocumentMessage(); doc != nil {
			mime := doc.GetMimetype()
			filename := strings.ToLower(doc.GetFileName())
			if strings.HasPrefix(mime, "video/") || strings.HasSuffix(filename, ".mp4") {
				return doc, true, mime
			}
			if strings.HasPrefix(mime, "image/") || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") {
				return doc, false, mime
			}
		}
	}

	return nil, false, ""
}

func HandlePendingBotCustomizationReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	senderUser := evt.Info.Sender.ToNonAD().User
	key := evt.Info.Chat.ToNonAD().String() + ":" + evt.Info.Sender.ToNonAD().String()

	cliutils.BotWizardMu.RLock()
	session, inWizard := cliutils.PendingWizardState[key]
	cliutils.BotWizardMu.RUnlock()

	if inWizard && time.Since(session.UpdatedAt) > cliutils.WizardSessionTTL {
		cliutils.BotWizardMu.Lock()
		delete(cliutils.PendingWizardState, key)
		cliutils.BotWizardMu.Unlock()
		inWizard = false
	}

	var s *StoreWrapper
	var okStore bool
	if client != nil && client.Store != nil {
		s, okStore = getSQLStore(client)
	}
	text := utils.ExtractMessageText(evt)

	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
		Evt:    evt,
	}
	p := fakeCtx.GetPrefix()
	var prefixes []string
	if client != nil {
		prefixes = activePrefixes(ctx, client)
	} else {
		prefixes = []string{cliutils.DefaultPrefix}
	}

	if inWizard && text != "" {
		for _, pref := range prefixes {
			if pref != "" && strings.HasPrefix(text, pref) {
				cliutils.BotWizardMu.Lock()
				delete(cliutils.PendingWizardState, key)
				cliutils.BotWizardMu.Unlock()
				return false
			}
		}
	}

	if !inWizard && okStore {
		rawPrompt, _ := s.GetSetting(ctx, cliutils.BotNameAwaitingInputPrefix+senderUser)
		if rawPrompt == "true" && text != "" && !strings.HasPrefix(text, p) {
			session = cliutils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			inWizard = true
		}
	}

	if !inWizard {
		return false
	}

	slog.Info("Wizard handling step", "chat", key, "step", session.Step, "text", text)

	switch session.Step {
	case "name":
		if text == "" {
			return false
		}
		newName := strings.TrimSpace(text)
		if okStore {
			_ = s.PutSetting(ctx, cliutils.BotNameSettingKey, newName)
			DismissBotNamePrompt(ctx, s)
			_ = s.PutSetting(ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")
		}
		cliutils.ClearInstructionCache()

		cliutils.BotWizardMu.Lock()
		cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "thumb", UpdatedAt: time.Now()}
		cliutils.BotWizardMu.Unlock()

		msg := Sprintf("Bot name set to %q.\n\nBot Customization Wizard (Step 2/4)\n\nPlease upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.", newName)
		_ = fakeCtx.Reply(msg)
		return true

	case "thumb":
		downloadable, isVideo, mime := ExtractMediaFromEvent(evt)
		slog.Info("Wizard Step 2/4 (thumb): Checking media payload", "chat", key, "mime", mime, "isVideo", isVideo, "foundMedia", downloadable != nil)

		if downloadable == nil {
			slog.Warn("Wizard Step 2/4 (thumb): No image/video/document media found in message", "chat", key)
			_ = fakeCtx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) for the bot thumbnail.")
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Starting media download", "chat", key, "mime", mime)
		data, err := client.Download(ctx, downloadable)

		if err != nil || len(data) == 0 {
			slog.Error("Wizard Step 2/4 (thumb): Media download failed", "chat", key, "err", err, "dataLen", len(data))
			_ = fakeCtx.Replyf("Failed to download media for thumbnail (error: %v). Please try sending another file.", err)
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Media downloaded successfully", "chat", key, "bytesLen", len(data), "isVideo", isVideo)
		authDir := GetSessionAuthDir(client)
		targetPath, errProc := ProcessAndSaveThumbnail(ctx, authDir, data, isVideo)
		if errProc != nil {
			slog.Error("Wizard Step 2/4 (thumb): Thumbnail processing failed", "chat", key, "err", errProc)
			_ = fakeCtx.Replyf("Failed to process thumbnail: %v", errProc)
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Thumbnail saved successfully", "chat", key, "targetPath", targetPath)

		if okStore {
			_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
		}

		cliutils.BotWizardMu.Lock()
		cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "prefix", UpdatedAt: time.Now()}
		cliutils.BotWizardMu.Unlock()

		bodyText := "Bot menu thumbnail updated successfully.\n\nBot Customization Wizard (Step 3/4)\n\nPlease send the symbol or prefix you want to use (e.g. ., !, / or 'none').\n\nOr click Skip below to keep current prefix."
		buttons := []struct{ ID, Text string }{
			{ID: p + "setbot prompt_prefix", Text: "Set Prefix"},
			{ID: p + "setbot skip 3", Text: "Skip"},
		}
		_ = sendInteractiveButtons(fakeCtx, bodyText, Sprintf("%s Setup", fakeCtx.GetBotName()), buttons)
		return true

	case "prefix":
		if text == "" {
			return false
		}
		newPrefix := strings.TrimSpace(text)
		if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
			newPrefix = "empty"
		}
		if okStore {
			_ = s.PutSetting(ctx, cliutils.PrefixSettingKey, newPrefix)
		}

		cliutils.BotWizardMu.Lock()
		cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
		cliutils.BotWizardMu.Unlock()

		bodyText := Sprintf("Prefix set to %q.\n\nBot Customization Wizard (Step 4/4)\n\nPlease send the text for your bot's WhatsApp status bio.\n\nOr click Skip to finish wizard.", newPrefix)
		buttons := []struct{ ID, Text string }{
			{ID: p + "setbot prompt_bio", Text: "Set Bio"},
			{ID: p + "setbot skip 4", Text: "Skip"},
		}
		_ = sendInteractiveButtons(fakeCtx, bodyText, Sprintf("%s Setup", fakeCtx.GetBotName()), buttons)
		return true

	case "bio":
		if text == "" {
			return false
		}
		newBio := strings.TrimSpace(text)
		_ = client.SetStatusMessage(ctx, types.SetStatusInput{Text: &newBio})

		if okStore {
			DismissBotNamePrompt(ctx, s)
		}

		cliutils.BotWizardMu.Lock()
		delete(cliutils.PendingWizardState, key)
		cliutils.BotWizardMu.Unlock()

		_ = sendWizardSummaryCard(fakeCtx)
		return true
	}

	return false
}

func handleSetBot(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	p := ctx.GetPrefix()
	senderUser := ctx.Sender.ToNonAD().User
	key := ctx.Chat.ToNonAD().String() + ":" + ctx.Sender.ToNonAD().String()
	args := strings.Fields(ctx.RawArgs)

	if len(args) > 0 {
		sub := strings.ToLower(args[0])

		switch sub {
		case "wizard", "setup", "reconfigure", "reconfig":
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "prompt_name", "name_prompt":
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Please type your desired bot display name (e.g. Jarvis, Meow, Fuzzy):")

		case "prompt_thumb", "thumb_prompt":
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "thumb", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.")

		case "prompt_prefix", "prefix_prompt":
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "prefix", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Please send the command prefix symbol or word you want to use (e.g. ., !, / or 'none'):")

		case "prompt_bio", "bio_prompt":
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Please send the text for your bot's WhatsApp status bio:")

		case "skip":
			stepNum := 0
			if len(args) > 1 {
				stepNum, _ = strconv.Atoi(args[1])
			}

			if stepNum == 3 {
				cliutils.BotWizardMu.Lock()
				cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
				cliutils.BotWizardMu.Unlock()

				bodyText := "Bot Customization Wizard (Step 4/4)\n\nPlease send the text for your bot's WhatsApp status bio.\n\nOr click Skip to finish."
				buttons := []struct{ ID, Text string }{
					{ID: p + "setbot prompt_bio", Text: "Set Bio"},
					{ID: p + "setbot skip 4", Text: "Skip"},
				}
				return sendInteractiveButtons(ctx, bodyText, Sprintf("%s Setup", ctx.GetBotName()), buttons)
			}

			if stepNum == 4 || stepNum == 0 {
				cliutils.BotWizardMu.Lock()
				delete(cliutils.PendingWizardState, key)
				cliutils.BotWizardMu.Unlock()
				DismissBotNamePrompt(ctx.Ctx, s)
				return sendWizardSummaryCard(ctx)
			}

		case "page":
			pageNum := 1
			if len(args) > 1 {
				pageNum, _ = strconv.Atoi(args[1])
			}
			return sendSetBotPage(ctx, pageNum)

		case "name", "setname":
			if len(args) < 2 {
				return ctx.Replyf("Usage: %sbotname <New Name>", p)
			}
			newName := strings.Join(args[1:], " ")
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameSettingKey, newName)
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")
			cliutils.ClearInstructionCache()
			return ctx.Replyf("Bot name successfully updated to: %q!", newName)

		case "prefix", "setprefix":
			if len(args) < 2 {
				return ctx.Replyf("Usage: %ssetprefix <symbol>", p)
			}
			newPrefix := args[1]
			if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
				newPrefix = "empty"
			}
			_ = s.PutSetting(ctx.Ctx, cliutils.PrefixSettingKey, newPrefix)
			return ctx.Replyf("Command prefix updated to: %q!", newPrefix)

		case "bio", "setbio":
			if len(args) < 2 {
				return ctx.Replyf("Usage: %sbio <text>", p)
			}
			newBio := strings.Join(args[1:], " ")
			if err := ctx.Client.SetStatusMessage(ctx.Ctx, types.SetStatusInput{Text: &newBio}); err != nil {
				return ctx.Reply("Failed to update status bio: " + err.Error())
			}
			return ctx.Reply("Bot status bio updated successfully!")

		case "reset":
			authDir := GetSessionAuthDir(ctx.Client)
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameSettingKey, "")
			ResetBotNamePromptDismissed(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, cliutils.PrefixSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
			cliutils.ClearInstructionCache()
			return ctx.Reply("All bot settings (Name, Thumbnail, Prefix) reset to default values.")

		case "setup_customize":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")
			cliutils.BotWizardMu.Lock()
			cliutils.PendingWizardState[key] = cliutils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			cliutils.BotWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "setup_continue":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")
			bodyText := "BOT NAME CUSTOMIZATION RECOMMENDED: Keeping default name WhatsRook is fine. You can start the customization wizard anytime using " + p + "reconfigure:"
			buttons := []struct{ ID, Text string }{
				{ID: p + "reconfigure", Text: "Start Wizard"},
				{ID: p + "setbot setup_ignore", Text: "Keep Default"},
			}
			return sendInteractiveButtons(ctx, bodyText, Sprintf("Powered by %s", ctx.GetBotName()), buttons)

		case "setup_ignore":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, cliutils.BotNameAwaitingInputPrefix+senderUser, "")
			return ctx.Replyf("Kept default bot name. Change anytime using %sreconfigure or %ssetbot", p, p)
		}
	}

	return sendSetBotPage(ctx, 1)
}

func sendSetBotPage(ctx *Context, pageNum int) error {
	p := ctx.GetPrefix()
	botName := ctx.GetBotName()
	curPrefix := p
	if curPrefix == "" {
		curPrefix = "(none)"
	}

	thumbStatus := "None (Default)"
	if s, ok := getStore(ctx); ok {
		if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
			if _, errStat := os.Stat(custom); errStat == nil {
				thumbStatus = "Custom Thumbnail"
			}
		}
	}

	tb := ctx.Text().
		Header("BOT CUSTOMIZATION").
		Field("Name", botName).
		Field("Thumbnail", thumbStatus).
		Field("Prefix", curPrefix)

	var buttons []struct{ ID, Text string }

	switch pageNum {
	case 1:
		buttons = []struct{ ID, Text string }{
			{ID: p + "reconfigure", Text: "Wizard"},
			{ID: p + "setbot prompt_name", Text: "Bot Name"},
			{ID: p + "setbot page 2", Text: "Next ▶️"},
		}
	case 2:
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot prompt_thumb", Text: "Thumbnail"},
			{ID: p + "setbot prompt_prefix", Text: "Prefix"},
			{ID: p + "setbot page 3", Text: "Next ▶️"},
		}
	default:
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot prompt_bio", Text: "Bio"},
			{ID: p + "setbot reset", Text: "Reset All"},
			{ID: p + "setbot page 1", Text: "◀️ Back"},
		}
	}

	return sendInteractiveButtons(ctx, tb.Trimmed(), Sprintf("%s Settings", botName), buttons)
}

func sendWizardSummaryCard(ctx *Context) error {
	p := ctx.GetPrefix()
	botName := ctx.GetBotName()
	curPrefix := p
	if curPrefix == "" {
		curPrefix = "(none)"
	}

	thumbStatus := "None (Default)"
	if s, ok := getStore(ctx); ok {
		DismissBotNamePrompt(ctx.Ctx, s)
		if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
			if _, errStat := os.Stat(custom); errStat == nil {
				thumbStatus = "Custom Thumbnail"
			}
		}
	}

	return ctx.Text().
		Header("Bot Customization Completed!").
		Section("BOT CONFIGURATION").
		Field("Name", botName).
		Field("Thumbnail", thumbStatus).
		Field("Prefix", curPrefix).
		Blank().
		Linef("Type %smenu anytime to view your updated bot commands menu! (Or %sreconfigure to adjust settings)", p, p).
		Reply()
}

func handleLikeStatusCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	statusKey := "likestatus_status"

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendLikeStatusMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("LikeStatus ENABLED. The bot will automatically react to status broadcasts with love emojis.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("LikeStatus DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("LikeStatus DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("LikeStatus ENABLED.")

	case "customize", "custom", "help":
		return sendLikeStatusCustomizeGuide(ctx)

	default:
		return ctx.Replyf("Usage: %slikestatus [on|off|toggle|customize]", ctx.GetPrefix())
	}
}

func sendLikeStatusMenu(ctx *Context, s *StoreWrapper) error {
	status, _ := s.GetSetting(ctx.Ctx, "likestatus_status")
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := Sprintf("╭━━━〔 LIFESTATUS AUTO-REACTION 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically reacts to status broadcasts with random love emojis (❤️, 💕, 💖, 💗, 💓, 💞, 💘, 💌, 🥰, 😍).", strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "likestatus off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "likestatus on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "likestatus customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, Sprintf("%s LikeStatus", ctx.GetBotName()), buttons)
}

func sendLikeStatusCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("LIKESTATUS CUSTOMIZATION GUIDE").
		Section("Description").
		Line("When enabled, WhatsRook will automatically react to every incoming status/story broadcast with a randomly selected love emoji.").
		Blank().
		Section("Commands").
		Bulletf("Enable Auto-Like  : %slikestatus on", p).
		Bulletf("Disable Auto-Like : %slikestatus off", p).
		Bulletf("Toggle Status     : %slikestatus toggle", p).
		Reply()
}

func handlePrefix(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	if ctx.RawArgs == "" {
		raw, err := s.GetSetting(ctx.Ctx, cliutils.PrefixSettingKey)
		if err != nil {
			return sendText(ctx, "Failed to retrieve prefix configuration.")
		}
		if raw == "" {
			return ctx.SendTextf("Prefix: %q (default)", cliutils.DefaultPrefix)
		}
		return ctx.SendTextf("Prefix(es): %s", raw)
	}

	parts := strings.Fields(ctx.RawArgs)
	if len(parts) == 0 {
		return sendText(ctx, "Usage: prefix <symbol or word...> (use 'empty' or 'none' for no prefix required)")
	}

	var parsedParts []string
	for _, p := range parts {
		if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
			parsedParts = append(parsedParts, "empty")
		} else if isWordPrefix(p) {
			parsedParts = append(parsedParts, p)
		} else {
			for _, r := range p {
				parsedParts = append(parsedParts, string(r))
			}
		}
	}

	stored := strings.Join(parsedParts, " ")
	if err := s.PutSetting(ctx.Ctx, cliutils.PrefixSettingKey, stored); err != nil {
		return sendText(ctx, "Failed to update prefix configuration.")
	}

	display := make([]string, 0, len(parsedParts))
	for _, p := range parsedParts {
		if p == "empty" {
			display = append(display, "(no prefix)")
		} else {
			display = append(display, Sprintf("%q", p))
		}
	}
	return ctx.SendTextf("Prefix updated to: %s", strings.Join(display, ", "))
}

func handlePrivacy(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only owner/sudo users can view or modify account privacy settings.")
	}

	if len(ctx.Args) >= 2 {
		name := strings.ToLower(ctx.Args[0])
		val := strings.ToLower(ctx.Args[1])
		return updatePrivacySetting(ctx, name, val)
	}

	privacy, err := ctx.Client.TryFetchPrivacySettings(ctx.Ctx, false)
	if err != nil {
		pSettings := ctx.Client.GetPrivacySettings(ctx.Ctx)
		privacy = &pSettings
	}

	p := ctx.GetPrefix()
	tb := ctx.Text().Header("WhatsApp Account Privacy Settings")

	if privacy != nil {
		tb.Field("Last Seen", string(privacy.LastSeen)).
			Field("Profile Photo", string(privacy.Profile)).
			Field("Status", string(privacy.Status)).
			Field("Read Receipts", string(privacy.ReadReceipts)).
			Field("Group Add", string(privacy.GroupAdd)).
			Field("Online", string(privacy.Online)).
			Field("Call Add", string(privacy.CallAdd))
	} else {
		tb.Line("Privacy settings unavailable.")
	}

	tb.Blank().Line("Tap a button below to configure privacy:")

	buttons := []struct{ ID, Text string }{
		{
			ID:   Sprintf("%sprivacy last all", p),
			Text: "Last Seen: Everyone",
		},
		{
			ID:   Sprintf("%sprivacy last contacts", p),
			Text: "Last Seen: Contacts",
		},
		{
			ID:   Sprintf("%sprivacy last none", p),
			Text: "Last Seen: Nobody",
		},
	}

	return sendInteractiveButtons(ctx, tb.Trimmed(), "Powered by WhatsRook", buttons)
}

func updatePrivacySetting(ctx *Context, nameStr, valStr string) error {
	var name types.PrivacySettingType
	var val types.PrivacySetting

	switch nameStr {
	case "last", "lastseen":
		name = types.PrivacySettingTypeLastSeen
	case "profile", "photo", "pfp":
		name = types.PrivacySettingTypeProfile
	case "status", "sw":
		name = types.PrivacySettingTypeStatus
	case "read", "readreceipts", "blue":
		name = types.PrivacySettingTypeReadReceipts
	case "group", "groupadd":
		name = types.PrivacySettingTypeGroupAdd
	case "online":
		name = types.PrivacySettingTypeOnline
	case "call", "calladd":
		name = types.PrivacySettingTypeCallAdd
	default:
		name = types.PrivacySettingType(nameStr)
	}

	switch valStr {
	case "everyone", "all":
		val = types.PrivacySettingAll
	case "contacts":
		val = types.PrivacySettingContacts
	case "nobody", "none":
		val = types.PrivacySettingNone
	case "match_last_seen", "matchlastseen":
		val = types.PrivacySettingMatchLastSeen
	default:
		val = types.PrivacySetting(valStr)
	}

	_, err := ctx.Client.SetPrivacySetting(ctx.Ctx, name, val)
	if err != nil {
		return ctx.Replyf("Failed to update privacy setting %s: %v", name, err)
	}

	return ctx.Replyf("Successfully updated privacy setting *%s* to *%s*.", name, val)
}

func handleSetCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: setcmd [command_name] (reply to a sticker)")
	}
	cmdName := strings.ToLower(ctx.Args[0])

	_, exists := Get(cmdName)
	if !exists {
		return ctx.Replyf(" Command %q does not exist.", cmdName)
	}

	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("Please reply to a sticker message.")
	}

	stk := quoted.StickerMessage
	if len(stk.FileSHA256) == 0 {
		return ctx.Reply("Invalid sticker (no FileSHA256 found).")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()
	shaHex := hex.EncodeToString(stk.FileSHA256)

	_, err := db.Exec(ctx.Ctx, `
		INSERT INTO bot_sticker_cmds (our_jid, sticker_sha256, command_name)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, sticker_sha256) DO UPDATE SET command_name=excluded.command_name
	`, ourJID, shaHex, cmdName)
	if err != nil {
		return ctx.Reply("Failed to link sticker command.")
	}

	return ctx.Replyf("Sticker linked to command %q.", cmdName)
}

func handleDelCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	quoted := ctx.GetQuotedMessage()
	if quoted != nil && quoted.StickerMessage != nil {
		stk := quoted.StickerMessage
		if len(stk.FileSHA256) == 0 {
			return ctx.Reply("Invalid sticker (no FileSHA256 found).")
		}
		shaHex := hex.EncodeToString(stk.FileSHA256)

		res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_sticker_cmds WHERE our_jid=$1 AND sticker_sha256=$2`, ourJID, shaHex)
		if err != nil {
			return ctx.Reply("Failed to remove sticker command.")
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return ctx.Reply("Mapped sticker not found.")
		}
		return ctx.Reply("Sticker link removed.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage:\n- delcmd [command_name]\n- delcmd (replying to a mapped sticker)")
	}

	cmdName := strings.ToLower(ctx.Args[0])
	res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_sticker_cmds WHERE our_jid=$1 AND command_name=$2`, ourJID, cmdName)
	if err != nil {
		return ctx.Reply("Failed to remove sticker command.")
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ctx.Replyf("No sticker linked to command %q.", cmdName)
	}

	return ctx.Replyf("Mapped sticker(s) for command %q removed.", cmdName)
}

func handleGetCmd(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	rows, err := db.Query(ctx.Ctx, `SELECT sticker_sha256, command_name FROM bot_sticker_cmds WHERE our_jid=$1`, ourJID)
	if err != nil {
		return ctx.Reply("Failed to query sticker commands.")
	}
	defer rows.Close()

	tb := ctx.Text().Header("Sticker Command Mappings")

	count := 0
	for rows.Next() {
		var sha, cmdName string
		if err := rows.Scan(&sha, &cmdName); err == nil {
			tb.Bulletf("%s -> %s", sha[:8]+"...", cmdName)
			count++
		}
	}

	if count == 0 {
		return ctx.Reply("No sticker commands configured.")
	}

	return tb.Reply()
}

func handleDisableCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %sdisablecmd <command_name>\nExample:\n- %sdisablecmd weather", p, p)
	}

	cmdName := strings.ToLower(ctx.Args[0])
	if cmdName == "enablecmd" || cmdName == "disablecmd" {
		return ctx.Reply("Cannot disable core system commands.")
	}

	_, exists := Get(cmdName)
	if !exists {
		return ctx.Replyf("Command %q does not exist.", cmdName)
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "disabled_commands")
	if err != nil {
		return ctx.Reply("Failed to retrieve disabled commands setting.")
	}

	disabled := strings.Fields(raw)
	for _, d := range disabled {
		if strings.EqualFold(d, cmdName) {
			return ctx.Replyf("Command %q is already disabled.", cmdName)
		}
	}

	disabled = append(disabled, cmdName)
	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(disabled, " ")); err != nil {
		return ctx.Reply("Failed to disable command.")
	}

	return ctx.Replyf("Command %q has been disabled.", cmdName)
}

func handleEnableCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage:\n- %senablecmd <command_name>\nExample:\n- %senablecmd weather", p, p)
	}

	cmdName := strings.ToLower(ctx.Args[0])
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "disabled_commands")
	if err != nil {
		return err
	}

	disabled := strings.Fields(raw)
	found := false
	var newDisabled []string

	for _, d := range disabled {
		if strings.EqualFold(d, cmdName) {
			found = true
		} else {
			newDisabled = append(newDisabled, d)
		}
	}

	if !found {
		return ctx.Replyf("Command %q is not currently disabled.", cmdName)
	}

	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(newDisabled, " ")); err != nil {
		return ctx.Reply("Failed to enable command.")
	}

	return ctx.Replyf("Command %q has been enabled.", cmdName)
}

func handleAutoVV(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		curr, _ := s.GetSetting(ctx.Ctx, "autovv")
		if curr == "" {
			curr = "off"
		}
		mode, _ := s.GetSetting(ctx.Ctx, "autovv_mode")
		if mode == "" {
			mode = "dm"
		}
		p := ctx.GetPrefix()
		modeText := "DM (Private to Owner)"
		if mode == "public" || mode == "chat" {
			modeText = "Public (Same Chat)"
		}

		bodyText := Sprintf("╭━━━〔 AUTO-VIEWONCE SETTINGS 〕━━━\n│ Status : %s\n│ Mode   : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically intercepts incoming ViewOnce media and re-uploads fresh non-expiring media.\n\nModes:\n• Private (DM): Unwrapped media is saved & sent privately to your DM.\n• Public (Chat): Unwrapped media is posted in the chat where it was received.", strings.ToUpper(curr), modeText)
		var actionButton struct{ ID, Text string }
		if curr == "on" {
			actionButton = struct{ ID, Text string }{ID: p + "autovv off", Text: "Deactivate"}
		} else {
			actionButton = struct{ ID, Text string }{ID: p + "autovv on", Text: "Activate"}
		}
		var modeButton struct{ ID, Text string }
		if mode == "public" || mode == "chat" {
			modeButton = struct{ ID, Text string }{ID: p + "autovv dm", Text: "Switch to DM (Private)"}
		} else {
			modeButton = struct{ ID, Text string }{ID: p + "autovv public", Text: "Switch to Public (Chat)"}
		}
		buttons := []struct{ ID, Text string }{
			actionButton,
			modeButton,
			{ID: p + "autovv customize", Text: "Guide"},
		}
		return sendInteractiveButtons(ctx, bodyText, Sprintf("%s AutoVV Settings", ctx.GetBotName()), buttons)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "autovv", "on")
		return ctx.Reply("Auto ViewOnce forwarding ENABLED.")
	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "autovv", "off")
		return ctx.Reply("Auto ViewOnce forwarding DISABLED.")
	case "dm", "private":
		_ = s.PutSetting(ctx.Ctx, "autovv_mode", "dm")
		return ctx.Reply("Auto ViewOnce delivery mode set to PRIVATE (Owner DM).")
	case "public", "chat":
		_ = s.PutSetting(ctx.Ctx, "autovv_mode", "public")
		return ctx.Reply("Auto ViewOnce delivery mode set to PUBLIC (Same Chat).")
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autovv")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autovv", "off")
			return ctx.Reply("Auto ViewOnce forwarding DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autovv", "on")
		return ctx.Reply("Auto ViewOnce forwarding ENABLED.")
	case "customize", "custom", "help":
		p := ctx.GetPrefix()
		return ctx.Replyf("╭━━━〔 AUTOVV GUIDE 〕━━━\n\nCommands:\n• %sautovv on\n• %sautovv off\n• %sautovv dm (Private DM)\n• %sautovv public (Group/Chat)\n• %sautovv toggle\n\nAutomatically intercepts ViewOnce media sent in chats, downloads media bytes immediately to prevent CDN expiry, and forwards clean unwrapped media to your DM or the public chat.", p, p, p, p, p)
	default:
		return ctx.Replyf("Usage: %sautovv [on|off|dm|public|toggle|customize]", ctx.GetPrefix())
	}
}

func handleAutoStatusSave(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		curr, _ := s.GetSetting(ctx.Ctx, "autostatussave")
		if curr == "" {
			curr = "off"
		}
		p := ctx.GetPrefix()
		bodyText := Sprintf("╭━━━〔 AUTO-STATUS SAVER 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically saves incoming WhatsApp status broadcasts to your DM.", strings.ToUpper(curr))
		var actionButton struct{ ID, Text string }
		if curr == "on" {
			actionButton = struct{ ID, Text string }{ID: p + "autostatus off", Text: "Deactivate"}
		} else {
			actionButton = struct{ ID, Text string }{ID: p + "autostatus on", Text: "Activate"}
		}
		buttons := []struct{ ID, Text string }{
			actionButton,
			{ID: p + "autostatus customize", Text: "Customize"},
		}
		return sendInteractiveButtons(ctx, bodyText, Sprintf("%s AutoStatus", ctx.GetBotName()), buttons)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "on")
		return ctx.Reply("Auto Status saving ENABLED. incoming status updates will be sent to your DM.")
	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "off")
		return ctx.Reply("Auto Status saving DISABLED.")
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autostatussave")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autostatussave", "off")
			return ctx.Reply("Auto Status saving DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "on")
		return ctx.Reply("Auto Status saving ENABLED.")
	case "customize", "custom", "help":
		p := ctx.GetPrefix()
		return ctx.Replyf("╭━━━〔 AUTOSTATUS GUIDE 〕━━━\n\nCommands:\n• %sautostatus on\n• %sautostatus off\n• %sautostatus toggle\n\nAutomatically intercepts contacts' status broadcasts and forwards them to your DM.", p, p, p)
	default:
		return ctx.Replyf("Usage: %sautostatus [on|off|toggle|customize]", ctx.GetPrefix())
	}
}
