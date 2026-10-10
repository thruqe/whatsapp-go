package send

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook/util/logger"
	"github.com/thruqe/whatsrook/util/webp"
)

// Image sends an image to a chat without quoting.
func Image(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return ImageWithMentions(ctx, client, chat, data, mimetype, caption, nil, quoted...)
}

// ImageWithMentions sends an image with mentioned users.
func ImageWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, mentions []types.JID, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 && quoted[0] != nil {
		ci = quoted[0]
	}
	if mentionStrs := ResolveMentionJIDStrings(ctx, client, mentions); len(mentionStrs) > 0 {
		if ci == nil {
			ci = &waE2E.ContextInfo{}
		}
		ci.MentionedJID = append(ci.MentionedJID, mentionStrs...)
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// Video sends a video to a chat without quoting.
func Video(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return VideoWithMentions(ctx, client, chat, data, mimetype, caption, nil, false, quoted...)
}

// VideoGif sends a looping GIF video to a chat without quoting.
func VideoGif(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return VideoWithMentions(ctx, client, chat, data, mimetype, caption, nil, true, quoted...)
}

// VideoWithProgress sends a video with an optional upload progress callback.
func VideoWithProgress(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, onProgress func(uploaded, total uint64), quoted ...*waE2E.ContextInfo) error {
	return videoWithProgressInternal(ctx, client, chat, data, mimetype, caption, false, onProgress, quoted...)
}

// VideoGifWithProgress sends a looping GIF video with an optional upload progress callback.
func VideoGifWithProgress(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, onProgress func(uploaded, total uint64), quoted ...*waE2E.ContextInfo) error {
	return videoWithProgressInternal(ctx, client, chat, data, mimetype, caption, true, onProgress, quoted...)
}

func videoWithProgressInternal(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, isGif bool, onProgress func(uploaded, total uint64), quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := client.UploadWithProgress(ctx, data, whatsmeow.MediaVideo, onProgress)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 && quoted[0] != nil {
		ci = quoted[0]
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   new(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// VideoWithMentions sends a video with mentions to a chat.
func VideoWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, mentions []types.JID, isGif bool, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 && quoted[0] != nil {
		ci = quoted[0]
	}
	if mentionStrs := ResolveMentionJIDStrings(ctx, client, mentions); len(mentionStrs) > 0 {
		if ci == nil {
			ci = &waE2E.ContextInfo{}
		}
		ci.MentionedJID = append(ci.MentionedJID, mentionStrs...)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   new(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// Audio sends an audio file to a chat.
func Audio(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype string, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 {
		ci = quoted[0]
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
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// Document sends a document file to a chat.
func Document(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, filename, caption string, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 {
		ci = quoted[0]
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileName:      &filename,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// Sticker sends a WebP sticker to a chat.
func Sticker(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, pack, author string, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		logger.Error("SendSticker: invalid payload data", "bytes", len(data))
		return fmt.Errorf("invalid sticker data: missing WebP header")
	}

	if meta, err := webp.GetStickerMetadata(data); err != nil || meta == nil {
		if withMeta, err := webp.AddStickerMetadata(data, pack, author); err == nil {
			data = withMeta
		} else {
			logger.Warn("SendSticker: could not inject sticker metadata", "err", err)
		}
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload sticker failed: %w", err)
	}

	width := uint32(512)
	height := uint32(512)
	isAnimated := false
	mimetype := "image/webp"

	var ci *waE2E.ContextInfo
	if len(quoted) > 0 {
		ci = quoted[0]
	}

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Width:         &width,
			Height:        &height,
			IsAnimated:    &isAnimated,
			ContextInfo:   ci,
		},
	}
	_, err = client.SendMessage(ctx, chat, msg)
	return err
}

// Sender convenience methods
func (s *Sender) Image(data []byte, mimetype, caption string) error {
	return Image(s.getContext(), s.Client, s.Chat, data, mimetype, caption)
}

func (s *Sender) Video(data []byte, mimetype, caption string) error {
	return Video(s.getContext(), s.Client, s.Chat, data, mimetype, caption)
}

func (s *Sender) Audio(data []byte, mimetype string) error {
	return Audio(s.getContext(), s.Client, s.Chat, data, mimetype)
}

func (s *Sender) Document(data []byte, mimetype, filename, caption string) error {
	return Document(s.getContext(), s.Client, s.Chat, data, mimetype, filename, caption)
}

func (s *Sender) Sticker(data []byte, pack, author string) error {
	return Sticker(s.getContext(), s.Client, s.Chat, data, pack, author)
}
