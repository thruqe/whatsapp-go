package qr

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	// 2. Query 404 for invalid path
	resp404, err := http.Get(baseURL + "/invalid")
	if err == nil {
		resp404.Body.Close()
		if resp404.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for invalid path, got %d", resp404.StatusCode)
		}
	}

	// 3. Query QR PNG before code is set (expect 503)
	respPNG, err := http.Get(baseURL + "/qr.png")
	if err != nil {
		t.Fatalf("failed to query qr.png: %v", err)
	}
	respPNG.Body.Close()
	if respPNG.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 before code set, got %d", respPNG.StatusCode)
	}

	// 4. Test SSE connection
	sseCtx, sseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sseCancel()

	sseReq, _ := http.NewRequestWithContext(sseCtx, "GET", baseURL+"/events", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("failed to connect to SSE /events: %v", err)
	}

	if ct := sseResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}

	// 5. Update code and read SSE event + QR PNG
	srv.UpdateCode("test-qr-data-123456789")

	reader := bufio.NewReader(sseResp.Body)
	line, err := reader.ReadString('\n')
	if err == nil && !strings.Contains(line, "data:") {
		t.Errorf("expected SSE event line starting with data:, got %q", line)
	}
	_ = sseResp.Body.Close()

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

	// 6. Mark as paired and close
	srv.SetPaired()
	time.Sleep(20 * time.Millisecond)

	if err := srv.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// 7. Verify server is stopped and port is released
	time.Sleep(20 * time.Millisecond)
	_, errPostClose := http.Get(baseURL + "/")
	if errPostClose == nil {
		t.Errorf("expected connection error after server close, but request succeeded")
	}
	fmt.Println("Server successfully closed and port released.")
}
