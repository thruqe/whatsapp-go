package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tcolgate/mp3"
)

// EnsureJPEG is an alias for JPEGConvert for backward compatibility.
func EnsureJPEG(ctx context.Context, inputBytes []byte) ([]byte, error) {
	return JPEGConvert(ctx, inputBytes)
}

// JPEGConvert decodes input image bytes (JPEG, PNG, GIF, WebP, etc.) and re-encodes to valid JPEG bytes.
func JPEGConvert(ctx context.Context, inputBytes []byte) ([]byte, error) {
	if len(inputBytes) == 0 {
		return inputBytes, nil
	}

	img, _, err := image.Decode(bytes.NewReader(inputBytes))
	if err == nil {
		var buf bytes.Buffer
		if errEnc := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); errEnc == nil && buf.Len() > 0 {
			return buf.Bytes(), nil
		}
	}

	// Fallback via ffmpeg for WebP / HEIC / MP4 / etc.
	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("pfp_in_%d.bin", time.Now().UnixNano()))
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("pfp_out_%d.jpg", time.Now().UnixNano()))

	if err := os.WriteFile(tmpIn, inputBytes, 0644); err != nil {
		return inputBytes, nil
	}
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-i", tmpIn, "-frames:v", "1", "-q:v", "2", tmpOut)
	if err := cmd.Run(); err != nil {
		return inputBytes, nil
	}

	jpgBytes, err := os.ReadFile(tmpOut)
	if err != nil || len(jpgBytes) == 0 {
		return inputBytes, nil
	}

	return jpgBytes, nil
}

// AudioPTTMeta contains converted Opus OGG data, duration in seconds, and 64-bin amplitude waveform bytes.
type AudioPTTMeta struct {
	Data      []byte
	Seconds   uint32
	Waveform  []byte
	Converted bool
}

// OpusPTTConvert converts audio bytes (MP3, WAV, AAC, M4A, etc.) to WhatsApp-compatible Opus OGG format using ffmpeg,
// and extracts duration in seconds and 64-bin normalized amplitude waveform bytes.
func OpusPTTConvert(ctx context.Context, audioBytes []byte) (*AudioPTTMeta, error) {
	if len(audioBytes) == 0 {
		return &AudioPTTMeta{Data: audioBytes, Converted: false}, nil
	}

	tempDir := os.TempDir()
	nowNano := time.Now().UnixNano()
	tempIn := filepath.Join(tempDir, fmt.Sprintf("audio_in_%d.tmp", nowNano))
	tempOut := filepath.Join(tempDir, fmt.Sprintf("audio_out_%d.opus", nowNano))
	tempPcm := filepath.Join(tempDir, fmt.Sprintf("audio_pcm_%d.raw", nowNano))

	if err := os.WriteFile(tempIn, audioBytes, 0644); err != nil {
		return &AudioPTTMeta{Data: audioBytes, Converted: false}, fmt.Errorf("failed to write temp audio file: %w", err)
	}
	defer os.Remove(tempIn)
	defer os.Remove(tempOut)
	defer os.Remove(tempPcm)

	// 1. Transcode to Ogg Opus suitable for WhatsApp PTT voice notes
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", tempIn,
		"-c:a", "libopus",
		"-b:a", "32k",
		"-vbr", "on",
		"-compression_level", "10",
		"-application", "voip",
		"-ar", "48000",
		"-ac", "1",
		"-f", "ogg",
		tempOut,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[WARN] EnsureOpusPTT: ffmpeg conversion failed: %v (%s)", err, string(out))
		return &AudioPTTMeta{Data: audioBytes, Converted: false}, nil
	}

	converted, err := os.ReadFile(tempOut)
	if err != nil || len(converted) == 0 {
		return &AudioPTTMeta{Data: audioBytes, Converted: false}, nil
	}

	meta := &AudioPTTMeta{
		Data:      converted,
		Converted: true,
	}

	// 2. Decode raw PCM to compute seconds & waveform samples matching WhatsApp waveform spec
	cmdPcm := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-i", tempOut, "-ac", "1", "-ar", "8000", "-f", "s16le", tempPcm)
	if errPcm := cmdPcm.Run(); errPcm == nil {
		pcmBytes, rErr := os.ReadFile(tempPcm)
		if rErr == nil && len(pcmBytes) >= 2 {
			meta.Seconds, meta.Waveform = extractWaveformAndDuration(pcmBytes, 8000)
		}
	}

	return meta, nil
}

// OpusPTT is an alias for OpusPTTConvert for concise caller ergonomics.
func OpusPTT(ctx context.Context, audioBytes []byte) (*AudioPTTMeta, error) {
	return OpusPTTConvert(ctx, audioBytes)
}

// EnsureOpusPTT is an alias for OpusPTTConvert.
func EnsureOpusPTT(ctx context.Context, audioBytes []byte) (*AudioPTTMeta, error) {
	return OpusPTTConvert(ctx, audioBytes)
}

// ExtractWaveformForTest exposes waveform extraction for test validation.
func ExtractWaveformForTest(pcmBytes []byte, sampleRate int) (uint32, []byte) {
	return extractWaveformAndDuration(pcmBytes, sampleRate)
}

func extractWaveformAndDuration(pcmBytes []byte, sampleRate int) (uint32, []byte) {
	numSamples := len(pcmBytes) / 2
	if numSamples == 0 {
		return 0, make([]byte, 64)
	}

	seconds := uint32(math.Round(float64(numSamples) / float64(sampleRate)))
	if seconds == 0 {
		seconds = 1
	}

	const numBins = 64
	type binData struct {
		sum   float64
		count uint32
	}
	bins := make([]binData, numBins)

	const scaleS16 = 1.0 / 32768.0
	numSamplesU64 := uint64(numSamples)

	for i := range numSamples {
		sampleVal := int16(uint16(pcmBytes[i*2]) | uint16(pcmBytes[i*2+1])<<8)
		sampleAbs := math.Abs(float64(sampleVal) * scaleS16)

		binIdx := int((uint64(i) * numBins) / numSamplesU64)
		if binIdx >= numBins {
			binIdx = numBins - 1
		}
		bins[binIdx].sum += sampleAbs
		bins[binIdx].count++
	}

	averages := make([]float64, numBins)
	var maxAvg float64
	for i := range numBins {
		if bins[i].count > 0 {
			averages[i] = bins[i].sum / float64(bins[i].count)
		}
		if averages[i] > maxAvg {
			maxAvg = averages[i]
		}
	}

	waveform := make([]byte, numBins)
	if maxAvg == 0.0 {
		return seconds, waveform
	}

	scale := 100.0 / maxAvg
	for i := range numBins {
		val := averages[i] * scale
		if val > 100.0 {
			val = 100.0
		}
		waveform[i] = byte(math.Round(val))
	}

	return seconds, waveform
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

// SplitAnnexBAccessUnits parses Annex-B H.264 stream into individual access units (video frames).
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

// AnnexBHasIDR reports whether the given Annex-B packet contains an IDR keyframe (NAL type 5).
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

// ExtensionFor maps a MIME type string to its corresponding file extension.
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
