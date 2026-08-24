package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"whatsrook/logger"

	"go.mau.fi/whatsmeow/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// formatTextResponse strips asterisks and emojis from text response
func (ctx *PluginContext) formatTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = RemoveEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return text
}

// SendText sends a simple text message to the current chat (with monospace format).
func (ctx *PluginContext) SendText(text string) error {
	ctx.StopAutoLoader()
	formatted := ctx.formatTextResponse(text)
	Logger.Debug("Building SendText", "text", text, "formatted", formatted)
	Logger.Debug("Sending SendText", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		Conversation: &formatted,
	})
	if err != nil {
		Logger.Error("SendText failed", "err", err)
	} else {
		Logger.Debug("SendText sent successfully")
	}
	return err
}

// SendTextf formats and sends a simple text message to the current chat.
func (ctx *PluginContext) SendTextf(format string, args ...any) error {
	return ctx.SendText(fmt.Sprintf(format, args...))
}

// SendSendMessage sends a message to any JID and automatically dismisses any active loader.
func (ctx *PluginContext) SendMessage(to types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	ctx.StopAutoLoader()
	var reqExtra whatsmeow.SendRequestExtra
	if len(extra) > 0 {
		reqExtra = extra[0]
	}
	return ctx.Client.SendMessage(ctx.GetSendContext(), to, msg, reqExtra)
}

// Send sends unified content (string or *waE2E.Message) with optional SendRequestExtra parameters.
func (ctx *PluginContext) Send(content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	ctx.StopAutoLoader()
	var msg *waE2E.Message
	switch v := content.(type) {
	case string:
		formatted := ctx.formatTextResponse(v)
		msg = &waE2E.Message{
			Conversation: &formatted,
		}
	case *waE2E.Message:
		msg = v
	default:
		return whatsmeow.SendResponse{}, fmt.Errorf("unsupported content type: %T", content)
	}

	var reqExtra whatsmeow.SendRequestExtra
	if len(extra) > 0 {
		reqExtra = extra[0]
	}
	return ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg, reqExtra)
}

// Sendf formats and sends unified text content with optional SendRequestExtra parameters.
func (ctx *PluginContext) Sendf(format string, args ...any) (whatsmeow.SendResponse, error) {
	return ctx.Send(fmt.Sprintf(format, args...))
}

// Edit edits an existing message in the current chat by message ID.
func (ctx *PluginContext) Edit(msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	ctx.StopAutoLoader()
	var msg *waE2E.Message
	switch v := content.(type) {
	case string:
		formatted := ctx.formatTextResponse(v)
		msg = &waE2E.Message{
			Conversation: &formatted,
		}
	case *waE2E.Message:
		msg = v
	default:
		return whatsmeow.SendResponse{}, fmt.Errorf("unsupported content type: %T", content)
	}

	editMsg := ctx.Client.BuildEdit(ctx.Chat, msgID, msg)
	var reqExtra whatsmeow.SendRequestExtra
	if len(extra) > 0 {
		reqExtra = extra[0]
	}
	return ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, editMsg, reqExtra)
}

// Delete revokes/deletes a message in the current chat by message ID.
func (ctx *PluginContext) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	ctx.StopAutoLoader()
	sJID := types.EmptyJID
	if len(senderJID) > 0 {
		sJID = senderJID[0]
	}
	revokeMsg := ctx.Client.BuildRevoke(ctx.Chat, sJID, msgID)
	return ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, revokeMsg)
}

// Reply sends a text message replying to the current message (with typing simulation and monospace format).
func (ctx *PluginContext) Reply(text string) error {
	_, err := ctx.ReplyWithID(text)
	return err
}

// Replyf formats and sends a text message replying to the current message.
func (ctx *PluginContext) Replyf(format string, args ...any) error {
	return ctx.Reply(fmt.Sprintf(format, args...))
}

// ReplyWithID sends a text message replying to the current message and returns the sent MessageID.
func (ctx *PluginContext) ReplyWithID(text string) (types.MessageID, error) {
	ctx.StopAutoLoader()
	formatted := ctx.formatTextResponse(text)
	cinfo := ctx.replyContextInfo()
	Logger.Debug("Building Reply", "text", text, "formatted", formatted, "context_info", cinfo)
	Logger.Debug("Sending Reply", "chat", ctx.Chat.String())
	resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cinfo,
		},
	})
	if err != nil {
		Logger.Error("Reply failed", "err", err)
		return "", err
	}
	Logger.Debug("Reply sent successfully", "msgID", resp.ID)
	return resp.ID, nil
}

func (ctx *PluginContext) replyContextInfo() *waE2E.ContextInfo {
	if ctx.Evt == nil {
		return nil
	}
	stanzaID := ctx.Evt.Info.ID
	participant := ctx.Sender.ToNonAD().String()
	return &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		Participant:   &participant,
		QuotedMessage: ctx.Evt.Message,
	}
}

// SendImage uploads and sends an image to the current chat.
func (ctx *PluginContext) SendImage(data []byte, mimetype, caption string) error {
	ctx.StopAutoLoader()
	if mimetype == "" {
		Logger.Warn("SendImage: mimetype is empty, defaulting to image/jpeg")
		mimetype = "image/jpeg"
	}
	Logger.Debug("Building SendImage", "data_len", len(data), "mimetype", mimetype, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		Logger.Error("SendImage: upload failed", "err", err)
		return fmt.Errorf("image upload failed: %w", err)
	}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending SendImage", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendImage failed", "err", err)
	} else {
		Logger.Debug("SendImage sent successfully")
	}
	return err
}

// SendImageWithMentions uploads and sends an image with mentioned JIDs to the current chat.
func (ctx *PluginContext) SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	ctx.StopAutoLoader()
	if mimetype == "" {
		Logger.Warn("SendImageWithMentions: mimetype is empty, defaulting to image/jpeg")
		mimetype = "image/jpeg"
	}
	var cinfo *waE2E.ContextInfo
	if len(mentions) > 0 {
		cinfo = &waE2E.ContextInfo{}
		for _, m := range mentions {
			if !m.IsEmpty() {
				cinfo.MentionedJID = append(cinfo.MentionedJID, m.ToNonAD().String())
			}
		}
	}
	Logger.Debug("Building SendImageWithMentions", "data_len", len(data), "mimetype", mimetype, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		Logger.Error("SendImageWithMentions: upload failed", "err", err)
		return fmt.Errorf("image upload failed: %w", err)
	}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending SendImageWithMentions", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendImageWithMentions failed", "err", err)
	} else {
		Logger.Debug("SendImageWithMentions sent successfully")
	}
	return err
}

