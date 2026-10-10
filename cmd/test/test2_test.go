package test

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"whatsrook/cmd/dispatch"
)

func TestTest2CommandRegistration(t *testing.T) {
	cmd, ok := dispatch.Get("test2")
	if !ok || cmd == nil {
		t.Fatalf("expected command 'test2' to be registered")
	}
	if cmd.Category != "owner" {
		t.Errorf("expected category 'owner', got %q", cmd.Category)
	}
	if !cmd.HideFromMenu {
		t.Errorf("expected command to be hidden from menu")
	}

	aliasCmd, okAlias := dispatch.Get("ghost")
	if !okAlias || aliasCmd != cmd {
		t.Errorf("expected alias 'ghost' to map to test2 command")
	}

	whisperCmd, okWhisper := dispatch.Get("whisper")
	if !okWhisper || whisperCmd == nil {
		t.Errorf("expected command 'whisper' to be registered")
	}

	testCmd, okTest := dispatch.Get("test")
	if !okTest || testCmd == nil {
		t.Errorf("expected command 'test' to be registered")
	}
	if testCmd.Category != "owner" {
		t.Errorf("expected category 'owner', got %q", testCmd.Category)
	}
}

func TestParseGhostArgs(t *testing.T) {
	tests := []struct {
		input         string
		wantTarget    string
		wantBlank     bool
		wantMention   bool
		wantFlash     bool
		wantEphemeral bool
		wantViewOnce  bool
		wantSKD       bool
		wantPM        bool
		wantText      string
	}{
		{
			input:    "",
			wantText: "",
		},
		{
			input:     "-b",
			wantBlank: true,
			wantText:  "",
		},
		{
			input:      "-to 1234567890 Hello ghost",
			wantTarget: "1234567890",
			wantText:   "Hello ghost",
		},
		{
			input:      "@1234567890 Secret message",
			wantTarget: "1234567890",
			wantText:   "Secret message",
		},
		{
			input:     "-f Disappearing message",
			wantFlash: true,
			wantText:  "Disappearing message",
		},
		{
			input:         "-e Secret note",
			wantEphemeral: true,
			wantText:      "Secret note",
		},
		{
			input:        "-vo Once only",
			wantViewOnce: true,
			wantText:     "Once only",
		},
		{
			input:    "-skd Group secret",
			wantSKD:  true,
			wantText: "Group secret",
		},
		{
			input:    "-pm Private reply",
			wantPM:   true,
			wantText: "Private reply",
		},
		{
			input:    "Plain text ghost message",
			wantText: "Plain text ghost message",
		},
	}

	for _, tt := range tests {
		got := parseGhostArgs(tt.input)
		if got.target != tt.wantTarget {
			t.Errorf("parseGhostArgs(%q).target = %q, want %q", tt.input, got.target, tt.wantTarget)
		}
		if got.blankOnly != tt.wantBlank {
			t.Errorf("parseGhostArgs(%q).blankOnly = %v, want %v", tt.input, got.blankOnly, tt.wantBlank)
		}
		if got.flash != tt.wantFlash {
			t.Errorf("parseGhostArgs(%q).flash = %v, want %v", tt.input, got.flash, tt.wantFlash)
		}
		if got.ephemeral != tt.wantEphemeral {
			t.Errorf("parseGhostArgs(%q).ephemeral = %v, want %v", tt.input, got.ephemeral, tt.wantEphemeral)
		}
		if got.viewOnce != tt.wantViewOnce {
			t.Errorf("parseGhostArgs(%q).viewOnce = %v, want %v", tt.input, got.viewOnce, tt.wantViewOnce)
		}
		if got.useSenderKey != tt.wantSKD {
			t.Errorf("parseGhostArgs(%q).useSenderKey = %v, want %v", tt.input, got.useSenderKey, tt.wantSKD)
		}
		if got.pm != tt.wantPM {
			t.Errorf("parseGhostArgs(%q).pm = %v, want %v", tt.input, got.pm, tt.wantPM)
		}
		if got.text != tt.wantText {
			t.Errorf("parseGhostArgs(%q).text = %q, want %q", tt.input, got.text, tt.wantText)
		}
	}
}

