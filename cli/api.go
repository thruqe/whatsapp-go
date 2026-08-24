// WebSocket hub for managing real-time connections, concurrent broadcasting,
// and safe read/write loops.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"whatsrook/logger"

	"github.com/coder/websocket"
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
// loops using JSON text framing.
func (h *Hub) ServeWS(dev bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: dev,
		})
		if err != nil {
			Logger.Error("ws accept failed", "err", err)
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

		// single writer goroutine — JSON text frames
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

		// reader loop — JSON text frames
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}

			if msgType != websocket.MessageText {
				Logger.Warn("rejected non-text frame: JSON text frames required")
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
				case c.send <- ackEvent(ctrl.ID, false, "server busy"):
				default:
				}
			}
		}

		Logger.Info("websocket client disconnected")
	}
}