// ReplyWithImage uploads and sends an image as a reply.
func (ctx *PluginContext) ReplyWithImage(data []byte, mimetype, caption string) error {
	return ctx.ReplyWithImageWithMentions(data, mimetype, caption, nil)
}

// ReplyWithImageWithMentions uploads and sends an image as a reply with mentioned JIDs.
func (ctx *PluginContext) ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	ctx.StopAutoLoader()
	if mimetype == "" {
		Logger.Warn("ReplyWithImageWithMentions: mimetype is empty, defaulting to image/jpeg")
		mimetype = "image/jpeg"
	}
	cinfo := ctx.replyContextInfo()
	if cinfo == nil && len(mentions) > 0 {
		cinfo = &waE2E.ContextInfo{}
	}
	if cinfo != nil {
		for _, m := range mentions {
			if !m.IsEmpty() {
				cinfo.MentionedJID = append(cinfo.MentionedJID, m.ToNonAD().String())
			}
		}
	}
	Logger.Debug("Building ReplyWithImageWithMentions", "data_len", len(data), "mimetype", mimetype, "caption", caption, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		Logger.Error("ReplyWithImageWithMentions: upload failed", "err", err)
		return fmt.Errorf("image upload failed: %w", err)
	}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending ReplyWithImageWithMentions", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("ReplyWithImageWithMentions failed", "err", err)
	} else {
		Logger.Debug("ReplyWithImageWithMentions sent successfully")
	}
	return err
}

// AlbumMediaItem represents an image or video item inside an album.
type AlbumMediaItem struct {
	Data     []byte
	Mimetype string
	Caption  string
}

// ReplyWithAlbum sends multiple images as an album reply with optional mentions on the first image.
func (ctx *PluginContext) ReplyWithAlbum(items []AlbumMediaItem, mentions []types.JID) error {
	ctx.StopAutoLoader()
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		mime := items[0].Mimetype
		if mime == "" {
			mime = "image/jpeg"
		}
		return ctx.ReplyWithImageWithMentions(items[0].Data, mime, items[0].Caption, mentions)
	}

	count := uint32(len(items))
	cinfo := ctx.replyContextInfo()
	if cinfo == nil && len(mentions) > 0 {
		cinfo = &waE2E.ContextInfo{}
	}
	if cinfo != nil {
		for _, m := range mentions {
			if !m.IsEmpty() {
				cinfo.MentionedJID = append(cinfo.MentionedJID, m.ToNonAD().String())
			}
		}
	}

	for i, item := range items {
		mime := item.Mimetype
		if mime == "" {
			mime = "image/jpeg"
		}
		uploaded, err := ctx.Client.Upload(ctx.Ctx, item.Data, whatsmeow.MediaImage)
		if err != nil {
			Logger.Error("ReplyWithAlbum: upload failed", "index", i, "err", err)
			continue
		}
		imgMsg := &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mime,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(item.Data))),
			Caption:       new(item.Caption),
		}
		if i == 0 {
			imgMsg.ContextInfo = cinfo
		}
		msg := &waE2E.Message{
			ImageMessage: imgMsg,
			AlbumMessage: &waE2E.AlbumMessage{
				ExpectedImageCount: new(count),
			},
		}
		_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
		if err != nil {
			Logger.Error("ReplyWithAlbum: send item failed", "index", i, "err", err)
		}
	}
	return nil
}

// SendVideo uploads and sends a video to the current chat.
func (ctx *PluginContext) SendVideo(data []byte, mimetype, caption string) error {
	return ctx.sendVideoInternal(data, mimetype, caption, false)
}

// SendVideoGif uploads and sends a video with GifPlayback enabled so it plays as an inline looping GIF.
func (ctx *PluginContext) SendVideoGif(data []byte, mimetype, caption string) error {
	return ctx.sendVideoInternal(data, mimetype, caption, true)
}

func (ctx *PluginContext) sendVideoInternal(data []byte, mimetype, caption string, gifPlayback bool) error {
	ctx.StopAutoLoader()
	if mimetype == "" || gifPlayback {
		mimetype = "video/mp4"
	}
	Logger.Debug("Building SendVideo", "data_len", len(data), "mimetype", mimetype, "caption", caption, "gif_playback", gifPlayback)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		Logger.Error("SendVideo: upload failed", "err", err)
		return fmt.Errorf("video upload failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
		},
	}
	if gifPlayback {
		msg.VideoMessage.GifPlayback = new(true)
	}
	Logger.Debug("Sending SendVideo", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendVideo failed", "err", err)
	} else {
		Logger.Debug("SendVideo sent successfully")
	}
	return err
}

// ReplyWithVideo uploads and sends a video as a reply.
func (ctx *PluginContext) ReplyWithVideo(data []byte, mimetype, caption string) error {
	return ctx.replyVideoInternal(data, mimetype, caption, false)
}

// ReplyWithVideoGif uploads and sends a video with GifPlayback enabled as a reply so it plays as an inline looping GIF.
func (ctx *PluginContext) ReplyWithVideoGif(data []byte, mimetype, caption string) error {
	return ctx.replyVideoInternal(data, mimetype, caption, true)
}

func (ctx *PluginContext) replyVideoInternal(data []byte, mimetype, caption string, gifPlayback bool) error {
	ctx.StopAutoLoader()
	if mimetype == "" || gifPlayback {
		mimetype = "video/mp4"
	}
	cinfo := ctx.replyContextInfo()
	Logger.Debug("Building replyVideoInternal", "data_len", len(data), "mimetype", mimetype, "caption", caption, "gif_playback", gifPlayback)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		Logger.Error("replyVideoInternal: upload failed", "err", err)
		return fmt.Errorf("video upload failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	if gifPlayback {
		msg.VideoMessage.GifPlayback = new(true)
	}
	Logger.Debug("Sending replyVideoInternal", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("replyVideoInternal failed", "err", err)
	} else {
		Logger.Debug("replyVideoInternal sent successfully")
	}
	return err
}

