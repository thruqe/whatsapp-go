package plugins

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/rand"
	"net/http"
	"net/url"
	"slices"
	"unicode"

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
	"whatsrook/cmd/store"
	"whatsrook/cmd/utils"
	"whatsrook/logger"
)

func resetAFKUserTracker() {
	utils.AfkUserTrackerLock.Lock()
	utils.AfkUserTracker = make(map[string]*utils.UserAFKState)
	utils.AfkUserTrackerLock.Unlock()
}

func init() {
	sort.Strings(utils.SupportedTimezones)

	Register(&Command{
		Name:        "afk",
		Alias:       "away",
		Description: "Set or customize your Away-From-Keyboard (AFK) status with customizable templates, @ placeholders, and last active tracking",
		Category:    "tools",
		IsPublic:    false,
		Handler:     handleAFK,
	})
	Register(&Command{
		Name:        "autobio",
		Alias:       "bioauto",
		Description: "Auto-update WhatsApp status bio every minute with time & inspirational quotes",
		Category:    "owner",
		IsPublic:    true,
		Handler:     handleAutoBio,
	})
	Register(&Command{
		Name:        "timezone",
		Alias:       "tz",
		Description: "View or configure timezone for automute schedules via poll replies",
		Category:    "group",
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
		Description: "View and update WhatsApp privacy settings (Last Seen, Profile Photo, Status, Read Receipts) via poll replies",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handlePrivacy,
	})
	Register(&Command{
		Name:        "setcmd",
		Description: "Link a sticker to a command trigger. Usage: setcmd [command_name] (replying to a sticker)",
		Category:    "tools",
		Handler:     handleSetCmd,
	})
	Register(&Command{
		Name:        "delcmd",
		Description: "Unlink a sticker from a command trigger. Usage: delcmd [command_name] or reply to a mapped sticker",
		Category:    "tools",
		Handler:     handleDelCmd,
	})
	Register(&Command{
		Name:        "getcmd",
		Description: "List all mapped sticker commands",
		Category:    "tools",
		Handler:     handleGetCmd,
	})
	Register(&Command{
		Name:        "discmd",
		Alias:       "disablecmd",
		Description: "Disable a command globally for normal users",
		Category:    "owner",
		Handler:     handleDisableCmd,
	})
	Register(&Command{
		Name:        "encmd",
		Alias:       "enablecmd",
		Description: "Enable a previously disabled command",
		Category:    "owner",
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
	Register(&Command{
		Name:        "autoreact",
		Alias:       "reactauto, autoemoji",
		Description: "Automatically react with emojis to incoming messages. Usage: autoreact [on|off|toggle|emoji <emojis...>|scope <all|group|dm>]",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleAutoReactCmd,
	})
	Register(&Command{
		Name:        "autoread",
		Alias:       "readauto, autoblue, readreceipt",
		Description: "Automatically mark incoming messages and status broadcasts as read. Usage: autoread [on|off|toggle|status <on|off>|scope <all|group|dm>]",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleAutoReadCmd,
	})
}

func UpdateOwnerLastActive(ctx context.Context, s *StoreWrapper) {
	now := time.Now()
	utils.AFKMu.Lock()
	utils.LastActiveCache = now
	utils.AFKMu.Unlock()

	if s != nil {
		_ = s.PutSetting(ctx, utils.AFKLastActiveKey, now.Format(time.RFC3339))
	}
}

func GetOwnerLastActive(ctx context.Context, s *StoreWrapper) time.Time {
	utils.AFKMu.RLock()
	cached := utils.LastActiveCache
	utils.AFKMu.RUnlock()
	if !cached.IsZero() {
		return cached
	}

	if s != nil {
		if val, err := s.GetSetting(ctx, utils.AFKLastActiveKey); err == nil && val != "" {
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
		_ = s.PutSetting(ctx.Ctx, utils.AFKStatusKey, "off")
		_ = s.PutSetting(ctx.Ctx, utils.AFKReasonKey, "")
		_ = s.PutSetting(ctx.Ctx, utils.AFKTimeKey, "")
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
			curr, _ := s.GetSetting(ctx.Ctx, utils.AFKTemplateKey)
			if curr == "" {
				curr = utils.DefaultAFKTemplate
			}
			return ctx.Reply("Current AFK Message Template:\n\n" + curr)
		}
		newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") {
			_ = s.PutSetting(ctx.Ctx, utils.AFKTemplateKey, "")
			return ctx.Reply("AFK message template reset to default.")
		}
		if err := s.PutSetting(ctx.Ctx, utils.AFKTemplateKey, newTpl); err != nil {
			return ctx.Reply("Failed to save AFK template: " + err.Error())
		}
		return ctx.Reply("Custom AFK message template updated successfully!\n\nUse `" + ctx.GetPrefix() + "afk msg reset` to restore default.")

	case "media":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK media.")
		}
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, utils.AFKMediaKey)
			if curr == "" {
				return ctx.Reply("No custom AFK media URL set.")
			}
			return ctx.Reply("Current AFK Media URL: " + curr)
		}
		urlVal := strings.TrimSpace(args[1])
		if strings.EqualFold(urlVal, "clear") || strings.EqualFold(urlVal, "off") || strings.EqualFold(urlVal, "none") {
			_ = s.PutSetting(ctx.Ctx, utils.AFKMediaKey, "")
			return ctx.Reply("AFK media URL cleared.")
		}
		if err := s.PutSetting(ctx.Ctx, utils.AFKMediaKey, urlVal); err != nil {
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

	_ = s.PutSetting(ctx.Ctx, utils.AFKStatusKey, "on")
	_ = s.PutSetting(ctx.Ctx, utils.AFKReasonKey, reason)
	_ = s.PutSetting(ctx.Ctx, utils.AFKTimeKey, nowStr)
	_ = s.PutSetting(ctx.Ctx, utils.AFKLastActiveKey, lastActiveStr)
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
		status, _ := s.GetSetting(ctx, utils.AFKStatusKey)
		if status == "on" {
			if !strings.HasPrefix(strings.TrimSpace(text), ".") && !strings.HasPrefix(strings.TrimSpace(text), "/") && !strings.HasPrefix(strings.TrimSpace(text), "!") && !strings.HasPrefix(strings.TrimSpace(text), "#") {
				_ = s.PutSetting(ctx, utils.AFKStatusKey, "off")
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

	status, _ := s.GetSetting(ctx, utils.AFKStatusKey)
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

	utils.AfkUserTrackerLock.Lock()
	uState, okState := utils.AfkUserTracker[senderJID.User]
	if !okState {
		uState = &utils.UserAFKState{}
		utils.AfkUserTracker[senderJID.User] = uState
	}
	if time.Since(uState.LastSent) < 1*time.Minute {
		utils.AfkUserTrackerLock.Unlock()
		return false
	}
	alreadySent := uState.HasSent
	uState.LastSent = time.Now()
	uState.HasSent = true
	utils.AfkUserTrackerLock.Unlock()

	reason, _ := s.GetSetting(ctx, utils.AFKReasonKey)
	if reason == "" {
		reason = "AFK (No reason specified)"
	}
	afkTime, _ := s.GetSetting(ctx, utils.AFKTimeKey)
	if afkTime == "" {
		afkTime = time.Now().Format("2006-01-02 15:04:05 MST")
	}
	lastActiveStr, _ := s.GetSetting(ctx, utils.AFKLastActiveKey)
	if lastActiveStr == "" {
		lastActiveStr = GetOwnerLastActive(ctx, s).Format("2006-01-02 15:04:05 MST")
	}

	template, _ := s.GetSetting(ctx, utils.AFKTemplateKey)
	if template == "" {
		template = utils.DefaultAFKTemplate
	}

	userTag := "@" + senderJID.User
	groupName := evt.Info.Chat.String()

	randomFact := utils.GetRandomFact(ctx)
	randomQuote := utils.GetRandomQuote(ctx)
	randomJoke := utils.GetRandomJoke(ctx)
	randomRizz := utils.GetRandomRizz(ctx)

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

	mediaURL, _ := s.GetSetting(ctx, utils.AFKMediaKey)
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
	case "on", "enable", "true", "start", "activate":
		if err := s.PutSetting(ctx.Ctx, "autobio_enabled", "true"); err != nil {
			return ctx.Reply("Failed to enable AutoBio.")
		}
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply("AutoBio ENABLED! Status bio will update every minute with local time and quotes.")

	case "off", "disable", "false", "stop", "deactivate":
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

	bodyText := NewText().
		Header("AUTOBIO CONFIGURATION").
		Field("Status", statusStr).
		Field("Timezone", tzStr).
		Blank().
		Line("Choose an option below to change status or view customization options.").
		Trimmed()

	actionText := "Activate"
	if enabled == "true" {
		actionText = "Deactivate"
	}

	options := []string{
		actionText,
		"Customize",
	}

	return sendPollReply(ctx, bodyText, options)
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

	utils.AutoBioRngMutex.Lock()
	quote := utils.BioQuotes[utils.AutoBioRng.Intn(len(utils.BioQuotes))]
	utils.AutoBioRngMutex.Unlock()

	return Sprintf("⏰ %s | %s", timeFormatted, quote)
}

func updateAutoBio(ctx context.Context, client *whatsmeow.Client) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if client == nil || client.Store == nil || client.Store.ID == nil || !client.IsConnected() || !client.IsLoggedIn() {
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
		if !client.IsConnected() || !client.IsLoggedIn() || errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "database is closed") || strings.Contains(err.Error(), "not connected") || strings.Contains(err.Error(), "disconnected") || strings.Contains(err.Error(), "timed out") || ctx.Err() != nil {
			return "", nil
		}
		logger.Error("AutoBio: Failed to update bio", "err", err)
		return "", err
	}

	logger.Debug("AutoBio: Bio updated", "bio", bioText, "timezone", tzStr)
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
			if resolved, okLoc := utils.ResolveTimezoneAlias(tzName); okLoc {
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
		if err != nil || idx < 1 || idx > len(utils.SupportedTimezones) {
			return ctx.Reply("Invalid timezone selection.")
		}
		tzName := utils.SupportedTimezones[idx-1]
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
	totalPages := (len(utils.SupportedTimezones) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := min(startIdx+pageSize, len(utils.SupportedTimezones))

	pageItems := utils.SupportedTimezones[startIdx:endIdx]
	p := ctx.GetPrefix()

	tb := ctx.Text().
		Headerf("Timezone Configuration (Page %d of %d, Total: %d)", page, totalPages, len(utils.SupportedTimezones)).
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

	var options []string
	for idx, tz := range pageItems {
		globalIdx := startIdx + idx + 1
		optText := Sprintf("%d. %s", globalIdx, tz)
		if len(optText) > 40 {
			optText = optText[:37] + "..."
		}
		options = append(options, optText)
	}

	if page < totalPages {
		nextPage := page + 1
		options = append(options, Sprintf("Next (Page %d)", nextPage))
	} else if page > 1 {
		options = append(options, "First Page")
	}

	tb.Blank().
		Line("Select an option from the poll below or type:").
		Linef("%stimezone <Name> (e.g. %stimezone Africa/Lagos)", p, p)

	return sendPollReply(ctx, tb.Trimmed(), options)
}

func handleReconfigure(ctx *Context) error {
	key := ctx.Chat.ToNonAD().String() + ":" + ctx.Sender.ToNonAD().String()
	utils.BotWizardMu.Lock()
	utils.PendingWizardState[key] = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
	utils.BotWizardMu.Unlock()

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

	detectedMime := http.DetectContentType(data)
	if strings.HasPrefix(detectedMime, "image/") {
		isVideo = false
	} else if strings.HasPrefix(detectedMime, "video/") {
		isVideo = true
	} else if len(data) >= 12 && string(data[4:8]) == "ftyp" { // MP4/MOV container magic bytes
		isVideo = true
	} else if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF { // JPEG magic bytes
		isVideo = false
	} else if len(data) >= 8 && string(data[1:4]) == "PNG" { // PNG magic bytes
		isVideo = false
	}

	if isVideo {
		targetPath := filepath.Join(authDir, "custom_menu_thumbnail.mp4")
		_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
		_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpeg"))
		_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.png"))
		_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.webp"))

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
			logger.Warn("ffmpeg video processing failed, checking raw video fallback", "err", err)
			if len(data) <= 10*1024*1024 {
				if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
					return "", errors.New("failed to write raw video fallback: " + errWrite.Error())
				}
			} else {
				return "", errors.New("video file too large (>10MB) and ffmpeg processing failed: " + err.Error())
			}
		}
		return targetPath, nil
	}

	// Save image directly as custom_menu_thumbnail.jpg without converting to video
	targetPath := filepath.Join(authDir, "custom_menu_thumbnail.jpg")
	_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
	_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.png"))
	_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.webp"))
	if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
		return "", errors.New("failed to save raw image thumbnail: " + errWrite.Error())
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

	utils.BotWizardMu.RLock()
	session, inWizard := utils.PendingWizardState[key]
	utils.BotWizardMu.RUnlock()

	if inWizard && time.Since(session.UpdatedAt) > utils.WizardSessionTTL {
		utils.BotWizardMu.Lock()
		delete(utils.PendingWizardState, key)
		utils.BotWizardMu.Unlock()
		inWizard = false
	}

	var s *StoreWrapper
	var okStore bool
	if client != nil && client.Store != nil {
		s, okStore = getSQLStore(client)
	}
	text := whatsrook.ExtractMessageText(evt)

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
		prefixes = []string{utils.DefaultPrefix}
	}

	if inWizard && text != "" {
		// Build the full set of prefixes to check against, always including the default "."
		// so that e.g. ".menu" is caught even when the configured prefix is "!" or similar.
		prefixSet := make([]string, 0, len(prefixes)+1)
		prefixSet = append(prefixSet, prefixes...)
		hasDot := slices.Contains(prefixes, utils.DefaultPrefix)
		if !hasDot {
			prefixSet = append(prefixSet, utils.DefaultPrefix)
		}

		for _, pref := range prefixSet {
			if pref == "" {
				continue
			}
			// Use a length-aware check: if the candidate text is strictly longer than the
			// prefix itself, it looks like a command invocation (e.g. ".menu"), not a bare
			// prefix entry (e.g. "."). Cancel the wizard in that case.
			if len(text) > len(pref) && strings.HasPrefix(text, pref) {
				utils.BotWizardMu.Lock()
				delete(utils.PendingWizardState, key)
				utils.BotWizardMu.Unlock()
				return false
			}
		}
	}

	if !inWizard && okStore {
		rawPrompt, _ := s.GetSetting(ctx, utils.BotNameAwaitingInputPrefix+senderUser)
		if rawPrompt == "true" && text != "" && !strings.HasPrefix(text, p) {
			session = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			inWizard = true
		}
	}

	if !inWizard {
		return false
	}

	logger.Info("Wizard handling step", "chat", key, "step", session.Step, "text", text)

	switch session.Step {
	case "name":
		if text == "" {
			return false
		}
		newName := strings.TrimSpace(text)
		if okStore {
			_ = s.PutSetting(ctx, utils.BotNameSettingKey, newName)
			DismissBotNamePrompt(ctx, s)
			_ = s.PutSetting(ctx, utils.BotNameAwaitingInputPrefix+senderUser, "")
		}
		utils.ClearInstructionCache()

		utils.BotWizardMu.Lock()
		utils.PendingWizardState[key] = utils.WizardSession{Step: "thumb", UpdatedAt: time.Now()}
		utils.BotWizardMu.Unlock()

		bodyText := Sprintf("Bot name set to %q.\n\nBot Customization Wizard (Step 2/4)\n\nPlease upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.\n\nOr select Skip below to keep the default thumbnail.", newName)
		_ = sendPollReply(fakeCtx, bodyText, []string{"Skip"})
		return true

	case "thumb":
		// Poll-vote events (e.g. the user clicking "Skip") arrive as PollUpdateMessages
		// with no media. We must NOT intercept them here — let Dispatch handle the callback.
		if evt.Message.GetPollUpdateMessage() != nil {
			return false
		}
		downloadable, isVideo, mime := ExtractMediaFromEvent(evt)
		logger.Info("Wizard Step 2/4 (thumb): Checking media payload", "chat", key, "mime", mime, "isVideo", isVideo, "foundMedia", downloadable != nil)

		if downloadable == nil {
			logger.Warn("Wizard Step 2/4 (thumb): No image/video/document media found in message", "chat", key)
			_ = fakeCtx.Reply("Upload a video or image to use as thumbnail.")
			return true
		}

		logger.Info("Wizard Step 2/4 (thumb): Starting media download", "chat", key, "mime", mime)
		data, err := client.Download(ctx, downloadable)

		if err != nil || len(data) == 0 {
			logger.Error("Wizard Step 2/4 (thumb): Media download failed", "chat", key, "err", err, "dataLen", len(data))
			_ = fakeCtx.Replyf("Failed to download media for thumbnail (error: %v). Please try sending another file.", err)
			return true
		}

		logger.Info("Wizard Step 2/4 (thumb): Media downloaded successfully", "chat", key, "bytesLen", len(data), "isVideo", isVideo)
		authDir := GetSessionAuthDir(client)
		targetPath, errProc := ProcessAndSaveThumbnail(ctx, authDir, data, isVideo)
		if errProc != nil {
			logger.Error("Wizard Step 2/4 (thumb): Thumbnail processing failed", "chat", key, "err", errProc)
			_ = fakeCtx.Replyf("Failed to process thumbnail: %v", errProc)
			return true
		}

		logger.Info("Wizard Step 2/4 (thumb): Thumbnail saved successfully", "chat", key, "targetPath", targetPath)

		if okStore {
			_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
		}

		utils.BotWizardMu.Lock()
		utils.PendingWizardState[key] = utils.WizardSession{Step: "prefix", UpdatedAt: time.Now()}
		utils.BotWizardMu.Unlock()

		bodyText := "Bot menu thumbnail updated successfully.\n\nBot Customization Wizard (Step 3/4)\n\nPlease type the symbol or prefix you want to use (e.g. ., !, / or 'none') and send it as a message.\n\nOr select Skip below to keep current prefix."
		options := []string{
			"Skip",
		}
		_ = sendPollReply(fakeCtx, bodyText, options)
		return true

	case "prefix":
		if text == "" {
			return false
		}
		newPrefix := strings.TrimSpace(text)
		if _, err := validateBotPrefixInput(newPrefix); err != nil {
			_ = fakeCtx.Reply("Invalid prefix. Use a single symbol or short word only (for example: !, /, . or bot). Sentences like 'hello there' are not allowed.")
			return true
		}
		if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
			newPrefix = "empty"
		}
		if okStore {
			_ = s.PutSetting(ctx, utils.PrefixSettingKey, newPrefix)
		}

		utils.BotWizardMu.Lock()
		utils.PendingWizardState[key] = utils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
		utils.BotWizardMu.Unlock()

		bodyText := Sprintf("Prefix set to %q.\n\nBot Customization Wizard (Step 4/4)\n\nPlease type the text for your bot's WhatsApp status bio and send it as a message.\n\nOr select Skip to finish.", newPrefix)
		bioOptions := []string{
			"Skip",
		}
		_ = sendPollReply(fakeCtx, bodyText, bioOptions)
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

		utils.BotWizardMu.Lock()
		delete(utils.PendingWizardState, key)
		utils.BotWizardMu.Unlock()

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
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "done", "finish":
			// User clicked "Done" on the wizard summary poll — just silently dismiss.
			DismissBotNamePrompt(ctx.Ctx, s)
			utils.BotWizardMu.Lock()
			delete(utils.PendingWizardState, key)
			utils.BotWizardMu.Unlock()
			return nil

		case "startover", "restart", "redo":
			// User clicked "Start Over" on the wizard summary poll — restart from step 1.
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Restarting wizard.\n\nBot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "prompt_name", "name_prompt":
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Please type your desired bot display name (e.g. Jarvis, Meow, Fuzzy):")

		case "prompt_thumb", "thumb_prompt":
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "thumb", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.")

		case "prompt_prefix", "prefix_prompt":
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "prefix", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Please send the command prefix symbol or word you want to use (e.g. ., !, / or 'none'):")

		case "prompt_bio", "bio_prompt":
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Please send the text for your bot's WhatsApp status bio:")

		case "0", "skip":
			stepNum := 0
			if len(args) > 1 {
				stepNum, _ = strconv.Atoi(args[1])
			}

			// If no explicit step number, check current wizard state to determine which step to skip
			if stepNum == 0 {
				utils.BotWizardMu.RLock()
				ws, hasWizard := utils.PendingWizardState[key]
				utils.BotWizardMu.RUnlock()
				if hasWizard {
					switch ws.Step {
					case "thumb":
						stepNum = 2
					case "prefix":
						stepNum = 3
					case "bio":
						stepNum = 4
					}
				}
			}

			if stepNum == 2 {
				// Skip thumbnail step, advance to prefix step
				utils.BotWizardMu.Lock()
				utils.PendingWizardState[key] = utils.WizardSession{Step: "prefix", UpdatedAt: time.Now()}
				utils.BotWizardMu.Unlock()

				bodyText := "Thumbnail step skipped.\n\nBot Customization Wizard (Step 3/4)\n\nPlease type the symbol or prefix you want to use (e.g. ., !, / or 'none') and send it as a message.\n\nOr select Skip to keep the current prefix."
				return sendPollReply(ctx, bodyText, []string{"Skip"})
			}

			if stepNum == 3 {
				// Skip prefix step, advance to bio step
				utils.BotWizardMu.Lock()
				utils.PendingWizardState[key] = utils.WizardSession{Step: "bio", UpdatedAt: time.Now()}
				utils.BotWizardMu.Unlock()

				bodyText := "Bot Customization Wizard (Step 4/4)\n\nPlease type the text for your bot's WhatsApp status bio and send it as a message.\n\nOr select Skip to finish."
				return sendPollReply(ctx, bodyText, []string{"Skip"})
			}

			// stepNum == 4 or fallback: finish wizard
			utils.BotWizardMu.Lock()
			delete(utils.PendingWizardState, key)
			utils.BotWizardMu.Unlock()
			DismissBotNamePrompt(ctx.Ctx, s)
			return sendWizardSummaryCard(ctx)

		case "page":
			// Ignore stale poll navigation callbacks that fire after the wizard is already
			// completed/dismissed — checking for an active wizard session guards this.
			utils.BotWizardMu.RLock()
			_, hasActiveWizard := utils.PendingWizardState[key]
			utils.BotWizardMu.RUnlock()
			if !hasActiveWizard {
				return nil
			}
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
			_ = s.PutSetting(ctx.Ctx, utils.BotNameSettingKey, newName)
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, utils.BotNameAwaitingInputPrefix+senderUser, "")
			utils.ClearInstructionCache()
			return ctx.Replyf("Bot name successfully updated to: %q!", newName)

		case "prefix", "setprefix":
			if len(args) < 2 {
				return ctx.Replyf("Usage: %ssetprefix <symbol>", p)
			}
			newPrefix := strings.Join(args[1:], " ")
			if _, err := validateBotPrefixInput(newPrefix); err != nil {
				return ctx.Reply("Invalid prefix. Use a single symbol or short word only (for example: !, /, . or bot). Sentences like 'hello there' are not allowed.")
			}
			if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
				newPrefix = "empty"
			}
			_ = s.PutSetting(ctx.Ctx, utils.PrefixSettingKey, newPrefix)
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
			_ = s.PutSetting(ctx.Ctx, utils.BotNameSettingKey, "")
			ResetBotNamePromptDismissed(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, utils.PrefixSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
			utils.ClearInstructionCache()
			return ctx.Reply("All bot settings (Name, Thumbnail, Prefix) reset to default values.")

		case "setup_customize":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, utils.BotNameAwaitingInputPrefix+senderUser, "")
			utils.BotWizardMu.Lock()
			utils.PendingWizardState[key] = utils.WizardSession{Step: "name", UpdatedAt: time.Now()}
			utils.BotWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "setup_continue":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, utils.BotNameAwaitingInputPrefix+senderUser, "")
			bodyText := "BOT NAME CUSTOMIZATION RECOMMENDED: Keeping default name WhatsRook is fine. You can start the customization wizard anytime using " + p + "reconfigure:"
			options := []string{
				"Start Wizard",
				"Keep Default",
			}
			return sendPollReply(ctx, bodyText, options)

		case "setup_ignore":
			DismissBotNamePrompt(ctx.Ctx, s)
			_ = s.PutSetting(ctx.Ctx, utils.BotNameAwaitingInputPrefix+senderUser, "")
			return ctx.Replyf("Kept default bot name. Change anytime using %sreconfigure or %ssetbot", p, p)

		default:
			return ctx.Replyf("Unknown setbot option. Usage: %ssetbot or %sreconfigure", p, p)
		}
	}

	utils.BotWizardMu.RLock()
	_, inActiveWizard := utils.PendingWizardState[key]
	utils.BotWizardMu.RUnlock()
	if inActiveWizard {
		return nil
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

	var options []string

	switch pageNum {
	case 1:
		options = []string{
			"Wizard",
			"Bot Name",
			"Next ▶️",
		}
	case 2:
		options = []string{
			"Thumbnail",
			"Prefix",
			"Next ▶️",
		}
	default:
		options = []string{
			"Bio",
			"Reset All",
			"◀️ Back",
		}
	}

	return sendPollReply(ctx, tb.Trimmed(), options)
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

	summaryText := ctx.Text().
		Header("Bot Customization Completed!").
		Section("BOT CONFIGURATION").
		Field("Name", botName).
		Field("Thumbnail", thumbStatus).
		Field("Prefix", curPrefix).
		Blank().
		Linef("Type %smenu anytime to view your updated bot commands menu!", p).
		Trimmed()

	return sendPollReply(ctx, summaryText, []string{"Done", "Start Over"})
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
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("LikeStatus ENABLED. The bot will automatically react to status broadcasts with love emojis.")

	case "off", "disable", "deactivate":
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

	bodyText := NewText().
		Header("LIFESTATUS AUTO-REACTION").
		Field("Status", strings.ToUpper(status)).
		Blank().
		Line("Automatically reacts to status broadcasts with random love emojis (❤️, 💕, 💖, 💗, 💓, 💞, 💘, 💌, 🥰, 😍).").
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

func validateBotPrefixInput(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("prefix is required")
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil, errors.New("prefix is required")
	}

	wordCount := 0
	for _, part := range parts {
		if strings.EqualFold(part, "none") || strings.EqualFold(part, "empty") {
			continue
		}
		if strings.ContainsAny(part, " \t\r\n") {
			return nil, errors.New("prefixes cannot contain spaces")
		}
		if isWordPrefix(part) {
			wordCount++
		}
	}

	if len(parts) > 1 && wordCount > 1 {
		return nil, errors.New("prefix must be a single symbol or short word, not a full sentence")
	}

	return parts, nil
}

