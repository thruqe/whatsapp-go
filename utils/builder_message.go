package utils

import (
	"fmt"

	"whatsrook/logger"

	"go.mau.fi/whatsmeow/types"
)

// MessageBuilder is a fluent builder for sending a message of any
// supported type (text, image, video, gif, audio, document, sticker, reaction).
type MessageBuilder struct {
	rook         *WARook
	text         string
	to           types.JID
	asReply      bool
	mentions     []types.JID
	groupMention bool
	reaction     string

	// media fields
	mediaData    []byte
	mediaMime    string
	mediaCaption string
	mediaName    string // filename for documents
	mediaKind    string // "image" | "video" | "video_gif" | "audio" | "document" | "sticker" | "reaction"
}

// Text sets or updates the text message content.
func (m *MessageBuilder) Text(text string) *MessageBuilder {
	m.text = text
	return m
}

// To overrides the destination JID (defaults to ctx.Chat).
func (m *MessageBuilder) To(jid types.JID) *MessageBuilder {
	m.to = jid
	return m
}

// AsReply causes the message to quote/reply to the triggering event.
func (m *MessageBuilder) AsReply() *MessageBuilder {
	m.asReply = true
	return m
}

// Mentions attaches JIDs to be mentioned in the message text.
func (m *MessageBuilder) Mentions(jids ...types.JID) *MessageBuilder {
	for _, j := range jids {
		if !j.IsEmpty() {
			m.mentions = append(m.mentions, j)
		}
	}
	return m
}

// GroupMention enables WhatsApp native @all group mention.
func (m *MessageBuilder) GroupMention() *MessageBuilder {
	m.groupMention = true
	return m
}

// WithImage attaches an image payload with optional caption.
func (m *MessageBuilder) WithImage(data []byte, mimetype, caption string) *MessageBuilder {
	m.mediaData = data
	m.mediaMime = mimetype
	m.mediaCaption = caption
	m.mediaKind = "image"
	return m
}

// WithVideo attaches a video payload with optional caption.
func (m *MessageBuilder) WithVideo(data []byte, mimetype, caption string) *MessageBuilder {
	m.mediaData = data
	m.mediaMime = mimetype
	m.mediaCaption = caption
	m.mediaKind = "video"
	return m
}

// WithVideoGif attaches a video payload configured for inline looping GIF playback.
func (m *MessageBuilder) WithVideoGif(data []byte, mimetype, caption string) *MessageBuilder {
	m.mediaData = data
	m.mediaMime = mimetype
	m.mediaCaption = caption
	m.mediaKind = "video_gif"
	return m
}

// WithAudio attaches an audio payload.
func (m *MessageBuilder) WithAudio(data []byte, mimetype string) *MessageBuilder {
	m.mediaData = data
	m.mediaMime = mimetype
	m.mediaKind = "audio"
	return m
}

// WithDocument attaches a document payload.
func (m *MessageBuilder) WithDocument(data []byte, filename, mimetype string) *MessageBuilder {
	m.mediaData = data
	m.mediaName = filename
	m.mediaMime = mimetype
	m.mediaKind = "document"
	return m
}

// WithSticker attaches a WebP sticker payload.
func (m *MessageBuilder) WithSticker(data []byte) *MessageBuilder {
	m.mediaData = data
	m.mediaMime = "image/webp"
	m.mediaKind = "sticker"
	return m
}

// WithReaction sets an emoji reaction on the current event.
func (m *MessageBuilder) WithReaction(emoji string) *MessageBuilder {
	m.reaction = emoji
	m.mediaKind = "reaction"
	return m
}

// Send dispatches the message and returns any error.
func (m *MessageBuilder) Send() error {
	ctx := m.rook.ctx
	ctx.StopAutoLoader()
	to := m.to
	if to.IsEmpty() {
		to = ctx.Chat
	}

	Logger.Debug("WARook: MessageBuilder.Send", "to", to.String(), "mediaKind", m.mediaKind, "asReply", m.asReply)

	switch m.mediaKind {
	case "reaction":
		return ctx.React(m.reaction)

	case "image":
		if len(m.mentions) > 0 {
			if m.asReply {
				return ctx.ReplyWithImageWithMentions(m.mediaData, m.mediaMime, m.mediaCaption, m.mentions)
			}
			return ctx.SendImageWithMentions(m.mediaData, m.mediaMime, m.mediaCaption, m.mentions)
		}
		if m.asReply {
			return ctx.ReplyWithImage(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return ctx.SendImage(m.mediaData, m.mediaMime, m.mediaCaption)

	case "video":
		if m.asReply {
			return ctx.ReplyWithVideo(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return ctx.SendVideo(m.mediaData, m.mediaMime, m.mediaCaption)

	case "video_gif":
		if m.asReply {
			return ctx.ReplyWithVideoGif(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return ctx.SendVideoGif(m.mediaData, m.mediaMime, m.mediaCaption)

	case "audio":
		if m.asReply {
			return ctx.ReplyWithAudio(m.mediaData, m.mediaMime)
		}
		return ctx.SendAudio(m.mediaData, m.mediaMime)

	case "document":
		if m.asReply {
			return ctx.ReplyWithDocument(m.mediaData, m.mediaMime, m.mediaName, "")
		}
		return ctx.SendDocument(m.mediaData, m.mediaMime, m.mediaName, "")

	case "sticker":
		if m.asReply {
			return ctx.ReplyWithSticker(m.mediaData)
		}
		return ctx.SendSticker(m.mediaData)

	default:
		// Plain text / Mentions / Group Mention
		if m.text == "" {
			return fmt.Errorf("WARook: MessageBuilder.Send called with no text or media")
		}
		if m.groupMention {
			if m.asReply {
				return ctx.ReplyWithGroupMention(m.text)
			}
			return ctx.SendTextWithGroupMention(m.text)
		}
		if len(m.mentions) > 0 {
			if m.asReply {
				return ctx.ReplyWithMentions(m.text, m.mentions)
			}
			return ctx.SendTextWithMentions(m.text, m.mentions)
		}
		if m.asReply {
			return ctx.Reply(m.text)
		}
		return ctx.SendText(m.text)
	}
}
