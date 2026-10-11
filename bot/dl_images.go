package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/thruqe/whatsrook"
)

// ImageItem represents a single image to be fetched and delivered.
type ImageItem struct {
	Bytes   []byte
	URL     string
	Caption string
}

// cleanCaption strips emoji characters to preserve the no-emoji rule.
func cleanCaption(text string) string {
	text = whatsrook.RemoveEmojis(text)
	return strings.TrimSpace(text)
}

// trailingMediaURLRegex matches trailing media shortener URLs often appended to social posts.
var trailingMediaURLRegex = regexp.MustCompile(`(?i)(?:https?://(?:t\.co|bit\.ly|tinyurl\.com)/\S+\s*)+$`)

// stripTrailingMediaURLs removes trailing media shortener links from text.
func stripTrailingMediaURLs(text string) string {
	return strings.TrimSpace(trailingMediaURLRegex.ReplaceAllString(text, ""))
}

// cleanTopicAndContext formats a topic title and optional context description cleanly for WhatsApp captions.
func cleanTopicAndContext(rawTitle, rawContext string) string {
	title := cleanCaption(stripTrailingMediaURLs(rawTitle))
	contextText := cleanCaption(stripTrailingMediaURLs(rawContext))

	if title == "" && contextText == "" {
		return "*Media Download*"
	}

	// If title is empty, promote contextText to title
	if title == "" {
		title = contextText
		contextText = ""
	}

	// If contextText duplicates or starts with title, deduplicate it
	if contextText != "" {
		if strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(contextText)) {
			contextText = ""
		} else if strings.HasPrefix(strings.ToLower(contextText), strings.ToLower(title)) {
			rem := strings.TrimSpace(contextText[len(title):])
			rem = strings.TrimLeft(rem, " -:\n\r\t")
			contextText = rem
		}
	}

	// If no contextText, try splitting multi-line or long title into topic and context
	if contextText == "" {
		if idx := strings.Index(title, "\n"); idx != -1 {
			firstLine := strings.TrimSpace(title[:idx])
			rest := strings.TrimSpace(title[idx+1:])
			if firstLine != "" && rest != "" {
				title = firstLine
				contextText = rest
			}
		} else if len(title) > 120 {
			// Look for sentence terminator within first 120 chars
			splitIdx := -1
			for _, term := range []string{". ", "! ", "? ", ": "} {
				if i := strings.Index(title[:120], term); i != -1 {
					if splitIdx == -1 || i < splitIdx {
						splitIdx = i + len(term) - 1
					}
				}
			}
			if splitIdx > 20 {
				contextText = strings.TrimSpace(title[splitIdx+1:])
				title = strings.TrimSpace(title[:splitIdx+1])
			}
		}
	}

	// Limit context description length to avoid overflowing WhatsApp caption limits
	if len(contextText) > 350 {
		cut := 350
		if lastSpace := strings.LastIndex(contextText[:cut], " "); lastSpace > 250 {
			cut = lastSpace
		}
		contextText = strings.TrimRight(contextText[:cut], " .,;:-") + "..."
	}

	if contextText != "" {
		return fmt.Sprintf("*%s*\n\n%s", title, contextText)
	}
	return fmt.Sprintf("*%s*", title)
}

// downloadAndSendDirectImage downloads a direct image URL and delivers it.
func downloadAndSendDirectImage(ctx *Context, rawURL string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 30*time.Second)
	imgBytes, err := FetchBytes(timeoutCtx, rawURL)
	cancel()
	if err != nil || len(imgBytes) == 0 {
		return fmt.Errorf("failed fetching direct image: %w", err)
	}

	return sendImageItems(ctx, []ImageItem{{Bytes: imgBytes}}, "")
}

