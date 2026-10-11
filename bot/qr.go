package main

import (
	"github.com/skip2/go-qrcode"
)

// TerminalString generates an ANSI UTF-8 block character string for terminal console rendering.
func TerminalString(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	return q.ToSmallString(false), nil
}

// RenderTerminal generates a terminal QR string or empty string on error.
func RenderTerminal(content string) string {
	str, _ := TerminalString(content)
	return str
}
