package webp

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestBuildAndVerifyExif(t *testing.T) {
	meta := &ExifStickerMetadata{
		PackID:    "test-pack-id-123",
		PackName:  "Test Pack",
		Publisher: "WhatsRook",
		Emojis:    []string{"🚀", "🔥"},
	}

	exif, err := buildExif(meta)
	if err != nil {
		t.Fatalf("buildExif failed: %v", err)
	}

	if !isWhatsAppStickerExif(exif) {
		t.Errorf("expected isWhatsAppStickerExif to return true")
	}

	if len(exif) <= len(ExifHeader) {
		t.Fatalf("expected exif length > %d, got %d", len(ExifHeader), len(exif))
	}

	jsonLen := binary.LittleEndian.Uint32(exif[14:18])
	if int(jsonLen) != len(exif)-len(ExifHeader) {
		t.Errorf("exif length mismatch: header says %d, actual payload %d", jsonLen, len(exif)-len(ExifHeader))
	}
}

func TestWebPChunkSerialization(t *testing.T) {
	// Synthetic minimal WebP with VP8 chunk
	vp8Data := []byte{0x00, 0x00, 0x00, 0x9d, 0x01, 0x2a, 0x20, 0x00, 0x20, 0x00} // 32x32 keyframe
	chunks := []WebPChunk{
		{
			Type: [4]byte{'V', 'P', '8', ' '},
			Data: vp8Data,
		},
	}

	serialized := serializeWebP(chunks)
	if len(serialized) < 12 {
		t.Fatalf("serialized data too short")
	}

	if string(serialized[0:4]) != "RIFF" || string(serialized[8:12]) != "WEBP" {
		t.Errorf("invalid WebP RIFF header: %q %q", string(serialized[0:4]), string(serialized[8:12]))
	}

	parsed, err := parseWebP(serialized)
	if err != nil {
		t.Fatalf("parseWebP failed: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(parsed))
	}

	if parsed[0].Type != [4]byte{'V', 'P', '8', ' '} {
		t.Errorf("expected VP8 chunk, got %s", string(parsed[0].Type[:]))
	}

	if !bytes.Equal(parsed[0].Data, vp8Data) {
		t.Errorf("chunk data mismatch")
	}
}

func TestAddAndGetStickerMetadata(t *testing.T) {
	// Create minimal valid WebP
	vp8Data := []byte{0x00, 0x00, 0x00, 0x9d, 0x01, 0x2a, 0x20, 0x00, 0x20, 0x00}
	initialWebP := serializeWebP([]WebPChunk{
		{
			Type: [4]byte{'V', 'P', '8', ' '},
			Data: vp8Data,
		},
	})

	withMeta, err := AddStickerMetadata(initialWebP, "My Pack", "My Author")
	if err != nil {
		t.Fatalf("AddStickerMetadata failed: %v", err)
	}

	meta, err := GetStickerMetadata(withMeta)
	if err != nil {
		t.Fatalf("GetStickerMetadata failed: %v", err)
	}
	if meta == nil {
		t.Fatalf("expected non-nil metadata")
	}

	if meta.PackName != "My Pack" {
		t.Errorf("PackName = %q, want 'My Pack'", meta.PackName)
	}
	if meta.Publisher != "My Author" {
		t.Errorf("Publisher = %q, want 'My Author'", meta.Publisher)
	}
}
