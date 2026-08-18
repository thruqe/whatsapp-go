package plugins

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"
)

func RecordRecentMessage(evt *events.Message) {
	cliutils.RecordRecentMessage(evt)
}

func GetRecentMessage(id string) (cliutils.RecentMessageEntry, bool) {
	return cliutils.GetRecentMessage(id)
}

func init() {
	Register(&Command{
		Name:        "sticker",
		Alias:       "s",
		Description: "Convert an image/video to a sticker. Optional pack metadata: sticker [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSticker,
	})
	Register(&Command{
		Name:        "circle",
		Description: "Convert an image/video to a circular sticker. Optional pack metadata: circle [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCircle,
	})
	Register(&Command{
		Name:        "crop",
		Description: "Convert an image/video to a square cropped sticker. Optional pack metadata: crop [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCrop,
	})
	Register(&Command{
		Name:        "steal",
		Alias:       "take",
		Description: "Steal/take a sticker and customize its metadata. Usage: reply to a sticker and optionally specify [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSteal,
	})
	Register(&Command{
		Name:        "mp4",
		Description: "Convert an animated sticker/video to MP4 format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP4,
	})
	Register(&Command{
		Name:        "mp3",
		Description: "Convert a video/audio to MP3 format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP3,
	})
	Register(&Command{
		Name:        "mp4url",
		Description: "Download video from direct URL and send as MP4",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP4URL,
	})
	Register(&Command{
		Name:        "black",
		Description: "Create a black video using the audio of a video/audio file",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleBlack,
	})
	Register(&Command{
		Name:        "trim",
		Description: "Trim a video. Usage: trim [start] [end] or trim [duration]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleTrim,
	})
	Register(&Command{
		Name:        "quote",
		Alias:       "q",
		Description: "Create a WhatsApp-style quote sticker from text or a replied message",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleQuote,
	})
}

func handleSticker(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	stickerData, err := processSticker(data, isVideo, packName, author, "")
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to process sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCircle(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	// apply transparent circle mask using ffmpeg's geq/alpha filter
	circleFilter := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0,geq=alpha_expr='if(lte(hypot(X-W/2,Y-H/2),W/2),255,0)'"
	stickerData, err := processSticker(data, isVideo, packName, author, circleFilter)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to process circular sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCrop(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	cropFilter := "crop='min(iw,ih)':'min(iw,ih)',scale=512:512"
	stickerData, err := processSticker(data, isVideo, packName, author, cropFilter)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to process cropped sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleMP4(ctx *Context) error {
	slog.Debug("handleMP4: fetching media for conversion", "chat", ctx.Chat.String(), "sender", ctx.Sender.String())
	data, mime, err := ctx.GetMedia()
	if err != nil {
		slog.Warn("handleMP4: no media found", "chat", ctx.Chat.String(), "err", err)
		return ctx.Reply("No media found in this message or the replied message.")
	}

	slog.Debug("handleMP4: starting conversion", "mime", mime, "size", len(data))
	mp4Data, err := processMP4(data, mime)
	if err != nil {
		slog.Error("handleMP4: processMP4 failed", "mime", mime, "size", len(data), "err", err)
		return ctx.Reply(fmt.Sprintf("⚠️ Failed to convert to MP4: %v", err))
	}

	slog.Debug("handleMP4: conversion successful, sending video", "outputSize", len(mp4Data))
	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleMP3(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	mp3Data, err := processMP3(data)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to convert to MP3: %v", err))
	}

	return ctx.ReplyWithAudio(mp3Data, "audio/ogg; codecs=opus")
}

func handleMP4URL(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Please provide a direct video URL.")
	}
	videoURL := ctx.Args[0]

	videoBytes, err := downloadFromURL(ctx.Ctx, videoURL)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to download video: %v", err))
	}

	mp4Data, err := processMP4(videoBytes, "video/mp4")
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to process video into MP4: %v", err))
	}

	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleBlack(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	blackData, err := processBlackVideo(data)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to create black video: %v", err))
	}

	return ctx.ReplyWithVideo(blackData, "video/mp4", "")
}

