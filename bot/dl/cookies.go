package dl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thruqe/whatsrook"
	"github.com/thruqe/whatsrook/util/logger"
	"whatsrook/bot/dispatch"
)

const (
	// CookiePromptPhrase is the exact phrase used when asking the user for cookies.
	CookiePromptPhrase = "Please paste your cookies for me to use."

	// Legacy setting key for backward compatibility
	SettingYTDLPCookies = "ytdlp_cookies"
)

func init() {
	dispatch.RegisterPreInterceptor("dl_cookie_reply", HandleCookieReplyIntercept)
}

// HandleCookieReplyIntercept intercepts replies to the bot's cookie prompt message.
func HandleCookieReplyIntercept(c *dispatch.Context, text string) bool {
	if c == nil || c.Evt == nil {
		return false
	}

	quoted := c.GetQuotedMessage()
	if quoted == nil {
		return false
	}

	quotedText := whatsrook.ExtractTextFromProto(quoted)
	if !strings.Contains(strings.ToLower(quotedText), "please paste your cookies for me to use") {
		return false
	}

	cleanText := strings.TrimSpace(text)
	if strings.EqualFold(cleanText, "cancel") || strings.EqualFold(cleanText, "nevermind") {
		_ = c.Reply("Cookie configuration cancelled.")
		return true
	}

	cookieContent := cleanText

	// Check if the user attached a document (e.g. cookies.txt file)
	if doc := c.Evt.Message.GetDocumentMessage(); doc != nil {
		rawBytes, err := c.Client.Download(c.GetSendContext(), doc)
		if err != nil {
			_ = c.Replyf("Failed to download attached cookies file: %v", err)
			return true
		}
		cookieContent = string(rawBytes)
	}

	// If the user typed a command prefix e.g. ".dl cookie <content>", strip it
	prefix := c.GetPrefix()
	if strings.HasPrefix(strings.ToLower(cookieContent), prefix+"dl cookie") {
		cookieContent = strings.TrimSpace(cookieContent[len(prefix+"dl cookie"):])
	}

	if strings.TrimSpace(cookieContent) == "" {
		_ = c.Reply("Provided cookie content was empty. Please paste your cookies or upload a cookies.txt file.")
		return true
	}

	if err := saveAndReportCookies(c, cookieContent); err != nil {
		_ = c.Replyf("Failed to save cookies: %v", err)
	}
	return true
}

// PlatformCookieGroup holds parsed cookie lines for a single platform.
type PlatformCookieGroup struct {
	Platform string
	Domain   string
	Lines    []string
}

// IdentifyPlatform maps a domain or hostname to a canonical platform identifier.
func IdentifyPlatform(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, "#httponly_")
	d = strings.TrimPrefix(d, ".")
	if idx := strings.Index(d, ":"); idx != -1 {
		d = d[:idx]
	}

	switch {
	case strings.Contains(d, "youtube.com") || strings.Contains(d, "youtu.be") || strings.Contains(d, "googlevideo.com") || strings.Contains(d, "ytimg.com"):
		return "youtube"
	case strings.Contains(d, "instagram.com") || strings.Contains(d, "cdninstagram.com"):
		return "instagram"
	case strings.Contains(d, "tiktok.com") || strings.Contains(d, "byteoversea.com") || strings.Contains(d, "ibytedtos.com"):
		return "tiktok"
	case strings.Contains(d, "twitter.com") || strings.Contains(d, "x.com") || strings.Contains(d, "twimg.com"):
		return "twitter"
	case strings.Contains(d, "facebook.com") || strings.Contains(d, "fb.com") || strings.Contains(d, "fb.watch") || strings.Contains(d, "messenger.com"):
		return "facebook"
	case strings.Contains(d, "reddit.com") || strings.Contains(d, "redd.it"):
		return "reddit"
	case strings.Contains(d, "soundcloud.com"):
		return "soundcloud"
	case strings.Contains(d, "twitch.tv"):
		return "twitch"
	case strings.Contains(d, "bilibili.com"):
		return "bilibili"
	case strings.Contains(d, "dailymotion.com"):
		return "dailymotion"
	case strings.Contains(d, "vimeo.com"):
		return "vimeo"
	case strings.Contains(d, "pinterest.com"):
		return "pinterest"
	case strings.Contains(d, "threads.net"):
		return "threads"
	case strings.Contains(d, "spotify.com"):
		return "spotify"
	case strings.Contains(d, "snapchat.com"):
		return "snapchat"
	case strings.Contains(d, "linkedin.com"):
		return "linkedin"
	case strings.Contains(d, "tumblr.com"):
		return "tumblr"
	case strings.Contains(d, "vk.com"):
		return "vk"
	case strings.Contains(d, "weibo.com"):
		return "weibo"
	}

	// Extract registered second-level domain (e.g. sub.crunchyroll.com -> crunchyroll)
	parts := strings.Split(d, ".")
	if len(parts) >= 2 {
		secondToLast := parts[len(parts)-2]
		if (secondToLast == "co" || secondToLast == "com" || secondToLast == "org" || secondToLast == "gov" || secondToLast == "edu") && len(parts) >= 3 {
			return parts[len(parts)-3]
		}
		return secondToLast
	}

	if d != "" {
		return d
	}
	return "generic"
}

