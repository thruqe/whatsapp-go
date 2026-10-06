// grol.go implements an embedded Grol (Go REPL Open Language) scripting engine
// for WhatsApp chat environments. It enables evaluating mathematical expressions,
// algorithms, string transformations, and scripting with safe session sandboxing.
package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/repl"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
)

var (
	grolInitOnce     sync.Once
	sessionsRegistry = newSessionManager(100, 30*time.Minute)
)

const (
	maxGrolOutputRunes = 3000
	maxGrolExecTimeout = 5 * time.Second
)

func init() {
	initGrol()
	dispatch.Register(&dispatch.Command{
		Name:        "grol",
		Alias:       "gr",
		Description: "Execute or format Grol (Go REPL Open Language) code",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleGrol,
	})
}

// initGrol ensures safe built-in functions are registered and diagnostic logging silenced.
func initGrol() {
	grolInitOnce.Do(func() {
		// Suppress fortio internal log output from polluting stdout/stderr
		log.SetOutput(io.Discard)
		log.SetLogLevel(log.Error)

		// Sandbox extensions: strictly disallow host disk reads/writes
		extCfg := extensions.Config{
			HasLoad:         false,
			HasSave:         false,
			UnrestrictedIOs: false,
		}
		if err := extensions.Init(&extCfg); err != nil {
			logger.Error("Failed to initialize grol extensions", "err", err)
		}
	})
}

// grolSession stores an interpreter state with mutual exclusion and activity tracking.
type grolSession struct {
	mu         sync.Mutex
	state      *eval.State
	lastAccess time.Time
}

// sessionManager maintains an LRU-evicting map of active interpreter sessions.
type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*grolSession
	order    []string
	max      int
	ttl      time.Duration
}

func newSessionManager(max int, ttl time.Duration) *sessionManager {
	if max <= 0 {
		max = 100
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &sessionManager{
		sessions: make(map[string]*grolSession),
		order:    make([]string, 0, max),
		max:      max,
		ttl:      ttl,
	}
}

func (m *sessionManager) getSession(key string) *grolSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.evictExpiredLocked(now)

	if sess, ok := m.sessions[key]; ok {
		sess.lastAccess = now
		m.touchOrderLocked(key)
		return sess
	}

	// Create new session
	sess := &grolSession{
		state:      eval.NewState(),
		lastAccess: now,
	}

	// Capacity enforcement
	if len(m.sessions) >= m.max && len(m.order) > 0 {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.sessions, oldest)
	}

	m.sessions[key] = sess
	m.order = append(m.order, key)
	return sess
}