func parseStickerMetadata(ctx *Context, raw string) (string, string) {
	packName := ctx.GetBotName()
	author := "Thruqe"
	if raw != "" {
		parts := strings.Split(raw, "|")
		if len(parts) > 0 {
			author = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			packName = strings.TrimSpace(parts[1])
		}
	}
	return author, packName
}

func handleSteal(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("Please reply to a sticker message.")
	}

	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get sticker media: %v", err))
	}

	if !strings.Contains(mimetype, "webp") {
		return ctx.Reply("The replied message is not a valid sticker (WebP).")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)

	updatedData, err := utils.AddStickerMetadata(data, packName, author)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to update sticker metadata: %v", err))
	}

	return ctx.ReplyWithSticker(updatedData)
}

func processSticker(data []byte, isVideo bool, packName, author, filter string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_sticker_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.webp")

	if isVideo {
		type attempt struct {
			fps     int
			quality int
		}
		attempts := []attempt{
			{fps: 15, quality: 40},
			{fps: 12, quality: 30},
			{fps: 10, quality: 20},
			{fps: 7, quality: 10},
		}

		var lastErr error
		var finalData []byte

		for idx, att := range attempts {
			_ = os.Remove(tempOut)

			vf := fmt.Sprintf("fps=%d,format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0", att.fps)
			if filter != "" {
				vf = filter
				if strings.Contains(vf, "fps=") {
					vf = strings.ReplaceAll(vf, "fps=15", fmt.Sprintf("fps=%d", att.fps))
				} else {
					vf = fmt.Sprintf("fps=%d,", att.fps) + vf
				}
				if !strings.Contains(vf, "format=yuva420p") {
					vf = "format=yuva420p," + vf
				}
			}

			cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-t", "8", "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", fmt.Sprintf("%d", att.quality), "-compression_level", "6", "-loop", "0", "-preset", "default", "-an", "-vsync", "0", "-pix_fmt", "yuva420p", tempOut)
			if out, err := cmd.CombinedOutput(); err != nil {
				lastErr = fmt.Errorf("ffmpeg failed at attempt %d (fps=%d, q=%d): %w (output: %s)", idx, att.fps, att.quality, err, string(out))
				continue
			}

			finalPath, err := utils.WriteStickerMetadata(tempOut, packName, author)
			if err != nil {
				lastErr = fmt.Errorf("sticker metadata failed at attempt %d: %w", idx, err)
				continue
			}

			data, err := os.ReadFile(finalPath)
			_ = os.Remove(finalPath)
			if err != nil {
				lastErr = fmt.Errorf("read failed at attempt %d: %w", idx, err)
				continue
			}

			if len(data) <= 500*1024 {
				return data, nil
			}
			finalData = data
		}

		if finalData != nil {
			return finalData, nil
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed to process video sticker")
	}

	vf := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0"
	if filter != "" {
		vf = filter
		if !strings.Contains(vf, "format=yuva420p") {
			vf = "format=yuva420p," + vf
		}
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", "40", "-compression_level", "6", "-pix_fmt", "yuva420p", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(out))
	}

	finalPath, err := utils.WriteStickerMetadata(tempOut, packName, author)
	if err != nil {
		return nil, err
	}
	defer os.Remove(finalPath)

	return os.ReadFile(finalPath)
}

func stripWebPMetadataChunks(data []byte) []byte {
	if len(data) < 12 || !bytes.HasPrefix(data, []byte("RIFF")) || string(data[8:12]) != "WEBP" {
		return data
	}

	var buf bytes.Buffer
	buf.Write(data[:12])

	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(uint32(data[offset+4]) | uint32(data[offset+5])<<8 | uint32(data[offset+6])<<16 | uint32(data[offset+7])<<24)
		chunkTotal := 8 + chunkSize
		if chunkSize%2 != 0 {
			chunkTotal++ // RIFF padding byte
		}

		if offset+chunkTotal > len(data) {
			break
		}

		// Keep image & animation payload chunks (VP8, VP8L, VP8X, ANIM, ANMF, ALPH), strip EXIF/XMP/metadata
		if chunkID == "VP8 " || chunkID == "VP8L" || chunkID == "VP8X" || chunkID == "ANIM" || chunkID == "ANMF" || chunkID == "ALPH" {
			buf.Write(data[offset : offset+chunkTotal])
		}

		offset += chunkTotal
	}

	result := buf.Bytes()
	if len(result) > 12 {
		riffSize := uint32(len(result) - 8)
		result[4] = byte(riffSize)
		result[5] = byte(riffSize >> 8)
		result[6] = byte(riffSize >> 16)
		result[7] = byte(riffSize >> 24)
		return result
	}

	return data
}

