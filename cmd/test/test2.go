// Package test hosts owner-only experimental commands used to probe how
// WhatsApp clients render interactive message types, webviews, and ghost messages.
package test

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
	"whatsrook/util/send"
)

const (
	// ZeroWidthInvisibleChars is a sequence of zero-width and directional formatting
	// characters that pass WhatsApp non-empty payload validation while rendering 0px width.
	ZeroWidthInvisibleChars = "\u200B\u200C\u200D\u200E\u200F"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:         "test2",
		Alias:        "ghost",
		Description:  "Send a whisper / ghost message only visible to a specific participant (as seen in image.png)",
		Category:     "owner",
		HideFromMenu: true,
		Handler:      handleTest2,
	})
	// Also register whisper alias
	dispatch.Register(&dispatch.Command{
		Name:         "whisper",
		Description:  "Send a whisper message only visible to a specific participant (with 'Only you can see this' header)",
		Category:     "owner",
		HideFromMenu: true,
		Handler:      handleTest2,
	})
}

// ghostParsedArgs stores execution flags for test2.
type ghostParsedArgs struct {
	target       string // explicit target phone number or JID
	blankOnly    bool
	mention      bool
	flash        bool
	ephemeral    bool
	viewOnce     bool
	useSenderKey bool   // -skd
	pm           bool   // -pm
	whisper      bool   // -whisper: send with BotInvokeMessage wrapper
	raw          bool   // -raw: send without BotInvokeMessage wrapper
	replyText    string // -reply <text>: send bot reply quoting the whisper
	sendAll      bool
	text         string
}

func parseGhostArgs(raw string) ghostParsedArgs {
	var parsed ghostParsedArgs
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return parsed
	}

	parts := strings.Fields(trimmed)
	var textParts []string

	for i := 0; i < len(parts); i++ {
		arg := parts[i]
		switch {
		case arg == "-to" || arg == "--to" || arg == "-target" || arg == "--target":
			if i+1 < len(parts) {
				parsed.target = parts[i+1]
				i++
			}
		case arg == "-reply" || arg == "--reply":
			if i+1 < len(parts) {
				parsed.replyText = parts[i+1]
				i++
			}
		case strings.HasPrefix(arg, "@") && len(arg) > 5 && isAllDigits(arg[1:]):
			parsed.target = arg[1:]
		case arg == "-b" || arg == "-blank" || arg == "--blank":
			parsed.blankOnly = true
		case arg == "-m" || arg == "-mention" || arg == "--mention" || arg == "-tag" || arg == "--tag":
			parsed.mention = true
		case arg == "-f" || arg == "-flash" || arg == "--flash" || arg == "-v" || arg == "-vanish" || arg == "--vanish":
			parsed.flash = true
		case arg == "-e" || arg == "-ephemeral" || arg == "--ephemeral":
			parsed.ephemeral = true
		case arg == "-vo" || arg == "-viewonce" || arg == "--viewonce":
			parsed.viewOnce = true
		case arg == "-w" || arg == "-whisper" || arg == "--whisper":
			parsed.whisper = true
		case arg == "-raw" || arg == "--raw":
			parsed.raw = true
		case arg == "-skd" || arg == "--skd" || arg == "-senderkey" || arg == "--senderkey":
			parsed.useSenderKey = true
		case arg == "-pm" || arg == "--pm" || arg == "-private" || arg == "--private":
			parsed.pm = true
		case arg == "-all" || arg == "--all":
			parsed.sendAll = true
		default:
			textParts = append(textParts, parts[i:]...)
			i = len(parts) // stop parsing flags
		}
	}

	parsed.text = strings.TrimSpace(strings.Join(textParts, " "))
	return parsed
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveTargetParticipant resolves the single participant intended to receive the ghost message.
func resolveTargetParticipant(ctx *dispatch.Context, explicitTarget string) (types.JID, error) {
	// 1. Explicit target string from -to, -target, or @number
	if explicitTarget != "" {
		clean := strings.TrimSpace(strings.TrimPrefix(explicitTarget, "@"))
		if strings.Contains(clean, "@") {
			if jid, err := types.ParseJID(clean); err == nil && !jid.IsEmpty() {
				return jid.ToNonAD(), nil
			}
		}
		var digits strings.Builder
		for _, r := range clean {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			}
		}
		if digits.Len() >= 5 {
			return types.NewJID(digits.String(), types.DefaultUserServer), nil
		}
	}

	// 2. Mentioned JIDs in ContextInfo (e.g. user selected @user via WhatsApp autocomplete)
	if ci := ctx.GetContextInfo(); ci != nil && len(ci.GetMentionedJID()) > 0 {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := types.ParseJID(m); err == nil && !parsed.IsEmpty() {
				return parsed.ToNonAD(), nil
			}
		}
	}

	// 3. Quoted message sender (if replying to a message)
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		return quotedSender.ToNonAD(), nil
	}

	// 4. In a direct 1:1 chat, target is the chat itself
	if !ctx.IsGroup() && !ctx.Chat.IsEmpty() {
		return ctx.Chat.ToNonAD(), nil
	}

	return types.EmptyJID, fmt.Errorf("no target participant identified")
}

