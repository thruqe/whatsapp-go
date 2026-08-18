// JSON-based WebSocket payload type definitions and serialisation.
package main

import (
	"context"
	"encoding/json"
	"time"

	"whatsrook/proto/wsproto"

	"github.com/coder/websocket"
	googleProto "google.golang.org/protobuf/proto"
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

// WriteWSMessage serializes and writes an EventMessage to a WebSocket connection using Protobuf.
func WriteWSMessage(ctx context.Context, conn *websocket.Conn, msg EventMessage) error {
	frame := EventMessageToProto(msg)
	data, err := googleProto.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, data)
}

// EventMessageToProto converts a high-level EventMessage into its Protobuf EventFrame representation.
func EventMessageToProto(evt EventMessage) *wsproto.EventFrame {
	frame := &wsproto.EventFrame{}
	if evt.ID != nil {
		frame.Id = *evt.ID
	}

	switch evt.Kind {
	case EventPairQR:
		frame.Type = wsproto.EventType_EVENT_TYPE_PAIR_QR
		if p, ok := evt.Payload.(PairQRPayload); ok {
			frame.Payload = &wsproto.EventFrame_PairQr{
				PairQr: &wsproto.PairQRPayload{Code: p.Code},
			}
		}
	case EventPairCode:
		frame.Type = wsproto.EventType_EVENT_TYPE_PAIR_CODE
		if p, ok := evt.Payload.(PairCodePayload); ok {
			frame.Payload = &wsproto.EventFrame_PairCode{
				PairCode: &wsproto.PairCodePayload{Code: p.Code},
			}
		}
	case EventPairSuccess:
		frame.Type = wsproto.EventType_EVENT_TYPE_PAIR_SUCCESS
	case EventPairError:
		frame.Type = wsproto.EventType_EVENT_TYPE_PAIR_ERROR
		if p, ok := evt.Payload.(PairErrorPayload); ok {
			frame.Payload = &wsproto.EventFrame_PairError{
				PairError: &wsproto.PairErrorPayload{Reason: p.Reason},
			}
		}
	case EventLoggedOut:
		frame.Type = wsproto.EventType_EVENT_TYPE_LOGGED_OUT
	case EventDisconnected:
		frame.Type = wsproto.EventType_EVENT_TYPE_DISCONNECTED
	case EventConnected:
		frame.Type = wsproto.EventType_EVENT_TYPE_CONNECTED
	case EventIncomingMessage:
		frame.Type = wsproto.EventType_EVENT_TYPE_INCOMING_MESSAGE
		if p, ok := evt.Payload.(IncomingMessagePayload); ok {
			msgPayload := &wsproto.IncomingMessagePayload{
				From:          p.From,
				Chat:          p.Chat,
				Sender:        p.Sender,
				Text:          p.Text,
				MessageId:     p.MessageID,
				TimestampUnix: p.Timestamp.Unix(),
				IsGroup:       p.IsGroup,
				IsFromMe:      p.IsFromMe,
			}
			if p.PushName != "" {
				msgPayload.PushName = &p.PushName
			}
			if p.MediaType != "" {
				msgPayload.MediaType = &p.MediaType
			}
			if p.QuotedID != "" {
				msgPayload.QuotedId = &p.QuotedID
			}
			if p.QuotedText != "" {
				msgPayload.QuotedText = &p.QuotedText
			}
			frame.Payload = &wsproto.EventFrame_Message{
				Message: msgPayload,
			}
		}
	case EventIncomingCall:
		frame.Type = wsproto.EventType_EVENT_TYPE_INCOMING_CALL
		if p, ok := evt.Payload.(IncomingCallPayload); ok {
			frame.Payload = &wsproto.EventFrame_IncomingCall{
				IncomingCall: &wsproto.IncomingCallPayload{
					CallId:        p.CallID,
					From:          p.From,
					TimestampUnix: p.Timestamp.Unix(),
				},
			}
		}
	case EventAck:
		frame.Type = wsproto.EventType_EVENT_TYPE_ACK
		if p, ok := evt.Payload.(AckPayload); ok {
			frame.Payload = &wsproto.EventFrame_Ack{
				Ack: &wsproto.AckPayload{
					Ok:    p.OK,
					Error: p.Error,
				},
			}
		}
	case EventStats:
		frame.Type = wsproto.EventType_EVENT_TYPE_STATS
		if p, ok := evt.Payload.(StatsPayload); ok {
			frame.Payload = &wsproto.EventFrame_Stats{
				Stats: &wsproto.StatsPayload{
					Connected:           p.Connected,
					LoggedIn:            p.LoggedIn,
					Jid:                 p.JID,
					PushName:            p.PushName,
					BotName:             p.BotName,
					Prefix:              p.Prefix,
					Mode:                p.Mode,
					UptimeSeconds:       p.UptimeSeconds,
					UptimeFormatted:     p.UptimeFormatted,
					MemoryUsedBytes:     p.MemoryUsedBytes,
					MemoryUsedFormatted: p.MemoryUsedFormatted,
					MemorySysBytes:      p.MemorySysBytes,
					ActivePluginsCount:  p.ActivePluginsCount,
					ConnectedWsClients:  p.ConnectedWSClients,
					PlatformOs:          p.PlatformOS,
					GoVersion:           p.GoVersion,
					AppVersion:          p.AppVersion,
					SessionPhone:        p.SessionPhone,
					NetworkPaused:       p.NetworkPaused,
					DbContactsCount:     p.DBContactsCount,
					DbDriver:            p.DBDriver,
					AnticallEnabled:     p.AnticallEnabled,
					LikestatusEnabled:   p.LikestatusEnabled,
					SudoersCount:        p.SudoersCount,
				},
			}
		}
	}

	return frame
}

