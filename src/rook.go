package src

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	Logger "whatsrook/src/logger"
)

// DefaultPollTimeout is the default duration a poll remains active before auto-deleting.
const DefaultPollTimeout = 25 * time.Second

// WARook is the per-request builder engine bound to a PluginContext.
type WARook struct {
	ctx *PluginContext
}

// From creates a WARook bound to the given plugin context.
func From(ctx *PluginContext) *WARook {
	return &WARook{ctx: ctx}
}

// NewPoll creates a PollBuilder for the given question (single-choice & 25s auto-delete by default).
func (r *WARook) NewPoll(question string) *PollBuilder {
	return NewPoll(r, question)
}

// NewMessage creates a MessageBuilder with optional text.
func (r *WARook) NewMessage(text string) *MessageBuilder {
	return &MessageBuilder{rook: r, text: text}
}

// NewText creates a TextBuilder bound to the current context.
func (r *WARook) NewText(initial ...string) *TextBuilder {
	return NewTextWithContext(r.ctx, initial...)
}

// NewList creates a ListBuilder with the given body and button-open label.
func (r *WARook) NewList(body, buttonText string) *ListBuilder {
	return &ListBuilder{rook: r, body: body, buttonText: buttonText}
}

// Response is the reply surface handed to reactive handlers.
type Response struct {
	ctx *PluginContext
}

func newResponse(ctx *PluginContext) *Response { return &Response{ctx: ctx} }

// Reply sends a quoted reply.
func (r *Response) Reply(text string) error { return r.ctx.Reply(text) }

// Send sends a plain text message (no quote).
func (r *Response) Send(text string) error { return r.ctx.SendText(text) }

// Image uploads and sends an image.
func (r *Response) Image(data []byte, mimetype, caption string) error {
	return r.ctx.SendImage(data, mimetype, caption)
}

// Audio uploads and sends an audio message.
func (r *Response) Audio(data []byte, mimetype string) error {
	return r.ctx.SendAudio(data, mimetype)
}

// Document uploads and sends a document file.
func (r *Response) Document(data []byte, filename, mimetype string) error {
	return r.ctx.SendDocument(data, mimetype, filename, "")
}

// React sends an emoji reaction to the triggering message.
func (r *Response) React(emoji string) error { return r.ctx.React(emoji) }

// Delete revokes a message by ID from the current chat.
func (r *Response) Delete(msgID types.MessageID) error {
	_, err := r.ctx.Delete(msgID)
	return err
}

// Rook returns a new WARook bound to the same context, enabling chained
// interactive flows from inside a handler.
func (r *Response) Rook() *WARook { return From(r.ctx) }

// PollRequest carries the data of a decrypted poll vote.
type PollRequest struct {
	PollMsgID       types.MessageID
	SelectedOptions []string
	Sender          types.JID
	Chat            types.JID
	Ctx             context.Context
}

// ListRequest carries the data of a list row selection.
type ListRequest struct {
	RowID  string
	Title  string
	Sender types.JID
	Chat   types.JID
	Ctx    context.Context
}

type pollRoute struct {
	chat           types.JID
	client         *whatsmeow.Client
	pollMsgID      types.MessageID
	precedingMsgID types.MessageID
	options        []string
	once           bool
	autoDelete     bool
	timer          *time.Timer
	fn             func(req PollRequest, res *Response)
}

type listRoute struct {
	once bool
	fn   func(req ListRequest, res *Response)
}

var (
	reactorsMu sync.RWMutex
	pollRoutes = make(map[types.MessageID]pollRoute)
	listRoutes = make(map[string]listRoute)
)

// PollRouteConfig configures a reactive route for a poll message.
type PollRouteConfig struct {
	PollMsgID      types.MessageID
	PrecedingMsgID types.MessageID
	Chat           types.JID
	Client         *whatsmeow.Client
	Options        []string
	Once           bool
	AutoDelete     bool
	Timeout        time.Duration
	Fn             func(req PollRequest, res *Response)
}

