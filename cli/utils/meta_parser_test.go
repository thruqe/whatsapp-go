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
