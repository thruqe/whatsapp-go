package webp

import (
	"encoding/binary"
	"testing"
)

// createDummyLosslessWebP returns minimal valid WebP data for testing.
func createDummyLosslessWebP() []byte {
	// RIFF header (12 bytes) + VP8L chunk (5 bytes data + 8 bytes header = 13 bytes + 1 padding = 14 bytes)
	// VP8L: signature 0x2f, width=1, height=1
	vp8lData := []byte{0x2f, 0x00, 0x00, 0x00, 0x00}
	chunkHeader := make([]byte, 8)
	copy(chunkHeader[0:4], "VP8L")
	binary.LittleEndian.PutUint32(chunkHeader[4:8], uint32(len(vp8lData)))

	totalLen := 4 + len(chunkHeader) + len(vp8lData) + 1 // "WEBP" + chunk + data + pad
	riffHeader := make([]byte, 12)
	copy(riffHeader[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riffHeader[4:8], uint32(totalLen))
	copy(riffHeader[8:12], "WEBP")

	out := append(riffHeader, chunkHeader...)
	out = append(out, vp8lData...)
	out = append(out, 0) // padding byte
	return out
}

func TestWebPStickerMetadata(t *testing.T) {
	rawWebP := createDummyLosslessWebP()

	// Initial check: no sticker metadata
	meta, err := GetStickerMetadata(rawWebP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Fatal("expected nil metadata for raw WebP")
	}

	// Add metadata
	withMeta, err := AddStickerMetadata(rawWebP, "TestPack", "TestAuthor")
	if err != nil {
		t.Fatalf("AddStickerMetadata failed: %v", err)
	}

	// Extract and verify
	parsed, err := GetStickerMetadata(withMeta)
	if err != nil {
		t.Fatalf("GetStickerMetadata failed: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected metadata to be present")
	}
	if parsed.PackName != "TestPack" {
		t.Fatalf("expected PackName 'TestPack', got %q", parsed.PackName)
	}
	if parsed.Publisher != "TestAuthor" {
		t.Fatalf("expected Publisher 'TestAuthor', got %q", parsed.Publisher)
	}
}
