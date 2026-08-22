package qr

import (
	"testing"
)

func TestEncodePNG(t *testing.T) {
	pngData, err := EncodePNG("test-qr", 200)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}
	if len(pngData) < 8 || string(pngData[1:4]) != "PNG" {
		t.Fatalf("expected valid PNG header, got bytes: %v", pngData[:8])
	}
}