func handlePrefix(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	if ctx.RawArgs == "" {
		raw, err := s.GetSetting(ctx.Ctx, utils.PrefixSettingKey)
		if err != nil {
			return sendText(ctx, "Failed to retrieve prefix configuration.")
		}
		if raw == "" {
			return ctx.SendTextf("Prefix: %q (default)", utils.DefaultPrefix)
		}
		return ctx.SendTextf("Prefix(es): %s", raw)
	}

	parts, err := validateBotPrefixInput(ctx.RawArgs)
	if err != nil {
		return sendText(ctx, "Invalid prefix. Use a single symbol or short word only (for example: !, /, . or bot). Sentences like 'hello there' are not allowed.")
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
	if err := s.PutSetting(ctx.Ctx, utils.PrefixSettingKey, stored); err != nil {
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

	tb.Blank().Line("Select an option from the poll below to configure privacy:")

	options := []string{
		"Last Seen: Everyone",
		"Last Seen: Contacts",
		"Last Seen: Nobody",
	}

	return sendPollReply(ctx, tb.Trimmed(), options)
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

	shaHex := hex.EncodeToString(stk.FileSHA256)

	if err := store.PutStickerCmd(ctx.Ctx, s.SQLStore, shaHex, cmdName); err != nil {
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

	quoted := ctx.GetQuotedMessage()
	if quoted != nil && quoted.StickerMessage != nil {
		stk := quoted.StickerMessage
		if len(stk.FileSHA256) == 0 {
			return ctx.Reply("Invalid sticker (no FileSHA256 found).")
		}
		shaHex := hex.EncodeToString(stk.FileSHA256)

		if err := store.DeleteStickerCmdBySHA(ctx.Ctx, s.SQLStore, shaHex); err != nil {
			return ctx.Reply("Failed to remove sticker command.")
		}
		return ctx.Reply("Sticker link removed.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage:\n- delcmd [command_name]\n- delcmd (replying to a mapped sticker)")
	}

	cmdName := strings.ToLower(ctx.Args[0])
	if err := store.DeleteStickerCmdByName(ctx.Ctx, s.SQLStore, cmdName); err != nil {
		return ctx.Reply("Failed to remove sticker command.")
	}

	return ctx.Replyf("Mapped sticker(s) for command %q removed.", cmdName)
}

func handleGetCmd(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	cmds, err := store.ListStickerCmds(ctx.Ctx, s.SQLStore)
	if err != nil {
		return ctx.Reply("Failed to query sticker commands.")
	}

	tb := ctx.Text().Header("Sticker Command Mappings")

	count := 0
	for _, sc := range cmds {
		sha := sc.StickerSHA256
		if len(sha) >= 8 {
			sha = sha[:8] + "..."
		}
		tb.Bulletf("%s -> %s", sha, sc.CommandName)
		count++
	}

	if count == 0 {
		return ctx.Reply("No sticker commands mapped yet.")
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
		modeText := "DM (Private to Owner)"
		if mode == "public" || mode == "chat" {
			modeText = "Public (Same Chat)"
		}

		bodyText := NewText().
			Header("AUTO-VIEWONCE SETTINGS").
			Field("Status", strings.ToUpper(curr)).
			Field("Mode", modeText).
			Blank().
			Line("Automatically intercepts incoming ViewOnce media and re-uploads fresh non-expiring media.").
			Blank().
			Section("Modes:").
			Bullet("Private (DM): Unwrapped media is saved & sent privately to your DM.").
			Bullet("Public (Chat): Unwrapped media is posted in the chat where it was received.").
			Trimmed()
		actionText := "Activate"
		if curr == "on" {
			actionText = "Deactivate"
		}
		modeAction := "Switch to DM (Private)"
		if mode != "public" && mode != "chat" {
			modeAction = "Switch to Public (Chat)"
		}
		options := []string{
			actionText,
			modeAction,
			"Guide",
		}
		return sendPollReply(ctx, bodyText, options)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, "autovv", "on")
		return ctx.Reply("Auto ViewOnce forwarding ENABLED.")
	case "off", "disable", "deactivate":
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
		return ctx.Text().
			Header("AUTOVV GUIDE").
			Section("Commands:").
			Bulletf("%sautovv on", p).
			Bulletf("%sautovv off", p).
			Bulletf("%sautovv dm (Private DM)", p).
			Bulletf("%sautovv public (Group/Chat)", p).
			Bulletf("%sautovv toggle", p).
			Blank().
			Line("Automatically intercepts ViewOnce media sent in chats, downloads media bytes immediately to prevent CDN expiry, and forwards clean unwrapped media to your DM or the public chat.").
			Reply()
	default:
		return ctx.Replyf("Usage: %sautovv [activate|deactivate|dm|public|toggle|customize]", ctx.GetPrefix())
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
		bodyText := NewText().
			Header("AUTO-STATUS SAVER").
			Field("Status", strings.ToUpper(curr)).
			Blank().
			Line("Automatically saves incoming WhatsApp status broadcasts to your DM.").
			Trimmed()
		actionText := "Activate"
		if curr == "on" {
			actionText = "Deactivate"
		}
		options := []string{
			actionText,
			"Customize",
		}
		return sendPollReply(ctx, bodyText, options)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "on")
		return ctx.Reply("Auto Status saving ENABLED. incoming status updates will be sent to your DM.")
	case "off", "disable", "deactivate":
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
		return ctx.Text().
			Header("AUTOSTATUS GUIDE").
			Section("Commands:").
			Bulletf("%sautostatus on", p).
			Bulletf("%sautostatus off", p).
			Bulletf("%sautostatus toggle", p).
			Blank().
			Line("Automatically intercepts contacts' status broadcasts and forwards them to your DM.").
			Reply()
	default:
		return ctx.Replyf("Usage: %sautostatus [on|off|toggle|customize]", ctx.GetPrefix())
	}
}

var defaultAutoReactEmojis = []string{
	"❤️", "🔥", "👍", "👏", "🎉", "✨", "💯", "🚀", "⚡", "🫡", "🤝", "🥳", "😎", "⭐", "🙌", "💙", "👀",
}

func parseEmojiList(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if strings.Contains(input, ",") {
		parts := strings.Split(input, ",")
		var res []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				res = append(res, trimmed)
			}
		}
		if len(res) > 0 {
			return res
		}
	}
	fields := strings.Fields(input)
	if len(fields) > 1 {
		return fields
	}
	var res []string
	for _, r := range input {
		if !unicode.IsSpace(r) {
			res = append(res, string(r))
		}
	}
	if len(res) > 0 {
		return res
	}
	return []string{input}
}

func handleAutoReactCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAutoReactMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate", "start":
		_ = s.PutSetting(ctx.Ctx, "autoreact_status", "on")
		return ctx.Reply("Auto-React ENABLED. The bot will automatically react with emojis to incoming messages.")

	case "off", "disable", "deactivate", "stop":
		_ = s.PutSetting(ctx.Ctx, "autoreact_status", "off")
		return ctx.Reply("Auto-React DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autoreact_status")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autoreact_status", "off")
			return ctx.Reply("Auto-React DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autoreact_status", "on")
		return ctx.Reply("Auto-React ENABLED. The bot will automatically react with emojis to incoming messages.")

	case "emoji", "emojis", "set":
		if len(args) < 2 || args[1] == "reset" || args[1] == "clear" || args[1] == "default" {
			_ = s.PutSetting(ctx.Ctx, "autoreact_emojis", strings.Join(defaultAutoReactEmojis, ","))
			return ctx.Replyf("Auto-React emojis reset to default:\n%s", strings.Join(defaultAutoReactEmojis, " "))
		}
		rawEmojiStr := strings.Join(args[1:], " ")
		emojis := parseEmojiList(rawEmojiStr)
		if len(emojis) == 0 {
			return ctx.Reply("Please provide one or more emojis to use for auto-reactions.")
		}
		_ = s.PutSetting(ctx.Ctx, "autoreact_emojis", strings.Join(emojis, ","))
		return ctx.Replyf("Auto-React emojis updated (%d configured):\n%s", len(emojis), strings.Join(emojis, " "))

	case "scope", "mode":
		if len(args) < 2 {
			currScope, _ := s.GetSetting(ctx.Ctx, "autoreact_scope")
			nextScope := "group"
			switch currScope {
			case "group":
				nextScope = "dm"
			case "dm":
				nextScope = "all"
			}
			_ = s.PutSetting(ctx.Ctx, "autoreact_scope", nextScope)
			return ctx.Replyf("Auto-React scope set to: %s", strings.ToUpper(nextScope))
		}
		targetScope := strings.ToLower(args[1])
		switch targetScope {
		case "all", "global":
			_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "all")
			return ctx.Reply("Auto-React scope set to ALL chats (Groups & DMs).")
		case "group", "groups", "g.us":
			_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "group")
			return ctx.Reply("Auto-React scope set to GROUP chats only.")
		case "dm", "pm", "private", "dms":
			_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "dm")
			return ctx.Reply("Auto-React scope set to DIRECT MESSAGES (DMs) only.")
		default:
			return ctx.Reply("Invalid scope. Valid options: `all`, `group`, `dm`.")
		}

	case "all", "global":
		_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "all")
		return ctx.Reply("Auto-React scope set to ALL chats (Groups & DMs).")

	case "group", "groups":
		_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "group")
		return ctx.Reply("Auto-React scope set to GROUP chats only.")

	case "dm", "pm", "private":
		_ = s.PutSetting(ctx.Ctx, "autoreact_scope", "dm")
		return ctx.Reply("Auto-React scope set to DIRECT MESSAGES (DMs) only.")

	case "help", "guide", "info":
		return sendAutoReactGuide(ctx)

	default:
		return ctx.Replyf("Usage: %sautoreact [activate|deactivate|toggle|emoji <emojis...>|scope <all|group|dm>|help]", ctx.GetPrefix())
	}
}