func processMP4(data []byte, mime string) ([]byte, error) {
	slog.Debug("processMP4: starting conversion", "inputBytes", len(data), "mime", mime)
	tmpDir, err := os.MkdirTemp("", "whatsrook_mp4_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ext := utils.ExtensionFor(mime)
	if ext == ".bin" || ext == "" {
		if bytes.HasPrefix(data, []byte("RIFF")) && len(data) > 12 && string(data[8:12]) == "WEBP" {
			ext = ".webp"
		} else if bytes.HasPrefix(data, []byte("GIF8")) {
			ext = ".gif"
		} else {
			ext = ".webp"
		}
	}

	tempIn := filepath.Join(tmpDir, "input"+ext)
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp input file: %w", err)
	}

	tempClean := tempIn
	if ext == ".webp" {
		cleanPath := filepath.Join(tmpDir, "clean.webp")
		// 1. Strip EXIF/XMP sticker metadata with webpmux tool if available
		cmdMux := exec.Command("webpmux", "-strip", tempIn, "-o", cleanPath)
		if errMux := cmdMux.Run(); errMux == nil {
			if cleanBytes, errRead := os.ReadFile(cleanPath); errRead == nil && len(cleanBytes) > 0 {
				tempClean = cleanPath
				slog.Debug("processMP4: webpmux -strip successful", "cleanBytes", len(cleanBytes))
			}
		} else {
			// 2. Pure Go RIFF WebP metadata stripper fallback
			if stripped := stripWebPMetadataChunks(data); len(stripped) > 0 {
				_ = os.WriteFile(cleanPath, stripped, 0644)
				tempClean = cleanPath
				slog.Debug("processMP4: Go RIFF WebP metadata stripper applied", "cleanBytes", len(stripped))
			}
		}
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	// 15-second strict timeout context to prevent FFmpeg hanging or looping endlessly on low-end devices
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// High-efficiency single-pass FFmpeg conversion tuned for low-power ARM/Android/Termux devices
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-ignore_loop", "0", "-i", tempClean,
		"-vf", "scale=ceil(iw/2)*2:ceil(ih/2)*2,fps=15,format=yuv420p",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "26", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", "-threads", "2", "-t", "5", tempOut)

	out, err := cmd.CombinedOutput()
	if err == nil {
		res, errRead := os.ReadFile(tempOut)
		if errRead == nil && len(res) > 0 {
			slog.Debug("processMP4: ultrafast ffmpeg conversion successful", "outputBytes", len(res))
			return res, nil
		}
	}

	slog.Debug("processMP4: single-pass failed, attempting dwebp PNG extraction fallback", "err", err, "out", string(out))

	// Try dwebp (Google official WebP decoder) to extract clean PNG frame for static stickers
	if ext == ".webp" {
		tempPng := filepath.Join(tmpDir, "frame.png")
		cmdDwebp := exec.Command("dwebp", tempClean, "-o", tempPng)
		if errDwebp := cmdDwebp.Run(); errDwebp == nil {
			cmdPngLoop := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-loop", "1", "-i", tempPng,
				"-vf", "scale=ceil(iw/2)*2:ceil(ih/2)*2,fps=15,format=yuv420p",
				"-c:v", "libx264", "-preset", "ultrafast", "-crf", "26", "-pix_fmt", "yuv420p",
				"-t", "3", "-movflags", "+faststart", "-threads", "2", tempOut)
			if errPng := cmdPngLoop.Run(); errPng == nil {
				res, errRead := os.ReadFile(tempOut)
				if errRead == nil && len(res) > 0 {
					slog.Debug("processMP4: dwebp -> png -> ffmpeg mp4 conversion successful", "outputBytes", len(res))
					return res, nil
				}
			}
		}
	}

	// Fallback for purely static 1-frame WebP / PNG / JPG images: loop for 3 seconds
	ctxLoop, cancelLoop := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelLoop()

	cmdLoop := exec.CommandContext(ctxLoop, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-loop", "1", "-i", tempClean,
		"-vf", "scale=ceil(iw/2)*2:ceil(ih/2)*2,fps=15,format=yuv420p",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "26", "-pix_fmt", "yuv420p",
		"-t", "3", "-movflags", "+faststart", "-threads", "2", tempOut)

	outLoop, errLoop := cmdLoop.CombinedOutput()
	if errLoop == nil {
		res, errRead := os.ReadFile(tempOut)
		if errRead == nil && len(res) > 0 {
			slog.Debug("processMP4: static loop conversion successful", "outputBytes", len(res))
			return res, nil
		}
	}

	slog.Error("processMP4: all ffmpeg conversion levels failed", "err", errLoop, "ffmpegOutput", string(outLoop))
	return nil, fmt.Errorf("ffmpeg mp4 conversion failed: %w (output: %s)", errLoop, string(outLoop))
}

