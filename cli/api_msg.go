// JSON-based WebSocket payload type definitions and serialisation.
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// ControlType identifies the kind of control message a WebSocket client sends.
type ControlType string

const (
	ControlSendMessage   ControlType = "send_message"
	ControlSendReaction  ControlType = "send_reaction"
	ControlEditMessage   ControlType = "edit_message"
	ControlRevokeMessage ControlType = "revoke_message"
	ControlGetStats      ControlType = "get_stats"
)

// EventType identifies the kind of event broadcast to WebSocket clients.
type EventType string

const (
	EventPairQR          EventType = "pair_qr"
	EventPairCode        EventType = "pair_code"
	EventPairSuccess     EventType = "pair_success"
	EventPairError       EventType = "pair_error"
	EventLoggedOut       EventType = "logged_out"
	EventDisconnected    EventType = "disconnected"
	EventConnected       EventType = "connected"
	EventIncomingMessage EventType = "message"
	EventIncomingCall    EventType = "incoming_call"
	EventAck             EventType = "ack"
	EventStats           EventType = "stats"
)

// ControlMessage is what clients send in to send data or request info from the bot.
type ControlMessage struct {
	Kind    ControlType     `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// EventMessage is what the bot sends out to clients over WebSocket.
type EventMessage struct {
	Kind    EventType `json:"type"`
	ID      *string   `json:"id,omitempty"`
	Payload any       `json:"payload"`
}

func ackEvent(id string, ok bool, errMsg string) EventMessage {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	return EventMessage{
		Kind: EventAck,
		ID:   &id,
		Payload: AckPayload{
			OK:    ok,
			Error: e,
		},
	}
}

func simpleEvent(kind EventType) EventMessage {
	return EventMessage{Kind: kind, Payload: map[string]any{}}
}

// AckPayload is sent in response to data control messages.
type AckPayload struct {
	OK    bool    `json:"ok"`
	Error *string `json:"error,omitempty"`
}

// StatsPayload carries comprehensive informative system, bot, and database statistics.
type StatsPayload struct {
	Connected           bool    `json:"connected"`
	LoggedIn            bool    `json:"logged_in"`
	JID                 *string `json:"jid,omitempty"`
	PushName            *string `json:"push_name,omitempty"`
	BotName             *string `json:"bot_name,omitempty"`
	Prefix              *string `json:"prefix,omitempty"`
	Mode                *string `json:"mode,omitempty"`
	UptimeSeconds       int64   `json:"uptime_seconds"`
	UptimeFormatted     string  `json:"uptime_formatted"`
	MemoryUsedBytes     uint64  `json:"memory_used_bytes"`
	MemoryUsedFormatted string  `json:"memory_used_formatted"`
	MemorySysBytes      uint64  `json:"memory_sys_bytes"`
	ActivePluginsCount  uint32  `json:"active_plugins_count"`
	ConnectedWSClients  uint32  `json:"connected_ws_clients"`
	PlatformOS          string  `json:"platform_os"`
	GoVersion           string  `json:"go_version"`
	AppVersion          string  `json:"app_version"`
	SessionPhone        string  `json:"session_phone"`
	NetworkPaused       bool    `json:"network_paused"`
	DBContactsCount     uint32  `json:"db_contacts_count"`
	DBDriver            string  `json:"db_driver"`
	AnticallEnabled     bool    `json:"anticall_enabled"`
	LikestatusEnabled   bool    `json:"likestatus_enabled"`
	SudoersCount        uint32  `json:"sudoers_count"`
}

// PairQRPayload carries QR code data for pairing.
type PairQRPayload struct {
	Code string `json:"code"`
}

// PairCodePayload carries pairing code data.
type PairCodePayload struct {
	Code string `json:"code"`
}

// PairErrorPayload carries pairing error reason.
type PairErrorPayload struct {
	Reason string `json:"reason"`
}

// IncomingMessagePayload structure for WebSocket message events.
type IncomingMessagePayload struct {
	From       string    `json:"from"`
	Chat       string    `json:"chat"`
	Sender     string    `json:"sender"`
	Text       string    `json:"text"`
	MessageID  string    `json:"message_id"`
	PushName   string    `json:"push_name,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	IsGroup    bool      `json:"is_group"`
	IsFromMe   bool      `json:"is_from_me"`
	MediaType  string    `json:"media_type,omitempty"`
	QuotedID   string    `json:"quoted_id,omitempty"`
	QuotedText string    `json:"quoted_text,omitempty"`
}

// IncomingCallPayload structure for incoming call offer events.
type IncomingCallPayload struct {
	CallID    string    `json:"call_id"`
	From      string    `json:"from"`
	Timestamp time.Time `json:"timestamp"`
}

// Typed payload structs for decoding control messages.

type SendMessagePayload struct {
	To          string  `json:"to"`
	Text        string  `json:"text"`
	QuoteID     *string `json:"quote_id,omitempty"`
	QuoteSender *string `json:"quote_sender,omitempty"`
}

// SendReactionPayload carries the data needed to react to a message.
type SendReactionPayload struct {
	To        string  `json:"to"`
	MessageID string  `json:"message_id"`
	Sender    *string `json:"sender,omitempty"`
	Emoji     string  `json:"emoji"`
}

// EditMessagePayload carries the data needed to edit an existing message.
type EditMessagePayload struct {
	To        string `json:"to"`
	MessageID string `json:"message_id"`
	NewText   string `json:"new_text"`
}

// RevokeMessagePayload carries the data needed to revoke (delete) a message.
type RevokeMessagePayload struct {
	To             string  `json:"to"`
	MessageID      string  `json:"message_id"`
	OriginalSender *string `json:"original_sender,omitempty"`
}

// GetStatsPayload is an empty payload for stats requests.
type GetStatsPayload struct{}

// WriteWSMessage serializes and writes an EventMessage to a WebSocket connection as a JSON text frame.
func WriteWSMessage(ctx context.Context, conn *websocket.Conn, msg EventMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}
