package test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicPageURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tunnel test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	u, err := publicPageURL(ctx)
	if err != nil {
		t.Fatalf("expected public page URL, got error: %v", err)
	}

	if !strings.HasPrefix(u, "https://") || !strings.Contains(u, "trycloudflare.com") {
		t.Fatalf("unexpected URL format: %s", u)
	}

	// Use 1.1.1.1 resolver for newly registered Cloudflare tunnel hostname
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", "1.1.1.1:53")
		},
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:  5 * time.Second,
			Resolver: resolver,
		}).DialContext,
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	// Wait briefly for Cloudflare DNS edge propagation
	var resp *http.Response
	for i := range 15 {
		time.Sleep(1 * time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		if i == 14 {
			t.Fatalf("fetch tunnel URL after retries: %v", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", resp.StatusCode)
	}
}
