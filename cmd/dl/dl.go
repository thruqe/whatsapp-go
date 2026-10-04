// Package dl implements an omni-media downloader plugin utilizing the yt-dlp
// execution environment, ffmpeg media transcoding pipelines, and interactive
// WhatsApp poll routing.
package dl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/builder"
	"whatsrook/util/logger"
	"whatsrook/util/media"
)

func init() {
	dispatch.Register(&dispatch.Command{
		Name:        "dl",
		Alias:       "download,ytdl,ytdlp,twitter,x,twt",
		Description: "Download video, audio, or pictures from any link",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleDL,
	})
}

// FormatMeta represents format-level metadata extracted by yt-dlp.
type FormatMeta struct {
	FormatID string `json:"format_id"`
	URL      string `json:"url"`
	Ext      string `json:"ext"`
	VCodec   string `json:"vcodec"`
	ACodec   string `json:"acodec"`
	AudioExt string `json:"audio_ext"`
	VideoExt string `json:"video_ext"`
}

// MediaMeta represents structured metadata extracted by yt-dlp.
type MediaMeta struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Extractor    string       `json:"extractor"`
	ExtractorKey string       `json:"extractor_key"`
	Duration     float64      `json:"duration"`
	Thumbnail    string       `json:"thumbnail"`
	Ext          string       `json:"ext"`
	URL          string       `json:"url"`
	VCodec       string       `json:"vcodec"`
	ACodec       string       `json:"acodec"`
	Type         string       `json:"_type"`
	Entries      []MediaMeta  `json:"entries"`
	Formats      []FormatMeta `json:"formats"`
}

func isVideoExt(ext string) bool {
	clean := strings.ToLower(strings.TrimPrefix(ext, "."))
	switch clean {
	case "mp4", "m4v", "webm", "mkv", "mov", "avi", "flv", "ts", "3gp", "wmv":
		return true
	}
	return false
}

func isAudioExt(ext string) bool {
	clean := strings.ToLower(strings.TrimPrefix(ext, "."))
	switch clean {
	case "mp3", "m4a", "ogg", "opus", "wav", "flac", "aac", "wma":
		return true
	}
	return false
}

// IsImage returns true if the extracted metadata indicates an image asset.
func (m *MediaMeta) IsImage() bool {
	if strings.EqualFold(m.Type, "image") {
		return true
	}
	cleanExt := strings.ToLower(strings.TrimPrefix(m.Ext, "."))
	switch cleanExt {
	case "jpg", "jpeg", "png", "webp", "bmp", "tiff", "heic", "avif":
		return true
	}
	if isVideoExt(cleanExt) || isAudioExt(cleanExt) {
		return false
	}
	if (m.VCodec == "none" || m.VCodec == "") && (m.ACodec == "none" || m.ACodec == "") && m.Duration == 0 {
		if cleanExt != "" && !isVideoExt(cleanExt) && !isAudioExt(cleanExt) {
			return true
		}
	}
	if len(m.Entries) > 0 {
		return m.Entries[0].IsImage()
	}
	return false
}

// IsGif returns true if the extracted metadata indicates an animated GIF / looping video without audio.
func (m *MediaMeta) IsGif() bool {
	cleanExt := strings.ToLower(strings.TrimPrefix(m.Ext, "."))
	if cleanExt == "gif" {
		return true
	}
	if strings.Contains(strings.ToLower(m.URL), "tweet_video") {
		return true
	}
	if strings.Contains(strings.ToLower(m.Thumbnail), "tweet_video_thumb") {
		return true
	}
	for _, f := range m.Formats {
		if strings.Contains(strings.ToLower(f.URL), "tweet_video") {
			return true
		}
	}
	if len(m.Entries) > 0 && m.Entries[0].IsGif() {
		return true
	}
	return false
}

// HasAudio reports whether the media metadata indicates an audio track.
func (m *MediaMeta) HasAudio() bool {
	if m.IsGif() {
		return false
	}
	if m.ACodec != "" && m.ACodec != "none" {
		return true
	}
	hasFormatWithAudio := false
	for _, f := range m.Formats {
		if f.ACodec != "" && f.ACodec != "none" {
			hasFormatWithAudio = true
			break
		}
		if f.AudioExt != "" && f.AudioExt != "none" {
			hasFormatWithAudio = true
			break
		}
	}
	if hasFormatWithAudio {
		return true
	}
	if len(m.Formats) > 0 {
		return false
	}
	return m.Duration > 0
}

// GetTitle returns a safe title fallback.
func (m *MediaMeta) GetTitle() string {
	t := strings.TrimSpace(m.Title)
	if t != "" {
		return t
	}
	if len(m.Entries) > 0 {
		if sub := strings.TrimSpace(m.Entries[0].Title); sub != "" {
			return sub
		}
	}
	if m.ID != "" {
		return m.ID
	}
	return "Media Download"
}

