package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sync"

	"whatsrook/logger"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// WARook is the per-request builder engine bound to a PluginContext.
type WARook struct {
	ctx *PluginContext
}

// From creates a WARook bound to the given plugin context.
func From(ctx *PluginContext) *WARook {
	return &WARook{ctx: ctx}
}

// NewButton creates a ButtonBuilder with the given body text.
func (r *WARook) NewButton(body string) *ButtonBuilder {
	return &ButtonBuilder{rook: r, body: body}
}

// NewPoll creates a PollBuilder for the given question (single-choice by default).
func (r *WARook) NewPoll(question string) *PollBuilder {
	return &PollBuilder{rook: r, question: question, single: true}
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

// ButtonRequest carries the data of a button click event.
type ButtonRequest struct {
	ButtonID    string
	DisplayText string
	Sender      types.JID
	Chat        types.JID
	Ctx         context.Context
}

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

type buttonRoute struct {
	once bool
	fn   func(req ButtonRequest, res *Response)
}

type pollRoute struct {
	options []string
	fn      func(req PollRequest, res *Response)
}

type listRoute struct {
	once bool
	fn   func(req ListRequest, res *Response)
}

var (
	reactorsMu   sync.RWMutex
	buttonRoutes = make(map[string]buttonRoute)
	pollRoutes   = make(map[types.MessageID]pollRoute)
	listRoutes   = make(map[string]listRoute)
)

// RegisterButtonHandler registers a reactive handler for a button ID.
// If once is true the handler auto-removes after its first invocation.
func RegisterButtonHandler(id string, once bool, fn func(req ButtonRequest, res *Response)) {
	reactorsMu.Lock()
	buttonRoutes[id] = buttonRoute{once: once, fn: fn}
	reactorsMu.Unlock()
}

// RegisterPollHandler registers a reactive handler for votes on a specific poll message.
// options must match the original poll option names for SHA-256 hash matching.
func RegisterPollHandler(pollMsgID types.MessageID, options []string, fn func(req PollRequest, res *Response)) {
	reactorsMu.Lock()
	pollRoutes[pollMsgID] = pollRoute{options: options, fn: fn}
	reactorsMu.Unlock()
}

// RegisterListHandler registers a reactive handler for a list row ID.
// If once is true the handler auto-removes after its first invocation.
func RegisterListHandler(rowID string, once bool, fn func(req ListRequest, res *Response)) {
	reactorsMu.Lock()
	listRoutes[rowID] = listRoute{once: once, fn: fn}
	reactorsMu.Unlock()
}

// DeregisterButtonHandlers removes registered handlers for the given button IDs.
func DeregisterButtonHandlers(ids ...string) {
	reactorsMu.Lock()
	for _, id := range ids {
		delete(buttonRoutes, id)
	}
	reactorsMu.Unlock()
}

// DeregisterPollHandler removes the registered handler for a poll message.
func DeregisterPollHandler(pollMsgID types.MessageID) {
	reactorsMu.Lock()
	delete(pollRoutes, pollMsgID)
	reactorsMu.Unlock()
}

// DispatchButtonClick looks up and fires a registered handler for buttonID.
// Returns true if a handler was found and fired.
func DispatchButtonClick(ctx *PluginContext, buttonID, displayText string) bool {
	if buttonID == "" {
		return false
	}
	reactorsMu.RLock()
	route, ok := buttonRoutes[buttonID]
	reactorsMu.RUnlock()
	if !ok {
		return false
	}
	if route.once {
		reactorsMu.Lock()
		delete(buttonRoutes, buttonID)
		reactorsMu.Unlock()
	}
	req := ButtonRequest{ButtonID: buttonID, DisplayText: displayText, Sender: ctx.Sender, Chat: ctx.Chat, Ctx: ctx.Ctx}
	res := newResponse(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Logger.Error("WARook: button handler panicked", "buttonID", buttonID, "panic", r)
			}
		}()
		route.fn(req, res)
	}()
	return true
}

// DispatchPollVoteEvent decrypts the poll vote in evt, matches selected option
// hashes against stored option names, and fires the registered handler.
// Returns true if a handler was found and fired.
func DispatchPollVoteEvent(ctx *PluginContext, evt *events.Message) bool {
	pollUpdate := evt.Message.GetPollUpdateMessage()
	if pollUpdate == nil {
		return false
	}
	key := pollUpdate.GetPollCreationMessageKey()
	if key == nil || key.GetID() == "" {
		return false
	}
	pollMsgID := types.MessageID(key.GetID())

	reactorsMu.RLock()
	route, ok := pollRoutes[pollMsgID]
	reactorsMu.RUnlock()
	if !ok {
		return false
	}

	decrypted, err := ctx.Client.DecryptPollVote(context.Background(), evt)
	if err != nil {
		Logger.Error("WARook: poll vote decryption failed", "pollMsgID", pollMsgID, "err", err)
		return false
	}

	var selectedOptions []string
	for _, optHash := range decrypted.SelectedOptions {
		for _, name := range route.options {
			h := sha256.Sum256([]byte(name))
			if bytes.Equal(h[:], optHash) {
				selectedOptions = append(selectedOptions, name)
				break
			}
		}
	}

	req := PollRequest{PollMsgID: pollMsgID, SelectedOptions: selectedOptions, Sender: ctx.Sender, Chat: ctx.Chat, Ctx: ctx.Ctx}
	res := newResponse(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Logger.Error("WARook: poll handler panicked", "pollMsgID", pollMsgID, "panic", r)
			}
		}()
		route.fn(req, res)
	}()
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