// RegisterPollRoute registers a reactive route with full lifecycle, timeout, and auto-delete management.
func RegisterPollRoute(cfg PollRouteConfig) {
	if cfg.Timeout <= 0 && cfg.AutoDelete {
		cfg.Timeout = DefaultPollTimeout
	}

	var timer *time.Timer
	if cfg.AutoDelete && cfg.Timeout > 0 {
		timer = time.AfterFunc(cfg.Timeout, func() {
			reactorsMu.Lock()
			r, exists := pollRoutes[cfg.PollMsgID]
			if exists {
				delete(pollRoutes, cfg.PollMsgID)
			}
			remaining := len(pollRoutes)
			reactorsMu.Unlock()

			if exists {
				Logger.Debug("WARook: poll expired after timeout and auto-deleted",
					"pollMsgID", cfg.PollMsgID,
					"chat", cfg.Chat.String(),
					"timeout", cfg.Timeout,
					"remainingActivePollRoutes", remaining,
				)
				client := cfg.Client
				if client == nil && r.client != nil {
					client = r.client
				}
				chat := cfg.Chat
				if chat.IsEmpty() && !r.chat.IsEmpty() {
					chat = r.chat
				}
				if client != nil && !chat.IsEmpty() {
					go func(cli *whatsmeow.Client, ch types.JID, pID, preID types.MessageID) {
						revokeMsg := cli.BuildRevoke(ch, types.EmptyJID, pID)
						_, _ = cli.SendMessage(context.Background(), ch, revokeMsg)
						if preID != "" {
							preRevoke := cli.BuildRevoke(ch, types.EmptyJID, preID)
							_, _ = cli.SendMessage(context.Background(), ch, preRevoke)
						}
						Logger.Debug("WARook: auto-deleted expired poll and preceding messages", "pollMsgID", pID, "precedingMsgID", preID)
					}(client, chat, cfg.PollMsgID, cfg.PrecedingMsgID)
				}
			}
		})
	}

	reactorsMu.Lock()
	pollRoutes[cfg.PollMsgID] = pollRoute{
		chat:           cfg.Chat,
		client:         cfg.Client,
		pollMsgID:      cfg.PollMsgID,
		precedingMsgID: cfg.PrecedingMsgID,
		options:        cfg.Options,
		once:           cfg.Once,
		autoDelete:     cfg.AutoDelete,
		timer:          timer,
		fn:             cfg.Fn,
	}
	total := len(pollRoutes)
	reactorsMu.Unlock()

	Logger.Debug("WARook: registered poll vote handler",
		"pollMsgID", cfg.PollMsgID,
		"precedingMsgID", cfg.PrecedingMsgID,
		"optionsCount", len(cfg.Options),
		"options", cfg.Options,
		"once", cfg.Once,
		"autoDelete", cfg.AutoDelete,
		"timeout", cfg.Timeout,
		"totalActivePollRoutes", total,
	)
}

// RegisterPollHandler registers a reactive handler for votes on a specific poll message.
// options must match the original poll option names for SHA-256 hash matching.
func RegisterPollHandler(pollMsgID types.MessageID, options []string, once bool, fn func(req PollRequest, res *Response)) {
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:  pollMsgID,
		Options:    options,
		Once:       once,
		AutoDelete: true,
		Timeout:    DefaultPollTimeout,
		Fn:         fn,
	})
}

// RegisterListHandler registers a reactive handler for a list row ID.
// If once is true the handler auto-removes after its first invocation.
func RegisterListHandler(rowID string, once bool, fn func(req ListRequest, res *Response)) {
	reactorsMu.Lock()
	listRoutes[rowID] = listRoute{once: once, fn: fn}
	reactorsMu.Unlock()
}

// DeregisterPollHandler removes the registered handler for a poll message and cancels any pending timeout.
func DeregisterPollHandler(pollMsgID types.MessageID) {
	reactorsMu.Lock()
	if r, ok := pollRoutes[pollMsgID]; ok {
		if r.timer != nil {
			r.timer.Stop()
		}
		delete(pollRoutes, pollMsgID)
	}
	total := len(pollRoutes)
	reactorsMu.Unlock()
	Logger.Debug("WARook: deregistered poll vote handler",
		"pollMsgID", pollMsgID,
		"totalActivePollRoutes", total,
	)
}

