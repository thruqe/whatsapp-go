package cliutils

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestRenderUserContext(t *testing.T) {
	d := Data{
		PushName: "John Doe",
		User:     types.NewJID("123456789", types.DefaultUserServer),
		IsSudo:   true,
	}

	rendered := RenderUserContext(d)
	if !strings.Contains(rendered, "User name: John Doe") {
		t.Errorf("expected rendered context to contain 'User name: John Doe', got %q", rendered)
	}
	if !strings.Contains(rendered, "Status: Owner/Sudo") {
		t.Errorf("expected rendered context to contain Status, got %q", rendered)
	}
}

func TestRenderQuotedContextWithImageBase64(t *testing.T) {
	d := Data{
		UserOfQuotedMessage:          "987654321",
		QuotedMessageParticipantRole: "Admin",
		QuotedMessageType:            "Image",
		QuotedMessageOfQuestion:      "Check out this photo!",
		QuotedImageBase64:            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		QuotedImageMimeType:          "image/png",
	}

	rendered := RenderQuotedContext(d)
	if !strings.Contains(rendered, "From: 987654321 (Admin)") {
		t.Errorf("expected quoted user and role info, got %q", rendered)
	}
	if !strings.Contains(rendered, "Message Type: Image") {
		t.Errorf("expected message type info, got %q", rendered)
	}
	if !strings.Contains(rendered, "Message Content: Check out this photo!") {
		t.Errorf("expected message content info, got %q", rendered)
	}
	if !strings.Contains(rendered, "Image Base64: data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==") {
		t.Errorf("expected image base64 data URI, got %q", rendered)
	}
}

func TestBuildRunCommandInstructionWithNameAndPrefix(t *testing.T) {
	cmds := []CommandInfo{
		{Name: "menu", Description: "Show menu", IsPublic: true},
		{Name: "ping", Description: "Check ping", IsPublic: true},
	}

	instrDot := BuildRunCommandInstructionWithNameAndPrefix(cmds, "WhatsRook", ".")
	if !strings.Contains(instrDot, "- .menu: Show menu") {
		t.Errorf("expected instruction to contain '.menu', got %q", instrDot)
	}
	if !strings.Contains(instrDot, "RUN_COMMAND: .<command_name>") {
		t.Errorf("expected instruction to reference active prefix '.', got %q", instrDot)
	}

	instrBang := BuildRunCommandInstructionWithNameAndPrefix(cmds, "WhatsRook", "!")
	if !strings.Contains(instrBang, "- !menu: Show menu") {
		t.Errorf("expected instruction to contain '!menu', got %q", instrBang)
	}
}

func TestParseRunCommand(t *testing.T) {
	tests := []struct {
		input       string
		expectedCmd string
		expectedRaw string
		expectedOk  bool
	}{
		{"RUN_COMMAND: .menu", "menu", "", true},
		{"RUN_COMMAND: !menu", "menu", "", true},
		{"RUN_COMMAND: menu", "menu", "", true},
		{"RUN_COMMAND: /ping 123", "ping", "123", true},
		{"`RUN_COMMAND: .menu`", "menu", "", true},
		{"Hello there!", "", "", false},
	}

	for _, tt := range tests {
		cmd, raw, ok := ParseRunCommand(tt.input)
		if ok != tt.expectedOk {
			t.Errorf("ParseRunCommand(%q) ok = %v, expected %v", tt.input, ok, tt.expectedOk)
		}
		if cmd != tt.expectedCmd {
			t.Errorf("ParseRunCommand(%q) cmd = %q, expected %q", tt.input, cmd, tt.expectedCmd)
		}
		if raw != tt.expectedRaw {
			t.Errorf("ParseRunCommand(%q) raw = %q, expected %q", tt.input, raw, tt.expectedRaw)
		}
	}
}
