package cliutils

import (
	"sync"
	"time"
	utils "whatsrook"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"fmt"
	_ "image/gif"
	_ "image/png"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tcolgate/mp3"
)

type RecentMessageEntry struct {
	ID           string
	Chat         types.JID
	Sender       types.JID
	PushName     string
	Text         string
	Timestamp    time.Time
	HasQuoted    bool
	QuotedID     string
	QuotedSender types.JID
	QuotedName   string
	QuotedText   string
}

const MaxRecentMessages = 1000

var (
	RecentMessagesMu sync.RWMutex
	RecentMessages   = make(map[string]RecentMessageEntry)
	RecentMsgOrder   []string
)

func RecordRecentMessage(evt *events.Message) {
	if evt == nil || evt.Message == nil || evt.Info.ID == "" {
		return
	}
	RecentMessagesMu.Lock()
	defer RecentMessagesMu.Unlock()

	sender := evt.Info.Sender
	pushName := evt.Info.PushName
	text := utils.ExtractMessageText(evt)

	entry := RecentMessageEntry{
		ID:        evt.Info.ID,
		Chat:      evt.Info.Chat,
		Sender:    sender,
		PushName:  pushName,
		Text:      text,
		Timestamp: evt.Info.Timestamp,
	}

	ci := utils.GetContextInfoFromProto(evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		entry.HasQuoted = true
		if ci.StanzaID != nil {
			entry.QuotedID = *ci.StanzaID
		}
		if ci.Participant != nil && *ci.Participant != "" {
			if pj, err := types.ParseJID(*ci.Participant); err == nil {
				entry.QuotedSender = pj
			}
		} else if ci.RemoteJID != nil && *ci.RemoteJID != "" {
			if pj, err := types.ParseJID(*ci.RemoteJID); err == nil {
				entry.QuotedSender = pj
			}
		}
		entry.QuotedText = utils.ExtractTextFromProto(ci.QuotedMessage)
	}

	if _, exists := RecentMessages[evt.Info.ID]; !exists {
		if len(RecentMsgOrder) >= MaxRecentMessages {
			oldest := RecentMsgOrder[0]
			RecentMsgOrder = RecentMsgOrder[1:]
			delete(RecentMessages, oldest)
		}
		RecentMsgOrder = append(RecentMsgOrder, evt.Info.ID)
	}
	RecentMessages[evt.Info.ID] = entry
}

func GetRecentMessage(id string) (RecentMessageEntry, bool) {
	RecentMessagesMu.RLock()
	defer RecentMessagesMu.RUnlock()
	entry, ok := RecentMessages[id]
	return entry, ok
}

// PrepareCallVideo converts video to audio (.mp3) and Annex-B H.264 video stream (.h264) via ffmpeg CLI.
func PrepareCallVideo(inputPath string) (string, string, error) {
	basePath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	mp3Path := basePath + ".mp3"
	h264Path := basePath + ".h264"

	// 1. Audio Extraction
	audioCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vn", "-ar", "16000", "-ac", "1", "-b:a", "64k", mp3Path)
	if out, err := audioCmd.CombinedOutput(); err != nil {
		log.Printf("[WARN] ffmpeg audio extraction failed for %s: %v (%s)", inputPath, err, string(out))
	}

	// 2. Video Transcode to Annex-B H.264 via ffmpeg CLI (Baseline profile, repeat SPS/PPS headers on all keyframes)
	videoCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-an", "-c:v", "libx264", "-profile:v", "baseline", "-level:v", "3.1", "-pix_fmt", "yuv420p", "-r", "15", "-g", "15", "-x264-params", "repeat-headers=1:keyint=15:min-keyint=15:scenecut=0", "-bsf:v", "h264_mp4toannexb", "-f", "h264", h264Path)
	if out, err := videoCmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ffmpeg video transcode failed for %s: %w (%s)", inputPath, err, string(out))
	}

	return mp3Path, h264Path, nil
}

// TranscodeToMP3 converts input audio to MP3 via ffmpeg CLI.
func TranscodeToMP3(inputPath string) (string, error) {
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".mp3"
	actualOut := outputPath
	if outputPath == inputPath {
		actualOut = inputPath + ".tmp.mp3"
	}

	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-ar", "16000", "-ac", "1", actualOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		if outputPath == inputPath {
			_ = os.Remove(actualOut)
		}
		return "", fmt.Errorf("ffmpeg transcode failed: %w (%s)", err, string(out))
	}

	if outputPath == inputPath {
		if err := os.Rename(actualOut, inputPath); err != nil {
			return "", fmt.Errorf("rename transcoded file: %w", err)
		}
	}

	return outputPath, nil
}