// downloadAndSendImages downloads and delivers single or multi-image galleries.
func downloadAndSendImages(ctx *Context, rawURL string, meta *MediaMeta) error {
	baseCaption := buildCaption(meta)

	// Case 1: Multiple entries already listed in meta (playlist/gallery)
	if len(meta.Entries) > 0 {
		var items []ImageItem
		for _, entry := range meta.Entries {
			entryURL := entry.URL
			if entryURL == "" {
				entryURL = entry.Thumbnail
			}
			if entryURL != "" {
				items = append(items, ImageItem{URL: entryURL})
			}
		}
		if len(items) > 0 {
			Debug("downloadAndSendImages: delivering multi-image gallery from meta entries", "count", len(items))
			return sendImageItems(ctx, items, baseCaption)
		}
	}

	// Case 2: Single direct image in meta.URL
	if meta.URL != "" && isDirectImage(meta.Ext) {
		timeoutCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 30*time.Second)
		imgBytes, err := FetchBytes(timeoutCtx, meta.URL)
		cancel()
		if err == nil && len(imgBytes) > 0 {
			return sendImageItems(ctx, []ImageItem{{Bytes: imgBytes}}, baseCaption)
		}
	}

	// Case 3: Download image(s) via yt-dlp
	cookiesPath, cleanup := getCookiesFilePath(ctx, rawURL)
	defer cleanup()

	nowNano := time.Now().UnixNano()
	tempOutTpl := filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_img_%d_%%(autonumber)03d.%%(ext)s", nowNano))
	defer func() {
		if matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_img_%d_*", nowNano))); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}()

	dlCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 90*time.Second)
	defer cancel()

	args := []string{
		"--no-warnings",
		"--max-downloads", "10",
		"-o", tempOutTpl,
	}
	if cookiesPath != "" {
		args = append(args, "--cookies", cookiesPath)
	}
	args = append(args, rawURL)

	title := safeMediaTitle(meta)
	out, progressMsgID, err := runYtdlpWithLiveProgress(ctx, dlCtx, "Images", title, args)
	if err != nil && !strings.Contains(string(out), "Maximum number of downloads reached") {
		return fmt.Errorf("image download failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_img_%d_*.*", nowNano)))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_img_%d_*", nowNano)))
	}
	if len(matches) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Failed:*\n_Title: `%s`_\n\n```\nDownloaded image file not found on disk\n```", title))
		}
		return fmt.Errorf("downloaded image file not found on disk")
	}

	sort.Strings(matches)
	var items []ImageItem
	for _, match := range matches {
		data, readErr := os.ReadFile(match)
		if readErr == nil && len(data) > 0 {
			items = append(items, ImageItem{Bytes: data})
		}
	}

	if len(items) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Processing Failed:*\n_Title: `%s`_\n\n```\nFailed to read downloaded image files\n```", title))
		}
		return fmt.Errorf("failed to read downloaded image files")
	}

	if progressMsgID != "" {
		_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Uploading Images...*\n_Title: `%s`_\n\n_Uploading gallery to WhatsApp..._", title))
	}

	sendErr := sendImageItems(ctx, items, baseCaption)
	if sendErr == nil && progressMsgID != "" {
		if _, delErr := ctx.Delete(progressMsgID); delErr != nil {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Complete!*\n_Title: `%s`_", title))
		}
	}
	return sendErr
}

// sendImageItems delivers a collection of images to the chat as an album or single image.
func sendImageItems(ctx *Context, items []ImageItem, baseCaption string) error {
	if len(items) == 0 {
		return fmt.Errorf("no images found to download")
	}

	type resolvedImage struct {
		bytes   []byte
		caption string
	}

	var resolved []resolvedImage
	for _, item := range items {
		imgBytes := item.Bytes
		if len(imgBytes) == 0 && item.URL != "" {
			timeoutCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 30*time.Second)
			fetched, err := FetchBytes(timeoutCtx, item.URL)
			cancel()
			if err != nil || len(fetched) == 0 {
				Warn("sendImageItems: failed to fetch image URL", "url", item.URL, "err", err)
				continue
			}
			imgBytes = fetched
		}
		if len(imgBytes) == 0 {
			continue
		}

		// Ensure JPEG compatibility for WhatsApp image rendering
		jpegData, errConv := EnsureJPEG(ctx.GetSendContext(), imgBytes)
		if errConv == nil && len(jpegData) > 0 {
			imgBytes = jpegData
		}

		resolved = append(resolved, resolvedImage{
			bytes:   imgBytes,
			caption: item.Caption,
		})
	}

	if len(resolved) == 0 {
		return fmt.Errorf("failed to fetch or process any image files")
	}

	// Single image delivery
	if len(resolved) == 1 {
		caption := resolved[0].caption
		if caption == "" {
			caption = baseCaption
		}
		return ctx.ReplyWithImage(resolved[0].bytes, "image/jpeg", caption)
	}

	// Multiple images: bundle into native WhatsApp album
	albumItems := make([]whatsrook.AlbumMediaItem, 0, len(resolved))
	for i, img := range resolved {
		caption := img.caption
		if i == 0 && caption == "" {
			caption = baseCaption
		}
		albumItems = append(albumItems, whatsrook.AlbumMediaItem{
			Data:     img.bytes,
			Mimetype: "image/jpeg",
			Caption:  caption,
			IsVideo:  false,
		})
	}

	Debug("sendImageItems: sending multi-image album", "count", len(albumItems))
	return ctx.ReplyWithAlbum(albumItems)
}
