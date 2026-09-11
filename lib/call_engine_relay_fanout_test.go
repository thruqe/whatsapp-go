package whatsmeow

import (
	"testing"
)

func TestRtpReplayFilter_DuplicateDetection(t *testing.T) {
	filter := newRtpReplayFilter()

	ssrc1 := uint32(0x12345678)
	ssrc2 := uint32(0x87654321)

	// First time seeing seq 10 on ssrc1 -> not duplicate
	if filter.Duplicate(ssrc1, 10) {
		t.Fatalf("expected seq 10 on ssrc1 to not be duplicate")
	}

	// Second time seeing seq 10 on ssrc1 -> duplicate
	if !filter.Duplicate(ssrc1, 10) {
		t.Fatalf("expected seq 10 on ssrc1 to be detected as duplicate")
	}

	// Seeing seq 10 on ssrc2 -> not duplicate (independent ssrc)
	if filter.Duplicate(ssrc2, 10) {
		t.Fatalf("expected seq 10 on ssrc2 to not be duplicate")
	}

	// Next seq on ssrc1 -> not duplicate
	if filter.Duplicate(ssrc1, 11) {
		t.Fatalf("expected seq 11 on ssrc1 to not be duplicate")
	}

	// Duplicate of seq 11 -> duplicate
	if !filter.Duplicate(ssrc1, 11) {
		t.Fatalf("expected seq 11 on ssrc1 to be detected as duplicate")
	}
}

func TestRtpReplayFilter_WraparoundSlotReplacement(t *testing.T) {
	filter := newRtpReplayFilter()
	ssrc := uint32(0xCAFEBABE)

	// Insert seq 5
	if filter.Duplicate(ssrc, 5) {
		t.Fatalf("expected seq 5 to not be duplicate")
	}
	if !filter.Duplicate(ssrc, 5) {
		t.Fatalf("expected seq 5 duplicate to be true")
	}

	// Insert seq 5 + 1024 (same ring slot index % 1024)
	if filter.Duplicate(ssrc, 5+1024) {
		t.Fatalf("expected seq 1029 to not be duplicate")
	}

	// Now seq 5 is overwritten by 1029 in slot 5; seq 1029 should be duplicate
	if !filter.Duplicate(ssrc, 5+1024) {
		t.Fatalf("expected seq 1029 to be duplicate")
	}
}
