package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

func transcodeVideo(ctx context.Context, inputData []byte) ([]byte, error) {
	slog.Debug("transcodeVideo: starting transcoding via ffmpeg")

	tmpDir, err := os.MkdirTemp("", "whatsrook_vid_*")
	if err != nil {
		slog.Error("transcodeVideo: failed to create temp dir", "err", err)
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tmpIn, inputData, 0644); err != nil {
		slog.Error("transcodeVideo: failed to write input", "err", err)
		return nil, fmt.Errorf("failed to write input: %w", err)
	}

	tmpOutName := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn,
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-profile:v", "main", "-level:v", "4.0",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		tmpOutName)

	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("ffmpeg video transcode failed", "err", err, "output", string(out))
		return nil, fmt.Errorf("ffmpeg video transcode failed: %w (output: %s)", err, string(out))
	}

	convertedData, err := os.ReadFile(tmpOutName)
	if err != nil {
		slog.Error("transcodeVideo: failed to read converted file", "err", err)
		return nil, err
	}

	slog.Debug("transcodeVideo: completed", "orig_size", len(inputData), "new_size", len(convertedData))
	return convertedData, nil
}

func transcodeAudio(ctx context.Context, inputData []byte) ([]byte, error) {
	slog.Debug("transcodeAudio: starting transcoding via ffmpeg")

	tmpDir, err := os.MkdirTemp("", "whatsrook_aud_*")
	if err != nil {
		slog.Error("transcodeAudio: failed to create temp dir", "err", err)
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tmpIn, inputData, 0644); err != nil {
		slog.Error("transcodeAudio: failed to write input", "err", err)
		return nil, fmt.Errorf("failed to write input: %w", err)
	}

	tmpOutName := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn,
		"-vn", "-c:a", "aac", "-b:a", "128k",
		tmpOutName)

	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("ffmpeg audio transcode failed", "err", err, "output", string(out))
		return nil, fmt.Errorf("ffmpeg audio transcode failed: %w (output: %s)", err, string(out))
	}

	convertedData, err := os.ReadFile(tmpOutName)
	if err != nil {
		slog.Error("transcodeAudio: failed to read converted file", "err", err)
		return nil, err
	}

	slog.Debug("transcodeAudio: completed", "orig_size", len(inputData), "new_size", len(convertedData))
	return convertedData, nil
}
