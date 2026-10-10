package send

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook/util/logger"
)

// AlbumMediaItem represents a media payload item to be bundled in a WhatsApp album.
type AlbumMediaItem struct {
	Data     []byte
	Mimetype string
	Caption  string
	IsVideo  bool
}

// Album sends multiple media items grouped as a WhatsApp album.
func Album(ctx context.Context, client *whatsmeow.Client, chat types.JID, items []AlbumMediaItem, quoted ...*waE2E.ContextInfo) error {
	var q *waE2E.ContextInfo
	if len(quoted) > 0 {
		q = quoted[0]
	}
	return sendAlbumInternal(ctx, client, chat, items, q)
}

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
			return Video(ctx, client, chat, it.Data, it.Mimetype, it.Caption, quoted)
		}
		return Image(ctx, client, chat, it.Data, it.Mimetype, it.Caption, quoted)
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
			logger.Error("sendAlbum: failed to upload media item", "isVideo", it.IsVideo, "bytes", len(it.Data), "err", err)
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
			return Video(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
		}
		return Image(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
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
		logger.Warn("sendAlbum: failed to send AlbumMessage header, falling back to sequential delivery", "err", err)
		for i, up := range uploadedItems {
			var sendErr error
			if up.item.IsVideo {
				sendErr = Video(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
			} else {
				sendErr = Image(ctx, client, chat, up.item.Data, up.item.Mimetype, up.item.Caption, quoted)
			}
			if sendErr != nil {
				logger.Error("sendAlbum fallback: failed to send media item", "index", i, "err", sendErr)
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
			logger.Error("sendAlbum: failed to send child media item", "index", i, "err", sendErr)
		}

		if i < len(uploadedItems)-1 {
			time.Sleep(150 * time.Millisecond)
		}
	}

	return nil
}

// Album sends album on *Sender
func (s *Sender) Album(items []AlbumMediaItem, quoted ...*waE2E.ContextInfo) error {
	return Album(s.getContext(), s.Client, s.Chat, items, quoted...)
}
