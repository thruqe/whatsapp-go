package src

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FetchURLBytes fetches the content of a target URL over HTTP GET.
func FetchURLBytes(ctx context.Context, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d fetching URL", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