func TestResolveTargetParticipant(t *testing.T) {
	// 1. Explicit phone number target
	ctx := &dispatch.Context{}
	target, err := resolveTargetParticipant(ctx, "1234567890")
	if err != nil {
		t.Fatalf("resolveTargetParticipant error: %v", err)
	}
	if target.User != "1234567890" || target.Server != types.DefaultUserServer {
		t.Errorf("unexpected target JID: %s", target)
	}

	// 2. Explicit full JID target
	target2, err := resolveTargetParticipant(ctx, "987654321@s.whatsapp.net")
	if err != nil {
		t.Fatalf("resolveTargetParticipant error: %v", err)
	}
	if target2.User != "987654321" {
		t.Errorf("unexpected target JID: %s", target2)
	}

	// 3. Fallback to 1:1 chat partner when in DM
	dmCtx := &dispatch.Context{
		Chat: types.NewJID("555444333", types.DefaultUserServer),
	}
	targetDM, err := resolveTargetParticipant(dmCtx, "")
	if err != nil {
		t.Fatalf("resolveTargetParticipant DM error: %v", err)
	}
	if targetDM.User != "555444333" {
		t.Errorf("unexpected DM target: %s", targetDM)
	}
}

func TestBuildGhostPayloads(t *testing.T) {
	// 1. Blank ghost message
	blankMsg := buildGhostBlankMessage()
	if blankMsg.GetConversation() != ZeroWidthInvisibleChars {
		t.Errorf("expected zero-width characters in blank message, got %q", blankMsg.GetConversation())
	}

	// 2. Normal text ghost message
	textMsg := buildGhostTextMessage("Ghost alert", nil)
	if textMsg.GetConversation() != "Ghost alert" {
		t.Errorf("expected conversation 'Ghost alert', got %q", textMsg.GetConversation())
	}

	// 3. Text ghost message with quoted context
	ci := &waE2E.ContextInfo{StanzaID: new("123")}
	quotedMsg := buildGhostTextMessage("Quoted ghost", ci)
	etm := quotedMsg.GetExtendedTextMessage()
	if etm == nil || etm.GetText() != "Quoted ghost" || etm.GetContextInfo().GetStanzaID() != "123" {
		t.Errorf("expected ExtendedTextMessage with ContextInfo, got %v", quotedMsg)
	}

	// 4. Ephemeral ghost wrapper
	ephMsg := buildGhostEphemeralMessage(blankMsg)
	if ephMsg.GetEphemeralMessage() == nil || ephMsg.GetEphemeralMessage().GetMessage() != blankMsg {
		t.Errorf("expected EphemeralMessage wrapper around inner blank message")
	}

	// 5. ViewOnce ghost wrapper
	voMsg := buildGhostViewOnceMessage(blankMsg)
	if voMsg.GetViewOnceMessage() == nil || voMsg.GetViewOnceMessage().GetMessage() != blankMsg {
		t.Errorf("expected ViewOnceMessage wrapper around inner blank message")
	}

	// 6. Native WhatsApp Whisper (BotInvokeMessage matching image.png)
	targetJID := types.NewJID("1234567890", types.DefaultUserServer)
	invokerJID := types.NewJID("9876543210", types.DefaultUserServer)
	whisperMsg := buildWhisperBotInvokeMessage("Hi Vivy.", targetJID, invokerJID, nil)

	if whisperMsg.GetBotInvokeMessage() == nil {
		t.Fatalf("expected BotInvokeMessage in whisperMsg")
	}
	inner := whisperMsg.GetBotInvokeMessage().GetMessage()
	if inner == nil || inner.GetExtendedTextMessage() == nil {
		t.Fatalf("expected inner ExtendedTextMessage inside BotInvokeMessage")
	}
	if inner.GetExtendedTextMessage().GetText() != "Hi Vivy." {
		t.Errorf("expected text 'Hi Vivy.', got %q", inner.GetExtendedTextMessage().GetText())
	}
	if whisperMsg.GetMessageContextInfo() == nil || whisperMsg.GetMessageContextInfo().GetBotMetadata() == nil {
		t.Fatalf("expected BotMetadata in MessageContextInfo")
	}
	if whisperMsg.GetMessageContextInfo().GetBotMetadata().GetInvokerJID() != invokerJID.String() {
		t.Errorf("expected InvokerJID %s, got %s", invokerJID, whisperMsg.GetMessageContextInfo().GetBotMetadata().GetInvokerJID())
	}
}