// handleTest2 dispatches a whisper/ghost message to a group that is visible only to a specific participant.
func handleTest2(ctx *dispatch.Context) error {
	opts := parseGhostArgs(ctx.RawArgs)
	targetJID, err := resolveTargetParticipant(ctx, opts.target)

	// In a group chat, if no participant was targeted, show usage guide
	if ctx.IsGroup() && (err != nil || targetJID.IsEmpty()) {
		guide := "👻 *Ghost Message / Whisper (Participant-Only)*\n\n" +
			"Sends a message with the native WhatsApp header:\n" +
			"👁️ _Only you can see this_\n" +
			"_Whispered to <Target>_\n\n" +
			"*How to use:*\n" +
			"  • Reply to a member's message: `.test2 [message]`\n" +
			"  • Mention a member: `.test2 @user [message]`\n" +
			"  • Specify phone number: `.test2 -to 1234567890 [message]`\n\n" +
			"*Options:*\n" +
			"  • `-reply <text>`: Have bot immediately reply quoting the whisper\n" +
			"  • `-raw`        : Send raw message without BotInvokeMessage\n" +
			"  • `-skd`        : Use restricted SenderKey distribution\n" +
			"  • `-pm`         : Send as private reply DM quoting group\n" +
			"  • `-blank`      : Invisible 0-width text bubble\n" +
			"  • `-flash`      : Auto-delete / revoke after 3 seconds"
		return ctx.Reply(guide)
	}

	if !opts.raw {
		opts.whisper = true
	}

	// If in DM and no explicit target, target is the chat partner
	if targetJID.IsEmpty() && !ctx.Chat.IsEmpty() {
		targetJID = ctx.Chat.ToNonAD()
	}

	return sendTargetedGhostMessage(ctx, targetJID, opts)
}

// sendTargetedGhostMessage sends the message so only targetJID can view it.
func sendTargetedGhostMessage(ctx *dispatch.Context, targetJID types.JID, opts ghostParsedArgs) error {
	msgText := opts.text
	if opts.blankOnly {
		msgText = ZeroWidthInvisibleChars
	} else if msgText == "" {
		msgText = "hello"
	}

	invokerJID := ctx.Sender
	if invokerJID.IsEmpty() && ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		invokerJID = ctx.Client.Store.ID.ToNonAD()
	}

	var msg *waE2E.Message
	if opts.whisper {
		// WhatsApp native whisper message: wrapped inside BotInvokeMessage
		// This produces the exact "Only you can see this / Whispered to <Target>" bubble in WhatsApp Web & Mobile
		msg = buildWhisperBotInvokeMessage(msgText, targetJID, invokerJID, ctx.ReplyContextInfo())
	} else if opts.blankOnly {
		msg = buildGhostBlankMessage()
	} else {
		// Clean direct message (Baileys-style ghost message)
		msg = buildGhostTextMessage(msgText, ctx.ReplyContextInfo())
	}

	if opts.ephemeral {
		msg = buildGhostEphemeralMessage(msg)
	}
	if opts.viewOnce {
		msg = buildGhostViewOnceMessage(msg)
	}

	var resp whatsmeow.SendResponse
	var err error

	if opts.pm && ctx.IsGroup() {
		// Native "Reply Privately" to user's direct chat quoting the group
		pmCI := ctx.ReplyContextInfo()
		if pmCI == nil {
			pmCI = &waE2E.ContextInfo{}
		}
		pmCI.RemoteJID = proto.String(ctx.Chat.String())
		pmCI.Participant = proto.String(targetJID.String())

		pmMsg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(msgText),
				ContextInfo: pmCI,
			},
		}
		resp, err = ctx.Client.SendMessage(ctx.GetSendContext(), targetJID, pmMsg)
	} else if ctx.IsGroup() {
		logger.Infof("[TEST2 GHOST] Sending message to group %s targeted exclusively to participant ID: %s (msg: %q)",
			ctx.Chat, targetJID, msgText)
		// Group message targeted exclusively to targetJID using normal send message
		extra := whatsmeow.SendRequestExtra{
			TargetParticipants: []types.JID{targetJID},
		}
		if opts.useSenderKey {
			extra.DirectGroupParticipant = true
		}
		resp, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg, extra)
		if err != nil {
			logger.Errorf("[TEST2 GHOST] Failed to send ghost message to target ID %s in group %s: %v", targetJID, ctx.Chat, err)
		} else {
			logger.Infof("[TEST2 GHOST] Successfully sent ghost message to target ID %s in group %s: msgID=%s", targetJID, ctx.Chat, resp.ID)
			_ = send.ReactMessage(ctx.GetSendContext(), ctx.Client, ctx.Chat, ctx.Evt.Info.ID, "👻")
		}
	} else {
		// Direct 1:1 chat
		logger.Infof("[TEST2 GHOST] Sending 1:1 message to chat ID: %s (msg: %q)", ctx.Chat, msgText)
		resp, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
		if err != nil {
			logger.Errorf("[TEST2 GHOST] Failed to send 1:1 message to %s: %v", ctx.Chat, err)
		} else {
			logger.Infof("[TEST2 GHOST] Successfully sent 1:1 message to chat ID %s: msgID=%s", ctx.Chat, resp.ID)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to send ghost message: %w", err)
	}

	// If -reply flag was provided, simulate bot quoting and replying to the whisper (as seen in image.png)
	if opts.replyText != "" && resp.ID != "" && ctx.Client != nil {
		time.Sleep(300 * time.Millisecond)
		replyCI := &waE2E.ContextInfo{
			StanzaID:      proto.String(string(resp.ID)),
			Participant:   proto.String(invokerJID.ToNonAD().String()),
			QuotedMessage: msg,
		}
		replyMsg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(opts.replyText),
				ContextInfo: replyCI,
			},
		}
		_, _ = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, replyMsg)
	}

	// If flash flag is set, auto-revoke after 3 seconds
	if opts.flash && resp.ID != "" {
		targetChat := ctx.Chat
		if opts.pm && ctx.IsGroup() {
			targetChat = targetJID
		}
		msgID := resp.ID
		go func() {
			time.Sleep(3 * time.Second)
			if ctx.Client != nil {
				_, delErr := send.Delete(context.Background(), ctx.Client, targetChat, msgID)
				if delErr != nil {
					logger.Warn("failed to revoke flash ghost message", "err", delErr)
				}
			}
		}()
	}

	modeName := "whisper (BotInvokeMessage)"
	if opts.raw {
		modeName = "direct ghost"
		if opts.useSenderKey {
			modeName = "SenderKey restricted"
		}
	} else if opts.pm {
		modeName = "private reply DM"
	}

	confirmText := fmt.Sprintf("👻 Ghost message sent to @%s (visible only to them via %s)", targetJID.User, modeName)
	return ctx.ReplyWithMentions(confirmText, []types.JID{targetJID})
}

