package builder

import (
	"fmt"

	Logger "whatsrook/logger"

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

// To overrides the destination JID (defaults to sender context chat).
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
	if m.rook == nil || m.rook.sender == nil {
		return fmt.Errorf("MessageBuilder: Send called without Sender context")
	}
	sender := m.rook.sender
	sender.StopAutoLoader()

	to := m.to
	if to.IsEmpty() {
		to = sender.GetChat()
	}

	Logger.Debug("WARook: MessageBuilder.Send", "to", to.String(), "mediaKind", m.mediaKind, "asReply", m.asReply)

	switch m.mediaKind {
	case "reaction":
		return sender.React(m.reaction)

	case "image":
		if len(m.mentions) > 0 {
			if m.asReply {
				return sender.ReplyWithImageWithMentions(m.mediaData, m.mediaMime, m.mediaCaption, m.mentions)
			}
			return sender.SendImageWithMentions(m.mediaData, m.mediaMime, m.mediaCaption, m.mentions)
		}
		if m.asReply {
			return sender.ReplyWithImage(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return sender.SendImage(m.mediaData, m.mediaMime, m.mediaCaption)

	case "video":
		if m.asReply {
			return sender.ReplyWithVideo(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return sender.SendVideo(m.mediaData, m.mediaMime, m.mediaCaption)

	case "video_gif":
		if m.asReply {
			return sender.ReplyWithVideoGif(m.mediaData, m.mediaMime, m.mediaCaption)
		}
		return sender.SendVideoGif(m.mediaData, m.mediaMime, m.mediaCaption)

	case "audio":
		if m.asReply {
			return sender.ReplyWithAudio(m.mediaData, m.mediaMime)
		}
		return sender.SendAudio(m.mediaData, m.mediaMime)

	case "document":
		if m.asReply {
			return sender.ReplyWithDocument(m.mediaData, m.mediaMime, m.mediaName, "")
		}
		return sender.SendDocument(m.mediaData, m.mediaMime, m.mediaName, "")

	case "sticker":
		if m.asReply {
			return sender.ReplyWithSticker(m.mediaData)
		}
		return sender.SendSticker(m.mediaData)

	default:
		// Plain text / Mentions / Group Mention
		if m.text == "" {
			return fmt.Errorf("WARook: MessageBuilder.Send called with no text or media")
		}
		if m.groupMention {
			if m.asReply {
				return sender.ReplyWithGroupMention(m.text)
			}
			return sender.SendTextWithGroupMention(m.text)
		}
		if len(m.mentions) > 0 {
			if m.asReply {
				return sender.ReplyWithMentions(m.text, m.mentions)
			}
			return sender.SendTextWithMentions(m.text, m.mentions)
		}
		if m.asReply {
			return sender.Reply(m.text)
		}
		return sender.SendText(m.text)
	}
}