var urlRegex = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"]+`)

// extractTargetURL extracts the media URL from command arguments or quoted message text.
func extractTargetURL(ctx *dispatch.Context) (string, []string) {
	// 1. Check in command arguments
	var explicitArgs []string
	var targetURL string
	for _, arg := range ctx.Args {
		if targetURL == "" && (strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://")) {
			targetURL = arg
		} else {
			explicitArgs = append(explicitArgs, arg)
		}
	}

	if targetURL != "" {
		return cleanURL(targetURL), explicitArgs
	}

	// 2. Check full raw arguments string
	if match := urlRegex.FindString(ctx.RawArgs); match != "" {
		return cleanURL(match), explicitArgs
	}

	// 3. Check quoted message body / caption
	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		if text := quoted.GetConversation(); text != "" {
			if match := urlRegex.FindString(text); match != "" {
				return cleanURL(match), explicitArgs
			}
		}
		if ext := quoted.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
			if match := urlRegex.FindString(ext.GetText()); match != "" {
				return cleanURL(match), explicitArgs
			}
		}
		if img := quoted.GetImageMessage(); img != nil && img.GetCaption() != "" {
			if match := urlRegex.FindString(img.GetCaption()); match != "" {
				return cleanURL(match), explicitArgs
			}
		}
		if vid := quoted.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
			if match := urlRegex.FindString(vid.GetCaption()); match != "" {
				return cleanURL(match), explicitArgs
			}
		}
		if doc := quoted.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
			if match := urlRegex.FindString(doc.GetCaption()); match != "" {
				return cleanURL(match), explicitArgs
			}
		}
	}

	return "", explicitArgs
}

func cleanURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, ".,;!?>)]}")
	return raw
}

func handleDL(ctx *dispatch.Context) error {
	// Handle cookie subcommands: .dl cookie ...
	if len(ctx.Args) > 0 {
		first := strings.ToLower(ctx.Args[0])
		if first == "cookie" || first == "cookies" {
			return handleCookieCommand(ctx)
		}
	}

	targetURL, remainingArgs := extractTargetURL(ctx)
	if targetURL == "" {
		return sendUsage(ctx)
	}

	// Verify required external tools
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return ctx.Reply("yt-dlp executable was not found in system PATH. Please ensure yt-dlp is installed.")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ctx.Reply("ffmpeg executable was not found in system PATH. Please ensure ffmpeg is installed.")
	}

	// 1. If direct image URL, download and send directly without yt-dlp overhead
	if isImageURL(targetURL) {
		logger.Debug("handleDL: direct image URL detected", "url", targetURL)
		if err := downloadAndSendDirectImage(ctx, targetURL); err == nil {
			return nil
		}
		logger.Warn("handleDL: direct image fetch failed, falling back to yt-dlp", "url", targetURL)
	}

	// 2. Fetch metadata using yt-dlp -J
	meta, errMeta := fetchMetadata(ctx, targetURL)
	if errMeta != nil {
		logger.Error("handleDL: metadata fetch failed", "url", targetURL, "err", errMeta)
		return sendFailureWithCookiePrompt(ctx, errMeta)
	}

	// 3. Automatically detect if the downloadable is an image (single or multi-image)
	if meta.IsImage() {
		logger.Debug("handleDL: target identified as image", "url", targetURL, "ext", meta.Ext, "entries", len(meta.Entries))
		if err := downloadAndSendImages(ctx, targetURL, meta); err != nil {
			return sendFailureWithCookiePrompt(ctx, err)
		}
		return nil
	}

	// 3. Check for explicit format selection passed via argument: .dl <url> audio / .dl <url> video
	formatChoice := ""
	for _, arg := range remainingArgs {
		clean := strings.ToLower(strings.TrimSpace(arg))
		if clean == "audio" || clean == "mp3" || clean == "opus" || clean == "sound" || clean == "-a" {
			formatChoice = "audio"
			break
		}
		if clean == "video" || clean == "mp4" || clean == "-v" {
			formatChoice = "video"
			break
		}
	}

	if formatChoice == "audio" {
		if meta != nil && !meta.HasAudio() {
			return ctx.Reply("⚠️ This media (GIF/silent video) does not contain an audio track.")
		}
		if err := downloadAndSendAudio(ctx, targetURL, meta); err != nil {
			return sendFailureWithCookiePrompt(ctx, err)
		}
		return nil
	}
	if formatChoice == "video" {
		if err := downloadAndSendVideo(ctx, targetURL, meta); err != nil {
			return sendFailureWithCookiePrompt(ctx, err)
		}
		return nil
	}

	// 4. If media has no audio track or is an animated GIF, download directly as video/gif without poll
	if meta != nil && (!meta.HasAudio() || meta.IsGif()) {
		logger.Debug("handleDL: silent media/gif detected, skipping poll and downloading directly", "url", targetURL)
		if err := downloadAndSendVideo(ctx, targetURL, meta); err != nil {
			return sendFailureWithCookiePrompt(ctx, err)
		}
		return nil
	}

	// 5. Send interactive WhatsApp poll: Video / Audio
	qTitle := meta.GetTitle()
	if len(qTitle) > 100 {
		qTitle = qTitle[:97] + "..."
	}
	durStr := formatDurationSeconds(meta.Duration)

	var question string
	if durStr != "" {
		question = fmt.Sprintf("Download: %s\nDuration: %s\nChoose media format:", qTitle, durStr)
	} else {
		question = fmt.Sprintf("Download: %s\nChoose media format:", qTitle)
	}
	if len(question) > 240 {
		question = question[:240]
	}

	poll := ctx.Poll(question)
	poll.AddOption("Video")
	poll.AddOption("Audio")
	poll.AllowedSenders(ctx.Sender)
	poll.AutoDelete(true)

	err := poll.OnceReply(func(req builder.PollRequest, res *builder.Response) {
		selected := ""
		if len(req.SelectedOptions) > 0 {
			selected = req.SelectedOptions[0]
		}
		selectedLower := strings.ToLower(selected)

		if strings.Contains(selectedLower, "audio") {
			if meta != nil && !meta.HasAudio() {
				_ = ctx.Reply("⚠️ This media does not contain an audio track.")
				return
			}
			if err := downloadAndSendAudio(ctx, targetURL, meta); err != nil {
				sendFailureWithCookiePrompt(ctx, err)
			}
		} else {
			if err := downloadAndSendVideo(ctx, targetURL, meta); err != nil {
				sendFailureWithCookiePrompt(ctx, err)
			}
		}
	})

	if err != nil {
		logger.Error("handleDL: failed to send interactive poll, falling back to direct video", "err", err)
		return downloadAndSendVideo(ctx, targetURL, meta)
	}

	return nil
}

