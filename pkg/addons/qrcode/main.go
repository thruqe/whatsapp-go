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
		if qText := req.QuotedText(); qText != "" {
			query = strings.TrimSpace(qText)
		}
	}

	if query == "" {
		sdk.Respond(fmt.Sprintf("Usage: %sqr <text or url> (or reply to a message)", req.EffectivePrefix()))
		return
	}

	encoded := url.QueryEscape(query)
	qrURL := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=600x600&margin=15&data=%s", encoded)

	client := sdk.CreateHTTPClient(15)
	resp, err := client.Get(qrURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if bytes, err := io.ReadAll(resp.Body); err == nil && len(bytes) > 0 {
				dataURL := sdk.ToDataURL("image/png", sdk.EncodeBase64(bytes))
				_ = sdk.SendImage(dataURL, "QR Code Generated")
				return
			}
		}
	}

	// Fallback to quickchart
	fallbackURL := fmt.Sprintf("https://quickchart.io/qr?size=600&margin=2&text=%s", encoded)
	if resp, err := client.Get(fallbackURL); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if bytes, err := io.ReadAll(resp.Body); err == nil && len(bytes) > 0 {
				dataURL := sdk.ToDataURL("image/png", sdk.EncodeBase64(bytes))
				_ = sdk.SendImage(dataURL, "QR Code Generated")
				return
			}
		}
	}

	sdk.RespondErr("Network error generating QR code.")
}
