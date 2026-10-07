package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

func convertToMP4(inputPath string) ([]byte, error) {
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("media_out_%d_%d.mp4", os.Getpid(), rand.Uint32()))

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "ultrafast",
		"-movflags", "+faststart",
		outPath,
	)

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("ffmpeg MP4 conversion failed: %w", err)
	}

	bytes, err := os.ReadFile(outPath)
	_ = os.Remove(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MP4 file: %w", err)
	}
	return bytes, nil
}

func convertToMP3(inputPath string) ([]byte, error) {
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("media_out_%d_%d.mp3", os.Getpid(), rand.Uint32()))

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-vn",
		"-ar", "16000",
		"-ac", "1",
		"-b:a", "64k",
		outPath,
	)

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("ffmpeg MP3 conversion failed: %w", err)
	}

	bytes, err := os.ReadFile(outPath)
	_ = os.Remove(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MP3 file: %w", err)
	}
	return bytes, nil
}

func createBlackVideo(inputPath string) ([]byte, error) {
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("media_black_%d_%d.mp4", os.Getpid(), rand.Uint32()))

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "lavfi",
		"-i", "color=c=black:s=720x720:r=30",
		"-i", inputPath,
		"-c:v", "libx264",
		"-tune", "stillimage",
		"-c:a", "aac",
		"-b:a", "128k",
		"-pix_fmt", "yuv420p",
		"-shortest",
		outPath,
	)

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("ffmpeg black video generation failed: %w", err)
	}

	bytes, err := os.ReadFile(outPath)
	_ = os.Remove(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read video output: %w", err)
	}
	return bytes, nil
}

func main() {
	req := sdk.Load()
	cmd := strings.ToLower(req.Command)

	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "{") {
		inputFile := os.Args[1]
		outputFile := "output.mp4"
		if len(os.Args) > 2 {
			outputFile = os.Args[2]
		}

		var bytes []byte
		var err error
		if cmd == "mp3" || strings.HasSuffix(outputFile, ".mp3") {
			bytes, err = convertToMP3(inputFile)
		} else if cmd == "black" {
			bytes, err = createBlackVideo(inputFile)
		} else {
			bytes, err = convertToMP4(inputFile)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Media processing error: %s\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(outputFile, bytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving to %s: %s\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Saved %d bytes to %s\n", len(bytes), outputFile)
		return
	}

	sdk.Respond(fmt.Sprintf(
		"*%s Media Engine*\n\n"+
			"• `%smp4`   : Convert quoted sticker/audio/video to MP4 format\n"+
			"• `%smp3`   : Convert quoted video/audio to MP3 audio format\n"+
			"• `%sblack` : Create a black background video with attached audio track\n"+
			"• `%strim <start> <end>` : Trim a video to target duration",
		req.EffectiveBotName(),
		req.EffectivePrefix(),
		req.EffectivePrefix(),
		req.EffectivePrefix(),
		req.EffectivePrefix(),
	))
}
