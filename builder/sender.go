package builder

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Sender abstracts the messaging context and capabilities required by builders.
type Sender interface {
	GetSendContext() context.Context
	GetClient() *whatsmeow.Client
	GetChat() types.JID
	GetSender() types.JID
	GetBotName() string
	StopAutoLoader()
	ReplyContextInfo() *waE2E.ContextInfo
	FormatTextResponse(text string) string

	// Reply methods (quote the triggering message)
	Reply(text string) error
	ReplyWithMentions(text string, mentions []types.JID) error
	ReplyWithImage(data []byte, mimetype, caption string) error
	ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error
	ReplyWithVideo(data []byte, mimetype, caption string) error
	ReplyWithVideoGif(data []byte, mimetype, caption string) error
	ReplyWithAudio(data []byte, mimetype string) error
	ReplyWithDocument(data []byte, mimetype, filename, caption string) error
	ReplyWithSticker(data []byte) error
	ReplyWithGroupMention(text string) error

	// Send methods (send without quoting)
	SendText(text string) error
	SendTextWithMentions(text string, mentions []types.JID) error
	SendImage(data []byte, mimetype, caption string) error
	SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error
	SendVideo(data []byte, mimetype, caption string) error
	SendVideoGif(data []byte, mimetype, caption string) error
	SendAudio(data []byte, mimetype string) error
	SendDocument(data []byte, mimetype, filename, caption string) error
	SendSticker(data []byte) error
	SendTextWithGroupMention(text string) error

	// Action methods
	React(emoji string) error
	Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error)
}