// fetchMetadata runs yt-dlp -J to retrieve JSON metadata for the media item.
func fetchMetadata(ctx *dispatch.Context, rawURL string) (*MediaMeta, error) {
	cookiesPath, cleanup := getCookiesFilePath(ctx, rawURL)
	defer cleanup()

	args := []string{"-J", "--no-warnings", "--no-playlist"}
	if cookiesPath != "" {
		args = append(args, "--cookies", cookiesPath)
	}
	args = append(args, rawURL)

	timeoutCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "yt-dlp", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp metadata dump error: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	var meta MediaMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal yt-dlp metadata failed: %w", err)
	}

	return &meta, nil
}

var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([a-zA-Z]|\x1b\][0-9];[^\a\x1b]*(?:\a|\x1b\\)`)

func safeMediaTitle(meta *MediaMeta) string {
	if meta == nil {
		return "Media Download"
	}
	t := strings.TrimSpace(meta.GetTitle())
	t = strings.ReplaceAll(t, "`", "'")
	if len(t) > 75 {
		t = strings.TrimSpace(t[:72]) + "..."
	}
	if t == "" {
		return "Media Download"
	}
	return t
}

func cleanYtdlpOutput(raw string) string {
	cleaned := ansiEscapeRegex.ReplaceAllString(raw, "")
	rawLines := strings.Split(cleaned, "\n")
	var lines []string
	for _, line := range rawLines {
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			var last string
			for _, part := range slices.Backward(parts) {
				t := strings.TrimRight(part, " \t")
				if t != "" {
					last = t
					break
				}
			}
			if last != "" {
				lines = append(lines, last)
			}
		} else {
			trimmed := strings.TrimRight(line, " \t")
			if trimmed != "" {
				lines = append(lines, trimmed)
			}
		}
	}
	if len(lines) == 0 {
		return "(starting download...)"
	}
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.Join(lines, "\n")
}