// DispatchPollVoteEvent decrypts the poll vote in evt, matches selected option
// hashes against stored option names, auto-deletes the poll message, and fires the registered handler.
// Returns true if a handler was found and fired.
func DispatchPollVoteEvent(ctx *PluginContext, evt *events.Message) bool {
	pollUpdate := evt.Message.GetPollUpdateMessage()
	if pollUpdate == nil {
		return false
	}
	key := pollUpdate.GetPollCreationMessageKey()
	if key == nil || key.GetID() == "" {
		Logger.Debug("WARook: poll vote message key is empty or nil",
			"sender", ctx.Sender.String(),
			"chat", ctx.Chat.String(),
		)
		return false
	}
	pollMsgID := types.MessageID(key.GetID())

	Logger.Debug("WARook: incoming poll vote event",
		"targetPollMsgID", pollMsgID,
		"sender", ctx.Sender.String(),
		"chat", ctx.Chat.String(),
		"senderTimestamp", evt.Info.Timestamp,
	)

	reactorsMu.RLock()
	route, ok := pollRoutes[pollMsgID]
	reactorsMu.RUnlock()
	if !ok {
		Logger.Debug("WARook: no registered reactive route for poll message",
			"targetPollMsgID", pollMsgID,
			"sender", ctx.Sender.String(),
			"chat", ctx.Chat.String(),
		)
		return false
	}

	// Stop expiration timer immediately upon receiving vote
	if route.timer != nil {
		route.timer.Stop()
	}

	// Deregister the route immediately so the poll cannot be used again
	if route.once || route.autoDelete {
		reactorsMu.Lock()
		delete(pollRoutes, pollMsgID)
		remaining := len(pollRoutes)
		reactorsMu.Unlock()
		Logger.Debug("WARook: poll handler consumed and deregistered on vote",
			"targetPollMsgID", pollMsgID,
			"remainingActivePollRoutes", remaining,
		)
	}

	// Auto-delete the poll message (and any preceding text body) from the chat
	if route.autoDelete {
		client := ctx.Client
		if client == nil {
			client = route.client
		}
		chat := ctx.Chat
		if chat.IsEmpty() {
			chat = route.chat
		}
		if client != nil && !chat.IsEmpty() {
			go func(cli *whatsmeow.Client, ch types.JID, pID, preID types.MessageID) {
				Logger.Debug("WARook: auto-deleting completed poll message on vote", "pollMsgID", pID, "chat", ch.String())
				revokeMsg := cli.BuildRevoke(ch, types.EmptyJID, pID)
				_, err := cli.SendMessage(context.Background(), ch, revokeMsg)
				if err != nil {
					Logger.Debug("WARook: auto-delete poll message failed", "pollMsgID", pID, "err", err)
				} else {
					Logger.Debug("WARook: auto-deleted completed poll message", "pollMsgID", pID)
				}
				if preID != "" {
					preRevoke := cli.BuildRevoke(ch, types.EmptyJID, preID)
					_, _ = cli.SendMessage(context.Background(), ch, preRevoke)
					Logger.Debug("WARook: auto-deleted completed preceding text message", "precedingMsgID", preID)
				}
			}(client, chat, pollMsgID, route.precedingMsgID)
		}
	}

	decryptStart := time.Now()
	decrypted, err := ctx.Client.DecryptPollVote(context.Background(), evt)
	if err != nil {
		Logger.Error("WARook: poll vote decryption failed",
			"targetPollMsgID", pollMsgID,
			"sender", ctx.Sender.String(),
			"chat", ctx.Chat.String(),
			"err", err,
			"duration", time.Since(decryptStart),
		)
		return false
	}

	Logger.Debug("WARook: poll vote decrypted successfully",
		"targetPollMsgID", pollMsgID,
		"selectedHashesCount", len(decrypted.SelectedOptions),
		"duration", time.Since(decryptStart),
	)

	var selectedOptions []string
	for _, optHash := range decrypted.SelectedOptions {
		matched := false
		for _, name := range route.options {
			h := sha256.Sum256([]byte(name))
			if bytes.Equal(h[:], optHash) {
				selectedOptions = append(selectedOptions, name)
				matched = true
				Logger.Debug("WARook: matched poll option hash to option name",
					"targetPollMsgID", pollMsgID,
					"optionName", name,
					"hashHex", fmt.Sprintf("%x", optHash),
				)
				break
			}
		}
		if !matched {
			Logger.Debug("WARook: unmapped option hash in poll vote",
				"targetPollMsgID", pollMsgID,
				"hashHex", fmt.Sprintf("%x", optHash),
				"expectedOptions", route.options,
			)
		}
	}

	Logger.Debug("WARook: dispatching poll vote to reactive callback",
		"targetPollMsgID", pollMsgID,
		"sender", ctx.Sender.String(),
		"chat", ctx.Chat.String(),
		"selectedOptions", selectedOptions,
	)

	reqCtx := ctx.Ctx
	if reqCtx == nil || reqCtx.Err() != nil {
		reqCtx = context.Background()
	}
	req := PollRequest{
		PollMsgID:       pollMsgID,
		SelectedOptions: selectedOptions,
		Sender:          ctx.Sender,
		Chat:            ctx.Chat,
		Ctx:             reqCtx,
	}
	res := newResponse(ctx)
	if route.fn != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					Logger.Error("WARook: poll handler panicked",
						"targetPollMsgID", pollMsgID,
						"sender", ctx.Sender.String(),
						"chat", ctx.Chat.String(),
						"panic", r,
					)
				}
			}()
			handlerStart := time.Now()
			route.fn(req, res)
			Logger.Debug("WARook: poll callback finished successfully",
				"targetPollMsgID", pollMsgID,
				"duration", time.Since(handlerStart),
			)
		}()
		return true
	}

	Logger.Debug("WARook: poll vote decrypted but route has no custom callback function",
		"targetPollMsgID", pollMsgID,
		"selectedOptions", selectedOptions,
	)
	return true
}

// DispatchListSelection looks up and fires a registered handler for a list row selection.
// Returns true if a handler was found and fired.
func DispatchListSelection(ctx *PluginContext, rowID, title string) bool {
	if rowID == "" {
		return false
	}
	reactorsMu.RLock()
	route, ok := listRoutes[rowID]
	reactorsMu.RUnlock()
	if !ok {
		return false
	}
	if route.once {
		reactorsMu.Lock()
		delete(listRoutes, rowID)
		reactorsMu.Unlock()
	}
	req := ListRequest{RowID: rowID, Title: title, Sender: ctx.Sender, Chat: ctx.Chat, Ctx: ctx.Ctx}
	res := newResponse(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Logger.Error("WARook: list handler panicked", "rowID", rowID, "panic", r)
			}
		}()
		route.fn(req, res)
	}()
	return true
}