func sendAutoReactMenu(ctx *Context, s *StoreWrapper) error {
	status, _ := s.GetSetting(ctx.Ctx, "autoreact_status")
	if status == "" {
		status = "off"
	}
	scope, _ := s.GetSetting(ctx.Ctx, "autoreact_scope")
	if scope == "" {
		scope = "all"
	}
	emojisStr, _ := s.GetSetting(ctx.Ctx, "autoreact_emojis")
	emojis := parseEmojiList(emojisStr)
	if len(emojis) == 0 {
		emojis = defaultAutoReactEmojis
	}

	displayEmojis := strings.Join(emojis, " ")
	if len(emojis) > 10 {
		displayEmojis = strings.Join(emojis[:10], " ") + Sprintf(" (+%d more)", len(emojis)-10)
	}

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	nextScopeAction := "Scope: GROUP"
	switch scope {
	case "group":
		nextScopeAction = "Scope: DM"
	case "dm":
		nextScopeAction = "Scope: ALL"
	}

	bodyText := ctx.Rook().NewText().
		Header("AUTO-REACT CONFIGURATION").
		Field("Status", strings.ToUpper(status)).
		Field("Scope", strings.ToUpper(scope)).
		Field("Emojis", displayEmojis).
		Blank().
		Line("Automatically sends emoji reactions to incoming messages.").
		Trimmed()

	options := []string{
		actionText,
		nextScopeAction,
		"Set Emojis",
		"Help / Guide",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendAutoReactGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("AUTO-REACT GUIDE").
		Line("Auto-React automatically adds emoji reactions to incoming WhatsApp messages.").
		Blank().
		Section("Commands:").
		Bulletf("`%sautoreact on` / `activate`   : Enable auto-reactions", p).
		Bulletf("`%sautoreact off` / `deactivate`: Disable auto-reactions", p).
		Bulletf("`%sautoreact toggle`            : Toggle on/off status", p).
		Bulletf("`%sautoreact emoji <emojis...>` : Configure custom emojis", p).
		Bulletf("`%sautoreact emoji reset`       : Reset emojis to default", p).
		Bulletf("`%sautoreact scope all`         : React in all chats", p).
		Bulletf("`%sautoreact scope group`       : React in groups only", p).
		Bulletf("`%sautoreact scope dm`          : React in DMs only", p).
		Blank().
		Section("Examples:").
		Linef("`%sautoreact emoji ❤️ 🔥 👍 ✨ 🚀`", p).
		Linef("`%sautoreact scope group`", p).
		Reply()
}

func handleAutoReadCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := getStore(ctx)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAutoReadMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable", "activate", "start":
		_ = s.PutSetting(ctx.Ctx, "autoread_status", "on")
		return ctx.Reply("Auto-Read ENABLED. Incoming messages will automatically be marked as read (blue tick).")

	case "off", "disable", "deactivate", "stop":
		_ = s.PutSetting(ctx.Ctx, "autoread_status", "off")
		return ctx.Reply("Auto-Read DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autoread_status")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autoread_status", "off")
			return ctx.Reply("Auto-Read DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autoread_status", "on")
		return ctx.Reply("Auto-Read ENABLED. Incoming messages will automatically be marked as read (blue tick).")

	case "status", "story", "stories":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			switch act {
			case "on", "enable", "activate":
				_ = s.PutSetting(ctx.Ctx, "autoread_status_view", "on")
				return ctx.Reply("Status auto-view/read ENABLED. Status broadcasts will automatically be marked as viewed.")
			case "off", "disable", "deactivate":
				_ = s.PutSetting(ctx.Ctx, "autoread_status_view", "off")
				return ctx.Reply("Status auto-view/read DISABLED.")
			}
		}
		curr, _ := s.GetSetting(ctx.Ctx, "autoread_status_view")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autoread_status_view", "off")
			return ctx.Reply("Status auto-view/read DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autoread_status_view", "on")
		return ctx.Reply("Status auto-view/read ENABLED. Status broadcasts will automatically be marked as viewed.")

	case "scope", "mode":
		if len(args) < 2 {
			currScope, _ := s.GetSetting(ctx.Ctx, "autoread_scope")
			nextScope := "group"
			switch currScope {
			case "group":
				nextScope = "dm"
			case "dm":
				nextScope = "all"
			}
			_ = s.PutSetting(ctx.Ctx, "autoread_scope", nextScope)
			return ctx.Replyf("Auto-Read scope set to: %s", strings.ToUpper(nextScope))
		}
		targetScope := strings.ToLower(args[1])
		switch targetScope {
		case "all", "global":
			_ = s.PutSetting(ctx.Ctx, "autoread_scope", "all")
			return ctx.Reply("Auto-Read scope set to ALL chats (Groups & DMs).")
		case "group", "groups", "g.us":
			_ = s.PutSetting(ctx.Ctx, "autoread_scope", "group")
			return ctx.Reply("Auto-Read scope set to GROUP chats only.")
		case "dm", "pm", "private", "dms":
			_ = s.PutSetting(ctx.Ctx, "autoread_scope", "dm")
			return ctx.Reply("Auto-Read scope set to DIRECT MESSAGES (DMs) only.")
		default:
			return ctx.Reply("Invalid scope. Valid options: `all`, `group`, `dm`.")
		}

	case "all", "global":
		_ = s.PutSetting(ctx.Ctx, "autoread_scope", "all")
		return ctx.Reply("Auto-Read scope set to ALL chats (Groups & DMs).")

	case "group", "groups":
		_ = s.PutSetting(ctx.Ctx, "autoread_scope", "group")
		return ctx.Reply("Auto-Read scope set to GROUP chats only.")

	case "dm", "pm", "private":
		_ = s.PutSetting(ctx.Ctx, "autoread_scope", "dm")
		return ctx.Reply("Auto-Read scope set to DIRECT MESSAGES (DMs) only.")

	case "help", "guide", "info":
		return sendAutoReadGuide(ctx)

	default:
		return ctx.Replyf("Usage: %sautoread [activate|deactivate|toggle|status <on|off>|scope <all|group|dm>|help]", ctx.GetPrefix())
	}
}

