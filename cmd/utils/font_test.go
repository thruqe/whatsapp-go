package cliutils

import (
	"testing"
)

func TestDefaultFontNormal(t *testing.T) {
	style := GetFontStyle()
	if style != "normal" {
		t.Errorf("expected default style to be 'normal', got %q", style)
	}

	input := "abcdefghijklmnopqrstuvwxyz"
	actual := ConvertFontStyle(input)
	if actual != input {
		t.Errorf("expected Convert(%q) = %q, got %q", input, input, actual)
	}
}

func TestURLPreservation(t *testing.T) {
	SetFontStyle("small-caps")
	defer SetFontStyle("normal")
	input := "Shortened URL: https://tinyurl.com/abc1234."
	expected := "sʜᴏʀᴛᴇɴᴇᴅ ᴜʀʟ: https://tinyurl.com/abc1234."
	actual := ConvertFontStyle(input)
	if actual != expected {
		t.Errorf("expected Convert(%q) = %q, got %q", input, expected, actual)
	}
}
