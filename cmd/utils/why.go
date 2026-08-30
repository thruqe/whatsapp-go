package cliutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	utils "whatsrook/src"
)

const DefaultWhyAPIURL = "https://why.com/api/ultimate-search"

// WhyAPIURL allows custom URL override (e.g. for testing).
var WhyAPIURL = DefaultWhyAPIURL

// WhyRequest represents the payload sent to why.com's ultimate-search API.
type WhyRequest struct {
	Action string `json:"action"`
	Query  string `json:"query"`
}

// WhyPull represents a suggested follow-up question or related exploration topic.
type WhyPull struct {
	Role      string `json:"role,omitempty"`
	Lens      string `json:"lens,omitempty"`
	Heat      int    `json:"heat,omitempty"`
	Label     string `json:"label,omitempty"`
	Query     string `json:"query,omitempty"`
	Anchor    string `json:"anchor,omitempty"`
	Gap       string `json:"gap,omitempty"`
	Grounding string `json:"grounding,omitempty"`
}

// WhyResponse represents the response returned by why.com's ultimate-search API.
type WhyResponse struct {
	Answer string    `json:"answer"`
	Prompt string    `json:"prompt,omitempty"`
	Pulls  []WhyPull `json:"pulls,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// QueryWhy performs a POST request to https://why.com/api/ultimate-search to retrieve an answer and follow-up pulls.
func QueryWhy(ctx context.Context, query string) (*WhyResponse, error) {
	reqBody, err := json.Marshal(WhyRequest{
		Action: "answer",
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	targetURL := WhyAPIURL
	if targetURL == "" {
		targetURL = DefaultWhyAPIURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:153.0) Gecko/20100101 Firefox/153.0")
	req.Header.Set("Origin", "https://why.com")
	req.Header.Set("Referer", "https://why.com/")
	req.Header.Set("Accept", "*/*")

	client := utils.NewHTTPClient(35 * time.Second)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach why.com API: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("why.com API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("why.com returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result WhyResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("why.com error: %s", result.Error)
	}

	return &result, nil
}
