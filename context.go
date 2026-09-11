package whatsrook

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

	"whatsrook/builder"
	"whatsrook/util/logger"
)

// ─── Text & Formatting Facades ─────────────────────────────────────────────

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
)

// ─── Interactive Loader Engine (No-Op) ─────────────────────────────────────

// Loader represents a no-op loader retained for interface compatibility.
type Loader struct{}

// StartLoader returns a no-op Loader.
func (ctx *PluginContext) StartLoader(initialText ...string) *Loader {
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

// ─── Plugin Context Model ──────────────────────────────────────────────────

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
	text = strings.ReplaceAll(text, "*", "")
	text = RemoveEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return text
}

func (c *PluginContext) replyContextInfo() *waE2E.ContextInfo {
	if c.Evt == nil {
		return nil
	}
	participant := c.Evt.Info.Sender.ToNonAD().String()
	stanzaID := c.Evt.Info.ID

	var quotedMsg *waE2E.Message
	if c.Evt.Message != nil {
		unwrapped := UnwrapMessageProto(c.Evt.Message)
		if unwrapped != nil {
			if cloned, ok := proto.Clone(unwrapped).(*waE2E.Message); ok && cloned != nil {
				stripContextInfo(cloned)
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

// GetPrefix returns the configured command prefix, default ".".
func (c *PluginContext) GetPrefix() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "prefix"); err == nil && val != "" {
			return val
		}
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

// IsOwner returns true if the sender is the primary bot owner.
func (c *PluginContext) IsOwner() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isOwnerOnce.Do(func() {
		c.isOwnerVal = c.calculateIsOwner()
	})
	return c.isOwnerVal
}

func (c *PluginContext) calculateIsOwner() bool {
	if c.Evt != nil && c.Evt.Info.IsFromMe {
		return true
	}
	if c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, c.Client.Store.LID) {
		return true
	}
	if c.Evt != nil && !c.Evt.Info.SenderAlt.IsEmpty() {
		if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, *c.Client.Store.ID) {
			return true
		}
		if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, c.Client.Store.LID) {
			return true
		}
	}
	return false
}

// IsSudo returns true if the sender is a sudo user or bot owner.
func (c *PluginContext) IsSudo() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isSudoOnce.Do(func() {
		c.isSudoVal = c.calculateIsSudo()
	})
	return c.isSudoVal
}

func (c *PluginContext) calculateIsSudo() bool {
	if c.IsOwner() {
		return true
	}
	if IsSudoRaw(c.GetSendContext(), c.Client, c.Sender) {
		return true
	}
	if c.Evt != nil {
		if !c.Evt.Info.SenderAlt.IsEmpty() {
			if IsSudoRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt) {
				c.Client.StoreLIDPNMapping(c.GetSendContext(), c.Evt.Info.SenderAlt, c.Sender)
				return true
			}
		}
		if c.Evt.Info.PushName != "" {
			if raw, err := GetClientSetting(c.GetSendContext(), c.Client, "sudoers"); err == nil && raw != "" {
				pushLower := strings.ToLower(strings.TrimSpace(c.Evt.Info.PushName))
				for sudoerStr := range strings.FieldsSeq(raw) {
					if strings.EqualFold(sudoerStr, pushLower) {
						return true
					}
				}
			}
		}
	}
	return false
}

// GetQuotedMessage returns the quoted message proto if the triggering message is a reply.
func (c *PluginContext) GetQuotedMessage() *waE2E.Message {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		return UnwrapMessageProto(ci.QuotedMessage)
	}
	return nil
}

// GetQuotedSender returns the quoted message sender JID.
func (c *PluginContext) GetQuotedSender() (types.JID, bool) {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return types.EmptyJID, false
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
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
	return types.EmptyJID, false
}

// GetContextInfo returns the ContextInfo from the triggering message.
func (c *PluginContext) GetContextInfo() *waE2E.ContextInfo {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	return GetContextInfoFromProto(c.Evt.Message)
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

// IsSenderAdmin checks if the sender has admin privileges in the provided groupInfo.
func (c *PluginContext) IsSenderAdmin(groupInfo *types.GroupInfo) bool {
	if c == nil || c.Client == nil || groupInfo == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, groupInfo, c.Sender)
}

// ResolveMentionJIDs resolves a JID to its full set of JIDs (phone-number JID + LID)
// needed in ContextInfo.MentionedJID, and the parsed user string (without @) for text mentions.
func (c *PluginContext) ResolveMentionJIDs(jid types.JID) ([]types.JID, string) {
	return ResolveMentionJIDs(c.GetSendContext(), c.Client, jid)
}

// ResolveMention resolves a JID to its primary normalized non-AD JID and user string (without @).
func (c *PluginContext) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(c.GetSendContext(), c.Client, jid)
}

// FormatMention returns "@user" string and the resolved primary JID.
func (c *PluginContext) FormatMention(jid types.JID) (string, types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	resolved := jid.ToNonAD()
	if len(jids) > 0 {
		resolved = jids[0]
	}
	return "@" + tagUser, resolved
}

