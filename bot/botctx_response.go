package main

import (
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook"
)

// SendText sends a plain text message without quoting.
func (c *PluginContext) SendText(text string) error {
	c.StopAutoLoader()
	return whatsrook.SendText(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text))
}

// SendTextWithMentions sends a text message with user mentions without quoting.
func (c *PluginContext) SendTextWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendTextWithMentions(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text), mentions)
}

// SendTextWithID sends a plain text message and returns the assigned message ID.
func (c *PluginContext) SendTextWithID(text string) (string, error) {
	c.StopAutoLoader()
	return whatsrook.SendTextWithID(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text))
}

// SendTextWithGroupMention sends a text message with group mention without quoting.
func (c *PluginContext) SendTextWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				NonJIDMentions: &nonJID,
			},
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// Reply sends a quoted text reply to the triggering message.
func (c *PluginContext) Reply(text string) error {
	c.StopAutoLoader()
	return whatsrook.SendReply(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text), c.replyContextInfo())
}

// Replyf formats and sends a quoted text reply.
func (c *PluginContext) Replyf(format string, args ...any) error {
	return c.Reply(fmt.Sprintf(format, args...))
}

// ReplyWithID sends a quoted text reply and returns the assigned message ID.
func (c *PluginContext) ReplyWithID(text string) (string, error) {
	c.StopAutoLoader()
	return whatsrook.SendReplyWithID(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text), c.replyContextInfo())
}

// ReplyWithMentions sends a quoted reply tagging mentioned user JIDs.
func (c *PluginContext) ReplyWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendReplyWithMentions(c.GetSendContext(), c.Client, c.Chat, c.formatTextResponse(text), mentions, c.replyContextInfo())
}

// ReplyWithGroupMention sends a quoted reply with group mention.
func (c *PluginContext) ReplyWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.NonJIDMentions = &nonJID

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ci,
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendImage sends an image without quoting.
func (c *PluginContext) SendImage(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendImage(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption)
}

// SendImageWithMentions sends an image with mentions without quoting.
func (c *PluginContext) SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendImageWithMentions(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, mentions)
}

// ReplyWithImage sends an image quoted to the triggering message.
func (c *PluginContext) ReplyWithImage(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendImage(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, c.replyContextInfo())
}

// ReplyWithImageWithMentions sends an image with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendImageWithMentions(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, mentions, c.replyContextInfo())
}

// SendVideo sends a video without quoting.
func (c *PluginContext) SendVideo(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendVideo(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption)
}

// SendVideoGif sends a looping GIF video without quoting.
func (c *PluginContext) SendVideoGif(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoGif(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption)
}

// SendVideoWithMentions sends a video with mentions without quoting.
func (c *PluginContext) SendVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoWithMentions(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, mentions, false)
}

// ReplyWithVideo sends a video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideo(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendVideo(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, c.replyContextInfo())
}

// ReplyWithVideoWithProgress sends a video quoted to the triggering message while reporting upload progress.
func (c *PluginContext) ReplyWithVideoWithProgress(data []byte, mimetype, caption string, onProgress func(uploaded, total uint64)) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoWithProgress(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, onProgress, c.replyContextInfo())
}

// ReplyWithVideoGif sends a GIF video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideoGif(data []byte, mimetype, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoGif(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, c.replyContextInfo())
}

// ReplyWithVideoGifWithProgress sends a looping GIF video quoted to the triggering message while reporting upload progress.
func (c *PluginContext) ReplyWithVideoGifWithProgress(data []byte, mimetype, caption string, onProgress func(uploaded, total uint64)) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoGifWithProgress(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, onProgress, c.replyContextInfo())
}

// ReplyWithVideoWithMentions sends a video with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	return whatsrook.SendVideoWithMentions(c.GetSendContext(), c.Client, c.Chat, data, mimetype, caption, mentions, false, c.replyContextInfo())
}

// SendAudio sends an audio file without quoting.
func (c *PluginContext) SendAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	return whatsrook.SendAudio(c.GetSendContext(), c.Client, c.Chat, data, mimetype)
}

// ReplyWithAudio sends an audio file quoted to the triggering message.
func (c *PluginContext) ReplyWithAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	return whatsrook.SendAudio(c.GetSendContext(), c.Client, c.Chat, data, mimetype, c.replyContextInfo())
}