// buildWhisperBotInvokeMessage builds the exact "Only you can see this / Whispered to <Target>"
// message structure as rendered natively by WhatsApp clients (seen in image.png).
func buildWhisperBotInvokeMessage(text string, targetJID types.JID, invokerJID types.JID, quotedCI *waE2E.ContextInfo) *waE2E.Message {
	targetStr := targetJID.ToNonAD().String()
	invokerStr := invokerJID.ToNonAD().String()

	ci := &waE2E.ContextInfo{
		Participant:  proto.String(targetStr),
		MentionedJID: []string{targetStr},
	}
	if quotedCI != nil {
		ci.StanzaID = quotedCI.StanzaID
		ci.QuotedMessage = quotedCI.QuotedMessage
		if quotedCI.RemoteJID != nil {
			ci.RemoteJID = quotedCI.RemoteJID
		}
	}

	botMetadata := &waAICommon.BotMetadata{
		InvokerJID: proto.String(invokerStr),
		PersonaID:  proto.String(targetJID.User),
	}

	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	innerMsg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ci,
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			BotMetadata:   botMetadata,
			MessageSecret: secret,
		},
	}

	return &waE2E.Message{
		BotInvokeMessage: &waE2E.FutureProofMessage{
			Message: innerMsg,
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			BotMetadata:   botMetadata,
			MessageSecret: secret,
		},
	}
}

// buildGhostTextMessage creates a normal text or extended text message.
func buildGhostTextMessage(text string, quotedCI *waE2E.ContextInfo) *waE2E.Message {
	if quotedCI != nil {
		return &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(text),
				ContextInfo: quotedCI,
			},
		}
	}
	return &waE2E.Message{
		Conversation: proto.String(text),
	}
}

// buildGhostBlankMessage creates a message with 0-width invisible unicode characters.
func buildGhostBlankMessage() *waE2E.Message {
	return &waE2E.Message{
		Conversation: proto.String(ZeroWidthInvisibleChars),
	}
}

// buildGhostEphemeralMessage wraps message into an ephemeral/disappearing container.
func buildGhostEphemeralMessage(inner *waE2E.Message) *waE2E.Message {
	return &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: inner,
		},
	}
}

// buildGhostViewOnceMessage wraps message into a viewOnce container.
func buildGhostViewOnceMessage(inner *waE2E.Message) *waE2E.Message {
	return &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: inner,
		},
	}
}