func formatBytes(bytes uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case bytes >= gib:
		return fmt.Sprintf("%.2fGiB", float64(bytes)/gib)
	case bytes >= mib:
		return fmt.Sprintf("%.2fMiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.2fKiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatSpeed(bytesPerSec float64) string {
	const (
		kib = 1024
		mib = 1024 * kib
	)
	switch {
	case bytesPerSec >= mib:
		return fmt.Sprintf("%.2fMiB/s", bytesPerSec/mib)
	case bytesPerSec >= kib:
		return fmt.Sprintf("%.2fKiB/s", bytesPerSec/kib)
	default:
		return fmt.Sprintf("%.0fB/s", bytesPerSec)
	}
}

func formatETA(sec float64) string {
	if sec <= 0 || sec > 86400 {
		return ""
	}
	total := int(sec)
	m := total / 60
	s := total % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatDuration(sec float64) string {
	if sec <= 0 {
		return "00:00"
	}
	total := int(math.Round(sec))
	m := total / 60
	s := total % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatTranscodeProgress(processedSec, totalSec, speed float64) string {
	var (
		speedStr string
		etaStr   string
	)
	if speed > 0 {
		speedStr = fmt.Sprintf("%.2fx", speed)
		if totalSec > processedSec {
			remaining := totalSec - processedSec
			etaSec := remaining / speed
			eta := formatETA(etaSec)
			if eta != "" {
				etaStr = "ETA " + eta
			}
		}
	}
	if speedStr == "" {
		speedStr = "Unknown speed"
	}
	if etaStr == "" {
		etaStr = "ETA Unknown"
	}

	if totalSec > 0 {
		pct := (processedSec / totalSec) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
		return fmt.Sprintf("[transcode] %5.1f%% of %s at %s %s", pct, formatDuration(totalSec), speedStr, etaStr)
	}

	return fmt.Sprintf("[transcode] %s processed at %s", formatDuration(processedSec), speedStr)
}

func probeMediaDuration(ctx context.Context, filePath string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", filePath)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	dur, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return dur
}

func makeTranscodeProgressCallback(
	ctx *dispatch.Context,
	progressMsgID string,
	mediaType string,
	title string,
) func(processedSec, totalSec, speed float64) {
	if progressMsgID == "" {
		return nil
	}
	var (
		lastEditTime time.Time
		transcodeMu  sync.Mutex
	)

	return func(processedSec, totalSec, speed float64) {
		transcodeMu.Lock()
		defer transcodeMu.Unlock()

		now := time.Now()
		isComplete := totalSec > 0 && processedSec >= totalSec
		if now.Sub(lastEditTime) < 1100*time.Millisecond && !isComplete {
			return
		}
		lastEditTime = now

		progressLine := formatTranscodeProgress(processedSec, totalSec, speed)
		updateText := fmt.Sprintf("*Transcoding %s...*\n_Title: `%s`_\n\n```\n%s\n```", mediaType, title, progressLine)
		_, _ = ctx.Edit(progressMsgID, updateText)
	}
}

func makeUploadProgressCallback(
	ctx *dispatch.Context,
	progressMsgID string,
	mediaType string,
	title string,
) func(uploaded, total uint64) {
	if progressMsgID == "" {
		return nil
	}
	var (
		lastEditTime time.Time
		uploadMu     sync.Mutex
		uploadStart  = time.Now()
	)

	return func(uploaded, total uint64) {
		if total == 0 {
			return
		}
		uploadMu.Lock()
		defer uploadMu.Unlock()

		now := time.Now()
		// Throttle edits to at most once per 1100ms, unless upload is complete
		if now.Sub(lastEditTime) < 1100*time.Millisecond && uploaded < total {
			return
		}
		lastEditTime = now

		pct := float64(uploaded) / float64(total) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
		elapsed := now.Sub(uploadStart).Seconds()
		var speedStr, etaStr string
		if elapsed > 0.2 && uploaded > 0 {
			speed := float64(uploaded) / elapsed
			speedStr = formatSpeed(speed)
			remaining := float64(total - uploaded)
			if speed > 0 && remaining > 0 {
				etaSec := remaining / speed
				etaStr = formatETA(etaSec)
			}
		}
		if speedStr == "" {
			speedStr = "Unknown B/s"
		}
		if etaStr == "" {
			etaStr = "ETA Unknown"
		} else {
			etaStr = "ETA " + etaStr
		}

		progressLine := fmt.Sprintf("[upload]  %5.1f%% of %s at %s %s", pct, formatBytes(total), speedStr, etaStr)
		updateText := fmt.Sprintf("*Uploading %s...*\n_Title: `%s`_\n\n```\n%s\n```", mediaType, title, progressLine)
		_, _ = ctx.Edit(progressMsgID, updateText)
	}
}

// runYtdlpWithLiveProgress executes yt-dlp while periodically editing a WhatsApp progress message.
func runYtdlpWithLiveProgress(
	ctx *dispatch.Context,
	dlCtx context.Context,
	mediaType string,
	title string,
	args []string,
) ([]byte, string, error) {
	initialMsg := fmt.Sprintf("*Downloading %s...*\n_Title: `%s`_\n\n```\n(starting yt-dlp...)\n```", mediaType, title)
	progressMsgID, err := ctx.ReplyWithID(initialMsg)
	if err != nil {
		logger.Warn("runYtdlpWithLiveProgress: failed to send initial progress message", "err", err)
	}

	var cmd *exec.Cmd
	if stdbufPath, errLook := exec.LookPath("stdbuf"); errLook == nil {
		fullArgs := append([]string{"-oL", "-eL", "yt-dlp"}, args...)
		cmd = exec.CommandContext(dlCtx, stdbufPath, fullArgs...)
	} else {
		cmd = exec.CommandContext(dlCtx, "yt-dlp", args...)
	}
	cmd.Env = append(os.Environ(),
		"PYTHONUNBUFFERED=1",
		"CI=1",
	)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Error:*\n_Title: `%s`_\n\n```\nFailed to open stdout pipe: %v\n```", title, err))
		}
		return nil, progressMsgID, err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Error:*\n_Title: `%s`_\n\n```\nFailed to open stderr pipe: %v\n```", title, err))
		}
		return nil, progressMsgID, err
	}

	if err := cmd.Start(); err != nil {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Error:*\n_Title: `%s`_\n\n```\nFailed to start yt-dlp: %v\n```", title, err))
		}
		return nil, progressMsgID, err
	}

	var (
		mu       sync.Mutex
		outBuf   bytes.Buffer
		updateCh = make(chan struct{}, 20)
		done     = make(chan struct{})
		readWg   sync.WaitGroup
	)

	readStream := func(r io.Reader) {
		defer readWg.Done()
		buf := make([]byte, 1024)
		for {
			n, rErr := r.Read(buf)
			if n > 0 {
				mu.Lock()
				outBuf.Write(buf[:n])
				mu.Unlock()

				select {
				case updateCh <- struct{}{}:
				default:
				}
			}
			if rErr != nil {
				return
			}
		}
	}

	readWg.Add(2)
	go readStream(stdoutPipe)
	go readStream(stderrPipe)

	if progressMsgID != "" {
		go func() {
			ticker := time.NewTicker(1200 * time.Millisecond)
			defer ticker.Stop()

			var lastEditedText string
			var lastEditTime time.Time

			doEdit := func() {
				mu.Lock()
				rawOutput := outBuf.String()
				mu.Unlock()

				cleaned := cleanYtdlpOutput(rawOutput)
				if cleaned != lastEditedText && time.Since(lastEditTime) >= 900*time.Millisecond {
					updateText := fmt.Sprintf("*Downloading %s...*\n_Title: `%s`_\n\n```\n%s\n```", mediaType, title, cleaned)
					_, _ = ctx.Edit(progressMsgID, updateText)
					lastEditedText = cleaned
					lastEditTime = time.Now()
				}
			}

			for {
				select {
				case <-done:
					return
				case <-updateCh:
					doEdit()
				case <-ticker.C:
					doEdit()
				}
			}
		}()
	}

	waitErr := cmd.Wait()
	readWg.Wait()
	close(done)

	mu.Lock()
	fullOutput := outBuf.Bytes()
	mu.Unlock()

	if waitErr != nil {
		if progressMsgID != "" {
			cleaned := cleanYtdlpOutput(string(fullOutput))
			if cleaned == "" || cleaned == "(starting download...)" {
				cleaned = waitErr.Error()
			}
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Failed:*\n_Title: `%s`_\n\n```\n%s\n```", title, cleaned))
		}
		return fullOutput, progressMsgID, waitErr
	}

	if progressMsgID != "" {
		_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Processing %s...*\n_Title: `%s`_\n\n_Transcoding media for WhatsApp compatibility..._", mediaType, title))
	}

	return fullOutput, progressMsgID, nil
}