// SendDocument uploads and sends a document.
func (ctx *PluginContext) SendDocument(data []byte, mimetype, filename, caption string) error {
	ctx.StopAutoLoader()
	if mimetype == "" {
		Logger.Warn("SendDocument: mimetype is empty, defaulting to application/octet-stream")
		mimetype = "application/octet-stream"
	}
	Logger.Debug("Building SendDocument", "data_len", len(data), "mimetype", mimetype, "filename", filename, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		Logger.Error("SendDocument: upload failed", "err", err)
		return fmt.Errorf("document upload failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			FileName:      &filename,
			Caption:       &caption,
		},
	}
	*msg.DocumentMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending SendDocument", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendDocument failed", "err", err)
	} else {
		Logger.Debug("SendDocument sent successfully")
	}
	return err
}

// ReplyWithDocument uploads and sends a document as a reply.
func (ctx *PluginContext) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	ctx.StopAutoLoader()
	if mimetype == "" {
		Logger.Warn("ReplyWithDocument: mimetype is empty, defaulting to application/octet-stream")
		mimetype = "application/octet-stream"
	}
	cinfo := ctx.replyContextInfo()
	Logger.Debug("Building ReplyWithDocument", "data_len", len(data), "mimetype", mimetype, "filename", filename, "caption", caption, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		Logger.Error("ReplyWithDocument: upload failed", "err", err)
		return fmt.Errorf("document upload failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			FileName:      &filename,
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.DocumentMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending ReplyWithDocument", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("ReplyWithDocument failed", "err", err)
	} else {
		Logger.Debug("ReplyWithDocument sent successfully")
	}
	return err
}

// SendSticker uploads and sends a sticker.
func (ctx *PluginContext) SendSticker(data []byte) error {
	ctx.StopAutoLoader()
	mimetype := "image/webp"
	Logger.Debug("Building SendSticker", "data_len", len(data))
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		Logger.Error("SendSticker: upload failed", "err", err)
		return fmt.Errorf("sticker upload failed: %w", err)
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
		},
	}
	*msg.StickerMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending SendSticker", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendSticker failed", "err", err)
	} else {
		Logger.Debug("SendSticker sent successfully")
	}
	return err
}

// ReplyWithSticker uploads and sends a sticker as a reply.
func (ctx *PluginContext) ReplyWithSticker(data []byte) error {
	ctx.StopAutoLoader()
	mimetype := "image/webp"
	cinfo := ctx.replyContextInfo()
	Logger.Debug("Building ReplyWithSticker", "data_len", len(data), "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		Logger.Error("ReplyWithSticker: upload failed", "err", err)
		return fmt.Errorf("sticker upload failed: %w", err)
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			ContextInfo:   cinfo,
		},
	}
	*msg.StickerMessage.FileLength = uint64(len(data))
	Logger.Debug("Sending ReplyWithSticker", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("ReplyWithSticker failed", "err", err)
	} else {
		Logger.Debug("ReplyWithSticker sent successfully")
	}
	return err
}

func (ctx *PluginContext) getContextInfo() *waE2E.ContextInfo {
	if ctx.Evt == nil || ctx.Evt.Message == nil {
		return nil
	}
	return GetContextInfoFromProto(ctx.Evt.Message)
}

// GetContextInfo returns the context info of the message if available.
func (ctx *PluginContext) GetContextInfo() *waE2E.ContextInfo {
	return ctx.getContextInfo()
}

// GetQuotedMessage returns the quoted message if this event has one.
func (ctx *PluginContext) GetQuotedMessage() *waE2E.Message {
	ci := ctx.getContextInfo()
	if ci != nil {
		return ci.QuotedMessage
	}
	return nil
}

// ResolvePN returns the standard phone number JID (@s.whatsapp.net) for a given JID.
// If the provided JID is already a phone number JID, it returns it directly (ToNonAD).
// If the JID is an LID (@lid), it looks up the mapped phone number in the client's LID store.
// If no phone number mapping is found, it returns the input JID.
func ResolvePN(ctx context.Context, client *whatsmeow.Client, jid types.JID) types.JID {
	if jid.IsEmpty() {
		return jid
	}
	jid = jid.ToNonAD()
	if jid.Server == types.DefaultUserServer || jid.Server == types.GroupServer || jid.Server == types.BroadcastServer {
		return jid
	}
	if jid.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return jid
}

// ResolvePN returns the standard phone number JID (@s.whatsapp.net) for a given JID using PluginContext.
func (ctx *PluginContext) ResolvePN(jid types.JID) types.JID {
	if ctx == nil {
		return jid.ToNonAD()
	}
	return ResolvePN(ctx.Ctx, ctx.Client, jid)
}

// GetQuotedSender returns the quoted message sender JID if available.
func (ctx *PluginContext) GetQuotedSender() (types.JID, bool) {
	ci := ctx.getContextInfo()
	if ci != nil {
		if ci.Participant != nil && *ci.Participant != "" {
			pj, err := types.ParseJID(*ci.Participant)
			if err == nil && !pj.IsEmpty() {
				return ctx.ResolvePN(pj), true
			}
		}
		if ci.RemoteJID != nil && *ci.RemoteJID != "" {
			pj, err := types.ParseJID(*ci.RemoteJID)
			if err == nil && !pj.IsEmpty() {
				return ctx.ResolvePN(pj), true
			}
		}
	}
	if !ctx.Chat.IsEmpty() && ctx.Chat.Server != "g.us" {
		return ctx.ResolvePN(ctx.Chat), true
	}
	return types.JID{}, false
}

// GetMentionedJIDs returns JIDs that were tagged/mentioned in the message.
func (ctx *PluginContext) GetMentionedJIDs() []types.JID {
	ci := ctx.getContextInfo()
	if ci == nil {
		return nil
	}
	var out []types.JID
	for _, m := range ci.MentionedJID {
		j, err := types.ParseJID(m)
		if err == nil {
			out = append(out, ctx.ResolvePN(j))
		}
	}
	return out
}

