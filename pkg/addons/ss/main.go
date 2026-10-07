package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type microLinkResponse struct {
	Data *microLinkData `json:"data"`
}

type microLinkData struct {
	Screenshot *microLinkScreenshot `json:"screenshot"`
}

type microLinkScreenshot struct {
	URL string `json:"url"`
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		if qText := req.QuotedText(); qText != "" {
			query = strings.TrimSpace(qText)
		}
	}

	if query == "" {
		p := req.EffectivePrefix()
		sdk.Respond(fmt.Sprintf(
			"Usage: %sss <URL>\n\nExamples:\n- %sss https://google.com\n- Reply to a message containing a URL with %sss",
			p, p, p,
		))
		return
	}

	tokens := strings.Fields(query)
	targetURL := ""
	if len(tokens) > 0 {
		targetURL = strings.Trim(tokens[0], "<>")
	}

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err == nil && parsed.Host != "" && strings.Contains(parsed.Host, ".") {
		client := sdk.CreateHTTPClient(20)

		// 1. Try thum.io
		thumURL := fmt.Sprintf("https://image.thum.io/get/width/1280/crop/800/noanimate/%s", targetURL)
		if resp, err := client.Get(thumURL); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if bytes, err := io.ReadAll(resp.Body); err == nil && len(bytes) > 5000 {
					dataURL := sdk.ToDataURL("image/jpeg", sdk.EncodeBase64(bytes))
					_ = sdk.SendImage(dataURL, fmt.Sprintf("Screenshot: %s", targetURL))
					return
				}
			}
		}

		// 2. Try microlink.io
		microlinkURL := fmt.Sprintf("https://api.microlink.io/?url=%s&screenshot=true", url.QueryEscape(targetURL))
		if resp, err := client.Get(microlinkURL); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var data microLinkResponse
				if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.Data != nil && data.Data.Screenshot != nil {
					if shotURL := data.Data.Screenshot.URL; shotURL != "" {
						if imgResp, err := client.Get(shotURL); err == nil {
							defer imgResp.Body.Close()
							if imgBytes, err := io.ReadAll(imgResp.Body); err == nil && len(imgBytes) > 5000 {
								dataURL := sdk.ToDataURL("image/jpeg", sdk.EncodeBase64(imgBytes))
								_ = sdk.SendImage(dataURL, fmt.Sprintf("Screenshot: %s", targetURL))
								return
							}
						}
					}
				}
			}
		}

		sdk.RespondErr(fmt.Sprintf("Failed to capture screenshot for `%s`. Please check the URL and try again.", targetURL))
	}

	sdk.RespondErr(fmt.Sprintf("Invalid URL `%s`. Please specify a valid web address (e.g. `https://github.com`).", targetURL))
}