// downloadAndSendAudio downloads the best audio track, transcodes it to Opus OGG, and sends it.
func downloadAndSendAudio(ctx *dispatch.Context, rawURL string, meta *MediaMeta) error {
	if meta != nil && !meta.HasAudio() {
		return fmt.Errorf("this media does not contain an audio track")
	}

	cookiesPath, cleanup := getCookiesFilePath(ctx, rawURL)
	defer cleanup()

	nowNano := time.Now().UnixNano()
	rawAudioTpl := filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_aud_raw_%d.%%(ext)s", nowNano))
	opusOut := filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_aud_out_%d.opus", nowNano))

	defer func() {
		if matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_aud_raw_%d.*", nowNano))); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
		_ = os.Remove(opusOut)
	}()

	dlCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 5*time.Minute)
	defer cancel()

	args := []string{
		"-f", "bestaudio/best",
		"--no-warnings",
		"--no-playlist",
		"--max-filesize", "90M",
		"-o", rawAudioTpl,
	}
	if cookiesPath != "" {
		args = append(args, "--cookies", cookiesPath)
	}
	args = append(args, rawURL)

	title := safeMediaTitle(meta)
	out, progressMsgID, err := runYtdlpWithLiveProgress(ctx, dlCtx, "Audio", title, args)
	if err != nil {
		return fmt.Errorf("audio download failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_aud_raw_%d.*", nowNano)))
	if len(matches) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Failed:*\n_Title: `%s`_\n\n```\nDownloaded audio file not found on disk\n```", title))
		}
		return fmt.Errorf("downloaded audio file not found on disk")
	}
	rawAudioFile := matches[0]

	// Determine media duration
	duration := 0.0
	if meta != nil && meta.Duration > 0 {
		duration = meta.Duration
	} else {
		duration = probeMediaDuration(dlCtx, rawAudioFile)
	}

	// Transcode audio to WhatsApp-compatible Opus OGG (48kHz, mono, VoIP tuned)
	transcodeCallback := makeTranscodeProgressCallback(ctx, progressMsgID, "Audio", title)
	if err := EnsureWhatsAppOpusWithProgress(ctx.GetSendContext(), rawAudioFile, opusOut, duration, transcodeCallback); err != nil {
		logger.Warn("EnsureWhatsAppOpus transcode failed, attempting OpusPTTConvert on raw bytes", "err", err)
		rawBytes, errRead := os.ReadFile(rawAudioFile)
		if errRead != nil {
			if progressMsgID != "" {
				_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Transcode Failed:*\n_Title: `%s`_\n\n```\n%v\n```", title, errRead))
			}
			return fmt.Errorf("read raw audio file failed: %w", errRead)
		}
		uploadCallback := makeUploadProgressCallback(ctx, progressMsgID, "Audio", title)
		sendErr := sendOpusAudioPayload(ctx, rawBytes, uploadCallback)
		if sendErr == nil && progressMsgID != "" {
			if _, delErr := ctx.Delete(progressMsgID); delErr != nil {
				_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Audio Sent!* (100%%)\n_Title: `%s`_", title))
			}
		}
		return sendErr
	}

	opusBytes, err := os.ReadFile(opusOut)
	if err != nil || len(opusBytes) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Audio Processing Failed:*\n_Title: `%s`_\n\n```\nFailed to read transcoded opus file\n```", title))
		}
		return fmt.Errorf("failed to read transcoded opus file: %w", err)
	}

	uploadCallback := makeUploadProgressCallback(ctx, progressMsgID, "Audio", title)
	sendErr := sendOpusAudioPayload(ctx, opusBytes, uploadCallback)
	if sendErr == nil && progressMsgID != "" {
		if _, delErr := ctx.Delete(progressMsgID); delErr != nil {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Audio Sent!* (100%%)\n_Title: `%s`_", title))
		}
	}
	return sendErr
}

