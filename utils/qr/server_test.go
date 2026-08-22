package qr

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestQRServerLifecycle(t *testing.T) {
	srv, err := StartServer()
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer srv.Close()

	if srv.Port() <= 0 {
		t.Errorf("expected positive port, got %d", srv.Port())
	}
	if srv.URL() == "" {
		t.Errorf("expected non-empty URL")
	}

	baseURL := srv.URL()

	// 1. Query index HTML
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("failed to query index: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if len(body) == 0 {
		t.Errorf("expected non-empty HTML body")
	}

	// 2. Query QR PNG before code is set (expect 503)
	respPNG, err := http.Get(baseURL + "/qr.png")
	if err != nil {
		t.Fatalf("failed to query qr.png: %v", err)
	}
	respPNG.Body.Close()
	if respPNG.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 before code set, got %d", respPNG.StatusCode)
	}

	// 3. Set code and query QR PNG (expect 200 with image/png)
	srv.UpdateCode("test-qr-data-123456789")
	respPNG2, err := http.Get(baseURL + "/qr.png")
	if err != nil {
		t.Fatalf("failed to query qr.png after update: %v", err)
	}
	defer respPNG2.Body.Close()
	if respPNG2.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 after update, got %d", respPNG2.StatusCode)
	}
	if ct := respPNG2.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %q", ct)
	}

	// 4. Mark as paired and close
	srv.SetPaired()
	time.Sleep(50 * time.Millisecond)

	if err := srv.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// 5. Verify server is stopped and port is released
	time.Sleep(50 * time.Millisecond)
	_, errPostClose := http.Get(baseURL + "/")
	if errPostClose == nil {
		t.Errorf("expected connection error after server close, but request succeeded")
	}
	fmt.Println("Server successfully closed and port released.")
}