func processMP3(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_mp3_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.opus")

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-c:a", "libopus", "-b:a", "32k", "-application", "voip", "-f", "ogg", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg opus failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func downloadFromURL(ctx context.Context, mediaURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func processBlackVideo(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_black_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=640x360:d=600", "-i", tempIn, "-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-tune", "stillimage", "-c:a", "aac", "-pix_fmt", "yuv420p", "-shortest", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg black failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func handleTrim(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: trim [start] [end] (e.g. trim 00:00:02 00:00:10) or trim [duration] (e.g. trim 10)")
	}

	start := "00:00:00"
	end := ctx.Args[0]
	if len(ctx.Args) > 1 {
		start = ctx.Args[0]
		end = ctx.Args[1]
	}

	_ = ctx.Reply(fmt.Sprintf(" Trimming video from %s to %s...", start, end))
	trimmedData, err := processTrim(data, start, end)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to trim video: %v", err))
	}

	return ctx.ReplyWithVideo(trimmedData, "video/mp4", "")
}

func processTrim(data []byte, start, end string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_trim_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-ss", start, "-to", end, "-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg trim failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func handleQuote(ctx *Context) error {
	var msg cliutils.QuoteMessage
	scheme := cliutils.DefaultQuoteScheme()

	quotedMsg := ctx.GetQuotedMessage()
	quotedSenderJID, hasQuotedSender := ctx.GetQuotedSender()

	slog.Info("handleQuote: Processing quote request",
		"args_len", len(ctx.Args),
		"has_quoted_msg", quotedMsg != nil,
		"has_quoted_sender", hasQuotedSender,
		"quoted_sender_jid", quotedSenderJID.String(),
		"chat_jid", ctx.Chat.String(),
		"sender_jid", ctx.Sender.String(),
	)

	senderPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, ctx.Sender)
	senderPushName := resolveUserPushName(ctx, senderPNJID, ctx.Sender)

	if !ctx.Evt.Info.Timestamp.IsZero() {
		msg.Timestamp = ctx.Evt.Info.Timestamp.Format("15:04")
	} else {
		msg.Timestamp = time.Now().Format("15:04")
	}

	if quotedMsg != nil && hasQuotedSender {
		repliedText := utils.ExtractTextFromProto(quotedMsg)
		slog.Info("handleQuote: Quoted message found", "replied_text", repliedText)

		if repliedText == "" {
			return ctx.Reply("The replied message contains no text or extractable caption.")
		}

		rawTargetJID := quotedSenderJID

		// Check message secrets in database store for accurate sender verification
		quotedStanzaID := ""
		if ci := ctx.GetContextInfo(); ci != nil && ci.StanzaID != nil {
			quotedStanzaID = *ci.StanzaID
		}

		if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.MsgSecrets != nil && quotedStanzaID != "" {
			if secret, realSender, err := ctx.Client.Store.MsgSecrets.GetMessageSecret(ctx.Ctx, ctx.Chat, rawTargetJID, types.MessageID(quotedStanzaID)); err == nil && len(secret) > 0 {
				slog.Info("handleQuote: Found message secret in store", "stanza_id", quotedStanzaID, "real_sender", realSender.String())
				if !realSender.IsEmpty() {
					rawTargetJID = realSender
				}
			}
		}

		targetPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, rawTargetJID)
		targetPushName := resolveUserPushName(ctx, targetPNJID, rawTargetJID)

		if len(ctx.Args) > 0 {
			// Case 1: User replied to a message WITH custom comment/text.
			// The outer bubble is the sender; the inner quoted box is the replied message.
			msg.Username = senderPushName
			msg.UserPhone = senderPNJID.User
			msg.Content = strings.TrimSpace(ctx.RawArgs)
			msg.AvatarPath = fetchAvatarPath(ctx, senderPNJID, ctx.Sender)
			msg.Quoted = true
			msg.QuotedName = targetPushName
			msg.QuotedText = repliedText
		} else {
			// Case 2: User replied to a message WITHOUT text (pure .quote).
			// The outer bubble is the author of the replied message.
			msg.Username = targetPushName
			msg.UserPhone = targetPNJID.User
			msg.Content = repliedText
			msg.AvatarPath = fetchAvatarPath(ctx, targetPNJID, rawTargetJID)

			// 1. Check in-memory recent messages cache for nested quote
			if quotedStanzaID != "" {
				if cached, ok := GetRecentMessage(quotedStanzaID); ok {
					if cached.Text != "" {
						msg.Content = cached.Text
					}
					if cached.HasQuoted && cached.QuotedText != "" {
						msg.Quoted = true
						msg.QuotedText = cached.QuotedText
						nestedPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, cached.QuotedSender)
						msg.QuotedName = resolveUserPushName(ctx, nestedPNJID, cached.QuotedSender)
					}
				}
			}

			// 2. Fallback: check inner protobuf context & message secrets if cache did not have nested quote
			if !msg.Quoted {
				if innerCI := utils.GetContextInfoFromProto(quotedMsg); innerCI != nil && innerCI.QuotedMessage != nil {
					nestedText := utils.ExtractTextFromProto(innerCI.QuotedMessage)
					slog.Info("handleQuote: Inner nested quote checked", "nested_text", nestedText)

					if nestedText != "" {
						msg.Quoted = true
						msg.QuotedText = nestedText
						nestedPushName := "Quoted User"
						var nestedRawJID types.JID
						if innerCI.Participant != nil && *innerCI.Participant != "" {
							if pj, err := types.ParseJID(*innerCI.Participant); err == nil {
								nestedRawJID = pj
							}
						} else if innerCI.RemoteJID != nil && *innerCI.RemoteJID != "" {
							if pj, err := types.ParseJID(*innerCI.RemoteJID); err == nil {
								nestedRawJID = pj
							}
						}

						if innerCI.StanzaID != nil && *innerCI.StanzaID != "" {
							nestedStanzaID := *innerCI.StanzaID
							if nestedCached, ok := GetRecentMessage(nestedStanzaID); ok {
								if nestedCached.Text != "" {
									msg.QuotedText = nestedCached.Text
								}
								if !nestedCached.Sender.IsEmpty() {
									nestedRawJID = nestedCached.Sender
								}
							} else if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.MsgSecrets != nil {
								if secret, realSender, err := ctx.Client.Store.MsgSecrets.GetMessageSecret(ctx.Ctx, ctx.Chat, nestedRawJID, types.MessageID(nestedStanzaID)); err == nil && len(secret) > 0 {
									slog.Info("handleQuote: Found nested message secret in store", "nested_stanza_id", nestedStanzaID, "real_sender", realSender.String())
									if !realSender.IsEmpty() {
										nestedRawJID = realSender
									}
								}
							}
						}

						if !nestedRawJID.IsEmpty() {
							nestedPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, nestedRawJID)
							nestedPushName = resolveUserPushName(ctx, nestedPNJID, nestedRawJID)
						}
						msg.QuotedName = nestedPushName
					}
				}
			}
		}

	} else if len(ctx.Args) > 0 {
		// Case 3: Standalone .quote <text> without replying.
		msg.Username = senderPushName
		msg.UserPhone = senderPNJID.User
		msg.Content = strings.TrimSpace(ctx.RawArgs)
		msg.AvatarPath = fetchAvatarPath(ctx, senderPNJID, ctx.Sender)
		msg.Quoted = false
	} else {
		return ctx.Reply("Usage: `.quote <text>` or reply to a message with `.quote [comment]`.")
	}

	img, err := cliutils.RenderQuote(ctx.Ctx, msg, scheme)
	if err != nil {
		slog.Error("handleQuote: RenderQuote failed", "err", err)
		return ctx.Reply("Failed to render quote image: " + err.Error())
	}

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return ctx.Reply("Failed to encode image: " + err.Error())
	}

	packName := ctx.GetBotName()
	author := "WhatsRook Quote"
	stickerData, err := processSticker(buf.Bytes(), false, packName, author, "")
	if err != nil {
		slog.Error("handleQuote: processSticker failed", "err", err)
		return ctx.Reply("Failed to convert quote image to sticker: " + err.Error())
	}

	return ctx.ReplyWithSticker(stickerData)
}