// downloadAndSendVideo downloads video, converts it to guarantee WhatsApp compatibility, and sends with caption.
func downloadAndSendVideo(ctx *dispatch.Context, rawURL string, meta *MediaMeta) error {
	cookiesPath, cleanup := getCookiesFilePath(ctx, rawURL)
	defer cleanup()

	nowNano := time.Now().UnixNano()
	rawVideoTpl := filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_vid_raw_%d.%%(ext)s", nowNano))
	waVideoOut := filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_vid_wa_%d.mp4", nowNano))

	defer func() {
		if matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_vid_raw_%d.*", nowNano))); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
		_ = os.Remove(waVideoOut)
	}()

	dlCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 7*time.Minute)
	defer cancel()

	args := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best[ext=mp4]/best",
		"--no-warnings",
		"--no-playlist",
		"--max-filesize", "90M",
		"-o", rawVideoTpl,
	}
	if cookiesPath != "" {
		args = append(args, "--cookies", cookiesPath)
	}
	args = append(args, rawURL)

	isGif := meta != nil && meta.IsGif()
	mediaLabel := "Video"
	if isGif {
		mediaLabel = "GIF"
	}

	title := safeMediaTitle(meta)
	out, progressMsgID, err := runYtdlpWithLiveProgress(ctx, dlCtx, mediaLabel, title, args)
	if err != nil {
		return fmt.Errorf("%s download failed: %w (%s)", strings.ToLower(mediaLabel), err, strings.TrimSpace(string(out)))
	}

	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("ytdl_vid_raw_%d.*", nowNano)))
	if len(matches) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Failed:*\n_Title: `%s`_\n\n```\nDownloaded %s file not found on disk\n```", title, strings.ToLower(mediaLabel)))
		}
		return fmt.Errorf("downloaded %s file not found on disk", strings.ToLower(mediaLabel))
	}
	rawVideoFile := matches[0]

	// Determine media duration
	duration := 0.0
	if meta != nil && meta.Duration > 0 {
		duration = meta.Duration
	} else {
		duration = probeMediaDuration(dlCtx, rawVideoFile)
	}

	// Transcode / remux video to ensure WhatsApp H.264/AAC/yuv420p/+faststart compatibility
	transcodeCtx, cancelTranscode := context.WithTimeout(ctx.GetSendContext(), 4*time.Minute)
	defer cancelTranscode()

	transcodeCallback := makeTranscodeProgressCallback(ctx, progressMsgID, mediaLabel, title)
	transcodeErr := EnsureWhatsAppVideoWithProgress(transcodeCtx, rawVideoFile, waVideoOut, duration, transcodeCallback)
	targetVideoPath := waVideoOut
	if transcodeErr != nil {
		logger.Warn("EnsureWhatsAppVideo failed, falling back to original video file", "err", transcodeErr)
		targetVideoPath = rawVideoFile
	}

	videoBytes, err := os.ReadFile(targetVideoPath)
	if err != nil || len(videoBytes) == 0 {
		if progressMsgID != "" {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*%s Processing Failed:*\n_Title: `%s`_\n\n```\nFailed to read processed file\n```", mediaLabel, title))
		}
		return fmt.Errorf("failed to read processed video file: %w", err)
	}

	caption := buildCaption(meta)
	uploadCallback := makeUploadProgressCallback(ctx, progressMsgID, mediaLabel, title)
	var sendErr error
	if isGif {
		sendErr = ctx.ReplyWithVideoGifWithProgress(videoBytes, "video/mp4", caption, uploadCallback)
	} else {
		sendErr = ctx.ReplyWithVideoWithProgress(videoBytes, "video/mp4", caption, uploadCallback)
	}
	if sendErr == nil && progressMsgID != "" {
		if _, delErr := ctx.Delete(progressMsgID); delErr != nil {
			_, _ = ctx.Edit(progressMsgID, fmt.Sprintf("*Download Complete!*\n_Title: `%s`_", title))
		}
	}
	return sendErr
}

