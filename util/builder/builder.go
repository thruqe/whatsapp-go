package builder

import (
	"time"

	"go.mau.fi/whatsmeow/types"
)

// DefaultPollTimeout is the default duration a poll remains active before auto-deleting.
const DefaultPollTimeout = 25 * time.Second

// WARook is the per-request builder engine bound to a Sender context.
type WARook struct {
	sender Sender
}

// From creates a WARook bound to the given sender context.
func From(sender Sender) *WARook {
	return &WARook{sender: sender}
}

// NewPoll creates a PollBuilder for the given question (single-choice & 25s auto-delete by default).
func (r *WARook) NewPoll(question string) *PollBuilder {
	return NewPoll(r, question)
}

// NewMessage creates a MessageBuilder with optional text.
func (r *WARook) NewMessage(text string) *MessageBuilder {
	return &MessageBuilder{rook: r, text: text}
}

// NewText creates a TextBuilder bound to the current sender context.
func (r *WARook) NewText(initial ...string) *TextBuilder {
	return NewTextWithSender(r.sender, initial...)
}

// NewList creates a ListBuilder with the given body and button-open label.
func (r *WARook) NewList(body, buttonText string) *ListBuilder {
	return &ListBuilder{rook: r, body: body, buttonText: buttonText}
}

// Sender returns the underlying Sender instance.
func (r *WARook) Sender() Sender {
	return r.sender
}

// Response is the reply surface handed to reactive handlers.
type Response struct {
	sender Sender
}

// NewResponse constructs a new Response wrapper around a Sender.
func NewResponse(sender Sender) *Response {
	return &Response{sender: sender}
}

// Reply sends a quoted reply.
func (r *Response) Reply(text string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.Reply(text)
}

// Send sends a plain text message (no quote).
func (r *Response) Send(text string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.SendText(text)
}

// Image uploads and sends an image.
func (r *Response) Image(data []byte, mimetype, caption string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.SendImage(data, mimetype, caption)
}

// Audio uploads and sends an audio message.
func (r *Response) Audio(data []byte, mimetype string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.SendAudio(data, mimetype)
}

// Document uploads and sends a document file.
func (r *Response) Document(data []byte, filename, mimetype string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.SendDocument(data, mimetype, filename, "")
}

// React sends an emoji reaction to the triggering message.
func (r *Response) React(emoji string) error {
	if r.sender == nil {
		return nil
	}
	return r.sender.React(emoji)
}

// Delete revokes a message by ID from the current chat.
func (r *Response) Delete(msgID types.MessageID) error {
	if r.sender == nil {
		return nil
	}
	_, err := r.sender.Delete(msgID)
	return err
}

// Rook returns a new WARook bound to the same context, enabling chained
// interactive flows from inside a handler.
func (r *Response) Rook() *WARook {
	return From(r.sender)
}
