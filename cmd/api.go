package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"whatsrook/src/logger"

	"github.com/coder/websocket"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var (
	// errClientNotInit indicates that the underlying WhatsApp socket client is not instantiated or connected.
	errClientNotInit = errors.New("client not initialized")
)

// ControlType specifies the operational action requested by an upstream WebSocket client.
type ControlType string

const (
	ControlSendMessage   ControlType = "send_message"
	ControlSendReaction  ControlType = "send_reaction"
	ControlEditMessage   ControlType = "edit_message"
	ControlRevokeMessage ControlType = "revoke_message"
	ControlGetStats      ControlType = "get_stats"
)

// EventType defines the category of telemetry, connection state, or message event pushed to clients.
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
	EventLog             EventType = "log"
)

// ControlMessage encapsulates an inbound client directive containing an action identifier, correlation ID, and payload.
type ControlMessage struct {
	Kind    ControlType     `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// EventMessage represents an outbound broadcast or targeted response pushed over the WebSocket stream.
type EventMessage struct {
	Kind    EventType `json:"type"`
	ID      *string   `json:"id,omitempty"`
	Payload any       `json:"payload"`
}

// AckPayload conveys the success or failure status of an asynchronous control operation back to the client.
type AckPayload struct {
	OK    bool    `json:"ok"`
	Error *string `json:"error,omitempty"`
}

// ackEvent constructs an EventMessage acknowledging a specific control command by its correlation ID.
func ackEvent(id string, ok bool, errMsg string) EventMessage {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	return EventMessage{
		Kind: EventAck,
		ID:   &id,
		Payload: AckPayload{
			OK:    ok,
			Error: errPtr,
		},
	}
}

// simpleEvent constructs an empty-payload EventMessage for basic state notifications.
func simpleEvent(kind EventType) EventMessage {
	return EventMessage{Kind: kind, Payload: map[string]any{}}
}

// StatsPayload provides runtime metrics, memory allocation stats, system info, and WhatsApp bot configuration.
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

// PairQRPayload delivers raw QR code matrix data for terminal or frontend pairing.
type PairQRPayload struct {
	Code string `json:"code"`
}

// PairCodePayload carries an alphanumeric link code for pairing via phone number.
type PairCodePayload struct {
	Code string `json:"code"`
}

// PairErrorPayload describes why a pairing attempt failed.
type PairErrorPayload struct {
	Reason string `json:"reason"`
}

// IncomingMessagePayload serializes received chat messages with sender metadata and optional quoted message context.
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

// IncomingCallPayload carries metadata regarding an incoming WhatsApp voice or video call signal.
type IncomingCallPayload struct {
	CallID    string    `json:"call_id"`
	From      string    `json:"from"`
	Timestamp time.Time `json:"timestamp"`
}

// SendMessagePayload defines parameters for outbound text messages, supporting replies via quoted stanza IDs.
type SendMessagePayload struct {
	To          string  `json:"to"`
	Text        string  `json:"text"`
	QuoteID     *string `json:"quote_id,omitempty"`
	QuoteSender *string `json:"quote_sender,omitempty"`
}

// SendReactionPayload defines the emoji reaction target, sender JID, and recipient chat.
type SendReactionPayload struct {
	To        string  `json:"to"`
	MessageID string  `json:"message_id"`
	Sender    *string `json:"sender,omitempty"`
	Emoji     string  `json:"emoji"`
}

// EditMessagePayload specifies the updated conversation string for an existing outgoing message.
type EditMessagePayload struct {
	To        string `json:"to"`
	MessageID string `json:"message_id"`
	NewText   string `json:"new_text"`
}

// RevokeMessagePayload defines the message ID and recipient required to revoke/delete a message for everyone.
type RevokeMessagePayload struct {
	To             string  `json:"to"`
	MessageID      string  `json:"message_id"`
	OriginalSender *string `json:"original_sender,omitempty"`
}

// wsClient maintains connection state and an individual outbound buffer for a connected client.
type wsClient struct {
	conn *websocket.Conn
	send chan EventMessage
}

// Hub manages concurrent client registrations, thread-safe broadcasts, and inbound control message routing.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	Control chan ControlMessage
}

// newHub initializes an empty Hub with an asynchronous control message channel.
func newHub() *Hub {
	return &Hub{
		clients: make(map[*wsClient]struct{}),
		Control: make(chan ControlMessage, 64),
	}
}

// Broadcast distributes an EventMessage concurrently across all active client channels with non-blocking drops for slow consumers.
func (h *Hub) Broadcast(evt EventMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.send <- evt:
		default:
		}
	}
}

// ConnectedClientsCount returns the active number of connected WebSocket subscribers.
func (h *Hub) ConnectedClientsCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeWS provides an HTTP handler that upgrades incoming connections to WebSockets and coordinates read/write loops.
func (h *Hub) ServeWS(dev bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: dev,
		})
		if err != nil {
			Logger.Error("ws accept failed", "err", err)
			return
		}

		client := &wsClient{
			conn: conn,
			send: make(chan EventMessage, 64),
		}

		h.mu.Lock()
		h.clients[client] = struct{}{}
		h.mu.Unlock()

		ctx, cancel := context.WithCancel(r.Context())
		defer func() {
			cancel()
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			_ = conn.Close(websocket.StatusNormalClosure, "session ended")
		}()

		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := conn.Ping(ctx); err != nil {
						cancel()
						return
					}
				case msg, ok := <-client.send:
					if !ok {
						return
					}
					data, err := json.Marshal(msg)
					if err != nil {
						Logger.Error("failed to marshal JSON event", "err", err)
						continue
					}
					if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
						cancel()
						return
					}
				}
			}
		}()

		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}
			if msgType != websocket.MessageText {
				Logger.Warn("rejected non-text frame")
				continue
			}

			var ctrl ControlMessage
			if err := json.Unmarshal(data, &ctrl); err != nil {
				Logger.Warn("bad JSON control frame", "err", err)
				continue
			}

			select {
			case h.Control <- ctrl:
			default:
				Logger.Warn("control channel full, dropping message", "id", ctrl.ID)
				select {
				case client.send <- ackEvent(ctrl.ID, false, "server busy"):
				default:
				}
			}
		}

		Logger.Info("websocket client disconnected")
	}
}

// Controller routes inbound control directives to their dedicated message handling routines.
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
		return EventMessage{
			Kind:    EventStats,
			ID:      &ctrl.ID,
			Payload: b.GetStatsPayload(ctx),
		}
	default:
		Logger.Warn("unknown control type", "kind", ctrl.Kind)
		return ackEvent(ctrl.ID, false, "unknown control type")
	}
}

// parseJIDAndClient verifies the active client state and parses the recipient JID string into a strongly-typed JID structure.
func (b *Bot) parseJIDAndClient(to string) (*whatsmeow.Client, types.JID, error) {
	cli := b.client.WAClient()
	if cli == nil {
		return nil, types.EmptyJID, errClientNotInit
	}
	jid, err := types.ParseJID(to)
	if err != nil {
		return nil, types.EmptyJID, err
	}
	return cli, jid, nil
}

// CSendMessage constructs and dispatches standard or quoted WhatsApp text messages.
func (b *Bot) CSendMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p SendMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		return ackEvent(ctrl.ID, false, "invalid payload: "+err.Error())
	}

	cli, jid, err := b.parseJIDAndClient(p.To)
	if err != nil {
		return ackEvent(ctrl.ID, false, err.Error())
	}

	var msg waE2E.Message
	if p.QuoteID != nil && p.QuoteSender != nil {
		msg = waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: &p.Text,
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    p.QuoteID,
					Participant: p.QuoteSender,
				},
			},
		}
	} else {
		msg = waE2E.Message{Conversation: &p.Text}
	}

	resp, err := cli.SendMessage(ctx, jid, &msg)
	if err != nil {
		Logger.Error("send failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}

	Logger.Info("sent", "id", resp.ID)
	return ackEvent(ctrl.ID, true, "")
}

// CSendReaction constructs and dispatches an emoji reaction against a specific message ID.
func (b *Bot) CSendReaction(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p SendReactionPayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		return ackEvent(ctrl.ID, false, "invalid payload: "+err.Error())
	}

	cli, jid, err := b.parseJIDAndClient(p.To)
	if err != nil {
		return ackEvent(ctrl.ID, false, err.Error())
	}

	senderJID := types.EmptyJID
	if p.Sender != nil {
		var err error
		senderJID, err = types.ParseJID(*p.Sender)
		if err != nil {
			return ackEvent(ctrl.ID, false, "invalid sender JID: "+err.Error())
		}
	}

	reactionMsg := cli.BuildReaction(jid, senderJID, types.MessageID(p.MessageID), p.Emoji)
	if _, err = cli.SendMessage(ctx, jid, reactionMsg); err != nil {
		Logger.Error("reaction failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}

	return ackEvent(ctrl.ID, true, "")
}

// CEditMessage submits an edit payload to alter the contents of a previously sent message.
func (b *Bot) CEditMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p EditMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		return ackEvent(ctrl.ID, false, "invalid payload: "+err.Error())
	}

	cli, jid, err := b.parseJIDAndClient(p.To)
	if err != nil {
		return ackEvent(ctrl.ID, false, err.Error())
	}

	editMsg := cli.BuildEdit(jid, p.MessageID, &waE2E.Message{
		Conversation: &p.NewText,
	})
	if _, err = cli.SendMessage(ctx, jid, editMsg); err != nil {
		Logger.Error("edit failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}

	return ackEvent(ctrl.ID, true, "")
}

// CRevokeMessage dispatches a protocol revoke message to delete a target message from chat history for all participants.
func (b *Bot) CRevokeMessage(ctx context.Context, ctrl ControlMessage) EventMessage {
	var p RevokeMessagePayload
	if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
		return ackEvent(ctrl.ID, false, "invalid payload: "+err.Error())
	}

	cli, jid, err := b.parseJIDAndClient(p.To)
	if err != nil {
		return ackEvent(ctrl.ID, false, err.Error())
	}

	var sender types.JID
	if p.OriginalSender != nil {
		sender = types.NewJID(*p.OriginalSender, types.DefaultUserServer)
	}

	revokeMsg := cli.BuildRevoke(jid, sender, p.MessageID)
	if _, err = cli.SendMessage(ctx, jid, revokeMsg); err != nil {
		Logger.Error("revoke failed", "err", err)
		return ackEvent(ctrl.ID, false, err.Error())
	}

	return ackEvent(ctrl.ID, true, "")
}