func runFFmpegWithProgress(
	ctx context.Context,
	totalDuration float64,
	onProgress func(processedSec, totalSec, speed float64),
	args ...string,
) error {
	fullArgs := make([]string, 0, len(args)+6)
	fullArgs = append(fullArgs, "-y", "-hide_banner", "-loglevel", "error")
	if onProgress != nil {
		fullArgs = append(fullArgs, "-progress", "pipe:1", "-nostats")
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, "ffmpeg", fullArgs...)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if onProgress == nil {
		if out, err := cmd.Output(); err != nil {
			errStr := strings.TrimSpace(errBuf.String())
			if errStr == "" {
				errStr = strings.TrimSpace(string(out))
			}
			return fmt.Errorf("ffmpeg transcode error: %w (%s)", err, errStr)
		}
		return nil
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe error: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start error: %w", err)
	}

	onProgress(0, totalDuration, 0)

	scanner := bufio.NewScanner(stdoutPipe)
	var (
		currentSec   float64
		currentSpeed float64
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "out_time_us":
			if us, pErr := strconv.ParseInt(val, 10, 64); pErr == nil {
				currentSec = float64(us) / 1000000.0
			}
		case "speed":
			cleanSpeed := strings.TrimSuffix(strings.TrimSpace(val), "x")
			if spd, pErr := strconv.ParseFloat(cleanSpeed, 64); pErr == nil {
				currentSpeed = spd
			}
		case "progress":
			if val == "end" {
				if totalDuration > 0 {
					currentSec = totalDuration
				}
				onProgress(currentSec, totalDuration, currentSpeed)
			} else {
				onProgress(currentSec, totalDuration, currentSpeed)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdout: %w", err)
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		errStr := strings.TrimSpace(errBuf.String())
		return fmt.Errorf("ffmpeg transcode error: %w (%s)", waitErr, errStr)
	}
	return nil
}

// EnsureWhatsAppVideo converts or remuxes a video file to meet WhatsApp's strict compatibility requirements:
// - MP4 container with faststart (moov atom at beginning)
// - H.264 (AVC) video codec, yuv420p pixel format, main profile, level 4.0
// - Even width & height dimensions
// - AAC audio codec, 44.1kHz or 48kHz stereo/mono
func EnsureWhatsAppVideo(ctx context.Context, inputPath, outputPath string) error {
	return EnsureWhatsAppVideoWithProgress(ctx, inputPath, outputPath, 0, nil)
}

// EnsureWhatsAppVideoWithProgress is EnsureWhatsAppVideo with transcoding progress estimation.
func EnsureWhatsAppVideoWithProgress(
	ctx context.Context,
	inputPath, outputPath string,
	totalDuration float64,
	onProgress func(processedSec, totalSec, speed float64),
) error {
	return runFFmpegWithProgress(ctx, totalDuration, onProgress,
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-profile:v", "main",
		"-level:v", "4.0",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		outputPath,
	)
}

// EnsureWhatsAppOpus converts an audio file into an OGG Opus stream suitable for WhatsApp.
func EnsureWhatsAppOpus(ctx context.Context, inputPath, outputPath string) error {
	return EnsureWhatsAppOpusWithProgress(ctx, inputPath, outputPath, 0, nil)
}

// EnsureWhatsAppOpusWithProgress is EnsureWhatsAppOpus with transcoding progress estimation.
func EnsureWhatsAppOpusWithProgress(
	ctx context.Context,
	inputPath, outputPath string,
	totalDuration float64,
	onProgress func(processedSec, totalSec, speed float64),
) error {
	return runFFmpegWithProgress(ctx, totalDuration, onProgress,
		"-i", inputPath,
		"-vn",
		"-c:a", "libopus",
		"-b:a", "48k",
		"-vbr", "on",
		"-compression_level", "10",
		"-application", "voip",
		"-ar", "48000",
		"-ac", "1",
		"-f", "ogg",
		outputPath,
	)
}

// sendOpusAudioPayload uploads and dispatches Opus audio with waveform & PTT metadata.
func sendOpusAudioPayload(ctx *dispatch.Context, audioBytes []byte, onProgress ...func(uploaded, total uint64)) error {
	meta, err := media.OpusPTTConvert(ctx.GetSendContext(), audioBytes)
	if err != nil || meta == nil || len(meta.Data) == 0 {
		meta = &media.AudioPTTMeta{
			Data: audioBytes,
		}
	}

	var prog func(uploaded, total uint64)
	if len(onProgress) > 0 {
		prog = onProgress[0]
	}

	uploaded, errUpload := ctx.Client.UploadWithProgress(ctx.GetSendContext(), meta.Data, whatsmeow.MediaAudio, prog)
	if errUpload != nil {
		// Fallback to standard context reply
		return ctx.ReplyWithAudio(meta.Data, "audio/ogg; codecs=opus")
	}

	ptt := true
	mimetype := "audio/ogg; codecs=opus"
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(meta.Data))),
			PTT:           &ptt,
			Seconds:       &meta.Seconds,
			Waveform:      meta.Waveform,
			ContextInfo:   ctx.ReplyContextInfo(),
		},
	}

	_, errSend := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, msg)
	return errSend
}

