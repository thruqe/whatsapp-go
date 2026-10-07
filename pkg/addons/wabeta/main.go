package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

var tagRegex = regexp.MustCompile(`<[^>]*>`)

func cleanHTML(text string) string {
	stripped := tagRegex.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(stripped))
}

func main() {
	_ = sdk.Load()
	client := sdk.CreateHTTPClient(15)

	resp, err := client.Get("https://wabetainfo.com/")
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error reaching WABetaInfo: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("WABetaInfo returned status: %s", resp.Status))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to read WABetaInfo homepage: %v", err))
	}
	homeHTML := string(body)

	postLinkRegex := regexp.MustCompile(`<a[^>]*href=["'](https?://wabetainfo\.com/[^"'/]+/)["']`)
	matches := postLinkRegex.FindAllStringSubmatch(homeHTML, -1)
	var articleURL string
	for _, m := range matches {
		u := m[1]
		if !strings.Contains(u, "/android/") &&
			!strings.Contains(u, "/ios/") &&
			!strings.Contains(u, "/news/") &&
			!strings.Contains(u, "/about/") &&
			!strings.Contains(u, "/disclaimer/") &&
			!strings.Contains(u, "/privacy-policy/") &&
			!strings.Contains(u, "/contact-us/") &&
			!strings.Contains(u, "/page/") {
			articleURL = u
			break
		}
	}

	if articleURL == "" {
		sdk.RespondErr("Failed to extract latest article link from WABetaInfo.")
	}

	artResp, err := client.Get(articleURL)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to fetch article: %v", err))
	}
	defer artResp.Body.Close()

	if artResp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("Article page returned status: %s", artResp.Status))
	}

	artBody, err := io.ReadAll(artResp.Body)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to read article content: %v", err))
	}
	articleHTML := string(artBody)

	titleRegex := regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*entry-title[^"']*["'][^>]*>.*?<h1[^>]*>(.*?)</h1>`)
	h1Fallback := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)

	title := "WABetaInfo Update"
	if tMatch := titleRegex.FindStringSubmatch(articleHTML); len(tMatch) > 1 {
		title = cleanHTML(tMatch[1])
	} else if hMatch := h1Fallback.FindStringSubmatch(articleHTML); len(hMatch) > 1 {
		title = cleanHTML(hMatch[1])
	}

	contentBlock := articleHTML
	if idx := strings.Index(articleHTML, "entry-content"); idx != -1 {
		contentBlock = articleHTML[idx:]
	}

	pRegex := regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	var paragraphs []string
	pMatches := pRegex.FindAllStringSubmatch(contentBlock, -1)
	for _, pm := range pMatches {
		txt := cleanHTML(pm[1])
		if len(txt) > 30 &&
			!strings.EqualFold(txt, "ADVERTISEMENT") &&
			!strings.HasPrefix(txt, "Connect with WABetaInfo") &&
			!strings.HasPrefix(txt, "Follow us on") {
			paragraphs = append(paragraphs, txt)
			if len(paragraphs) >= 6 {
				break
			}
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "*%s*\n\n", title)
	if len(paragraphs) > 0 {
		out.WriteString(strings.Join(paragraphs, "\n\n"))
	} else {
		out.WriteString("Read more at: " + articleURL)
	}

	sdk.Respond(strings.TrimSpace(out.String()))
}