// AudioDuration calculates MP3 duration in pure Go by reading frame headers.
func AudioDuration(path string) (time.Duration, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open audio file: %w", err)
	}
	defer file.Close()

	var totalDuration float64
	decoder := mp3.NewDecoder(file)
	var frame mp3.Frame
	var skipped int

	for {
		if err := decoder.Decode(&frame, &skipped); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("decode mp3 frame: %w", err)
		}
		totalDuration += frame.Duration().Seconds()
	}

	return time.Duration(totalDuration * float64(time.Second)), nil
}

func annexBStartCodeLen(data []byte, i int) int {
	if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
		return 4
	}
	if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
		return 3
	}
	return 0
}

func SplitAnnexBAccessUnits(data []byte) [][]byte {
	var units [][]byte
	auStart := -1
	hasSliceInAU := false
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		naluType := data[naluStart] & 0x1f

		isSlice := naluType == 1 || naluType == 5
		isAUBoundary := naluType == 9 || naluType == 7 || (isSlice && hasSliceInAU)

		if isAUBoundary && auStart >= 0 && i > auStart {
			unit := data[auStart:i]
			if hasVideoPayload(unit) {
				units = append(units, unit)
			}
			auStart = i
			hasSliceInAU = isSlice
		} else {
			if auStart < 0 {
				auStart = i
			}
			if isSlice {
				hasSliceInAU = true
			}
		}
		i = naluStart + 1
	}
	if auStart >= 0 && auStart < len(data) {
		unit := data[auStart:]
		if hasVideoPayload(unit) {
			units = append(units, unit)
		}
	}
	return units
}

func hasVideoPayload(data []byte) bool {
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		naluType := data[naluStart] & 0x1f
		if naluType == 5 || naluType == 1 {
			return true
		}
		i = naluStart + 1
	}
	return false
}

func AnnexBHasIDR(data []byte) bool {
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		if data[naluStart]&0x1f == 5 {
			return true
		}
		i = naluStart + 1
	}
	return false
}

func ExtensionFor(mimetype string) string {
	mimetype = strings.ToLower(strings.TrimSpace(mimetype))
	if idx := strings.Index(mimetype, ";"); idx != -1 {
		mimetype = strings.TrimSpace(mimetype[:idx])
	}
	var ext string
	switch {
	case strings.Contains(mimetype, "mp4") || mimetype == "video/mp4" || mimetype == "video/m4v":
		ext = ".mp4"
	case strings.Contains(mimetype, "3gpp") || strings.Contains(mimetype, "3gp"):
		ext = ".3gp"
	case strings.Contains(mimetype, "webm"):
		ext = ".webm"
	case strings.Contains(mimetype, "quicktime") || strings.Contains(mimetype, "mov"):
		ext = ".mov"
	case strings.Contains(mimetype, "mkv") || strings.Contains(mimetype, "matroska"):
		ext = ".mkv"
	case strings.Contains(mimetype, "avi"):
		ext = ".avi"
	case strings.Contains(mimetype, "ogg") || strings.Contains(mimetype, "opus"):
		ext = ".ogg"
	case strings.Contains(mimetype, "mpeg") || strings.Contains(mimetype, "mp3"):
		ext = ".mp3"
	case strings.Contains(mimetype, "wav"):
		ext = ".wav"
	case strings.Contains(mimetype, "aac") || strings.Contains(mimetype, "m4a"):
		ext = ".m4a"
	case strings.Contains(mimetype, "webp"):
		ext = ".webp"
	case strings.Contains(mimetype, "jpeg") || strings.Contains(mimetype, "jpg"):
		ext = ".jpg"
	case strings.Contains(mimetype, "png"):
		ext = ".png"
	case strings.Contains(mimetype, "gif"):
		ext = ".gif"
	case strings.Contains(mimetype, "pdf"):
		ext = ".pdf"
	default:
		ext = ".bin"
	}
	log.Printf("[DEBUG] Mapped mimetype %q to extension %q", mimetype, ext)
	return ext
}
