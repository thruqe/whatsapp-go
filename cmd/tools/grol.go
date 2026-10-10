// grol.go implements an embedded Grol (Go REPL Open Language) scripting engine
// for WhatsApp chat environments. It enables evaluating mathematical expressions,
// algorithms, string transformations, and scripting with safe session sandboxing.
package tools

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image/png"
	"io"
	"strings"
	"sync"
	"time"

	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/object"
	"grol.io/grol/repl"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
)

var (
	//go:embed discord.gr
	discordLibraryCode string

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
	initSessionState(sess.state)

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

func initSessionState(state *eval.State) {
	if discordLibraryCode == "" {
		return
	}
	opts := repl.EvalStringOptions()
	opts.MaxDepth = 3000
	buf := &strings.Builder{}
	state.Out = buf
	state.LogOut = buf
	state.NoLog = true
	_, _, _, _ = repl.EvalOne(context.Background(), state, discordLibraryCode, buf, opts)
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

	replyText, err := runGrol(ctx, sessionKey, code, flags)
	if err != nil {
		return ctx.Replyf("*Grol Error:*\n```\n%v\n```", err)
	}

	return ctx.Reply(replyText)
}

// RunGrolScript evaluates Grol code in the context of an incoming message and returns raw output string.
func RunGrolScript(ctx *dispatch.Context, code string) (string, error) {
	sessionKey := getSessionKey(ctx)
	flags := parseGrolFlags(code)
	c := flags.code
	if strings.TrimSpace(c) == "" {
		c = code
	}
	c = normalizeSmartQuotes(c)
	c = stripMarkdownFences(c)

	var pCtx *dispatch.Context
	var baseCtx context.Context
	if ctx != nil {
		pCtx = ctx
		baseCtx = ctx.GetSendContext()
	} else {
		baseCtx = context.Background()
	}

	evalCtx, cancel := context.WithTimeout(baseCtx, maxGrolExecTimeout)
	defer cancel()

	sess := sessionsRegistry.getSession(sessionKey)
	sess.mu.Lock()
	defer sess.mu.Unlock()

	var imagesSent int
	attachWhatsAppExtensions(sess.state, pCtx, &imagesSent)

	outBuf := &strings.Builder{}
	sess.state.Out = outBuf
	sess.state.LogOut = outBuf
	sess.state.NoLog = true

	opts := repl.EvalStringOptions()
	opts.MaxDepth = eval.DefaultMaxDepth - 1
	opts.MaxDuration = maxGrolExecTimeout

	incomplete, panicked, errs, _ := repl.EvalOne(evalCtx, sess.state, c, outBuf, opts)
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

	// Auto-send image if an image was newly created and returned without calling SendImage explicitly
	if len(errs) == 0 && imagesSent == 0 && pCtx != nil && out != "" {
		cleanOut := strings.Trim(out, `"`)
		if imgMap, ok := sess.state.Extensions["image.new"].ClientData.(extensions.ImageMap); ok {
			if img, found := imgMap[object.String{Value: cleanOut}]; found {
				var buf bytes.Buffer
				if err := png.Encode(&buf, img.Image); err == nil {
					_ = pCtx.ReplyWithImage(buf.Bytes(), "image/png", "")
					imagesSent++
				}
			}
		}
	}

	if len(errs) > 0 {
		return out, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return out, nil
}

// runGrol executes or formats Grol code with strict timeout and output size limits.
func runGrol(callerCtx any, sessionKey, code string, flags grolParsedFlags) (string, error) {
	var pCtx *dispatch.Context
	var baseCtx context.Context

	switch c := callerCtx.(type) {
	case *dispatch.Context:
		pCtx = c
		baseCtx = c.GetSendContext()
	case context.Context:
		baseCtx = c
	default:
		baseCtx = context.Background()
	}

	evalCtx, cancel := context.WithTimeout(baseCtx, maxGrolExecTimeout)
	defer cancel()

	var state *eval.State
	var sessionLock *sync.Mutex

	if flags.cleanState || flags.formatOnly {
		state = eval.NewState()
		if !flags.formatOnly {
			initSessionState(state)
		}
	} else {
		sess := sessionsRegistry.getSession(sessionKey)
		sessionLock = &sess.mu
		sessionLock.Lock()
		defer sessionLock.Unlock()
		state = sess.state
	}

	var imagesSent int
	attachWhatsAppExtensions(state, pCtx, &imagesSent)

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

	// Auto-send image if an image was newly created and returned without calling SendImage explicitly
	if len(errs) == 0 && imagesSent == 0 && pCtx != nil && out != "" {
		cleanOut := strings.Trim(out, `"`)
		if imgMap, ok := state.Extensions["image.new"].ClientData.(extensions.ImageMap); ok {
			if img, found := imgMap[object.String{Value: cleanOut}]; found {
				var buf bytes.Buffer
				if err := png.Encode(&buf, img.Image); err == nil {
					_ = pCtx.ReplyWithImage(buf.Bytes(), "image/png", "")
					imagesSent++
				}
			}
		}
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
		if imagesSent > 0 {
			return "*Grol Output:* `(image generated and sent successfully)`", nil
		}
		return "*Grol Output:* `(executed successfully, nil result)`", nil
	}

	return fmt.Sprintf("*Grol Output:*\n```\n%s\n```", out), nil
}

func attachWhatsAppExtensions(state *eval.State, ctx *dispatch.Context, imagesSent *int) {
	delete(state.Extensions, "read")

	sendImgExt := object.Extension{
		Name:     "SendImage",
		MinArgs:  1,
		MaxArgs:  2,
		Help:     "SendImage(name, [caption]): send image from canvas/image.new to WhatsApp",
		ArgTypes: []object.Type{object.STRING, object.STRING},
		Callback: func(cdata any, _ string, args []object.Object) object.Object {
			imgName := args[0].(object.String).Value
			caption := ""
			if len(args) > 1 {
				caption = args[1].(object.String).Value
			}
			imgMap, ok := state.Extensions["image.new"].ClientData.(extensions.ImageMap)
			if !ok {
				return object.Errorf("image extension not initialized")
			}
			img, ok := imgMap[args[0]]
			if !ok {
				return object.Errorf("image not found: %q", imgName)
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, img.Image); err != nil {
				return object.Errorf("error encoding png: %v", err)
			}
			if ctx != nil && ctx.Client != nil {
				if err := ctx.ReplyWithImage(buf.Bytes(), "image/png", caption); err != nil {
					return object.Errorf("error sending image: %v", err)
				}
				if imagesSent != nil {
					*imagesSent++
				}
			} else if imagesSent != nil {
				*imagesSent++
			}
			return object.String{Value: "image_sent"}
		},
	}
	state.Extensions["SendImage"] = sendImgExt
	state.Extensions["send_image"] = sendImgExt
	state.Extensions["sendImage"] = sendImgExt

	msgComplexExt := object.Extension{
		Name:     "ChannelMessageSendComplex",
		MinArgs:  1,
		MaxArgs:  2,
		Help:     "ChannelMessageSendComplex(map, [imgName]): send formatted text and/or image to WhatsApp",
		ArgTypes: []object.Type{object.MAP, object.STRING},
		Callback: func(cdata any, _ string, args []object.Object) object.Object {
			if ctx == nil {
				return object.Errorf("context not available")
			}
			msgMap, ok := args[0].(object.Map)
			if !ok {
				return object.Errorf("first argument must be a map")
			}
			content := ""
			if val, found := msgMap.Get(object.String{Value: "content"}); found {
				if s, ok := val.(object.String); ok {
					content = s.Value
				}
			}
			var imgName string
			if len(args) > 1 {
				if s, ok := args[1].(object.String); ok {
					imgName = s.Value
				}
			}
			if imgName != "" {
				imgMap, ok := state.Extensions["image.new"].ClientData.(extensions.ImageMap)
				if ok {
					if img, ok := imgMap[object.String{Value: imgName}]; ok {
						var buf bytes.Buffer
						if err := png.Encode(&buf, img.Image); err == nil {
							if ctx.Client != nil {
								_ = ctx.ReplyWithImage(buf.Bytes(), "image/png", content)
							}
							if imagesSent != nil {
								*imagesSent++
							}
							return object.String{Value: "msg_sent"}
						}
					}
				}
			}
			if content != "" && ctx.Client != nil {
				_ = ctx.Reply(content)
			}
			return object.String{Value: "msg_sent"}
		},
	}
	state.Extensions["ChannelMessageSendComplex"] = msgComplexExt
	state.Extensions["InteractionRespond"] = msgComplexExt
	state.Extensions["ChannelMessageSend"] = object.Extension{
		Name:     "ChannelMessageSend",
		MinArgs:  2,
		MaxArgs:  2,
		ArgTypes: []object.Type{object.STRING, object.STRING},
		Callback: func(cdata any, _ string, args []object.Object) object.Object {
			if ctx != nil && ctx.Client != nil {
				_ = ctx.Reply(args[1].(object.String).Value)
			}
			return object.String{Value: "msg_sent"}
		},
	}
}