// ParseUserJID parses and validates a raw JID, LID, or phone number string.
// It handles leading @, + signs, formatted numbers (+1 (234) 567-8900), LID domains (lid.whatsapp.net -> lid),
// c.us -> s.whatsapp.net, and device IDs (:1 -> ToNonAD()).
func ParseUserJID(raw string) (types.JID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.EmptyJID, fmt.Errorf("empty JID string")
	}

	clean := strings.TrimLeft(raw, "@")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return types.EmptyJID, fmt.Errorf("empty JID string after trimming '@'")
	}

	if idx := strings.IndexByte(clean, '@'); idx != -1 {
		userPart := clean[:idx]
		domainPart := strings.ToLower(clean[idx+1:])
		userPart = strings.TrimLeft(userPart, "+")
		if devIdx := strings.IndexByte(userPart, ':'); devIdx != -1 {
			userPart = userPart[:devIdx]
		}

		switch domainPart {
		case "c.us", "s.whatsapp.net":
			domainPart = types.DefaultUserServer
		case "lid.whatsapp.net", "lid":
			domainPart = types.HiddenUserServer
		}

		if domainPart == types.DefaultUserServer || domainPart == types.HiddenUserServer {
			var digits strings.Builder
			for _, r := range userPart {
				if r >= '0' && r <= '9' {
					digits.WriteRune(r)
				}
			}
			if digits.Len() > 0 {
				userPart = digits.String()
			}
		}

		parsed, err := types.ParseJID(userPart + "@" + domainPart)
		if err != nil {
			return types.EmptyJID, err
		}
		cleanJID := parsed.ToNonAD()
		if cleanJID.IsEmpty() || cleanJID.User == "" {
			return types.EmptyJID, fmt.Errorf("invalid JID user")
		}
		return cleanJID, nil
	}

	var digits strings.Builder
	for _, r := range clean {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}

	if digits.Len() == 0 {
		return types.EmptyJID, fmt.Errorf("no valid digits in JID string: %q", raw)
	}

	parsed := types.NewJID(digits.String(), types.DefaultUserServer).ToNonAD()
	if parsed.IsEmpty() || parsed.User == "" {
		return types.EmptyJID, fmt.Errorf("invalid JID user")
	}
	return parsed, nil
}

// GetArgsJIDs parses any phone numbers or JID strings in command args.
func (ctx *PluginContext) GetArgsJIDs() []types.JID {
	var out []types.JID
	for _, arg := range ctx.Args {
		if j, err := ParseUserJID(arg); err == nil && !j.IsEmpty() {
			out = append(out, ctx.ResolvePN(j))
		}
	}
	return out
}

// IsSameUserRaw compares two JIDs, resolving and matching any LID mappings.
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	Logger.Debug("IsSameUserRaw checking", "a", a.String(), "b", b.String())
	a = a.ToNonAD()
	b = b.ToNonAD()
	if a == b {
		Logger.Debug("IsSameUserRaw result: true (direct match)", "a", a.String(), "b", b.String())
		return true
	}

	aPN := a
	bPN := b

	if a.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, a); err == nil && !pn.IsEmpty() {
			aPN = pn.ToNonAD()
		}
	}
	if b.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, b); err == nil && !pn.IsEmpty() {
			bPN = pn.ToNonAD()
		}
	}

	if aPN == bPN {
		Logger.Debug("IsSameUserRaw result: true (PN match)", "a", a.String(), "b", b.String())
		return true
	}

	aLID := a
	bLID := b
	if a.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, a); err == nil && !lid.IsEmpty() {
			aLID = lid.ToNonAD()
		}
	}
	if b.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, b); err == nil && !lid.IsEmpty() {
			bLID = lid.ToNonAD()
		}
	}

	res := aLID == bLID
	Logger.Debug("IsSameUserRaw result", "a", a.String(), "b", b.String(), "result", res)
	return res
}

// IsSameUser compares two JIDs, resolving and matching any LID mappings.
func (ctx *PluginContext) IsSameUser(a, b types.JID) bool {
	res := IsSameUserRaw(ctx.Ctx, ctx.Client, a, b)
	Logger.Debug("IsSameUser helper check", "a", a.String(), "b", b.String(), "result", res)
	return res
}

