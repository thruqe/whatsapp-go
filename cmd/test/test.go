// Package test hosts owner-only experimental commands.
package test

import (
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
	"whatsrook/util/send"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:         "test",
		Description:  "Send a ghost message (defaults to 'hello') visible only to a specific participant (mirroring Baileys sendGhostToGroup)",
		Category:     "owner",
		HideFromMenu: true,
		Handler:      handleTest,
	})
}

// handleTest sends a ghost message to the group visible only to the target participant,
// mirroring Baileys' sendGhostToGroup. Defaults to sending "hello".
func handleTest(ctx *dispatch.Context) error {
	opts := parseGhostArgs(ctx.RawArgs)
	targetJID, err := resolveTargetParticipant(ctx, opts.target)

	logger.Infof("[TEST CMD] handleTest invoked: chat=%s isGroup=%v rawArgs=%q targetJID=%s resolveErr=%v",
		ctx.Chat, ctx.IsGroup(), ctx.RawArgs, targetJID, err)

	// In a group chat, if no participant was targeted, show usage guide
	if ctx.IsGroup() && (err != nil || targetJID.IsEmpty()) {
		guide := "👻 *Ghost Message Test (Participant-Only)*\n\n" +
			"Sends a message in the group chat that is encrypted and delivered\n" +
			"ONLY to the targeted participant ID. No one else in the group sees it.\n\n" +
			"*Usage:*\n" +
			"  • Reply to a member's message: `" + ctx.GetPrefix() + "test [optional text]`\n" +
			"  • Mention a member: `" + ctx.GetPrefix() + "test @user [optional text]`\n" +
			"  • Specify phone number: `" + ctx.GetPrefix() + "test -to 1234567890 [optional text]`\n\n" +
			"Default message is *hello* if no text is specified."
		return ctx.Reply(guide)
	}

	if targetJID.IsEmpty() && !ctx.Chat.IsEmpty() {
		targetJID = ctx.Chat.ToNonAD()
	}

	msgText := opts.text
	if msgText == "" {
		msgText = "hello"
	}

	var msg *waE2E.Message
	if opts.blankOnly {
		msg = buildGhostBlankMessage()
	} else {
		msg = &waE2E.Message{
			Conversation: proto.String(msgText),
		}
	}

	if opts.ephemeral {
		msg = buildGhostEphemeralMessage(msg)
	}
	if opts.viewOnce {
		msg = buildGhostViewOnceMessage(msg)
	}

	if ctx.IsGroup() {
		logger.Infof("[TEST CMD] Sending normal group message to group %s targeted ONLY to participant ID: %s (msg: %q)",
			ctx.Chat, targetJID, msgText)

		// We use the normal SendMessage with TargetParticipants to restrict delivery
		// to just the specific participant ID we want to send the message to.
		resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg, whatsmeow.SendRequestExtra{
			TargetParticipants: []types.JID{targetJID},
		})
		if err != nil {
			logger.Errorf("[TEST CMD] Failed to send ghost message to target ID %s in group %s: %v", targetJID, ctx.Chat, err)
			return fmt.Errorf("failed to send ghost message to group: %w", err)
		}

		logger.Infof("[TEST CMD] Successfully dispatched ghost message to target ID %s in group %s: msgID=%s serverTimestamp=%v",
			targetJID, ctx.Chat, resp.ID, resp.Timestamp)

		_ = send.ReactMessage(ctx.GetSendContext(), ctx.Client, ctx.Chat, ctx.Evt.Info.ID, "👻")

		confirm := fmt.Sprintf("👻 Ghost message %q sent in group %s to participant ID: %s (msgID: %s, visible only to them)",
			msgText, ctx.Chat.User, targetJID.String(), resp.ID)
		return ctx.ReplyWithMentions(confirm, []types.JID{targetJID})
	}

	// 1:1 chat fallback
	logger.Infof("[TEST CMD] Sending 1:1 fallback message to chat ID: %s (msg: %q)", ctx.Chat, msgText)
	resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		logger.Errorf("[TEST CMD] Failed to send 1:1 message to %s: %v", ctx.Chat, err)
		return fmt.Errorf("failed to send message: %w", err)
	}
	logger.Infof("[TEST CMD] 1:1 message sent to %s: msgID=%s", ctx.Chat, resp.ID)
	return ctx.Replyf("Ghost message %q sent to ID: %s (msgID: %s)", msgText, ctx.Chat.String(), resp.ID)
}
