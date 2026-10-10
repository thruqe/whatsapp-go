package botctx

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatsrook/util/builder"
	"whatsrook/util/logger"
	"whatsrook/util/message"
)

// Text & Formatting Facades
var (
	// Bold returns plain text without WhatsApp bold formatting symbols (*).
	Bold = builder.Bold
	// Boldf formats text according to format specifier without bold formatting symbols.
	Boldf = builder.Boldf
	// Italic returns plain text without WhatsApp italic formatting symbols (_).
	Italic = builder.Italic
	// Italicf formats text according to format specifier without italic formatting symbols.
	Italicf = builder.Italicf
	// Code returns plain text without WhatsApp inline code formatting symbols (`).
	Code = builder.Code
	// Codef formats text according to format specifier without inline code formatting symbols.
	Codef = builder.Codef
	// CodeBlock returns plain text without WhatsApp code block formatting symbols (```).
	CodeBlock = builder.CodeBlock
	// Strike returns plain text without WhatsApp strikethrough formatting symbols (~).
	Strike = builder.Strike
	// Strikef formats text according to format specifier without strikethrough formatting symbols.
	Strikef = builder.Strikef
	// Quote returns plain text without WhatsApp quote formatting symbols (>).
	Quote = builder.Quote
	// Quotef formats text according to format specifier without quote formatting symbols.
	Quotef = builder.Quotef
	// NewText creates an interactive text builder instance.
	NewText = builder.NewText
	// Sprintf returns a formatted string.
	Sprintf = fmt.Sprintf
)

// Loader represents a no-op loader retained for interface compatibility.
type Loader struct{}

// StartLoader returns a no-op Loader.
func (c *PluginContext) StartLoader(initialText ...string) *Loader {
	return &Loader{}
}

// Cancel is a no-op.
func (l *Loader) Cancel() {}

// MessageID returns an empty MessageID.
func (l *Loader) MessageID() types.MessageID { return "" }

// Stop is a no-op.
func (l *Loader) Stop() {}

// Done is a no-op.
func (l *Loader) Done(finalText string) {}

// Delete is a no-op.
func (l *Loader) Delete() {}

// CancelLoader is a no-op loader cancellation function.
func CancelLoader(id string) bool { return false }

// PluginContext captures the invocation execution environment for external/native plugin actions.
type PluginContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Client     *whatsmeow.Client
	Evt        *events.Message

	Command string
	Args    []string
	RawArgs string

	Chat   types.JID
	Sender types.JID

	isOwnerOnce sync.Once
	isOwnerVal  bool
	isSudoOnce  sync.Once
	isSudoVal   bool
}

// Cancel invokes context cancellation if configured.
func (c *PluginContext) Cancel() {
	if c.CancelFunc != nil {
		c.CancelFunc()
	}
}

// GetSendContext returns an active, non-canceled Context suitable for network dispatch.
func (c *PluginContext) GetSendContext() context.Context {
	if c == nil || c.Ctx == nil || c.Ctx.Err() != nil {
		return context.Background()
	}
	return c.Ctx
}

// GetClient returns the underlying whatsmeow client instance.
func (c *PluginContext) GetClient() *whatsmeow.Client {
	if c == nil {
		return nil
	}
	return c.Client
}

// GetChat returns the current chat JID.
func (c *PluginContext) GetChat() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Chat
}

// GetSender returns the triggering sender JID.
func (c *PluginContext) GetSender() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Sender
}

// IsGroup returns true if the current message event was sent in a group chat.
func (c *PluginContext) IsGroup() bool {
	if c == nil {
		return false
	}
	if c.Evt != nil && c.Evt.Info.IsGroup {
		return true
	}
	return c.Chat.Server == types.GroupServer
}

// FormatTextResponse formats the text response stripping unwanted symbols.
func (c *PluginContext) FormatTextResponse(text string) string {
	return c.formatTextResponse(text)
}

// ReplyContextInfo returns the ContextInfo configured for quoted reply.
func (c *PluginContext) ReplyContextInfo() *waE2E.ContextInfo {
	return c.replyContextInfo()
}

func (c *PluginContext) formatTextResponse(text string) string {
	return message.FormatTextResponseRaw(text)
}

func (c *PluginContext) replyContextInfo() *waE2E.ContextInfo {
	if c.Evt == nil {
		return nil
	}
	participant := c.Evt.Info.Sender.ToNonAD().String()
	stanzaID := c.Evt.Info.ID

	var quotedMsg *waE2E.Message
	if c.Evt.Message != nil {
		unwrapped := message.UnwrapMessageProto(c.Evt.Message)
		if unwrapped != nil {
			if cloned, ok := proto.Clone(unwrapped).(*waE2E.Message); ok && cloned != nil {
				message.StripContextInfo(cloned)
				quotedMsg = cloned
			}
		}
	}

	ci := &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		QuotedMessage: quotedMsg,
	}
	if c.Evt.Info.IsGroup {
		ci.Participant = &participant
	}
	return ci
}

