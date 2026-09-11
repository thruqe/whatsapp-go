package media

import (
	"bytes"
	"testing"
)

func TestExtensionFor(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/jpeg", ".jpg"},
		{"image/jpg", ".jpg"},
		{"image/png", ".png"},
		{"image/webp", ".webp"},
		{"image/gif", ".gif"},
		{"video/mp4", ".mp4"},
		{"video/mp4; codecs=avc1", ".mp4"},
		{"audio/ogg; codecs=opus", ".ogg"},
		{"audio/mpeg", ".mp3"},
		{"audio/wav", ".wav"},
		{"application/pdf", ".pdf"},
		{"unknown/type", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			if got := ExtensionFor(tt.mime); got != tt.want {
				t.Errorf("ExtensionFor(%q) = %q, want %q", tt.mime, got, tt.want)
			}
		})
	}
}

func TestExtractWaveformForTest(t *testing.T) {
	// Empty PCM
	secs, wave := ExtractWaveformForTest(nil, 8000)
	if secs != 0 || len(wave) != 64 {
		t.Fatalf("expected 0 secs and 64 bins for empty PCM, got secs=%d len=%d", secs, len(wave))
	}

	// 1 second of 8000Hz 16-bit mono silence (8000 samples = 16000 bytes)
	silence := make([]byte, 16000)
	secs, wave = ExtractWaveformForTest(silence, 8000)
	if secs != 1 {
		t.Errorf("expected 1 sec, got %d", secs)
	}
	if len(wave) != 64 {
		t.Errorf("expected 64 bins, got %d", len(wave))
	}

	// Test non-silent synthetic waveform
	pcm := make([]byte, 16000)
	for i := range 8000 {
		// Sawtooth wave
		val := int16(i * 4)
		pcm[i*2] = byte(val & 0xFF)
		pcm[i*2+1] = byte((val >> 8) & 0xFF)
	}
	secs, wave = ExtractWaveformForTest(pcm, 8000)
	if secs != 1 {
		t.Errorf("expected 1 sec, got %d", secs)
	}
	hasNonZero := false
	for _, b := range wave {
		if b > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Errorf("expected non-zero waveform values")
	}
}

func TestAnnexBFunctions(t *testing.T) {
	// 4-byte start code + NAL type 5 (IDR): 0x00, 0x00, 0x00, 0x01, 0x65 (0x65 & 0x1f = 5)
	idrAU := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0xAA, 0xBB, 0xCC}
	if !AnnexBHasIDR(idrAU) {
		t.Errorf("expected AnnexBHasIDR to be true for IDR NAL")
	}

	// 4-byte start code + NAL type 1 (Non-IDR slice): 0x00, 0x00, 0x00, 0x01, 0x61 (0x61 & 0x1f = 1)
	nonIdrAU := []byte{0x00, 0x00, 0x00, 0x01, 0x61, 0x11, 0x22, 0x33}
	if AnnexBHasIDR(nonIdrAU) {
		t.Errorf("expected AnnexBHasIDR to be false for non-IDR slice")
	}

	combined := append(idrAU, nonIdrAU...)
	units := SplitAnnexBAccessUnits(combined)
	if len(units) != 2 {
		t.Fatalf("expected 2 access units, got %d", len(units))
	}
	if !bytes.Equal(units[0], idrAU) {
		t.Errorf("expected unit 0 to match idrAU")
	}
	if !bytes.Equal(units[1], nonIdrAU) {
		t.Errorf("expected unit 1 to match nonIdrAU")
	}
}