// GetTargets resolves targets from reply, mentions, or arguments.
// If in a P2P chat and no other target is provided (or if the provided target/send is ours),
// we fall back to the chat JID (as long as it isn't ours).
func (ctx *PluginContext) GetTargets() []types.JID {
	if ctx.Client.Store.ID == nil {
		return nil
	}
	ourJID := *ctx.Client.Store.ID

	// 1. Quoted message sender (excluding ours)
	if q, ok := ctx.GetQuotedSender(); ok {
		if !ctx.IsSameUser(q, ourJID) {
			return []types.JID{ctx.ResolvePN(q)}
		}
	}

	// 2. Mentioned JIDs (excluding ours)
	if m := ctx.GetMentionedJIDs(); len(m) > 0 {
		var filtered []types.JID
		for _, j := range m {
			if !ctx.IsSameUser(j, ourJID) {
				filtered = append(filtered, ctx.ResolvePN(j))
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	// 3. Arguments JIDs (excluding ours)
	if argsJIDs := ctx.GetArgsJIDs(); len(argsJIDs) > 0 {
		var filtered []types.JID
		for _, j := range argsJIDs {
			if !ctx.IsSameUser(j, ourJID) {
				filtered = append(filtered, ctx.ResolvePN(j))
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	// 4. In a P2P chat (chat server is not g.us) and the chat JID is not ours
	if ctx.Chat.Server != "g.us" {
		if !ctx.IsSameUser(ctx.Chat, ourJID) {
			return []types.JID{ctx.ResolvePN(ctx.Chat)}
		}
	}

	return nil
}

// IsOwner checks if the message sender is the bot owner (the connected WhatsApp account JID).
func (ctx *PluginContext) IsOwner() bool {
	if ctx.Client.Store.ID != nil {
		return ctx.IsSameUser(ctx.Sender, *ctx.Client.Store.ID)
	}
	return false
}

// IsSudo checks if the message sender is a registered sudo user or the bot owner.
func (ctx *PluginContext) IsSudo() bool {
	Logger.Debug("IsSudo checking", "sender", ctx.Sender.String())
	if ctx.IsOwner() {
		Logger.Debug("IsSudo result: true (bot owner)", "sender", ctx.Sender.String())
		return true
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		Logger.Debug("IsSudo result: false (settings store unavailable)", "sender", ctx.Sender.String())
		return false
	}
	raw, err := getSetting(ctx.Ctx, s, "sudoers")
	if err != nil || raw == "" {
		Logger.Debug("IsSudo result: false (no sudoers configured)", "sender", ctx.Sender.String())
		return false
	}

	for sudoerStr := range strings.FieldsSeq(raw) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if ctx.IsSameUser(ctx.Sender, sudoerJID) {
				Logger.Debug("IsSudo result: true (sudoer list match)", "sender", ctx.Sender.String())
				return true
			}
		}
	}
	Logger.Debug("IsSudo result: false", "sender", ctx.Sender.String())
	return false
}

// IsSudoRaw checks if a given sender is a registered sudo user or the bot owner.
func IsSudoRaw(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client == nil {
		return false
	}
	if client.Store.ID != nil && IsSameUserRaw(ctx, client, *client.Store.ID, sender) {
		return true
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}
	raw, err := getSetting(ctx, s, "sudoers")
	if err != nil || raw == "" {
		return false
	}
	for sudoerStr := range strings.FieldsSeq(raw) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if IsSameUserRaw(ctx, client, sender, sudoerJID) {
				return true
			}
		}
	}
	return false
}

func getSetting(ctx context.Context, s *sqlstore.SQLStore, key string) (string, error) {
	if s == nil {
		return "", nil
	}
	db := s.GetDB()
	if db == nil {
		return "", nil
	}
	ourJID := s.JID
	if parsed, err := types.ParseJID(s.JID); err == nil && !parsed.IsEmpty() {
		ourJID = parsed.ToNonAD().String()
	}
	var val string
	err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2", ourJID, key).Scan(&val)
	if err != nil {
		err = db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE (our_jid='' OR our_jid IS NULL) AND key=$1 LIMIT 1", key).Scan(&val)
	}
	return val, err
}

// IsTargetSudo checks if a given target is a registered sudo user or the bot owner.
func (ctx *PluginContext) IsTargetSudo(target types.JID) bool {
	if ctx == nil || ctx.Client == nil {
		return false
	}
	return IsSudoRaw(ctx.Ctx, ctx.Client, target)
}

// IsTargetOwner checks if a given target is the bot owner.
func (ctx *PluginContext) IsTargetOwner(target types.JID) bool {
	if ctx == nil || ctx.Client == nil || ctx.Client.Store.ID == nil {
		return false
	}
	return ctx.IsSameUser(target, *ctx.Client.Store.ID)
}

// GetMedia retrieves media bytes and mimetype from the message or its quoted message.
func (ctx *PluginContext) GetMedia() ([]byte, string, error) {
	// First check the main message
	if data, mimetype, ok := ctx.extractMediaFromMessage(ctx.Evt.Message); ok {
		return data, mimetype, nil
	}
	// Then check quoted message
	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		if data, mimetype, ok := ctx.extractMediaFromMessage(quoted); ok {
			return data, mimetype, nil
		}
	}
	return nil, "", fmt.Errorf("no media found in message or quoted message")
}

func (ctx *PluginContext) extractMediaFromMessage(msg *waE2E.Message) ([]byte, string, bool) {
	if msg == nil {
		return nil, "", false
	}
	var downloadable whatsmeow.DownloadableMessage
	var mimetype string

	if img := msg.GetImageMessage(); img != nil {
		downloadable = img
		mimetype = img.GetMimetype()
	} else if vid := msg.GetVideoMessage(); vid != nil {
		downloadable = vid
		mimetype = vid.GetMimetype()
	} else if aud := msg.GetAudioMessage(); aud != nil {
		downloadable = aud
		mimetype = aud.GetMimetype()
	} else if doc := msg.GetDocumentMessage(); doc != nil {
		downloadable = doc
		mimetype = doc.GetMimetype()
	} else if stk := msg.GetStickerMessage(); stk != nil {
		downloadable = stk
		mimetype = stk.GetMimetype()
	}

	if downloadable == nil {
		return nil, "", false
	}

	data, err := ctx.Client.Download(ctx.Ctx, downloadable)
	if err != nil {
		return nil, "", false
	}
	if mimetype == "" {
		if msg.GetImageMessage() != nil {
			mimetype = "image/jpeg"
		} else if msg.GetVideoMessage() != nil {
			mimetype = "video/mp4"
		} else if msg.GetAudioMessage() != nil {
			mimetype = "audio/ogg"
		} else if msg.GetDocumentMessage() != nil {
			mimetype = "application/octet-stream"
		} else if msg.GetStickerMessage() != nil {
			mimetype = "image/webp"
		}
	}
	return data, mimetype, true
}

// SendAudio uploads and sends an audio file (converted to Opus PTT voice note if supported, or standard audio track if raw format).
func (ctx *PluginContext) SendAudio(data []byte, mimetype string) error {
	ctx.StopAutoLoader()
	meta, errMeta := EnsureOpusPTT(ctx.Ctx, data)
	isPTT := false
	if errMeta == nil && meta != nil && meta.Converted && len(meta.Data) > 0 {
		data = meta.Data
		mimetype = "audio/ogg; codecs=opus"
		isPTT = true
	} else {
		if mimetype == "" || strings.Contains(mimetype, "opus") {
			mimetype = "audio/mp4"
		}
	}

	Logger.Debug("Building SendAudio", "data_len", len(data), "mimetype", mimetype, "isPTT", isPTT)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		Logger.Error("SendAudio: upload failed", "err", err)
		return fmt.Errorf("audio upload failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			PTT:           new(isPTT),
		},
	}
	if isPTT && meta != nil {
		if meta.Seconds > 0 {
			msg.AudioMessage.Seconds = new(meta.Seconds)
		}
		if len(meta.Waveform) > 0 {
			msg.AudioMessage.Waveform = meta.Waveform
		}
	}
	Logger.Debug("Sending SendAudio", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("SendAudio failed", "err", err)
	} else {
		Logger.Debug("SendAudio sent successfully")
	}
	return err
}

// ReplyWithAudio uploads and sends an audio file reply.
func (ctx *PluginContext) ReplyWithAudio(data []byte, mimetype string) error {
	ctx.StopAutoLoader()
	meta, errMeta := EnsureOpusPTT(ctx.Ctx, data)
	isPTT := false
	if errMeta == nil && meta != nil && meta.Converted && len(meta.Data) > 0 {
		data = meta.Data
		mimetype = "audio/ogg; codecs=opus"
		isPTT = true
	} else {
		if mimetype == "" || strings.Contains(mimetype, "opus") {
			mimetype = "audio/mp4"
		}
	}

	cinfo := ctx.replyContextInfo()
	Logger.Debug("Building ReplyWithAudio", "data_len", len(data), "mimetype", mimetype, "isPTT", isPTT, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		Logger.Error("ReplyWithAudio: upload failed", "err", err)
		return fmt.Errorf("audio upload failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			PTT:           new(isPTT),
			ContextInfo:   cinfo,
		},
	}
	if meta != nil {
		if meta.Seconds > 0 {
			msg.AudioMessage.Seconds = new(meta.Seconds)
		}
		if len(meta.Waveform) > 0 {
			msg.AudioMessage.Waveform = meta.Waveform
		}
	}
	Logger.Debug("Sending ReplyWithAudio", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("ReplyWithAudio failed", "err", err)
	} else {
		Logger.Debug("ReplyWithAudio sent successfully")
	}
	return err
}