func sendAutoReadMenu(ctx *Context, s *StoreWrapper) error {
	status, _ := s.GetSetting(ctx.Ctx, "autoread_status")
	if status == "" {
		status = "off"
	}
	scope, _ := s.GetSetting(ctx.Ctx, "autoread_scope")
	if scope == "" {
		scope = "all"
	}
	statusView, _ := s.GetSetting(ctx.Ctx, "autoread_status_view")
	if statusView == "" {
		statusView = "off"
	}

	actionText := "Activate"
	if status == "on" {
		actionText = "Deactivate"
	}

	nextScopeAction := "Scope: GROUP"
	switch scope {
	case "group":
		nextScopeAction = "Scope: DM"
	case "dm":
		nextScopeAction = "Scope: ALL"
	}

	statusViewAction := "Status View: ON"
	if statusView == "on" {
		statusViewAction = "Status View: OFF"
	}

	bodyText := ctx.Rook().NewText().
		Header("AUTO-READ CONFIGURATION").
		Field("Status", strings.ToUpper(status)).
		Field("Scope", strings.ToUpper(scope)).
		Field("Status Broadcasts", strings.ToUpper(statusView)).
		Blank().
		Line("Automatically sends read receipts (blue ticks) for incoming messages.").
		Trimmed()

	options := []string{
		actionText,
		nextScopeAction,
		statusViewAction,
		"Help / Guide",
	}

	return sendPollReply(ctx, bodyText, options)
}

func sendAutoReadGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("AUTO-READ GUIDE").
		Line("Auto-Read automatically marks incoming messages and status broadcasts as read (blue ticks).").
		Blank().
		Section("Commands:").
		Bulletf("`%sautoread on` / `activate`   : Enable auto-read", p).
		Bulletf("`%sautoread off` / `deactivate`: Disable auto-read", p).
		Bulletf("`%sautoread toggle`            : Toggle auto-read on/off", p).
		Bulletf("`%sautoread scope all`         : Read all chats (Groups & DMs)", p).
		Bulletf("`%sautoread scope group`       : Read group messages only", p).
		Bulletf("`%sautoread scope dm`          : Read private messages only", p).
		Bulletf("`%sautoread status on|off`     : Auto-view status broadcasts", p).
		Blank().
		Section("Examples:").
		Linef("`%sautoread on`", p).
		Linef("`%sautoread status on`", p).
		Linef("`%sautoread scope group`", p).
		Reply()
}

func HandleAutoReact(ctx context.Context, client *whatsmeow.Client, s *StoreWrapper, evt *events.Message) {
	if evt == nil || evt.Info.IsFromMe || client == nil || s == nil {
		return
	}
	if evt.Info.Chat.String() == "status@broadcast" || evt.Info.Chat.Server == "broadcast" {
		return
	}

	status, _ := s.GetSetting(ctx, "autoreact_status")
	if status != "on" {
		return
	}

	scope, _ := s.GetSetting(ctx, "autoreact_scope")
	if scope == "group" && evt.Info.Chat.Server != "g.us" {
		return
	}
	if (scope == "dm" || scope == "private") && evt.Info.Chat.Server == "g.us" {
		return
	}

	emojisStr, _ := s.GetSetting(ctx, "autoreact_emojis")
	emojis := parseEmojiList(emojisStr)
	if len(emojis) == 0 {
		emojis = defaultAutoReactEmojis
	}

	emoji := emojis[rand.Intn(len(emojis))]

	senderJID := evt.Info.Sender
	if senderJID.IsEmpty() {
		senderJID = evt.Info.Chat
	}

	reactionMsg := client.BuildReaction(evt.Info.Chat, senderJID, evt.Info.ID, emoji)
	_, err := client.SendMessage(ctx, evt.Info.Chat, reactionMsg)
	if err != nil {
		logger.Debug("autoreact: failed to send reaction", "chat", evt.Info.Chat.String(), "msg_id", evt.Info.ID, "err", err)
	} else {
		logger.Debug("autoreact: reaction sent", "chat", evt.Info.Chat.String(), "msg_id", evt.Info.ID, "emoji", emoji)
	}
}

