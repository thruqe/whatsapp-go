package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type stickerMetadata struct {
	StickerPackID        string   `json:"sticker-pack-id"`
	StickerPackName      string   `json:"sticker-pack-name"`
	StickerPackPublisher string   `json:"sticker-pack-publisher"`
	Emojis               []string `json:"emojis"`
}

func writeExifMetadata(webpBytes []byte, packName, author string) []byte {
	meta := stickerMetadata{
		StickerPackID:        "whatsrook-sticker-pack",
		StickerPackName:      packName,
		StickerPackPublisher: author,
		Emojis:               []string{"✂️"},
	}
	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		return webpBytes
	}

	header := []byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x01\x00A\x01\x04\x00\x00\x00\x00\x00\x16\x00\x00\x00\x00\x00\x00\x00")
	exifPayload := append(header, jsonBytes...)

	if len(webpBytes) < 12 || string(webpBytes[0:4]) != "RIFF" || string(webpBytes[8:12]) != "WEBP" {
		return webpBytes
	}

	var result bytes.Buffer
	result.Write(webpBytes[0:12])

	result.WriteString("EXIF")
	exifLen := uint32(len(exifPayload))
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], exifLen)
	result.Write(lenBuf[:])
	result.Write(exifPayload)
	if len(exifPayload)%2 != 0 {
		result.WriteByte(0)
	}

	result.Write(webpBytes[12:])

	out := result.Bytes()
	totalRiffLen := uint32(len(out) - 8)
	binary.LittleEndian.PutUint32(out[4:8], totalRiffLen)
	return out
}

func convertToCropSticker(inputPath, packName, author string) ([]byte, error) {
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("crop_%d_%d.webp", os.Getpid(), rand.Uint32()))

	vf := "crop='min(iw,ih)':'min(iw,ih)',scale=512:512"

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-t", "8",
		"-vf", vf,
		"-vcodec", "libwebp",
		"-lossless", "0",
		"-q:v", "35",
		"-compression_level", "6",
		"-loop", "0",
		"-preset", "default",
		"-an",
		"-pix_fmt", "yuva420p",
		outPath,
	)

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("ffmpeg failed to process crop sticker: %w", err)
	}

	webpRaw, err := os.ReadFile(outPath)
	_ = os.Remove(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sticker output: %w", err)
	}

	return writeExifMetadata(webpRaw, packName, author), nil
}

func main() {
	req := sdk.Load()
	query := req.Query()

	packName := req.EffectiveBotName()
	author := "WhatsRook"

	if query != "" {
		parts := strings.Split(query, "|")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			author = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			packName = strings.TrimSpace(parts[1])
		}
	}

	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "{") {
		inputFile := os.Args[1]
		outputFile := "crop.webp"
		if len(os.Args) > 2 {
			outputFile = os.Args[2]
		}

		bytes, err := convertToCropSticker(inputFile, packName, author)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Crop sticker error: %s\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(outputFile, bytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving sticker to %s: %s\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Saved %d bytes to %s\n", len(bytes), outputFile)
		return
	}

	sdk.Respond(fmt.Sprintf(
		"Reply to an image or video with `%scrop [author | pack]` to convert it to a square-cropped sticker.",
		req.EffectivePrefix(),
	))
}