// SendTextWithMentions sends a text message with WhatsApp mentions.
func (ctx *PluginContext) SendTextWithMentions(text string, jids []types.JID) error {
	ctx.StopAutoLoader()
	formatted := ctx.formatMentionTextResponse(text)
	var mentioned []string
	for _, j := range jids {
		if !j.IsEmpty() {
			mentioned = append(mentioned, j.String())
		}
	}
	Logger.Debug("Building SendTextWithMentions", "text", text, "formatted", formatted, "mentioned_jids", mentioned)
	Logger.Debug("Sending SendTextWithMentions", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	})
	if err != nil {
		Logger.Error("SendTextWithMentions failed", "err", err)
	} else {
		Logger.Debug("SendTextWithMentions sent successfully")
	}
	return err
}

// ReplyWithMentions sends a text message with WhatsApp mentions replying to the current message.
func (ctx *PluginContext) ReplyWithMentions(text string, jids []types.JID) error {
	ctx.StopAutoLoader()
	formatted := ctx.formatMentionTextResponse(text)
	var mentioned []string
	for _, j := range jids {
		if !j.IsEmpty() {
			mentioned = append(mentioned, j.String())
		}
	}
	cInfo := ctx.replyContextInfo()
	if cInfo != nil {
		cInfo.MentionedJID = mentioned
	} else {
		cInfo = &waE2E.ContextInfo{
			MentionedJID: mentioned,
		}
	}
	Logger.Debug("Building ReplyWithMentions", "text", text, "formatted", formatted, "mentioned_jids", mentioned, "context_info", cInfo)
	Logger.Debug("Sending ReplyWithMentions", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cInfo,
		},
	})
	if err != nil {
		Logger.Error("ReplyWithMentions failed", "err", err)
	} else {
		Logger.Debug("ReplyWithMentions sent successfully")
	}
	return err
}

// ResolveMentionRaw returns the canonical Phone Number JID (resolving any LID to @s.whatsapp.net)
// and the username / phone number string suitable for @mentions in text.
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, jid types.JID) (types.JID, string) {
	resolved := ResolvePN(ctx, client, jid)
	return resolved, resolved.User
}

// ResolveMention returns the resolved Phone Number JID and username matching display representation for mentions.
func (ctx *PluginContext) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(ctx.Ctx, ctx.Client, jid)
}

// SendTextWithGroupMention sends a text message featuring WhatsApp's native @all group mention via NonJIDMentions.
func (ctx *PluginContext) SendTextWithGroupMention(text string) error {
	ctx.StopAutoLoader()
	formatted := ctx.formatMentionTextResponse(text)

	var nonJID uint32 = 1
	cInfo := &waE2E.ContextInfo{
		NonJIDMentions: &nonJID,
	}

	Logger.Debug("Sending SendTextWithGroupMention", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cInfo,
		},
	})
	if err != nil {
		Logger.Error("SendTextWithGroupMention failed", "err", err)
	}
	return err
}

// ReplyWithGroupMention sends a text message featuring WhatsApp's native @all group mention replying to the current message.
func (ctx *PluginContext) ReplyWithGroupMention(text string) error {
	ctx.StopAutoLoader()
	formatted := ctx.formatMentionTextResponse(text)

	var nonJID uint32 = 1
	cInfo := ctx.replyContextInfo()
	if cInfo == nil {
		cInfo = &waE2E.ContextInfo{}
	}
	cInfo.NonJIDMentions = &nonJID

	Logger.Debug("Sending ReplyWithGroupMention", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cInfo,
		},
	})
	if err != nil {
		Logger.Error("ReplyWithGroupMention failed", "err", err)
	}
	return err
}

// React sends an emoji reaction to the current message (e.g. "✅" or "❌").
func (ctx *PluginContext) React(emoji string) error {
	ctx.StopAutoLoader()
	if ctx == nil || ctx.Client == nil || ctx.Evt == nil {
		return fmt.Errorf("cannot react: context, client, or event is nil")
	}
	msg := ctx.Client.BuildReaction(ctx.Chat, ctx.Evt.Info.Sender, ctx.Evt.Info.ID, emoji)
	_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	if err != nil {
		Logger.Error("React failed", "emoji", emoji, "err", err)
	} else {
		Logger.Debug("React sent successfully", "emoji", emoji)
	}
	return err
}

// FormatMention resolves a target JID and returns its "@username" string representation along with the resolved JID for mentions.
func (ctx *PluginContext) FormatMention(jid types.JID) (string, types.JID) {
	resolvedJID, username := ctx.ResolveMention(jid)
	return "@" + username, resolvedJID
}

func (ctx *PluginContext) formatMentionTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = RemoveEmojis(text)
	return text
}

// Protobuf Helper: encodeProtoMessage marshal and base64 encodes a message
func EncodeProtoMessage(msg *waE2E.Message) (string, error) {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// Protobuf Helper: decodeProtoMessage base64 decodes and unmarshals a message
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var msg waE2E.Message
	err = proto.Unmarshal(bytes, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// IsViewOnceMessage returns true if the message is a ViewOnce container or contains a ViewOnce media attachment.
func IsViewOnceMessage(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}

	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		msg = msg.EphemeralMessage.Message
	}

	if msg.ViewOnceMessage != nil || msg.ViewOnceMessageV2 != nil || msg.ViewOnceMessageV2Extension != nil {
		return true
	}
	if img := msg.GetImageMessage(); img != nil && img.GetViewOnce() {
		return true
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		return true
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetViewOnce() {
		return true
	}
	if doc := msg.GetDocumentWithCaptionMessage(); doc != nil && doc.Message != nil {
		return IsViewOnceMessage(doc.Message)
	}
	return false
}

// ExtractViewOnceMessage unwraps any ViewOnce message container and clears the ViewOnce flag.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}

	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		msg = msg.EphemeralMessage.Message
	}

	var inner *waE2E.Message
	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		inner = msg.ViewOnceMessage.Message
	} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		inner = msg.ViewOnceMessageV2.Message
	} else if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		inner = msg.ViewOnceMessageV2Extension.Message
	} else if msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil {
		inner = msg.DocumentWithCaptionMessage.Message
	} else {
		inner = msg
	}

	cloned := proto.Clone(inner).(*waE2E.Message)

	cloned.ViewOnceMessage = nil
	cloned.ViewOnceMessageV2 = nil
	cloned.ViewOnceMessageV2Extension = nil

	if cloned.ImageMessage != nil {
		cloned.ImageMessage.ViewOnce = new(false)
	}
	if cloned.VideoMessage != nil {
		cloned.VideoMessage.ViewOnce = new(false)
	}
	if cloned.AudioMessage != nil {
		cloned.AudioMessage.ViewOnce = new(false)
	}

	return cloned
}

