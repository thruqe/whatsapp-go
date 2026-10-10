package send

import (
	"strings"
	"testing"
)

func TestBuildWebviewPayload(t *testing.T) {
	target := "https://example.com/test"
	payload, err := BuildWebviewPayload("Open In-App Webview", target)
	if err != nil {
		t.Fatalf("unexpected error building payload: %v", err)
	}

	if !strings.Contains(payload, `"webview_presentation":"full"`) {
		t.Errorf("expected full presentation, got: %s", payload)
	}
	if !strings.Contains(payload, `"webview_interaction":true`) {
		t.Errorf("expected webview_interaction true, got: %s", payload)
	}
	if !strings.Contains(payload, target) {
		t.Errorf("expected target URL in payload, got: %s", payload)
	}
}

func TestBuildWebviewMessage(t *testing.T) {
	opts := WebviewMessageOpts{
		Title:      "🎮 Test Title",
		Body:       "Test body text",
		Footer:     "Test footer",
		ButtonText: "Open In-App Webview",
		URL:        "https://example.com/portal",
	}

	msg, err := BuildWebviewMessage(opts)
	if err != nil {
		t.Fatalf("unexpected error building message: %v", err)
	}
	if msg == nil || msg.InteractiveMessage == nil {
		t.Fatalf("expected valid direct InteractiveMessage")
	}
	if msg.MessageContextInfo == nil || msg.MessageContextInfo.GetWeblinkRenderConfig() != 0 {
		t.Errorf("expected WeblinkRenderConfig_WEBVIEW (0), got: %v", msg.MessageContextInfo)
	}

	im := msg.InteractiveMessage
	if im.GetHeader().GetTitle() != opts.Title {
		t.Errorf("expected title %q, got %q", opts.Title, im.GetHeader().GetTitle())
	}
	if im.GetBody().GetText() != opts.Body {
		t.Errorf("expected body %q, got %q", opts.Body, im.GetBody().GetText())
	}
	if im.GetFooter().GetText() != opts.Footer {
		t.Errorf("expected footer %q, got %q", opts.Footer, im.GetFooter().GetText())
	}

	nf := im.GetNativeFlowMessage()
	if nf == nil || len(nf.Buttons) != 1 {
		t.Fatalf("expected 1 button, got: %+v", nf)
	}
	if *nf.Buttons[0].Name != "cta_url" {
		t.Errorf("expected button name 'cta_url', got %s", *nf.Buttons[0].Name)
	}
	if !strings.Contains(*nf.Buttons[0].ButtonParamsJSON, opts.URL) {
		t.Errorf("expected button params to contain URL %s, got: %s", opts.URL, *nf.Buttons[0].ButtonParamsJSON)
	}
}
