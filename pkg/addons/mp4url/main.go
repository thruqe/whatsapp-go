package main

import (
	"fmt"
	"io"
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
		sdk.Respond(fmt.Sprintf(
			"Usage: %smp4url <direct_video_url>\n\nExample: `%smp4url https://example.com/video.mp4`",
			req.EffectivePrefix(),
			req.EffectivePrefix(),
		))
		return
	}

	targetURL := strings.Fields(query)[0]
	targetURL = strings.Trim(targetURL, "<>")

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err == nil && parsed.Host != "" && strings.Contains(parsed.Host, ".") {
		client := sdk.CreateHTTPClient(60)
		resp, err := client.Get(targetURL)
		if err != nil {
			sdk.RespondErr(fmt.Sprintf("Network error fetching video: %s", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			sdk.RespondErr(fmt.Sprintf("Video server returned status %d", resp.StatusCode))
			return
		}

		// Read up to 64 MiB + 1 byte
		maxBytes := int64(64 * 1024 * 1024)
		limited := io.LimitReader(resp.Body, maxBytes+1)
		bytes, err := io.ReadAll(limited)
		if err != nil {
			sdk.RespondErr(fmt.Sprintf("Failed to download video stream: %s", err))
			return
		}

		if len(bytes) == 0 {
			sdk.RespondErr("Downloaded empty video payload.")
			return
		}

		if int64(len(bytes)) > maxBytes {
			sdk.RespondErr(fmt.Sprintf(
				"Video file is too large (%.2f MB). Maximum size is 64 MB.",
				float64(len(bytes))/(1024.0*1024.0),
			))
			return
		}

		dataURL := sdk.ToDataURL("video/mp4", sdk.EncodeBase64(bytes))
		caption := fmt.Sprintf("🎬 Video: %s", targetURL)
		sdk.SendVideo(dataURL, caption)
		return
	}

	sdk.RespondErr(fmt.Sprintf("Invalid video URL `%s`.", targetURL))
}