// GetPrefixes returns all configured command prefixes, default ["."].
func (c *PluginContext) GetPrefixes() []string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "prefix"); err == nil && strings.TrimSpace(val) != "" {
			parts := strings.Fields(val)
			if len(parts) > 0 {
				var res []string
				for _, p := range parts {
					if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
						res = append(res, "")
					} else {
						res = append(res, p)
					}
				}
				if len(res) > 0 {
					return res
				}
			}
		}
	}
	return []string{"."}
}

// GetPrefix returns the primary configured command prefix, default ".".
func (c *PluginContext) GetPrefix() string {
	prefixes := c.GetPrefixes()
	if len(prefixes) > 0 {
		return prefixes[0]
	}
	return "."
}

// GetBotName returns the configured bot display name, default "WhatsRook".
func (c *PluginContext) GetBotName() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "bot_name"); err == nil && val != "" {
			return val
		}
	}
	return "WhatsRook"
}

// GetStickerPack returns the configured default sticker pack name, defaulting to GetBotName().
func (c *PluginContext) GetStickerPack() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "sticker_pack"); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return c.GetBotName()
}

// GetStickerAuthor returns the configured default sticker author/publisher, defaulting to "WhatsRook".
func (c *PluginContext) GetStickerAuthor() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "sticker_author"); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return "WhatsRook"
}

// GetQuotedMessage returns the quoted message proto if the triggering message is a reply.
func (c *PluginContext) GetQuotedMessage() *waE2E.Message {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	ci := message.GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		return message.UnwrapMessageProto(ci.QuotedMessage)
	}
	return nil
}

// GetQuotedSender returns the quoted message sender JID.
func (c *PluginContext) GetQuotedSender() (types.JID, bool) {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return types.EmptyJID, false
	}
	ci := message.GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.Participant != nil && *ci.Participant != "" {
		if parsed, err := types.ParseJID(*ci.Participant); err == nil && !parsed.IsEmpty() {
			return parsed.ToNonAD(), true
		}
	}
	// Fallback for 1-on-1 (DM) replies where Participant may be omitted by WhatsApp clients:
	if ci != nil && ci.QuotedMessage != nil && !c.Chat.IsEmpty() && c.Chat.Server != "g.us" && c.Chat.Server != "broadcast" {
		if c.Evt.Info.IsFromMe {
			return c.Chat.ToNonAD(), true
		}
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			return c.Client.Store.ID.ToNonAD(), true
		}
	}
	// Fallback for replies where Participant is omitted because the user replied to themselves (fromMe):
	if ci != nil && ci.QuotedMessage != nil && c.Evt.Info.IsFromMe {
		if !c.Sender.IsEmpty() {
			return c.Sender.ToNonAD(), true
		}
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			return c.Client.Store.ID.ToNonAD(), true
		}
	}
	return types.EmptyJID, false
}

// GetContextInfo returns the ContextInfo from the triggering message.
func (c *PluginContext) GetContextInfo() *waE2E.ContextInfo {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	return message.GetContextInfoFromProto(c.Evt.Message)
}

// GetMentionedJIDs returns any mentioned JIDs in the triggering message context.
func (c *PluginContext) GetMentionedJIDs() []types.JID {
	ci := c.GetContextInfo()
	if ci == nil || len(ci.MentionedJID) == 0 {
		return nil
	}
	var res []types.JID
	for _, m := range ci.MentionedJID {
		if parsed, err := types.ParseJID(m); err == nil {
			res = append(res, parsed)
		}
	}
	return res
}

// GetMedia downloads media bytes and mimetype from the triggering message or quoted message.
func (c *PluginContext) GetMedia() ([]byte, string, error) {
	if c.Client == nil {
		return nil, "", fmt.Errorf("client unavailable")
	}

	if c.Evt != nil && c.Evt.Message != nil {
		if data, mime, err := message.Download(c.GetSendContext(), c.Client, c.Evt.Message); err == nil && len(data) > 0 {
			return data, mime, nil
		}
	}
	if quoted := c.GetQuotedMessage(); quoted != nil {
		if data, mime, err := message.Download(c.GetSendContext(), c.Client, quoted); err == nil && len(data) > 0 {
			return data, mime, nil
		}
	}
	logger.Warn("PluginContext.GetMedia: no media found or download failed")
	return nil, "", fmt.Errorf("no media found")
}
