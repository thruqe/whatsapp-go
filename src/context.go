package src

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// PluginContext is passed to every command handler.
type PluginContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Client     *whatsmeow.Client
	Evt        *events.Message

	Command string   // the command word itself, e.g. "ping"
	Args    []string // remaining whitespace-split args
	RawArgs string   // everything after the command, unsplit

	Chat   types.JID
	Sender types.JID

	autoLoaderMu  sync.Mutex
	autoLoader    *Loader
	loaderTimer   *time.Timer
	loaderStopped bool
}

// Cancel invokes the context cancel function if set.
func (c *PluginContext) Cancel() {
	if c.CancelFunc != nil {
		c.CancelFunc()
	}
}

// GetSendContext returns c.Ctx if active and non-canceled, or fallback context.Background() to prevent context canceled errors.
func (c *PluginContext) GetSendContext() context.Context {
	if c == nil || c.Ctx == nil {
		return context.Background()
	}
	if err := c.Ctx.Err(); err != nil {
		return context.Background()
	}
	return c.Ctx
}

// GetPrefix returns the primary active command prefix from the database settings, or "." default.
func (c *PluginContext) GetPrefix() string {
	if c.Client == nil || c.Client.Store == nil || c.Client.Store.Identities == nil {
		return "."
	}
	s, ok := c.Client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return "."
	}
	raw, err := s.GetSetting(c.Ctx, "prefix")
	if err != nil || raw == "" {
		return "."
	}
	parts := strings.Fields(raw)
	if len(parts) > 0 {
		if strings.EqualFold(parts[0], "none") || strings.EqualFold(parts[0], "empty") {
			return ""
		}
		p := parts[0]
		if isWordPrefix(p) {
			return p + " "
		}
		return p
	}
	return "."
}

func isWordPrefix(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// GetBotName returns the configured bot display name from database settings, or "WhatsRook" default.
func (c *PluginContext) GetBotName() string {
	if c.Client == nil || c.Client.Store == nil || c.Client.Store.Identities == nil {
		return "WhatsRook"
	}
	s, ok := c.Client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return "WhatsRook"
	}
	raw, err := s.GetSetting(c.Ctx, "bot_name")
	if err != nil || strings.TrimSpace(raw) == "" {
		return "WhatsRook"
	}
	return strings.TrimSpace(raw)
}

// HasArgs returns true if the command was invoked with any positional arguments.
func (c *PluginContext) HasArgs() bool {
	return len(c.Args) > 0
}

// GetArg returns the argument at the given 0-indexed position, or empty string if out of bounds.
func (c *PluginContext) GetArg(index int) string {
	if index >= 0 && index < len(c.Args) {
		return c.Args[index]
	}
	return ""
}

// GetArgOrDefault returns the argument at position index, or defaultVal if out of bounds or empty.
func (c *PluginContext) GetArgOrDefault(index int, defaultVal string) string {
	val := c.GetArg(index)
	if val == "" {
		return defaultVal
	}
	return val
}

// ReplyError sends a formatted error reply to the current chat.
func (c *PluginContext) ReplyError(msg string) error {
	return c.Reply("⚠️ " + msg)
}

// ReplyErrorf formats and sends an error reply to the current chat.
func (c *PluginContext) ReplyErrorf(format string, args ...any) error {
	return c.Reply("⚠️ " + fmt.Sprintf(format, args...))
}

// Text initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) Text(initial ...string) *TextBuilder {
	return NewTextWithContext(c, initial...)
}

// NewText initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) NewText(initial ...string) *TextBuilder {
	return NewTextWithContext(c, initial...)
}

// NewTextf initializes a new TextBuilder with formatted text bound to this PluginContext.
func (c *PluginContext) NewTextf(format string, args ...any) *TextBuilder {
	b := NewTextWithContext(c)
	b.Textf(format, args...)
	return b
}

// Poll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) Poll(question string) *PollBuilder {
	return c.Rook().NewPoll(question)
}

// NewPoll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) NewPoll(question string) *PollBuilder {
	return c.Rook().NewPoll(question)
}

// Rook returns a WARook builder engine bound to this PluginContext.
func (c *PluginContext) Rook() *WARook {
	return From(c)
}

// StartAutoLoader arms a delayed loader that appears in chat with "Please wait"
// if an operation takes longer than delay. If delay <= 0, 1200ms is used.
func (c *PluginContext) StartAutoLoader(delay ...time.Duration) {
	if c == nil || c.Client == nil {
		return
	}
	c.autoLoaderMu.Lock()
	defer c.autoLoaderMu.Unlock()

	if c.loaderStopped || c.loaderTimer != nil || c.autoLoader != nil {
		return
	}

	d := 1200 * time.Millisecond
	if len(delay) > 0 && delay[0] > 0 {
		d = delay[0]
	}

	c.loaderTimer = time.AfterFunc(d, func() {
		c.autoLoaderMu.Lock()
		if c.loaderStopped || c.loaderTimer == nil {
			c.autoLoaderMu.Unlock()
			return
		}
		c.loaderTimer = nil
		c.autoLoaderMu.Unlock()

		l := c.StartLoader("Please wait")

		c.autoLoaderMu.Lock()
		if c.loaderStopped {
			c.autoLoaderMu.Unlock()
			l.Delete()
			return
		}
		c.autoLoader = l
		c.autoLoaderMu.Unlock()
	})
}

// StopAutoLoader disarms any pending timer or stops and deletes an active loader message.
func (c *PluginContext) StopAutoLoader() {
	if c == nil {
		return
	}
	c.autoLoaderMu.Lock()
	c.loaderStopped = true
	if c.loaderTimer != nil {
		c.loaderTimer.Stop()
		c.loaderTimer = nil
	}
	loader := c.autoLoader
	c.autoLoader = nil
	c.autoLoaderMu.Unlock()

	if loader != nil {
		loader.Delete()
	}
}