// ControlProtoToMessage converts a Protobuf ControlFrame message into internal ControlMessage format.
func ControlProtoToMessage(frame *wsproto.ControlFrame) (ControlMessage, error) {
	ctrl := ControlMessage{
		ID: frame.Id,
	}

	switch frame.Type {
	case wsproto.ControlType_CONTROL_TYPE_SEND_MESSAGE:
		ctrl.Kind = ControlSendMessage
		if p := frame.GetSendMessage(); p != nil {
			payload := SendMessagePayload{
				To:          p.To,
				Text:        p.Text,
				QuoteID:     p.QuoteId,
				QuoteSender: p.QuoteSender,
			}
			b, _ := json.Marshal(payload)
			ctrl.Payload = b
		}
	case wsproto.ControlType_CONTROL_TYPE_SEND_REACTION:
		ctrl.Kind = ControlSendReaction
		if p := frame.GetSendReaction(); p != nil {
			payload := SendReactionPayload{
				To:        p.To,
				MessageID: p.MessageId,
				Sender:    p.Sender,
				Emoji:     p.Emoji,
			}
			b, _ := json.Marshal(payload)
			ctrl.Payload = b
		}
	case wsproto.ControlType_CONTROL_TYPE_EDIT_MESSAGE:
		ctrl.Kind = ControlEditMessage
		if p := frame.GetEditMessage(); p != nil {
			payload := EditMessagePayload{
				To:        p.To,
				MessageID: p.MessageId,
				NewText:   p.NewText,
			}
			b, _ := json.Marshal(payload)
			ctrl.Payload = b
		}
	case wsproto.ControlType_CONTROL_TYPE_REVOKE_MESSAGE:
		ctrl.Kind = ControlRevokeMessage
		if p := frame.GetRevokeMessage(); p != nil {
			payload := RevokeMessagePayload{
				To:             p.To,
				MessageID:      p.MessageId,
				OriginalSender: p.OriginalSender,
			}
			b, _ := json.Marshal(payload)
			ctrl.Payload = b
		}
	case wsproto.ControlType_CONTROL_TYPE_GET_STATS:
		ctrl.Kind = ControlGetStats
	}

	return ctrl, nil
}
