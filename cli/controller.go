// WebSocket control message handlers – send, edit, revoke messages, react, get stats, etc.
package main

import (
	"context"
	"encoding/json"

	"whatsrook/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func (b *Bot) Controller(ctx context.Context, ctrl ControlMessage) EventMessage {
	switch ctrl.Kind {
	case ControlSendMessage:
		return b.CSendMessage(ctx, ctrl)
	case ControlSendReaction:
		return b.CSendReaction(ctx, ctrl)
	case ControlEditMessage:
		return b.CEditMessage(ctx, ctrl)
	case ControlRevokeMessage:
		return b.CRevokeMessage(ctx, ctrl)
	case ControlGetStats:
		return b.CGetStats(ctx, ctrl)
	default:
		Logger.Warn("unknown control type", "kind", ctrl.Kind)
		return ackEvent(ctrl.ID, false, "unknown control type")
	}
}

func (b *Bot) CSendMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	cli := b.client.WAClient()
	if cli == nil {
		return ackEvent(ctrl.ID, false, "client not initialized")
	}
	var p SendMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		Logger.Warn("bad send_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		Logger.Warn("invalid JID", "to", p.To, "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	var msg waE2E.Message
	if p.QuoteID != nil && p.QuoteSender != nil {
		msg = waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: new(p.Text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    p.QuoteID,
					Participant: p.QuoteSender,
				},
			},
		}
	} else {
		msg = waE2E.Message{Conversation: new(p.Text)}
	}
	resp, err := cli.SendMessage(ctx, jid, &msg)
	if err != nil {
		Logger.Error("send failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	Logger.Info("sent", "id", resp.ID)
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) CSendReaction(ctx context.Context, ctrl ControlMessage) EventMessage {
	cli := b.client.WAClient()
	if cli == nil {
		return ackEvent(ctrl.ID, false, "client not initialized")
	}
	var p SendReactionPayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		Logger.Warn("bad send_reaction payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		Logger.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	senderJID := types.EmptyJID
	if p.Sender != nil {
		senderJID, err = types.ParseJID(*p.Sender)
		if err != nil {
			Logger.Warn("invalid sender JID", "err", err)
			return ackEvent(ctrl.ID, false, "invalid sender JID: "+err.Error())
		}
	}
	_, err = cli.SendMessage(ctx, jid, cli.BuildReaction(jid, senderJID, types.MessageID(p.MessageID), p.Emoji))
	if err != nil {
		Logger.Error("reaction failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) CEditMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	cli := b.client.WAClient()
	if cli == nil {
		return ackEvent(ctrl.ID, false, "client not initialized")
	}
	var p EditMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		Logger.Warn("bad edit_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		Logger.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	_, err = cli.SendMessage(ctx, jid, cli.BuildEdit(jid, p.MessageID, &waE2E.Message{
		Conversation: new(string),
	}))
	if err != nil {
		Logger.Error("edit failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) CRevokeMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	cli := b.client.WAClient()
	if cli == nil {
		return ackEvent(ctrl.ID, false, "client not initialized")
	}
	var p RevokeMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		Logger.Warn("bad revoke_message payload", "err", err)
		return ackEvent(ctrl.ID, false, "invalid payload")
	}
	jid, err := types.ParseJID(p.To)
	if err != nil {
		Logger.Warn("invalid JID", "err", err)
		return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
	}
	var revokeMsg *waE2E.Message
	if p.OriginalSender != nil {
		revokeMsg = cli.BuildRevoke(jid, types.NewJID(*p.OriginalSender, types.DefaultUserServer), p.MessageID)
	} else {
		revokeMsg = cli.BuildRevoke(jid, types.EmptyJID, p.MessageID)
	}
	_, err = cli.SendMessage(ctx, jid, revokeMsg)
	if err != nil {
		Logger.Error("revoke failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}
	return ackEvent(ctrl.ID, true, "")
}

func (b *Bot) CGetStats(ctx context.Context, ctrl ControlMessage) EventMessage {
	stats := b.GetStatsPayload(ctx)
	return EventMessage{
		Kind:    EventStats,
		ID:      &ctrl.ID,
		Payload: stats,
	}
}
