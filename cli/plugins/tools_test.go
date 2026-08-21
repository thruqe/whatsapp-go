package plugins

import (
	"context"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	cliutils "whatsrook/cli/utils"
)

func TestFancyCommandResolution(t *testing.T) {
	// Verify IndexedFonts has Small Caps at #14
	if len(cliutils.IndexedFonts) < 14 {
		t.Fatalf("expected at least 14 indexed fonts, got %d", len(cliutils.IndexedFonts))
	}
	font14 := cliutils.IndexedFonts[13]
	if font14.Key != "small-caps" {
		t.Errorf("expected font #14 to be small-caps, got %s (%s)", font14.Name, font14.Key)
	}

	testText := "Hello World"
	curr := cliutils.GetFontStyle()
	cliutils.SetFontStyle("small-caps")
	expected14 := cliutils.ConvertFontStyle(testText)
	cliutils.SetFontStyle(curr)

	if expected14 != "ʜᴇʟʟᴏ ᴡᴏʀʟᴅ" {
		t.Errorf("expected 'ʜᴇʟʟᴏ ᴡᴏʀʟᴅ', got %q", expected14)
	}
}

func TestExtractTextFromProto_Quoted(t *testing.T) {
	sample := "Hello from replied message"
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &sample,
		},
	}
	extracted := extractTextFromProto(msg)
	if strings.TrimSpace(extracted) != sample {
		t.Errorf("expected %q, got %q", sample, extracted)
	}
}

func TestFancyHandler_ReplyResolution(t *testing.T) {
	repliedText := "Sample message to convert"
	quotedMsg := &waE2E.Message{
		Conversation: &repliedText,
	}

	stanzaID := "ABC123XYZ"
	participant := "1234567890@s.whatsapp.net"
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:     "MSG001",
			Chat:   types.NewJID("1234567890", types.DefaultUserServer),
			Sender: types.NewJID("1234567890", types.DefaultUserServer),
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: new(string),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:      &stanzaID,
					Participant:   &participant,
					QuotedMessage: quotedMsg,
				},
			},
		},
	}

	ctx := &Context{
		Ctx:     context.Background(),
		Evt:     evt,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
		Args:    []string{"14"},
		RawArgs: "14",
	}

	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		t.Fatal("expected non-nil quoted message")
	}

	text := strings.TrimSpace(extractTextFromProto(quoted))
	if text != repliedText {
		t.Fatalf("expected extracted text %q, got %q", repliedText, text)
	}

	curr := cliutils.GetFontStyle()
	cliutils.SetFontStyle("small-caps")
	converted := cliutils.ConvertFontStyle(text)
	cliutils.SetFontStyle(curr)

	if converted != "sᴀᴍᴘʟᴇ ᴍᴇssᴀɢᴇ ᴛᴏ ᴄᴏɴᴠᴇʀᴛ" {
		t.Errorf("expected 'sᴀᴍᴘʟᴇ ᴍᴇssᴀɢᴇ ᴛᴏ ᴄᴏɴᴠᴇʀᴛ', got %q", converted)
	}
}
