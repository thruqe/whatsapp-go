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
	query := req.Query()

	if query == "" {
		sdk.Respond("Usage: weather [city/town]")
		return
	}

	wttrURL := fmt.Sprintf("https://wttr.in/%s?format=4", url.PathEscape(query))
	httpReq, err := http.NewRequest(http.MethodGet, wttrURL, nil)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to construct request: %v", err))
	}
	httpReq.Header.Set("User-Agent", "curl/8.0.0")

	client := sdk.CreateHTTPClient(10)
	resp, err := client.Do(httpReq)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error fetching weather: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("Weather service returned status: %s", resp.Status))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Error reading weather response: %v", err))
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || strings.Contains(trimmed, "Unknown location") {
		sdk.Respond(fmt.Sprintf("Could not find weather info for %q.", query))
		return
	}

	sdk.Respond(trimmed)
}
