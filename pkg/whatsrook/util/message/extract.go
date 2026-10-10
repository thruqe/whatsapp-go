package message

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// GetMediaType returns the human-readable media classification string.
func GetMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return "text"
	}
	if msg.ImageMessage != nil {
		return "image"
	}
	if msg.VideoMessage != nil {
		if msg.VideoMessage.GetGifPlayback() {
			return "gif"
		}
		return "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "voice"
		}
		return "audio"
	}
	if msg.DocumentMessage != nil {
		return "document"
	}
	if msg.StickerMessage != nil {
		return "sticker"
	}
	return "text"
}

// ExtractMessageText extracts the textual payload from incoming message events.
func ExtractMessageText(evt *events.Message) string {
	if evt == nil || evt.Message == nil {
		return ""
	}
	return ExtractTextFromProto(evt.Message)
}

// ExtractTextFromProto extracts displayable text from raw message protobuf payloads.
func ExtractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	if conv := msg.GetConversation(); conv != "" {
		return conv
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil {
		return poll.GetName()
	}
	if pollV2 := msg.GetPollCreationMessageV2(); pollV2 != nil {
		return pollV2.GetName()
	}
	if pollV3 := msg.GetPollCreationMessageV3(); pollV3 != nil {
		return pollV3.GetName()
	}
	if viewOnce := msg.GetViewOnceMessage(); viewOnce != nil && viewOnce.Message != nil {
		return ExtractTextFromProto(viewOnce.Message)
	}
	if viewOnceV2 := msg.GetViewOnceMessageV2(); viewOnceV2 != nil && viewOnceV2.Message != nil {
		return ExtractTextFromProto(viewOnceV2.Message)
	}
	if viewOnceV2Ext := msg.GetViewOnceMessageV2Extension(); viewOnceV2Ext != nil && viewOnceV2Ext.Message != nil {
		return ExtractTextFromProto(viewOnceV2Ext.Message)
	}
	if protoMsg := msg.GetProtocolMessage(); protoMsg != nil {
		if ephem := protoMsg.GetEphemeralExpiration(); ephem > 0 {
			return ""
		}
	}
	return ""
}

// ExtractMediaFromEvent extracts the downloadable media payload, indicator, and MIME type.
func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}
	dl, mime, ok := ExtractMedia(evt.Message)
	return dl, ok, mime
}

// ExtractMedia extracts downloadable media from a message protobuf.
func ExtractMedia(msg *waE2E.Message) (whatsmeow.DownloadableMessage, string, bool) {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil, "", false
	}
	if img := msg.GetImageMessage(); img != nil {
		return img, img.GetMimetype(), true
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, vid.GetMimetype(), true
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud, aud.GetMimetype(), true
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc, doc.GetMimetype(), true
	}
	if stk := msg.GetStickerMessage(); stk != nil {
		return stk, stk.GetMimetype(), true
	}
	return nil, "", false
}

// Download downloads decrypted media bytes and mimetype from a message.
func Download(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message) ([]byte, string, error) {
	if client == nil {
		return nil, "", fmt.Errorf("client unavailable")
	}
	downloadable, mime, ok := ExtractMedia(msg)
	if !ok || downloadable == nil {
		return nil, "", fmt.Errorf("no media found in message")
	}
	data, err := client.Download(ctx, downloadable)
	if err != nil {
		return nil, "", err
	}
	return data, mime, nil
}

// IsViewOnceMessage determines if a message payload represents view-once media.
func IsViewOnceMessage(msg *waE2E.Message) bool {
	if msg == nil {
		return false
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
	return false
}

// ExtractViewOnceMessage unwraps the inner message from a view-once wrapper.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}
	if vo := msg.GetViewOnceMessage(); vo != nil && vo.Message != nil {
		return vo.Message
	}
	if vo2 := msg.GetViewOnceMessageV2(); vo2 != nil && vo2.Message != nil {
		return vo2.Message
	}
	if vo2Ext := msg.GetViewOnceMessageV2Extension(); vo2Ext != nil && vo2Ext.Message != nil {
		return vo2Ext.Message
	}
	return msg
}

// UnwrapAndSendViewOnceMessage unwraps a view-once message and dispatches it as standard media.
func UnwrapAndSendViewOnceMessage(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message, senderJID types.JID, pushName string, targetJID types.JID, quoteID string, sourceChat ...types.JID) error {
	if client == nil || msg == nil {
		return fmt.Errorf("client or message unavailable")
	}
	unwrapped := ExtractViewOnceMessage(msg)
	if unwrapped == nil {
		return fmt.Errorf("failed to unwrap view-once message")
	}

	clone := proto.Clone(unwrapped).(*waE2E.Message)
	if img := clone.GetImageMessage(); img != nil {
		img.ViewOnce = new(false)
	} else if vid := clone.GetVideoMessage(); vid != nil {
		vid.ViewOnce = new(false)
	} else if aud := clone.GetAudioMessage(); aud != nil {
		aud.ViewOnce = new(false)
	}

	header := fmt.Sprintf("*Anti-ViewOnce*: from @%s", senderJID.User)
	if pushName != "" {
		header += fmt.Sprintf(" (%s)", pushName)
	}
	if len(sourceChat) > 0 && !sourceChat[0].IsEmpty() && sourceChat[0] != targetJID {
		header += fmt.Sprintf(" in %s", sourceChat[0].String())
	}

	if img := clone.GetImageMessage(); img != nil {
		if origCap := img.GetCaption(); origCap != "" {
			img.Caption = new(header + "\n\n" + origCap)
		} else {
			img.Caption = new(header)
		}
	} else if vid := clone.GetVideoMessage(); vid != nil {
		if origCap := vid.GetCaption(); origCap != "" {
			vid.Caption = new(header + "\n\n" + origCap)
		} else {
			vid.Caption = new(header)
		}
	}

	_, err := client.SendMessage(ctx, targetJID, clone)
	return err
}

var emojiRegex = regexp.MustCompile(`[\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}\x{1F1E0}-\x{1F1FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{FE00}-\x{FE0F}\x{1F900}-\x{1F9FF}\x{1FA70}-\x{1FAFF}]`)

// RemoveEmojis strips emoji characters from a string.
func RemoveEmojis(s string) string {
	return emojiRegex.ReplaceAllString(s, "")
}

// FormatTextResponseRaw applies global output sanitization or formatting.
func FormatTextResponseRaw(text string) string {
	return text
}

// EncodeProtoMessage encodes a protobuf message to a hex string.
func EncodeProtoMessage(msg proto.Message) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("message is nil")
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// DecodeProtoMessage decodes a protobuf message from a hex or base64 string.
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	encoded = strings.TrimSpace(encoded)
	var data []byte
	var err error
	if hexData, hErr := hex.DecodeString(encoded); hErr == nil && len(hexData) > 0 {
		data = hexData
	} else if b64Data, bErr := base64.StdEncoding.DecodeString(encoded); bErr == nil && len(b64Data) > 0 {
		data = b64Data
	} else {
		return nil, fmt.Errorf("invalid encoded protobuf payload")
	}

	var msg waE2E.Message
	if err = proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message protobuf: %w", err)
	}
	return &msg, nil
}

// Sprintf returns a formatted string.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
