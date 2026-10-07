package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type whyRequest struct {
	Action string `json:"action"`
	Query  string `json:"query"`
}

type whyResponse struct {
	Answer string    `json:"answer"`
	Pulls  []whyPull `json:"pulls"`
	Error  string    `json:"error"`
}

type whyPull struct {
	Label string `json:"label"`
	Query string `json:"query"`
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond("*Why.com AI Search*\n\nUsage:\n• why <question or topic>")
		return
	}

	payload := whyRequest{
		Action: "answer",
		Query:  query,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to marshal request: %v", err))
	}

	httpReq, err := http.NewRequest(http.MethodPost, "https://why.com/api/ultimate-search", bytes.NewReader(payloadBytes))
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to create request: %v", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Origin", "https://why.com")
	httpReq.Header.Set("Referer", "https://why.com/")

	client := sdk.CreateHTTPClient(30)
	resp, err := client.Do(httpReq)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error contacting why.com: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("Why.com API returned status: %s", resp.Status))
	}

	var data whyResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to parse Why.com response: %v", err))
	}

	if data.Error != "" {
		sdk.RespondErr(fmt.Sprintf("Why.com error: %s", data.Error))
	}

	answer := strings.TrimSpace(data.Answer)
	if answer == "" {
		sdk.Respond("No answer found.")
		return
	}

	var out strings.Builder
	fmt.Fprintf(&out, "💡 *Why.com Analysis*\n\n%s", answer)

	var count int
	for _, pull := range data.Pulls {
		text := pull.Label
		if text == "" {
			text = pull.Query
		}
		if text != "" {
			if count == 0 {
				out.WriteString("\n\n*Related Explorations:*")
			}
			fmt.Fprintf(&out, "\n• %s", text)
			count++
			if count >= 3 {
				break
			}
		}
	}

	sdk.Respond(out.String())
}