// FormatMentionJIDs returns "@user" string and all associated JIDs (PN + LID).
func (c *PluginContext) FormatMentionJIDs(jid types.JID) (string, []types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	return "@" + tagUser, jids
}

// GetContactName returns the display push name or full name of a contact, or falls back to the phone/user ID.
func (c *PluginContext) GetContactName(jid types.JID) string {
	return ResolveContactName(c.GetSendContext(), c.Client, jid)
}

// ResolvePN returns the normalized non-AD phone number JID.
func (c *PluginContext) ResolvePN(jid types.JID) types.JID {
	return jid.ToNonAD()
}

// IsSameUser compares two JIDs ignoring device suffixes.
func (c *PluginContext) IsSameUser(a, b types.JID) bool {
	return IsSameUserRaw(c.GetSendContext(), c.Client, a, b)
}

// IsTargetSudo checks if a target JID is a sudo user or owner.
func (c *PluginContext) IsTargetSudo(target types.JID) bool {
	if c == nil || c.Client == nil {
		return false
	}
	return IsSudoRaw(c.GetSendContext(), c.Client, target)
}

// IsTargetOwner checks if a target JID is the bot owner.
func (c *PluginContext) IsTargetOwner(target types.JID) bool {
	if c == nil || c.Client == nil || c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && c.IsSameUser(target, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && c.IsSameUser(target, c.Client.Store.LID) {
		return true
	}
	return false
}

// GetTargets resolves target user JIDs from quoted reply, mentions, or arguments.
func (c *PluginContext) GetTargets() []types.JID {
	if c == nil {
		return nil
	}
	if q, ok := c.GetQuotedSender(); ok && !q.IsEmpty() {
		return []types.JID{q.ToNonAD()}
	}
	if m := c.GetMentionedJIDs(); len(m) > 0 {
		var resolved []types.JID
		for _, j := range m {
			if !j.IsEmpty() {
				resolved = append(resolved, j.ToNonAD())
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if len(c.Args) > 0 {
		var resolved []types.JID
		for _, arg := range c.Args {
			if strings.Contains(arg, "@") {
				if parsed, err := types.ParseJID(arg); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			clean := strings.TrimLeft(arg, "@+")
			if strings.Contains(clean, "@") {
				if parsed, err := types.ParseJID(clean); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			if len(clean) >= 5 {
				resolved = append(resolved, types.NewJID(clean, types.DefaultUserServer))
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if !c.Chat.IsEmpty() && c.Chat.Server != "g.us" && c.Chat.Server != "broadcast" {
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			if !c.IsSameUser(c.Chat, *c.Client.Store.ID) {
				if len(c.Args) == 0 {
					c.Args = []string{c.Chat.ToNonAD().String()}
				}
				return []types.JID{c.Chat.ToNonAD()}
			}
		} else {
			if len(c.Args) == 0 {
				c.Args = []string{c.Chat.ToNonAD().String()}
			}
			return []types.JID{c.Chat.ToNonAD()}
		}
	}
	return nil
}

// GetMedia downloads media bytes and mimetype from the triggering message or quoted message.
func (c *PluginContext) GetMedia() ([]byte, string, error) {
	if c.Client == nil {
		return nil, "", fmt.Errorf("client unavailable")
	}
	extract := func(msg *waE2E.Message) ([]byte, string, bool) {
		msg = UnwrapMessageProto(msg)
		if msg == nil {
			return nil, "", false
		}
		var downloadable whatsmeow.DownloadableMessage
		var mime string
		if img := msg.GetImageMessage(); img != nil {
			downloadable = img
			mime = img.GetMimetype()
		} else if vid := msg.GetVideoMessage(); vid != nil {
			downloadable = vid
			mime = vid.GetMimetype()
		} else if aud := msg.GetAudioMessage(); aud != nil {
			downloadable = aud
			mime = aud.GetMimetype()
		} else if doc := msg.GetDocumentMessage(); doc != nil {
			downloadable = doc
			mime = doc.GetMimetype()
		} else if stk := msg.GetStickerMessage(); stk != nil {
			downloadable = stk
			mime = stk.GetMimetype()
		}
		if downloadable == nil {
			return nil, "", false
		}
		data, err := c.Client.Download(c.GetSendContext(), downloadable)
		if err != nil {
			logger.Warn("PluginContext.GetMedia: download failed", "mime", mime, "err", err)
			return nil, "", false
		}
		return data, mime, true
	}

	if c.Evt != nil && c.Evt.Message != nil {
		if data, mime, ok := extract(c.Evt.Message); ok {
			return data, mime, nil
		}
	}
	if quoted := c.GetQuotedMessage(); quoted != nil {
		if data, mime, ok := extract(quoted); ok {
			return data, mime, nil
		}
	}
	return nil, "", fmt.Errorf("no media found")
}

// IsAdmin checks if a specific JID is a group admin.
func (c *PluginContext) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, info, jid)
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (c *PluginContext) AmIAdmin(info *types.GroupInfo) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsBotAdminRaw(c.GetSendContext(), c.Client, info)
}
