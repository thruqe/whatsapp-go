package info

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	utils "whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	AliveTemplateKey  = "alive_template"
	AliveMediaKey     = "alive_media"
	AliveMediaTypeKey = "alive_media_type"
	AliveMediaMimeKey = "alive_media_mime"
	AliveMediaFileKey = "alive_media_file"

	DefaultAliveTpl      = "@user I am alive\n\nuse {prefix}alive customize to see how alive message can be customize"
	DefaultAliveTemplate = DefaultAliveTpl
)

var (
	StartTime = time.Now()

	MenuThumbPromptsMu      sync.RWMutex
	PendingMenuThumbPrompts = make(map[string]time.Time)
)

func extractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
		return *msg.ImageMessage.Caption
	}
	if msg.VideoMessage != nil && msg.VideoMessage.Caption != nil {
		return *msg.VideoMessage.Caption
	}
	if msg.DocumentMessage != nil && msg.DocumentMessage.Caption != nil {
		return *msg.DocumentMessage.Caption
	}
	return ""
}

func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}
	msg := utils.UnwrapMessageProto(evt.Message)
	if msg == nil {
		return nil, false, ""
	}
	if img := msg.GetImageMessage(); img != nil {
		return img, false, img.GetMimetype()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, true, vid.GetMimetype()
	}
	return nil, false, ""
}

func GetSessionAuthDir(client *whatsmeow.Client) string {
	baseDir := "media"
	if client != nil && client.Store != nil && client.Store.ID != nil && client.Store.ID.User != "" {
		baseDir = filepath.Join("sessions", client.Store.ID.User)
	}
	return baseDir
}

func ProcessAndSaveThumbnail(ctx context.Context, dir string, data []byte, isVideo bool) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	thumbPath := filepath.Join(dir, "menu_thumb.jpg")
	if isVideo {
		tmpIn := filepath.Join(dir, "tmp_menu.mp4")
		if err := os.WriteFile(tmpIn, data, 0644); err != nil {
			return "", err
		}
		defer os.Remove(tmpIn)
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn, "-ss", "00:00:01", "-vframes", "1", "-vf", "scale=640:360:force_original_aspect_ratio=decrease", thumbPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("ffmpeg thumbnail extraction failed: %v (%s)", err, string(out))
		}
		return thumbPath, nil
	}
	if err := os.WriteFile(thumbPath, data, 0644); err != nil {
		return "", err
	}
	return thumbPath, nil
}