// UnwrapAndSendViewOnceMessage downloads the encrypted ViewOnce media, re-uploads it with fresh media keys (preventing "media unavailable" CDN expiry), and sends the clean unwrapped message object to target JID.
// quoteID is the stanza ID of the original ViewOnce message; when non-empty the forwarded message quotes it so the recipient can see whose VV was intercepted.
func UnwrapAndSendViewOnceMessage(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message, senderJID types.JID, pushName string, targetJID types.JID, quoteID string, sourceChat ...types.JID) error {
	if msg == nil || client == nil {
		Logger.Error("[AutoVV] UnwrapAndSendViewOnceMessage: invalid nil arguments", "msg_nil", msg == nil, "client_nil", client == nil)
		return fmt.Errorf("invalid arguments")
	}

	var srcChat types.JID
	if len(sourceChat) > 0 {
		srcChat = sourceChat[0]
	}

	Logger.Info("[AutoVV] UnwrapAndSendViewOnceMessage started",
		"sender_jid", senderJID.String(),
		"sender_non_ad", senderJID.ToNonAD().String(),
		"push_name", pushName,
		"target_jid", targetJID.String(),
		"quote_id", quoteID,
		"source_chat", srcChat.String(),
		"is_group_source", srcChat.Server == "g.us",
	)

	unwrapped := ExtractViewOnceMessage(msg)
	if unwrapped == nil {
		Logger.Error("[AutoVV] Failed to extract inner ViewOnce message")
		return fmt.Errorf("failed to extract inner ViewOnce message")
	}

	var mediaType string
	if img := unwrapped.GetImageMessage(); img != nil {
		mediaType = "image"
		Logger.Debug("[AutoVV] Downloading ViewOnce image", "mimetype", img.GetMimetype(), "file_length", img.GetFileLength())
		data, err := client.Download(ctx, img)
		if err != nil {
			Logger.Error("[AutoVV] Failed to download viewonce image", "err", err)
			return fmt.Errorf("failed to download viewonce image: %w", err)
		}
		if len(data) == 0 {
			Logger.Error("[AutoVV] Downloaded viewonce image data is empty")
			return fmt.Errorf("downloaded viewonce image data is empty")
		}
		uploaded, errUp := client.Upload(ctx, data, whatsmeow.MediaImage)
		if errUp != nil {
			Logger.Error("[AutoVV] Failed to upload unwrapped viewonce image", "err", errUp)
			return fmt.Errorf("failed to upload unwrapped viewonce image: %w", errUp)
		}
		Logger.Debug("[AutoVV] Image uploaded successfully", "data_len", len(data), "url", uploaded.URL)
		img.URL = &uploaded.URL
		img.DirectPath = &uploaded.DirectPath
		img.MediaKey = uploaded.MediaKey
		img.FileEncSHA256 = uploaded.FileEncSHA256
		img.FileSHA256 = uploaded.FileSHA256
		img.FileLength = new(uint64(len(data)))
		img.ViewOnce = new(false)
	} else if vid := unwrapped.GetVideoMessage(); vid != nil {
		mediaType = "video"
		Logger.Debug("[AutoVV] Downloading ViewOnce video", "mimetype", vid.GetMimetype(), "file_length", vid.GetFileLength())
		data, err := client.Download(ctx, vid)
		if err != nil {
			Logger.Error("[AutoVV] Failed to download viewonce video", "err", err)
			return fmt.Errorf("failed to download viewonce video: %w", err)
		}
		if len(data) == 0 {
			Logger.Error("[AutoVV] Downloaded viewonce video data is empty")
			return fmt.Errorf("downloaded viewonce video data is empty")
		}
		uploaded, errUp := client.Upload(ctx, data, whatsmeow.MediaVideo)
		if errUp != nil {
			Logger.Error("[AutoVV] Failed to upload unwrapped viewonce video", "err", errUp)
			return fmt.Errorf("failed to upload unwrapped viewonce video: %w", errUp)
		}
		Logger.Debug("[AutoVV] Video uploaded successfully", "data_len", len(data), "url", uploaded.URL)
		vid.URL = &uploaded.URL
		vid.DirectPath = &uploaded.DirectPath
		vid.MediaKey = uploaded.MediaKey
		vid.FileEncSHA256 = uploaded.FileEncSHA256
		vid.FileSHA256 = uploaded.FileSHA256
		vid.FileLength = new(uint64(len(data)))
		vid.ViewOnce = new(false)
	} else if aud := unwrapped.GetAudioMessage(); aud != nil {
		mediaType = "audio"
		Logger.Debug("[AutoVV] Downloading ViewOnce audio", "mimetype", aud.GetMimetype(), "file_length", aud.GetFileLength())
		data, err := client.Download(ctx, aud)
		if err != nil {
			Logger.Error("[AutoVV] Failed to download viewonce audio", "err", err)
			return fmt.Errorf("failed to download viewonce audio: %w", err)
		}
		if len(data) == 0 {
			Logger.Error("[AutoVV] Downloaded viewonce audio data is empty")
			return fmt.Errorf("downloaded viewonce audio data is empty")
		}
		meta, cErr := EnsureOpusPTT(ctx, data)
		if cErr == nil && meta != nil && len(meta.Data) > 0 {
			data = meta.Data
			if meta.Seconds > 0 {
				aud.Seconds = new(meta.Seconds)
			}
			if len(meta.Waveform) > 0 {
				aud.Waveform = meta.Waveform
			}
		}
		uploaded, errUp := client.Upload(ctx, data, whatsmeow.MediaAudio)
		if errUp != nil {
			Logger.Error("[AutoVV] Failed to upload unwrapped viewonce audio", "err", errUp)
			return fmt.Errorf("failed to upload unwrapped viewonce audio: %w", errUp)
		}
		Logger.Debug("[AutoVV] Audio uploaded successfully", "data_len", len(data), "url", uploaded.URL)
		aud.URL = &uploaded.URL
		aud.DirectPath = &uploaded.DirectPath
		aud.MediaKey = uploaded.MediaKey
		aud.FileEncSHA256 = uploaded.FileEncSHA256
		aud.FileSHA256 = uploaded.FileSHA256
		aud.FileLength = new(uint64(len(data)))
		aud.PTT = new(true)
		aud.ViewOnce = new(false)
	}

	// Quote the original ViewOnce message so the recipient can see whose VV was forwarded.
	if quoteID != "" {
		participant := senderJID.ToNonAD().String()
		quotedClean := ExtractViewOnceMessage(msg)
		if quotedClean != nil {
			if img := quotedClean.GetImageMessage(); img != nil {
				img.ContextInfo = nil
			} else if vid := quotedClean.GetVideoMessage(); vid != nil {
				vid.ContextInfo = nil
			} else if aud := quotedClean.GetAudioMessage(); aud != nil {
				aud.ContextInfo = nil
			}
		} else {
			quotedClean = msg
		}

		ci := &waE2E.ContextInfo{
			StanzaID:      &quoteID,
			Participant:   &participant,
			QuotedMessage: quotedClean,
		}

		if !srcChat.IsEmpty() && srcChat != targetJID {
			remoteJID := srcChat.ToNonAD().String()
			ci.RemoteJID = &remoteJID
		}

		remoteJIDStr := "<none>"
		if ci.RemoteJID != nil {
			remoteJIDStr = *ci.RemoteJID
		}

		Logger.Info("[AutoVV] Quoted ContextInfo prepared",
			"stanza_id", quoteID,
			"participant", participant,
			"remote_jid", remoteJIDStr,
			"media_type", mediaType,
			"target_jid", targetJID.String(),
		)

		if img := unwrapped.GetImageMessage(); img != nil {
			img.ContextInfo = ci
		} else if vid := unwrapped.GetVideoMessage(); vid != nil {
			vid.ContextInfo = ci
		} else if aud := unwrapped.GetAudioMessage(); aud != nil {
			aud.ContextInfo = ci
		}
	} else {
		Logger.Warn("[AutoVV] quoteID is empty; forwarding without quote")
	}

	resp, err := client.SendMessage(ctx, targetJID, unwrapped)
	if err != nil {
		Logger.Error("[AutoVV] SendMessage failed", "target_jid", targetJID.String(), "err", err)
		return err
	}
	Logger.Info("[AutoVV] Forwarded message sent successfully", "target_jid", targetJID.String(), "resp_id", resp.ID, "timestamp", resp.Timestamp)
	return nil
}

