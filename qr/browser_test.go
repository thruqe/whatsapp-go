package qr

import (
	"testing"
)

func TestOpenBrowser(t *testing.T) {
	// Verify OpenBrowser executes cleanly without panicking.
	// In headless or CI environments without GUI desktop, it returns an error or succeeds starting the command,
	// which is handled gracefully.
	_ = OpenBrowser("http://127.0.0.1:8080")
}

func TestServerOpenBrowser(t *testing.T) {
	srv, err := StartServer()
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer srv.Close()

	_ = srv.OpenBrowser()
}