// buildCaption formats a clean caption with topic and description context for media messages.
func buildCaption(meta *MediaMeta) string {
	return cleanTopicAndContext(meta.GetTitle(), meta.Description)
}

func formatDurationSeconds(sec float64) string {
	if sec <= 0 {
		return ""
	}
	total := int(sec)
	m := total / 60
	s := total % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func isImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Path)
	return strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".jpeg") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".webp") ||
		strings.HasSuffix(path, ".gif") ||
		strings.HasSuffix(path, ".bmp")
}

func isDirectImage(ext string) bool {
	clean := strings.ToLower(strings.TrimPrefix(ext, "."))
	return clean == "jpg" || clean == "jpeg" || clean == "png" || clean == "webp"
}

func sendUsage(ctx *dispatch.Context) error {
	p := ctx.GetPrefix()
	tb := dispatch.NewText()
	tb.Line("*Omni-Media Downloader (yt-dlp)*").Blank()
	tb.Line("*Usage:*")
	tb.Linef("- %sdl <url>  (Fetches media and sends interactive Video/Audio poll)", p)
	tb.Linef("- %sdl <url> video  (Directly downloads video)", p)
	tb.Linef("- %sdl <url> audio  (Directly downloads audio as WhatsApp Opus)", p)
	tb.Linef("- Reply to any message containing a link with *%sdl*", p)
	tb.Blank()
	tb.Line("*Cookies Management:*")
	tb.Linef("- Reply to a `cookies.txt` document with *%sdl cookie*", p)
	tb.Linef("- %sdl cookie <cookie text>", p)
	tb.Linef("- %sdl cookies  (Check cookies status)", p)
	tb.Linef("- %sdl cookie clear  (Remove stored cookies)", p)
	tb.Blank()
	tb.Line("*Features:*")
	tb.Line("- Automatic image detection (Instagram, Reddit, X/Twitter, direct links)")
	tb.Line("- WhatsApp-compatible H.264 video with faststart streaming")
	tb.Line("- Voice note Opus OGG audio with interactive waveform")

	return ctx.Reply(tb.String())
}
