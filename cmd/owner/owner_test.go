package owner

import (
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/cmd/dispatch"
)

func TestShellWorkingDirPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "whatsrook-sh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "sub_bin")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	chatKey := "test_chat_1"

	// 1. Initial directory should be valid and fallback to os.Getwd()
	initial := getShellWorkingDir(chatKey)
	if initial == "" {
		t.Fatalf("expected non-empty initial working dir")
	}

	// 2. Change working directory
	setShellWorkingDir(chatKey, subDir)
	current := getShellWorkingDir(chatKey)
	if current != subDir {
		t.Fatalf("expected working dir %q, got %q", subDir, current)
	}

	// 3. Different chat should have its own independent directory
	otherChatKey := "test_chat_2"
	otherDir := getShellWorkingDir(otherChatKey)
	if otherDir == subDir {
		t.Fatalf("expected different chat to not share modified directory without being set")
	}

	// 4. If directory is removed, it should safely fallback to default
	_ = os.Remove(subDir)
	fallback := getShellWorkingDir(chatKey)
	if fallback == subDir {
		t.Fatalf("expected fallback when directory was removed, got %q", fallback)
	}
}

func TestCleanShellOutput(t *testing.T) {
	raw := "Hello\x1b[31m World\x1b[0m\rOverwritten!\nLine 2\r\nLine 3"
	cleaned := CleanShellOutput(raw)
	if cleaned == "" {
		t.Fatalf("expected non-empty cleaned output")
	}
	if cleaned != "Overwritten!\nLine 2\nLine 3" {
		t.Errorf("unexpected output: %q", cleaned)
	}
}

func TestShellSessionRCPath(t *testing.T) {
	chatKey := "test_chat_persistence@s.whatsapp.net"
	rcPath := getShellSessionRCPath(chatKey)
	if rcPath == "" {
		t.Fatalf("expected non-empty rcPath")
	}
	defer os.Remove(rcPath)

	if !filepath.IsAbs(rcPath) {
		t.Errorf("expected absolute rcPath, got %q", rcPath)
	}
}

func TestExtractImageFromContext(t *testing.T) {
	// 1. Direct attached image
	directImg := &waE2E.ImageMessage{
		Mimetype: new(string),
	}
	*directImg.Mimetype = "image/jpeg"
	ctxDirect := &dispatch.Context{
		Evt: &events.Message{
			Message: &waE2E.Message{
				ImageMessage: directImg,
			},
		},
	}
	dl, mime := extractImageFromContext(ctxDirect)
	if dl == nil || mime != "image/jpeg" {
		t.Errorf("expected direct image extraction, got dl=%v, mime=%q", dl, mime)
	}

	// 2. Quoted image message in reply
	quotedImg := &waE2E.ImageMessage{
		Mimetype: new(string),
	}
	*quotedImg.Mimetype = "image/png"
	textWithQuote := ".pp"
	ctxQuoted := &dispatch.Context{
		Evt: &events.Message{
			Message: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: &textWithQuote,
					ContextInfo: &waE2E.ContextInfo{
						QuotedMessage: &waE2E.Message{
							ImageMessage: quotedImg,
						},
					},
				},
			},
		},
	}
	dlQuoted, mimeQuoted := extractImageFromContext(ctxQuoted)
	if dlQuoted == nil || mimeQuoted != "image/png" {
		t.Errorf("expected quoted image extraction, got dl=%v, mime=%q", dlQuoted, mimeQuoted)
	}

	// 3. Plain text message without any image or quote
	plainText := "hello"
	ctxPlain := &dispatch.Context{
		Evt: &events.Message{
			Message: &waE2E.Message{
				Conversation: &plainText,
			},
		},
	}
	dlPlain, _ := extractImageFromContext(ctxPlain)
	if dlPlain != nil {
		t.Errorf("expected nil media for plain message, got %v", dlPlain)
	}

	// 4. Nil context
	dlNil, _ := extractImageFromContext(nil)
	if dlNil != nil {
		t.Errorf("expected nil for nil context, got %v", dlNil)
	}
}
