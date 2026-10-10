package qr

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRenderTerminal(t *testing.T) {
	str := RenderTerminal("test-code")
	if str == "" {
		t.Fatal("expected non-empty terminal QR string")
	}
}

func TestServer(t *testing.T) {
	s, err := StartServer()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Close()

	if s.Port() <= 0 {
		t.Fatalf("expected valid port, got %d", s.Port())
	}
	if s.URL() == "" {
		t.Fatal("expected non-empty URL")
	}

	// Test index page returns clean HTML with only the QR image and script
	resp, err := http.Get(s.URL() + "/")
	if err != nil {
		t.Fatalf("failed to fetch index: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, `<img id="qr" src="/qr.png" alt="QR Code">`) {
		t.Fatalf("expected qr image element, got %s", content)
	}
	if strings.Contains(content, "WhatsRook") || strings.Contains(content, "How to connect") {
		t.Fatalf("expected minimal QR page with no headers or instructions, got: %s", content)
	}

	// Before code is ready
	pngResp, err := http.Get(s.URL() + "/qr.png")
	if err != nil {
		t.Fatalf("failed to fetch qr.png: %v", err)
	}
	pngResp.Body.Close()
	if pngResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before code is ready, got %d", pngResp.StatusCode)
	}

	// Update code
	s.UpdateCode("sample-qr-code")

	// After code is ready
	pngResp2, err := http.Get(s.URL() + "/qr.png")
	if err != nil {
		t.Fatalf("failed to fetch qr.png after update: %v", err)
	}
	defer pngResp2.Body.Close()
	if pngResp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for qr.png, got %d", pngResp2.StatusCode)
	}
	if ct := pngResp2.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected image/png content type, got %s", ct)
	}

	// Test events endpoint
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL()+"/events", nil)
	evtResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to /events: %v", err)
	}
	defer evtResp.Body.Close()
	if ct := evtResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	s.SetPaired()
	time.Sleep(50 * time.Millisecond)
}
