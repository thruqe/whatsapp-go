package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"whatsrook/pkg/addons/sdk"
)

type article struct {
	title       string
	description string
	url         string
}

var tagRegex = regexp.MustCompile(`<[^>]*>`)

func cleanHTML(text string) string {
	stripped := tagRegex.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(stripped))
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		p := req.EffectivePrefix()
		sdk.Respond(fmt.Sprintf(
			"*AP News Country Headlines*\n\nUsage:\n• %snews <country> (e.g. %snews nigeria, %snews japan, %snews usa, %snews uk)",
			p, p, p, p, p,
		))
		return
	}

	country := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(query)), " ", "-")
	hubURL := fmt.Sprintf("https://apnews.com/hub/%s", country)

	client := sdk.CreateHTTPClient(15)
	resp, err := client.Get(hubURL)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error fetching news: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		sdk.Respond(fmt.Sprintf("No news topic hub found for %q.", query))
		return
	}
	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("AP News returned status: %s", resp.Status))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to read news response: %v", err))
	}

	htmlContent := string(body)

	promoRegex := regexp.MustCompile(`(?s)<div class="PagePromo"[^>]*>(.*?)</div>\s*</div>\s*</div>`)
	linkRegex := regexp.MustCompile(`href="(https?://apnews\.com/article/[^"]+|/article/[^"]+)"`)
	titleRegex := regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)
	altTitleRegex := regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<a[^>]*>(.*?)</a>`)
	descRegex := regexp.MustCompile(`(?s)<div class="PagePromo-description"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)

	var articles []article
	seen := make(map[string]bool)

	matches := promoRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range matches {
		block := match[1]
		var artURL string
		if linkMatch := linkRegex.FindStringSubmatch(block); len(linkMatch) > 1 {
			artURL = linkMatch[1]
			if strings.HasPrefix(artURL, "/") {
				artURL = "https://apnews.com" + artURL
			}
		}
		if artURL == "" || seen[artURL] {
			continue
		}

		var title string
		if tMatch := titleRegex.FindStringSubmatch(block); len(tMatch) > 1 {
			title = cleanHTML(tMatch[1])
		} else if altMatch := altTitleRegex.FindStringSubmatch(block); len(altMatch) > 1 {
			title = cleanHTML(altMatch[1])
		}
		if title == "" {
			continue
		}

		var desc string
		if dMatch := descRegex.FindStringSubmatch(block); len(dMatch) > 1 {
			desc = cleanHTML(dMatch[1])
		}

		seen[artURL] = true
		articles = append(articles, article{
			title:       title,
			description: desc,
			url:         artURL,
		})

		if len(articles) >= 5 {
			break
		}
	}

	if len(articles) == 0 {
		sdk.Respond(fmt.Sprintf("No recent news articles found for %q.", query))
		return
	}

	displayCountry := titleCase(strings.ReplaceAll(country, "-", " "))
	var out strings.Builder
	fmt.Fprintf(&out, "*AP News - %s*\n\n", displayCountry)
	for i, art := range articles {
		fmt.Fprintf(&out, "%d. *%s*\n", i+1, art.title)
		if art.description != "" {
			fmt.Fprintf(&out, "   %s\n", art.description)
		}
		fmt.Fprintf(&out, "   %s\n\n", art.url)
	}

	sdk.Respond(strings.TrimSpace(out.String()))
}