// ExtractPlatformFromURL extracts the platform name from a media URL.
func ExtractPlatformFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		u, err = url.Parse("https://" + rawURL)
		if err != nil {
			return "generic"
		}
	}
	return IdentifyPlatform(u.Hostname())
}

// ParseAndGroupCookies parses Netscape or JSON cookies and groups them by platform.
func ParseAndGroupCookies(content string) (map[string]*PlatformCookieGroup, int, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, 0, fmt.Errorf("empty cookie content")
	}

	// 1. Try parsing as JSON (e.g. Cookie-Editor export)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		groups, count, err := parseJSONCookies(trimmed)
		if err == nil && count > 0 {
			return groups, count, nil
		}
	}

	// 2. Parse as Netscape tab/space delimited format
	return parseNetscapeCookies(trimmed)
}

func parseNetscapeCookies(content string) (map[string]*PlatformCookieGroup, int, error) {
	lines := strings.Split(content, "\n")
	groups := make(map[string]*PlatformCookieGroup)
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip pure comments unless it's an HttpOnly cookie line
		if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#HttpOnly_") {
			continue
		}

		fields := strings.Split(trimmed, "\t")
		if len(fields) < 7 {
			// Fallback: multiple spaces
			fields = strings.Fields(trimmed)
			if len(fields) < 7 {
				continue
			}
		}

		domain := fields[0]
		platform := IdentifyPlatform(domain)
		if platform == "" {
			platform = "generic"
		}

		netscapeLine := strings.Join(fields[:7], "\t")
		if g, exists := groups[platform]; exists {
			g.Lines = append(g.Lines, netscapeLine)
		} else {
			groups[platform] = &PlatformCookieGroup{
				Platform: platform,
				Domain:   strings.TrimPrefix(domain, "#HttpOnly_"),
				Lines:    []string{netscapeLine},
			}
		}
		count++
	}

	if count == 0 {
		return nil, 0, fmt.Errorf("no valid Netscape cookie records found")
	}

	return groups, count, nil
}