// IsAdminRaw checks if a specific JID is a group admin.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, info *types.GroupInfo, jid types.JID) bool {
	Logger.Debug("IsAdminRaw checking", "jid", jid.String(), "group", info.JID.String())
	target := jid.ToNonAD()
	for _, p := range info.Participants {
		if IsSameUserRaw(ctx, client, p.JID, target) {
			res := p.IsAdmin || p.IsSuperAdmin
			Logger.Debug("IsAdminRaw result", "jid", jid.String(), "isAdmin", res, "isSuperAdmin", p.IsSuperAdmin)
			return res
		}
	}
	Logger.Debug("IsAdminRaw result: false (not a participant)", "jid", jid.String())
	return false
}

// IsAdmin checks if a specific JID is a group admin.
func (ctx *PluginContext) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	res := IsAdminRaw(ctx.Ctx, ctx.Client, info, jid)
	Logger.Debug("IsAdmin helper check", "jid", jid.String(), "result", res)
	return res
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (ctx *PluginContext) AmIAdmin(info *types.GroupInfo) bool {
	Logger.Debug("AmIAdmin checking")
	if ctx.Client.Store.ID == nil {
		Logger.Debug("AmIAdmin result: false (bot JID nil)")
		return false
	}
	res := ctx.IsAdmin(info, *ctx.Client.Store.ID)
	Logger.Debug("AmIAdmin result", "result", res)
	return res
}

// IsSenderAdmin checks if the command sender is a group admin or bot sudoer.
func (ctx *PluginContext) IsSenderAdmin(info *types.GroupInfo) bool {
	Logger.Debug("IsSenderAdmin checking", "sender", ctx.Sender.String())
	if ctx.IsSudo() {
		Logger.Debug("IsSenderAdmin result: true (is sudo)")
		return true
	}
	res := ctx.IsAdmin(info, ctx.Sender)
	Logger.Debug("IsSenderAdmin result", "sender", ctx.Sender.String(), "result", res)
	return res
}

// RemoveEmojis strips emoji runes from a string.
func RemoveEmojis(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 0x1F000 && r <= 0x1F9FF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1FA00 && r <= 0x1FAFF) || (r >= 0x1F1E0 && r <= 0x1F1FF) {
			continue // skip emoji
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// IsSaveText checks if the text content contains the word "save".
func IsSaveText(text string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(text)), "save")
}

// GetDirectMessageText extracts conversation or extended text content from a waE2E message.
func GetDirectMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	if msg.GetExtendedTextMessage() != nil {
		sb.WriteString(" ")
		sb.WriteString(msg.GetExtendedTextMessage().GetText())
	}
	if msg.GetConversation() != "" {
		sb.WriteString(" ")
		sb.WriteString(msg.GetConversation())
	}
	return sb.String()
}

// SanitizeJID replaces special characters in a JID string for safe file/key representation.
func SanitizeJID(s string) string {
	res := strings.NewReplacer("@", "_at_", ":", "_", ".", "_").Replace(s)
	Logger.Debugf("Sanitized JID from %s to %s", s, res)
	return res
}

// IsKnownLanguageCode checks if a language code or name is valid for TTS.
func IsKnownLanguageCode(lang string) bool {
	clean := strings.ToLower(strings.TrimSpace(lang))
	if clean == "" {
		return false
	}
	// Common ISO language codes
	known := map[string]bool{
		"en": true, "es": true, "fr": true, "de": true, "it": true, "pt": true,
		"ru": true, "ja": true, "ko": true, "zh": true, "ar": true, "hi": true,
		"tr": true, "nl": true, "pl": true, "sv": true, "id": true, "th": true,
		"vi": true, "he": true, "uk": true, "cs": true, "el": true, "hu": true,
		"ro": true, "sk": true, "da": true, "fi": true, "no": true, "sw": true,
	}
	return known[clean] || len(clean) == 2 || len(clean) == 5
}