func resolveUserPushName(ctx *Context, pnjid types.JID, rawJID types.JID) string {
	if !rawJID.IsEmpty() && ctx.Evt != nil && ctx.Evt.Info.Sender.ToNonAD().User == rawJID.ToNonAD().User && ctx.Evt.Info.PushName != "" {
		return ctx.Evt.Info.PushName
	}

	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, pnjid); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			}
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.BusinessName != "" {
				return contact.BusinessName
			}
		}
		// Fallback to raw LID contact lookup if PN contact was not found
		if rawJID != pnjid && !rawJID.IsEmpty() {
			if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, rawJID); err == nil && contact.Found {
				if contact.PushName != "" {
					return contact.PushName
				}
				if contact.FullName != "" {
					return contact.FullName
				}
				if contact.BusinessName != "" {
					return contact.BusinessName
				}
			}
		}
	}

	if pnjid.User != "" {
		return pnjid.User
	}
	return "User"
}

func fetchAvatarPath(ctx *Context, pnjid types.JID, rawJID types.JID) string {
	if ctx.Client == nil {
		slog.Warn("fetchAvatarPath: client is nil")
		return ""
	}

	queryJIDs := []types.JID{pnjid}
	if !rawJID.IsEmpty() && rawJID != pnjid {
		queryJIDs = append(queryJIDs, rawJID)
	}

	for _, target := range queryJIDs {
		slog.Info("fetchAvatarPath: Requesting profile picture info", "target_jid", target.String())
		ppInfo, err := ctx.Client.GetProfilePictureInfo(ctx.Ctx, target, &whatsmeow.GetProfilePictureParams{})
		if err != nil {
			slog.Warn("fetchAvatarPath: GetProfilePictureInfo error", "target_jid", target.String(), "err", err)
			continue
		}
		if ppInfo == nil || ppInfo.URL == "" {
			slog.Warn("fetchAvatarPath: GetProfilePictureInfo returned empty URL", "target_jid", target.String())
			continue
		}

		slog.Info("fetchAvatarPath: Profile picture URL fetched successfully", "target_jid", target.String(), "url", ppInfo.URL)
		imgData, errDownload := utils.FetchURLBytes(ctx.Ctx, ppInfo.URL)
		if errDownload != nil || len(imgData) == 0 {
			slog.Warn("fetchAvatarPath: Downloading avatar bytes failed", "err", errDownload, "len", len(imgData))
			continue
		}

		tmpFile, errTmp := os.CreateTemp("", "quote_avatar_*.jpg")
		if errTmp != nil {
			slog.Error("fetchAvatarPath: CreateTemp failed", "err", errTmp)
			continue
		}
		_, _ = tmpFile.Write(imgData)
		_ = tmpFile.Close()

		slog.Info("fetchAvatarPath: Avatar downloaded and saved to disk", "path", tmpFile.Name())
		return tmpFile.Name()
	}

	return ""
}