func HandleAutoRead(ctx context.Context, client *whatsmeow.Client, s *StoreWrapper, evt *events.Message) {
	if evt == nil || evt.Info.IsFromMe || client == nil || s == nil {
		return
	}

	if evt.Info.Chat.String() == "status@broadcast" || evt.Info.Chat.Server == "broadcast" {
		statusView, _ := s.GetSetting(ctx, "autoread_status_view")
		generalStatus, _ := s.GetSetting(ctx, "autoread_status")
		if statusView == "on" || (generalStatus == "on" && statusView != "off") {
			senderJID := evt.Info.Sender
			if senderJID.IsEmpty() {
				senderJID = evt.Info.Chat
			}
			err := client.MarkRead(ctx, []types.MessageID{evt.Info.ID}, evt.Info.Timestamp, types.StatusBroadcastJID, senderJID)
			if err != nil {
				logger.Debug("autoread: failed to mark status as read", "msg_id", evt.Info.ID, "err", err)
			} else {
				logger.Debug("autoread: status marked as read", "msg_id", evt.Info.ID, "sender", senderJID.String())
			}
		}
		return
	}

	status, _ := s.GetSetting(ctx, "autoread_status")
	if status != "on" {
		return
	}

	scope, _ := s.GetSetting(ctx, "autoread_scope")
	if scope == "group" && evt.Info.Chat.Server != "g.us" {
		return
	}
	if (scope == "dm" || scope == "private") && evt.Info.Chat.Server == "g.us" {
		return
	}

	senderJID := evt.Info.Sender
	if senderJID.IsEmpty() {
		senderJID = evt.Info.Chat
	}

	err := client.MarkRead(ctx, []types.MessageID{evt.Info.ID}, evt.Info.Timestamp, evt.Info.Chat, senderJID)
	if err != nil {
		logger.Debug("autoread: failed to mark message as read", "chat", evt.Info.Chat.String(), "msg_id", evt.Info.ID, "err", err)
	} else {
		logger.Debug("autoread: message marked as read", "chat", evt.Info.Chat.String(), "msg_id", evt.Info.ID)
	}
}