// SendDocument sends a document without quoting.
func (c *PluginContext) SendDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendDocument(c.GetSendContext(), c.Client, c.Chat, data, mimetype, filename, caption)
}

// ReplyWithDocument sends a document quoted to the triggering message.
func (c *PluginContext) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	return whatsrook.SendDocument(c.GetSendContext(), c.Client, c.Chat, data, mimetype, filename, caption, c.replyContextInfo())
}

// SendSticker sends a WebP sticker without quoting.
func (c *PluginContext) SendSticker(data []byte) error {
	c.StopAutoLoader()
	return whatsrook.SendSticker(c.GetSendContext(), c.Client, c.Chat, data, c.GetStickerPack(), c.GetStickerAuthor())
}

// ReplyWithSticker sends a WebP sticker quoted to the triggering message.
func (c *PluginContext) ReplyWithSticker(data []byte) error {
	c.StopAutoLoader()
	return whatsrook.SendSticker(c.GetSendContext(), c.Client, c.Chat, data, c.GetStickerPack(), c.GetStickerAuthor(), c.replyContextInfo())
}

// AlbumMediaItem aliases whatsrook.AlbumMediaItem
type AlbumMediaItem = whatsrook.AlbumMediaItem

// SendAlbum sends multiple media items grouped as a WhatsApp album without quoting.
func (c *PluginContext) SendAlbum(items []AlbumMediaItem) error {
	c.StopAutoLoader()
	return whatsrook.SendAlbum(c.GetSendContext(), c.Client, c.Chat, items)
}

// ReplyWithAlbum sends multiple media items grouped as a WhatsApp album quoted to the triggering message.
func (c *PluginContext) ReplyWithAlbum(items []AlbumMediaItem) error {
	c.StopAutoLoader()
	return whatsrook.SendAlbum(c.GetSendContext(), c.Client, c.Chat, items, c.replyContextInfo())
}

// Edit edits an existing message.
func (c *PluginContext) Edit(msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	return whatsrook.SendEdit(c.GetSendContext(), c.Client, c.Chat, msgID, content, extra...)
}

// Delete deletes/revokes a message for everyone.
func (c *PluginContext) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	return whatsrook.SendDelete(c.GetSendContext(), c.Client, c.Chat, msgID, senderJID...)
}

// React sends an emoji reaction to the triggering message.
func (c *PluginContext) React(emoji string) error {
	targetID := ""
	if c.Evt != nil {
		targetID = c.Evt.Info.ID
	}
	return whatsrook.SendReactMessage(c.GetSendContext(), c.Client, c.Chat, targetID, emoji)
}

// ReactMessage sends an emoji reaction to a specific target message ID.
func (c *PluginContext) ReactMessage(targetID string, emoji string) error {
	return whatsrook.SendReactMessage(c.GetSendContext(), c.Client, c.Chat, targetID, emoji)
}

// Text initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) Text(initial ...string) *TextBuilder {
	return NewTextWithSender(c, initial...)
}

// NewText initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) NewText(initial ...string) *TextBuilder {
	return NewTextWithSender(c, initial...)
}

// Rook returns a WARook builder engine bound to this PluginContext.
func (c *PluginContext) Rook() *WARook {
	return From(c)
}

// Poll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) Poll(question string) *PollBuilder {
	return From(c).NewPoll(question)
}

// NewPoll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) NewPoll(question string) *PollBuilder {
	return From(c).NewPoll(question)
}

// StartAutoLoader is a no-op method.
func (c *PluginContext) StartAutoLoader(delay ...time.Duration) {}

// StopAutoLoader is a no-op method.
func (c *PluginContext) StopAutoLoader() {}

// SendWebview sends an in-app WebView button message.
func (c *PluginContext) SendWebview(opts whatsrook.WebviewMessageOpts) error {
	c.StopAutoLoader()
	return whatsrook.SendWebview(c.GetSendContext(), c.Client, c.Chat, opts)
}

// ReplyWebview sends an in-app WebView button message quoting the triggering message.
func (c *PluginContext) ReplyWebview(opts whatsrook.WebviewMessageOpts) error {
	c.StopAutoLoader()
	opts.Quoted = c.replyContextInfo()
	return whatsrook.SendWebview(c.GetSendContext(), c.Client, c.Chat, opts)
}

var _ Sender = (*PluginContext)(nil)
