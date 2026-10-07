package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

func main() {
	req := sdk.Load()
	raw := req.Query()

	if raw == "" {
		sdk.Respond("Usage: shorturl [url]")
		return
	}

	targetURL := raw
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		targetURL = "https://" + raw
	}

	encoded := url.QueryEscape(targetURL)
	client := sdk.CreateHTTPClient(10)

	// Try TinyURL first
	tinyURL := fmt.Sprintf("https://tinyurl.com/api-create.php?url=%s", encoded)
	if resp, err := client.Get(tinyURL); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if body, err := io.ReadAll(resp.Body); err == nil {
				short := strings.TrimSpace(string(body))
				if strings.HasPrefix(short, "http://") || strings.HasPrefix(short, "https://") {
					sdk.Respond(fmt.Sprintf("Shortened URL: %s", short))
					return
				}
			}
		}
	}

	// Fallback: is.gd
	isgdURL := fmt.Sprintf("https://is.gd/create.php?format=simple&url=%s", encoded)
	if resp, err := client.Get(isgdURL); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if body, err := io.ReadAll(resp.Body); err == nil {
				short := strings.TrimSpace(string(body))
				if strings.HasPrefix(short, "http://") || strings.HasPrefix(short, "https://") {
					sdk.Respond(fmt.Sprintf("Shortened URL: %s", short))
					return
				}
			}
		}
	}

	sdk.RespondErr("Failed to shorten URL. Please check if the URL is valid.")
}
