package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProcessStickerMedia_Video(t *testing.T) {
	// Generate a 1-second test MP4 using ffmpeg lavfi
	tmpDir, err := os.MkdirTemp("", "sticker_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testMP4Path := filepath.Join(tmpDir, "test.mp4")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=15",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", testMP4Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Skipping sticker video test because ffmpeg is not functional: %v (output: %s)", err, string(out))
	}

	videoBytes, err := os.ReadFile(testMP4Path)
	if err != nil {
		t.Fatalf("Failed to read test MP4: %v", err)
	}

	resBytes, err := processSticker(videoBytes, true, "WhatsRook", "Thruqe", "")
	if err != nil {
		t.Fatalf("ProcessStickerMedia video conversion failed: %v", err)
	}

	if len(resBytes) == 0 {
		t.Fatalf("ProcessStickerMedia returned empty byte slice")
	}
	if len(resBytes) > 500*1024 {
		t.Errorf("Sticker size (%d bytes) exceeds WhatsApp 500KB limit", len(resBytes))
	}
}

func TestProcessStickerMedia_Image(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sticker_img_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testImgPath := filepath.Join(tmpDir, "test.jpg")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=blue:s=300x300:d=0.1",
		"-frames:v", "1", testImgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Skipping sticker image test because ffmpeg is not functional: %v (output: %s)", err, string(out))
	}

	imgBytes, err := os.ReadFile(testImgPath)
	if err != nil {
		t.Fatalf("Failed to read test image: %v", err)
	}

	resBytes, err := processSticker(imgBytes, false, "WhatsRook", "Thruqe", "")
	if err != nil {
		t.Fatalf("ProcessStickerMedia image conversion failed: %v", err)
	}

	if len(resBytes) == 0 {
		t.Fatalf("ProcessStickerMedia returned empty byte slice")
	}
}
