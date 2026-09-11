package whatsrook

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestExtractMediaFromEvent_NilAndEmpty(t *testing.T) {
	dl, isVid, mime := ExtractMediaFromEvent(nil)
	if dl != nil || isVid || mime != "" {
		t.Fatalf("expected nil/false/empty for nil event, got %v, %v, %s", dl, isVid, mime)
	}

	dl, isVid, mime = ExtractMediaFromEvent(&events.Message{})
	if dl != nil || isVid || mime != "" {
		t.Fatalf("expected nil/false/empty for empty event, got %v, %v, %s", dl, isVid, mime)
	}
}

func TestExtractMediaFromEvent_DirectImage(t *testing.T) {
	mime := "image/jpeg"
	img := &waE2E.ImageMessage{
		Mimetype: proto.String(mime),
	}
	evt := &events.Message{
		Message: &waE2E.Message{
			ImageMessage: img,
		},
	}

	dl, isVid, resMime := ExtractMediaFromEvent(evt)
	if dl == nil {
		t.Fatal("expected downloadable message, got nil")
	}
	if dl != img {
		t.Fatalf("expected dl to be img, got %v", dl)
	}
	if isVid {
		t.Fatal("expected isVid to be false for image")
	}
	if resMime != mime {
		t.Fatalf("expected mime %q, got %q", mime, resMime)
	}
}

func TestExtractMediaFromEvent_DirectVideo(t *testing.T) {
	mime := "video/mp4"
	vid := &waE2E.VideoMessage{
		Mimetype: proto.String(mime),
	}
	evt := &events.Message{
		Message: &waE2E.Message{
			VideoMessage: vid,
		},
	}

	dl, isVid, resMime := ExtractMediaFromEvent(evt)
	if dl == nil {
		t.Fatal("expected downloadable message, got nil")
	}
	if dl != vid {
		t.Fatalf("expected dl to be vid, got %v", dl)
	}
	if !isVid {
		t.Fatal("expected isVid to be true for video")
	}
	if resMime != mime {
		t.Fatalf("expected mime %q, got %q", mime, resMime)
	}
}

func TestExtractMediaFromEvent_DocumentVideo(t *testing.T) {
	mime := "video/mp4"
	doc := &waE2E.DocumentMessage{
		Mimetype: proto.String(mime),
		FileName: proto.String("clip.mp4"),
	}
	evt := &events.Message{
		Message: &waE2E.Message{
			DocumentMessage: doc,
		},
	}

	dl, isVid, resMime := ExtractMediaFromEvent(evt)
	if dl == nil {
		t.Fatal("expected downloadable message, got nil")
	}
	if !isVid {
		t.Fatal("expected isVid to be true for mp4 document")
	}
	if resMime != mime {
		t.Fatalf("expected mime %q, got %q", mime, resMime)
	}
}

func TestExtractMediaFromEvent_QuotedReply(t *testing.T) {
	mime := "image/png"
	img := &waE2E.ImageMessage{
		Mimetype: proto.String(mime),
	}
	quotedMsg := &waE2E.Message{
		ImageMessage: img,
	}
	evt := &events.Message{
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(".gpp"),
				ContextInfo: &waE2E.ContextInfo{
					QuotedMessage: quotedMsg,
				},
			},
		},
	}

	dl, isVid, resMime := ExtractMediaFromEvent(evt)
	if dl == nil {
		t.Fatal("expected downloadable media from quoted reply, got nil")
	}
	if isVid {
		t.Fatal("expected isVid to be false for image quoted reply")
	}
	if resMime != mime {
		t.Fatalf("expected mime %q, got %q", mime, resMime)
	}
}

func TestExtractTextFromProto(t *testing.T) {
	text := "Hello World"
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	if extracted := ExtractTextFromProto(msg); extracted != text {
		t.Fatalf("expected %q, got %q", text, extracted)
	}

	// Ephemeral unwrapped
	ephemMsg := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: msg,
		},
	}
	if extracted := ExtractTextFromProto(ephemMsg); extracted != text {
		t.Fatalf("expected %q from unwrapped ephemeral, got %q", text, extracted)
	}
}
