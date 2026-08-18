// WebSocket hub for managing real-time connections, concurrent broadcasting,
// and safe read/write loops.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"whatsrook/proto/wsproto"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
)

// Hub manages all connected WebSocket clients and provides concurrent
// broadcast of events and collection of control messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}

	// Inbound control messages from any WS client
	Control chan ControlMessage
}

type wsClient struct {
	conn *websocket.Conn
	send chan EventMessage
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[*wsClient]struct{}),
		Control: make(chan ControlMessage, 64),
	}
}

// Broadcast sends an event to all connected WebSocket clients.
func (h *Hub) Broadcast(evt EventMessage) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- evt:
		default:
			// slow client — drop
		}
	}
}

// ConnectedClientsCount returns the current number of active WebSocket clients.
func (h *Hub) ConnectedClientsCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeWS returns an HTTP handler that upgrades connections to the
// WebSocket protocol, registers them with the hub, and runs read/write
// loops using Protobuf binary framing.
func (h *Hub) ServeWS(dev bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subprotocols := []string{"protobuf"}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: dev,
			Subprotocols:       subprotocols,
		})
		if err != nil {
			slog.Error("ws accept failed", "err", err)
			return
		}

		c := &wsClient{
			conn: conn,
			send: make(chan EventMessage, 64),
		}

		h.mu.Lock()
		h.clients[c] = struct{}{}
		h.mu.Unlock()

		ctx, cancel := context.WithCancel(r.Context())

		defer func() {
			cancel()

			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()

			_ = conn.Close(websocket.StatusNormalClosure, "session ended")
		}()

		// single writer goroutine — Protobuf binary frames only
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

				case msg, ok := <-c.send:
					if !ok {
						return
					}

					frame := EventMessageToProto(msg)
					data, err := proto.Marshal(frame)
					if err != nil {
						slog.Error("failed to marshal proto event frame", "err", err)
						continue
					}

					if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
						cancel()
						return
					}
				}
			}
		}()

		// reader loop — Protobuf binary frames only
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}

			if msgType != websocket.MessageBinary {
				slog.Warn("rejected non-binary text frame: Protobuf binary frames required")
				continue
			}

			var frame wsproto.ControlFrame
			if err := proto.Unmarshal(data, &frame); err != nil {
				slog.Warn("bad protobuf control frame", "err", err)
				continue
			}

			ctrl, err := ControlProtoToMessage(&frame)
			if err != nil {
				slog.Warn("failed to convert proto control frame", "err", err)
				continue
			}

			select {
			case h.Control <- ctrl:
			default:
				slog.Warn("control channel full, dropping message", "id", ctrl.ID)
				select {
				case c.send <- ackEvent(ctrl.ID, false, "server busy"):
				default:
				}
			}
		}

		slog.Info("websocket client disconnected", "subprotocol", conn.Subprotocol())
	}
}