type jsonCookieItem struct {
	Domain         string  `json:"domain"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	HTTPOnly       bool    `json:"httpOnly"`
	ExpirationDate float64 `json:"expirationDate"`
	Expires        float64 `json:"expires"`
}

func parseJSONCookies(content string) (map[string]*PlatformCookieGroup, int, error) {
	var items []jsonCookieItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return nil, 0, err
	}

	groups := make(map[string]*PlatformCookieGroup)
	count := 0

	for _, item := range items {
		if item.Domain == "" || item.Name == "" {
			continue
		}

		domain := item.Domain
		platform := IdentifyPlatform(domain)
		if platform == "" {
			platform = "generic"
		}

		includeSubdomains := "TRUE"
		if !strings.HasPrefix(domain, ".") {
			includeSubdomains = "FALSE"
		}

		path := item.Path
		if path == "" {
			path = "/"
		}

		secureStr := "FALSE"
		if item.Secure {
			secureStr = "TRUE"
		}

		expiry := int64(item.ExpirationDate)
		if expiry == 0 {
			expiry = int64(item.Expires)
		}
		if expiry == 0 {
			expiry = time.Now().Add(365 * 24 * time.Hour).Unix()
		}

		domainPrefix := domain
		if item.HTTPOnly {
			domainPrefix = "#HttpOnly_" + domain
		}

		netscapeLine := fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%s\t%s",
			domainPrefix, includeSubdomains, path, secureStr, expiry, item.Name, item.Value,
		)

		if g, exists := groups[platform]; exists {
			g.Lines = append(g.Lines, netscapeLine)
		} else {
			groups[platform] = &PlatformCookieGroup{
				Platform: platform,
				Domain:   domain,
				Lines:    []string{netscapeLine},
			}
		}
		count++
	}

	return groups, count, nil
}

// saveAndReportCookies parses, groups, and saves cookies to the database table.
func saveAndReportCookies(ctx *dispatch.Context, content string) error {
	groups, totalCount, err := ParseAndGroupCookies(content)
	if err != nil || totalCount == 0 {
		return ctx.Reply("No valid cookies found. Please ensure you are pasting Netscape format (tab-separated) or JSON cookies.")
	}

	s, ok := dispatch.GetStore(ctx)
	if !ok {
		return ctx.Reply("Database store is unavailable.")
	}

	var updatedPlatforms []string
	for platform, g := range groups {
		var sb strings.Builder
		sb.WriteString("# Netscape HTTP Cookie File\n\n")
		for _, line := range g.Lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		cookieText := sb.String()

		if err := s.PutPlatformCookie(ctx.GetSendContext(), platform, g.Domain, cookieText); err != nil {
			logger.Warn("Failed to persist platform cookie to database", "platform", platform, "err", err)
		}
		updatedPlatforms = append(updatedPlatforms, fmt.Sprintf("- %s: %d cookies (%s)", platform, len(g.Lines), g.Domain))
	}
	sort.Strings(updatedPlatforms)

	// Save merged cookies to session media cookies.txt
	if merged, err := s.GetAllPlatformCookiesMerged(ctx.GetSendContext()); err == nil && merged != "" {
		sessDir := dispatch.GetSessionMediaDir(ctx.Client)
		_ = os.MkdirAll(sessDir, 0755)
		_ = os.WriteFile(filepath.Join(sessDir, "cookies.txt"), []byte(merged), 0600)
	}

	tb := dispatch.NewText()
	tb.Line("*Cookies saved successfully!*").Blank()
	tb.Linef("Total: %d cookies across %d platform(s):", totalCount, len(groups))
	for _, p := range updatedPlatforms {
		tb.Line(p)
	}
	tb.Blank()
	tb.Line("Future downloads for these platforms will automatically use these cookies.")

	return ctx.Reply(tb.String())
}

// getCookiesFilePath retrieves the appropriate cookies file path for a target URL.
func getCookiesFilePath(ctx *dispatch.Context, rawURL string) (string, func()) {
	platform := ExtractPlatformFromURL(rawURL)

	var cookieData string
	if s, ok := dispatch.GetStore(ctx); ok {
		// 1. Try platform-specific cookies from bot_platform_cookies table
		if platform != "" && platform != "generic" {
			if val, err := s.GetPlatformCookie(ctx.GetSendContext(), platform); err == nil && strings.TrimSpace(val) != "" {
				cookieData = val
			}
		}

		// 2. If no platform-specific cookies found, fallback to merged cookies across all platforms
		if cookieData == "" {
			if val, err := s.GetAllPlatformCookiesMerged(ctx.GetSendContext()); err == nil && strings.TrimSpace(val) != "" {
				cookieData = val
			}
		}

		// 3. Fallback to legacy setting
		if cookieData == "" {
			if val, err := s.GetSetting(ctx.GetSendContext(), SettingYTDLPCookies); err == nil && strings.TrimSpace(val) != "" {
				cookieData = val
			}
		}
	}

	// 4. Fallback to session media cookies.txt
	if cookieData == "" {
		sessDir := dispatch.GetSessionMediaDir(ctx.Client)
		sessCookiePath := filepath.Join(sessDir, "cookies.txt")
		if info, err := os.Stat(sessCookiePath); err == nil && info.Size() > 0 {
			return sessCookiePath, func() {}
		}
	}

	if cookieData != "" {
		tmpFile, err := os.CreateTemp("", "ytdlp_cookies_*.txt")
		if err == nil {
			_, _ = tmpFile.WriteString(strings.TrimSpace(cookieData))
			_ = tmpFile.Close()
			return tmpFile.Name(), func() {
				_ = os.Remove(tmpFile.Name())
			}
		}
	}

	return "", func() {}
}

// handleCookieCommand manages cookies configuration (.dl cookie [args]).
func handleCookieCommand(ctx *dispatch.Context) error {
	args := ctx.Args[1:] // args after "cookie"

	// Subaction: clear [platform]
	if len(args) > 0 && (args[0] == "clear" || args[0] == "del" || args[0] == "delete" || args[0] == "remove") {
		targetPlatform := ""
		if len(args) > 1 {
			targetPlatform = strings.ToLower(strings.TrimSpace(args[1]))
		}

		if s, ok := dispatch.GetStore(ctx); ok {
			if targetPlatform != "" && targetPlatform != "all" {
				_ = s.DeletePlatformCookie(ctx.GetSendContext(), targetPlatform)
				// Re-sync session cookies.txt
				if merged, err := s.GetAllPlatformCookiesMerged(ctx.GetSendContext()); err == nil {
					sessDir := dispatch.GetSessionMediaDir(ctx.Client)
					_ = os.WriteFile(filepath.Join(sessDir, "cookies.txt"), []byte(merged), 0600)
				}
				return ctx.Replyf("Stored cookies for platform %q have been deleted.", targetPlatform)
			}
			_ = s.DeleteAllPlatformCookies(ctx.GetSendContext())
			_ = s.DeleteSetting(ctx.GetSendContext(), SettingYTDLPCookies)
		}
		sessDir := dispatch.GetSessionMediaDir(ctx.Client)
		_ = os.Remove(filepath.Join(sessDir, "cookies.txt"))
		return ctx.Reply("All stored yt-dlp cookies have been deleted successfully.")
	}

	// Subaction: status / check / list
	if len(args) > 0 && (args[0] == "status" || args[0] == "check" || args[0] == "info" || args[0] == "list") {
		return reportCookieStatus(ctx)
	}

	// If user provided raw cookie text as argument: .dl cookie <content>
	if len(args) > 0 {
		cookieText := strings.TrimSpace(strings.Join(args, " "))
		return saveAndReportCookies(ctx, cookieText)
	}

	// Check if replying to a document (e.g. cookies.txt file)
	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		if doc := quoted.GetDocumentMessage(); doc != nil {
			rawBytes, err := ctx.Client.Download(ctx.GetSendContext(), doc)
			if err != nil {
				return ctx.Replyf("Failed to download quoted cookies file: %v", err)
			}
			return saveAndReportCookies(ctx, string(rawBytes))
		}
		if text := quoted.GetConversation(); text != "" {
			return saveAndReportCookies(ctx, text)
		}
		if ext := quoted.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
			return saveAndReportCookies(ctx, ext.GetText())
		}
	}

	// Check if message itself has an attached document
	if ctx.Evt != nil && ctx.Evt.Message != nil {
		if doc := ctx.Evt.Message.GetDocumentMessage(); doc != nil {
			rawBytes, err := ctx.Client.Download(ctx.GetSendContext(), doc)
			if err != nil {
				return ctx.Replyf("Failed to download attached cookies file: %v", err)
			}
			return saveAndReportCookies(ctx, string(rawBytes))
		}
	}

	// If no data provided: show status and instructions
	return reportCookieStatus(ctx)
}

func reportCookieStatus(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	s, ok := dispatch.GetStore(ctx)

	var platforms []string
	totalCookies := 0

	if ok {
		if list, err := s.ListPlatformCookies(ctx.GetSendContext()); err == nil && len(list) > 0 {
			for _, pc := range list {
				count := 0
				for line := range strings.SplitSeq(pc.Cookies, "\n") {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "#") {
						count++
					}
				}
				totalCookies += count
				updated := ""
				if !pc.UpdatedAt.IsZero() {
					updated = fmt.Sprintf(" (updated: %s)", pc.UpdatedAt.Format("2006-01-02 15:04"))
				}
				platforms = append(platforms, fmt.Sprintf("- %s: %d cookies%s", pc.Platform, count, updated))
			}
		}
	}

	tb := dispatch.NewText()
	if len(platforms) > 0 {
		tb.Line("*yt-dlp Platform Cookies: ACTIVE*").Blank()
		tb.Linef("Total: %d cookies stored across %d platform(s):", totalCookies, len(platforms))
		for _, pl := range platforms {
			tb.Line(pl)
		}
		tb.Blank()
		tb.Line("To update cookies:")
		tb.Linef("- Reply to a `cookies.txt` document with *%sdl cookie*", p)
		tb.Linef("- Or simply reply to any download error prompt saying %q", CookiePromptPhrase)
		tb.Linef("- To clear specific platform: *%sdl cookie clear <platform>*", p)
		tb.Linef("- To clear all: *%sdl cookie clear*", p)
	} else {
		tb.Line("*yt-dlp Platform Cookies: NOT CONFIGURED*").Blank()
		tb.Line("Platforms like YouTube (age-gated/bot check), Instagram, X/Twitter, and TikTok require cookies.")
		tb.Blank()
		tb.Line("How to configure cookies:")
		tb.Line("1. Export your cookies in Netscape format (e.g. using 'Get cookies.txt LOCALLY' extension).")
		tb.Linef("2. Reply to the `cookies.txt` file with *%sdl cookie*", p)
		tb.Linef("3. Or run: *%sdl cookie <paste Netscape cookies>*", p)
		tb.Linef("4. Or reply directly to the message asking %q", CookiePromptPhrase)
	}

	return ctx.Reply(tb.String())
}

// sendFailureWithCookiePrompt handles media failures and guides the user to provide cookies.
func sendFailureWithCookiePrompt(ctx *dispatch.Context, err error) error {
	p := ctx.GetPrefix()
	errMsg := err.Error()
	if len(errMsg) > 250 {
		errMsg = errMsg[:247] + "..."
	}

	tb := dispatch.NewText()
	tb.Line("*Media Download Failed*").Blank()
	tb.Linef("Error: %s", errMsg).Blank()
	tb.Line("*Authentication or Bot Verification Required?*")
	tb.Line("This platform often blocks automated downloads unless authenticated cookies are supplied.")
	tb.Blank()
	tb.Line(CookiePromptPhrase)
	tb.Line("(Simply reply to this message with your cookies text or cookies.txt file)")
	tb.Blank()
	tb.Line("Alternative commands:")
	tb.Linef("- Reply to a `cookies.txt` document with: *%sdl cookie*", p)
	tb.Linef("- Run: *%sdl cookie <paste content>*", p)
	tb.Linef("- Check active platform cookies: *%sdl cookies*", p)

	return ctx.Reply(tb.String())
}
