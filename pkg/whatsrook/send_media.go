package whatsrook

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// SendImage sends an image to a chat without quoting.
func SendImage(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendImageWithMentions(ctx, client, chat, data, mimetype, caption, nil, quoted...)
}

var Image = SendImage

// SendImageWithMentions sends an image with mentioned users.
func SendImageWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, mentions []types.JID, quoted ...*waE2E.ContextInfo) error {
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

var ImageWithMentions = SendImageWithMentions

// SendVideo sends a video to a chat without quoting.
func SendVideo(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendVideoWithMentions(ctx, client, chat, data, mimetype, caption, nil, false, quoted...)
}

var Video = SendVideo

// SendVideoGif sends a looping GIF video to a chat without quoting.
func SendVideoGif(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendVideoWithMentions(ctx, client, chat, data, mimetype, caption, nil, true, quoted...)
}

var VideoGif = SendVideoGif

// SendVideoWithProgress sends a video with an optional upload progress callback.
func SendVideoWithProgress(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, onProgress func(uploaded, total uint64), quoted ...*waE2E.ContextInfo) error {
	return videoWithProgressInternal(ctx, client, chat, data, mimetype, caption, false, onProgress, quoted...)
}

var VideoWithProgress = SendVideoWithProgress

// SendVideoGifWithProgress sends a looping GIF video with an optional upload progress callback.
func SendVideoGifWithProgress(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, onProgress func(uploaded, total uint64), quoted ...*waE2E.ContextInfo) error {
	return videoWithProgressInternal(ctx, client, chat, data, mimetype, caption, true, onProgress, quoted...)
}

var VideoGifWithProgress = SendVideoGifWithProgress

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

// SendVideoWithMentions sends a video with mentions to a chat.
func SendVideoWithMentions(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, caption string, mentions []types.JID, isGif bool, quoted ...*waE2E.ContextInfo) error {
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

var VideoWithMentions = SendVideoWithMentions

// SendAudio sends an audio file to a chat.
func SendAudio(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype string, quoted ...*waE2E.ContextInfo) error {
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

var Audio = SendAudio

// SendDocument sends a document file to a chat.
func SendDocument(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, mimetype, filename, caption string, quoted ...*waE2E.ContextInfo) error {
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

var Document = SendDocument

// SendSticker sends a WebP sticker to a chat.
func SendSticker(ctx context.Context, client *whatsmeow.Client, chat types.JID, data []byte, pack, author string, quoted ...*waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		defaultLogger.Errorf("SendSticker: invalid payload data, bytes=%d", len(data))
		return fmt.Errorf("invalid sticker data: missing WebP header")
	}

	if meta, err := GetStickerMetadata(data); err != nil || meta == nil {
		if withMeta, err := AddStickerMetadata(data, pack, author); err == nil {
			data = withMeta
		} else {
			defaultLogger.Warnf("SendSticker: could not inject sticker metadata: %v", err)
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

var Sticker = SendSticker

// AlbumMediaItem represents a media payload item to be bundled in a WhatsApp album.
type AlbumMediaItem struct {
	Data     []byte
	Mimetype string
	Caption  string
	IsVideo  bool
}

// SendAlbum sends multiple media items grouped as a WhatsApp album.
func SendAlbum(ctx context.Context, client *whatsmeow.Client, chat types.JID, items []AlbumMediaItem, quoted ...*waE2E.ContextInfo) error {
	var q *waE2E.ContextInfo
	if len(quoted) > 0 {
		q = quoted[0]
	}
	return sendAlbumInternal(ctx, client, chat, items, q)
}

var Album = SendAlbum

func sendAlbumInternal(ctx context.Context, client *whatsmeow.Client, chat types.JID, items []AlbumMediaItem, quoted *waE2E.ContextInfo) error {
	if client == nil {
		return fmt.Errorf("client unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var validItems []AlbumMediaItem
	for _, it := range items {
		if len(it.Data) > 0 {
			validItems = append(validItems, it)
		}
	}

	if len(validItems) == 0 {
		return fmt.Errorf("no media items to send in album")
	}

	// Single item delivery does not require an album wrapper.
	if len(validItems) == 1 {
		it := validItems[0]
		if it.IsVideo {
			return SendVideo(ctx, client, chat, it.Data, it.Mimetype, it.Caption, quoted)
		}
		return SendImage(ctx, client, chat, it.Data, it.Mimetype, it.Caption, quoted)
	}

	type uploadedAlbumItem struct {
		item     AlbumMediaItem
		uploaded whatsmeow.UploadResponse
	}

	var uploadedItems []uploadedAlbumItem
	for _, it := range validItems {
		mediaType := whatsmeow.MediaImage
		if it.IsVideo {
			mediaType = whatsmeow.MediaVideo
		}
		up, err := client.Upload(ctx, it.Data, mediaType)
		if err != nil {
			defaultLogger.Errorf("sendAlbum: failed to upload media item (isVideo=%v, bytes=%d): %v", it.IsVideo, len(it.Data), err)
			continue
		}
		uploadedItems = append(uploadedItems, uploadedAlbumItem{
			item:     it,
			uploaded: up,
		})
	}

	if len(uploadedItems) == 0 {
		return fmt.Errorf("failed to upload any album media items")
	}

	// If only 1 item succeeded in uploading, send it directly.
	if len(uploadedItems) == 1 {
		up := uploadedItems[0]
		if up.item.IsVideo {
			return SendVideo(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
		}
		return SendImage(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
	}

	var imgCount, vidCount uint32
	for _, up := range uploadedItems {
		if up.item.IsVideo {
			vidCount++
		} else {
			imgCount++
		}
	}

	albumMsg := &waE2E.Message{
		AlbumMessage: &waE2E.AlbumMessage{
			ExpectedImageCount: &imgCount,
			ExpectedVideoCount: &vidCount,
			ContextInfo:        quoted,
		},
	}

	resp, err := client.SendMessage(ctx, chat, albumMsg)
	if err != nil {
		defaultLogger.Warnf("sendAlbum: failed to send AlbumMessage header, falling back to sequential delivery: %v", err)
		for i, up := range uploadedItems {
			var sendErr error
			if up.item.IsVideo {
				sendErr = SendVideo(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
			} else {
				sendErr = SendImage(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
			}
			if sendErr != nil {
				defaultLogger.Errorf("sendAlbum fallback: failed to send media item (index=%d): %v", i, sendErr)
			}
			if i < len(uploadedItems)-1 {
				time.Sleep(300 * time.Millisecond)
			}
		}
		return nil
	}

	parentKey := client.BuildMessageKey(chat, types.EmptyJID, resp.ID)
	assocType := waE2E.MessageAssociation_MEDIA_ALBUM

	for i, up := range uploadedItems {
		mimetype := up.item.Mimetype
		caption := up.item.Caption
		var captionPtr *string
		if caption != "" {
			captionPtr = &caption
		}

		childMsg := &waE2E.Message{
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageAssociation: &waE2E.MessageAssociation{
					AssociationType:  &assocType,
					ParentMessageKey: parentKey,
					MessageIndex:     new(int32(i)),
				},
			},
		}

		if up.item.IsVideo {
			if mimetype == "" {
				mimetype = "video/mp4"
			}
			childMsg.VideoMessage = &waE2E.VideoMessage{
				URL:           &up.uploaded.URL,
				DirectPath:    &up.uploaded.DirectPath,
				MediaKey:      up.uploaded.MediaKey,
				Mimetype:      &mimetype,
				FileEncSHA256: up.uploaded.FileEncSHA256,
				FileSHA256:    up.uploaded.FileSHA256,
				FileLength:    new(uint64(len(up.item.Data))),
				Caption:       captionPtr,
			}
		} else {
			if mimetype == "" {
				mimetype = "image/jpeg"
			}
			childMsg.ImageMessage = &waE2E.ImageMessage{
				URL:           &up.uploaded.URL,
				DirectPath:    &up.uploaded.DirectPath,
				MediaKey:      up.uploaded.MediaKey,
				Mimetype:      &mimetype,
				FileEncSHA256: up.uploaded.FileEncSHA256,
				FileSHA256:    up.uploaded.FileSHA256,
				FileLength:    new(uint64(len(up.item.Data))),
				Caption:       captionPtr,
			}
		}

		if _, sendErr := client.SendMessage(ctx, chat, childMsg); sendErr != nil {
			defaultLogger.Errorf("sendAlbum: failed to send child media item (index=%d): %v", i, sendErr)
		}

		if i < len(uploadedItems)-1 {
			time.Sleep(150 * time.Millisecond)
		}
	}

	return nil
}

// Media methods on *Sender
func (s *Sender) Image(data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendImage(s.getContext(), s.Client, s.Chat, data, mimetype, caption, quoted...)
}

func (s *Sender) Video(data []byte, mimetype, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendVideo(s.getContext(), s.Client, s.Chat, data, mimetype, caption, quoted...)
}

func (s *Sender) Audio(data []byte, mimetype string, quoted ...*waE2E.ContextInfo) error {
	return SendAudio(s.getContext(), s.Client, s.Chat, data, mimetype, quoted...)
}

func (s *Sender) Document(data []byte, mimetype, filename, caption string, quoted ...*waE2E.ContextInfo) error {
	return SendDocument(s.getContext(), s.Client, s.Chat, data, mimetype, filename, caption, quoted...)
}

func (s *Sender) Sticker(data []byte, pack, author string, quoted ...*waE2E.ContextInfo) error {
	return SendSticker(s.getContext(), s.Client, s.Chat, data, pack, author, quoted...)
}

func (s *Sender) Album(items []AlbumMediaItem, quoted ...*waE2E.ContextInfo) error {
	return SendAlbum(s.getContext(), s.Client, s.Chat, items, quoted...)
}