func (m *sessionManager) deleteSession(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[key]; ok {
		delete(m.sessions, key)
		for i, k := range m.order {
			if k == key {
				m.order = append(m.order[:i], m.order[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

func (m *sessionManager) evictExpiredLocked(now time.Time) {
	for key, sess := range m.sessions {
		if now.Sub(sess.lastAccess) > m.ttl {
			delete(m.sessions, key)
			for i, k := range m.order {
				if k == key {
					m.order = append(m.order[:i], m.order[i+1:]...)
					break
				}
			}
		}
	}
}

func (m *sessionManager) touchOrderLocked(key string) {
	for i, k := range m.order {
		if k == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.order = append(m.order, key)
}

// grolParsedFlags holds configuration parsed from user CLI input.
type grolParsedFlags struct {
	formatOnly bool
	showParse  bool
	compact    bool
	cleanState bool
	resetState bool
	showHelp   bool
	code       string
}

// parseGrolFlags parses leading command flags and separates them from code.
func parseGrolFlags(raw string) grolParsedFlags {
	trimmed := strings.TrimSpace(raw)
	var flags grolParsedFlags

	if trimmed == "" {
		return flags
	}

	parts := strings.Fields(trimmed)
	idx := 0
FlagLoop:
	for idx < len(parts) {
		token := parts[idx]
		switch token {
		case "-f", "--format":
			flags.formatOnly = true
			idx++
		case "-p", "--parse":
			flags.showParse = true
			idx++
		case "-c", "--compact":
			flags.compact = true
			idx++
		case "-clean", "--clean":
			flags.cleanState = true
			idx++
		case "reset", "clear":
			flags.resetState = true
			idx++
			break FlagLoop
		case "help", "-h", "--help":
			flags.showHelp = true
			idx++
			break FlagLoop
		default:
			break FlagLoop
		}
	}

	// Reconstruct code from the point where flags stopped
	if idx < len(parts) {
		// Find position of parts[idx] in trimmed
		searchSub := parts[idx]
		pos := strings.Index(trimmed, searchSub)
		if pos != -1 {
			flags.code = strings.TrimSpace(trimmed[pos:])
		}
	}

	return flags
}

// normalizeSmartQuotes converts typographical smart quotes to standard ASCII equivalents.
func normalizeSmartQuotes(s string) string {
	r := strings.NewReplacer(
		"“", "\"",
		"”", "\"",
		"‘", "'",
		"’", "'",
		"«", "\"",
		"»", "\"",
	)
	return r.Replace(s)
}

// stripMarkdownFences strips surrounding markdown code blocks (``` or `).
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") && len(s) >= 6 {
		s = s[3 : len(s)-3]
		// Drop optional language identifier on the first line
		if idx := strings.IndexByte(s, '\n'); idx != -1 {
			tag := strings.ToLower(strings.TrimSpace(s[:idx]))
			if tag == "grol" || tag == "go" || tag == "javascript" || tag == "js" || tag == "py" || tag == "" {
				s = s[idx+1:]
			}
		}
		return strings.TrimSpace(s)
	}

	if strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") && len(s) >= 2 {
		return strings.TrimSpace(s[1 : len(s)-1])
	}

	return s
}

// extractQuotedCode extracts text payload from a quoted WhatsApp message.
func extractQuotedCode(ctx *dispatch.Context) string {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ""
	}

	switch {
	case quoted.GetConversation() != "":
		return quoted.GetConversation()
	case quoted.GetExtendedTextMessage() != nil && quoted.GetExtendedTextMessage().GetText() != "":
		return quoted.GetExtendedTextMessage().GetText()
	case quoted.GetImageMessage() != nil && quoted.GetImageMessage().GetCaption() != "":
		return quoted.GetImageMessage().GetCaption()
	case quoted.GetVideoMessage() != nil && quoted.GetVideoMessage().GetCaption() != "":
		return quoted.GetVideoMessage().GetCaption()
	case quoted.GetDocumentMessage() != nil && quoted.GetDocumentMessage().GetCaption() != "":
		return quoted.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}

// getSessionKey builds a unique session identifier per chat (and per sender in groups).
func getSessionKey(ctx *dispatch.Context) string {
	chatStr := ctx.Chat.String()
	if ctx.IsGroup() {
		return chatStr + ":" + ctx.Sender.ToNonAD().User
	}
	return chatStr
}

// grolHelpText returns formatted instructions and examples.
func grolHelpText(prefix string) string {
	return fmt.Sprintf("*Grol Interpreter (Go REPL Open Language)*\n\n"+
		"*Usage:*\n"+
		"• `%[1]sgrol <code or math>` — evaluate expression\n"+
		"• `%[1]sgrol -f <code>` — format/beautify code\n"+
		"• `%[1]sgrol -p <code>` — show parse tree and eval\n"+
		"• `%[1]sgrol -c <code>` — compact output\n"+
		"• `%[1]sgrol -clean <code>` — run in isolated state\n"+
		"• `%[1]sgrol reset` — clear session variables\n"+
		"• Reply to any message with `%[1]sgrol` to evaluate it\n\n"+
		"*Examples:*\n"+
		"• `%[1]sgrol 2 + 2 * 10`\n"+
		"• `%[1]sgrol fact = func(n) { if n <= 1 { return 1 } n * fact(n-1) }; fact(6)`\n"+
		"• `%[1]sgrol [1, 2, 3].map(func(x) { x * 2 })`\n"+
		"• `%[1]sgrol sprintf(\"%%.2f\", sqrt(2))`\n"+
		"• `%[1]sgrol json({\"name\": \"Grol\", \"ok\": true})`",
		prefix)
}

func handleGrol(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	flags := parseGrolFlags(ctx.RawArgs)
	sessionKey := getSessionKey(ctx)

	if flags.showHelp {
		return ctx.Reply(grolHelpText(p))
	}

	if flags.resetState {
		existed := sessionsRegistry.deleteSession(sessionKey)
		if existed {
			return ctx.Reply("Session reset. All saved Grol variables have been cleared.")
		}
		return ctx.Reply("No active Grol session found to reset.")
	}

	code := flags.code
	if strings.TrimSpace(code) == "" {
		code = extractQuotedCode(ctx)
	}

	code = normalizeSmartQuotes(code)
	code = stripMarkdownFences(code)

	if strings.TrimSpace(code) == "" {
		return ctx.Reply(grolHelpText(p))
	}

	replyText, err := runGrol(ctx.GetSendContext(), sessionKey, code, flags)
	if err != nil {
		return ctx.Replyf("*Grol Error:*\n```\n%v\n```", err)
	}

	return ctx.Reply(replyText)
}

// runGrol executes or formats Grol code with strict timeout and output size limits.
func runGrol(ctx context.Context, sessionKey, code string, flags grolParsedFlags) (string, error) {
	evalCtx, cancel := context.WithTimeout(ctx, maxGrolExecTimeout)
	defer cancel()

	var state *eval.State
	var sessionLock *sync.Mutex

	if flags.cleanState || flags.formatOnly {
		state = eval.NewState()
	} else {
		sess := sessionsRegistry.getSession(sessionKey)
		sessionLock = &sess.mu
		sessionLock.Lock()
		defer sessionLock.Unlock()
		state = sess.state
	}

	outBuf := &strings.Builder{}
	state.Out = outBuf
	state.LogOut = outBuf
	state.NoLog = true

	opts := repl.EvalStringOptions()
	opts.MaxDepth = eval.DefaultMaxDepth - 1
	opts.MaxDuration = maxGrolExecTimeout
	opts.Compact = flags.compact
	opts.FormatOnly = flags.formatOnly
	opts.ShowParse = flags.showParse

	incomplete, panicked, errs, formatted := repl.EvalOne(evalCtx, state, code, outBuf, opts)
	if incomplete && len(errs) == 0 {
		errs = append(errs, "Incomplete input (missing closing parenthesis, brace, or bracket)")
	}
	if panicked && len(errs) == 0 {
		errs = append(errs, "Execution panicked")
	}

	out := strings.TrimSpace(outBuf.String())
	if runes := []rune(out); len(runes) > maxGrolOutputRunes {
		out = string(runes[:maxGrolOutputRunes]) + "\n... (truncated)"
	}

	if flags.formatOnly {
		cleanFormatted := strings.TrimSpace(formatted)
		if len(errs) > 0 {
			return fmt.Sprintf("*Grol Parse Error:*\n```\n%s\n```", strings.Join(errs, "\n")), nil
		}
		return fmt.Sprintf("*Grol Formatted:*\n```grol\n%s\n```", cleanFormatted), nil
	}

	if flags.showParse {
		if len(errs) > 0 {
			return fmt.Sprintf("*Grol Parse & Eval Result:*\n```\n%s\n```\n\n*Errors:*\n```\n%s\n```",
				out, strings.Join(errs, "\n")), nil
		}
		return fmt.Sprintf("*Grol Parse & Eval Result:*\n```\n%s\n```", out), nil
	}

	// Standard evaluation output
	if len(errs) > 0 {
		if out != "" {
			return fmt.Sprintf("*Grol Output:*\n```\n%s\n```\n\n*Error:*\n```\n%s\n```",
				out, strings.Join(errs, "\n")), nil
		}
		return fmt.Sprintf("*Grol Error:*\n```\n%s\n```", strings.Join(errs, "\n")), nil
	}

	if out == "" {
		return "*Grol Output:* `(executed successfully, nil result)`", nil
	}

	return fmt.Sprintf("*Grol Output:*\n```\n%s\n```", out), nil
}
