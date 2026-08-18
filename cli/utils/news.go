package cliutils

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	promoRegex       = regexp.MustCompile(`(?s)<div class="PagePromo"[^>]*>(.*?)</div>\s*</div>\s*</div>`)
	linkRegex        = regexp.MustCompile(`href="(https?://apnews\.com/article/[^"]+|/article/[^"]+)"`)
	titleRegex       = regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)
	altTitleRegex    = regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<a[^>]*>(.*?)</a>`)
	descRegex        = regexp.MustCompile(`(?s)<div class="PagePromo-description"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)
	imageSrcsetRegex = regexp.MustCompile(`srcset="(https://dims\.apnews\.com/dims4/[^" ]+|https://assets\.apnews\.com/[^" ]+)"`)
	imageSrcRegex    = regexp.MustCompile(`src="(https://dims\.apnews\.com/dims4/[^"]+|https://assets\.apnews\.com/[^"]+)"`)
	stripTagsRegex   = regexp.MustCompile(`<[^>]*>`)
)

type NewsArticle struct {
	Title       string
	Description string
	URL         string
	ImageURL    string
}

func CleanHTMLText(input string) string {
	cleaned := stripTagsRegex.ReplaceAllString(input, "")
	cleaned = html.UnescapeString(cleaned)
	return strings.TrimSpace(cleaned)
}

func ParseAPNewsHTML(htmlContent string) []NewsArticle {
	var articles []NewsArticle
	seenURLs := make(map[string]bool)

	matches := promoRegex.FindAllString(htmlContent, -1)
	for _, match := range matches {
		var art NewsArticle

		if linkMatch := linkRegex.FindStringSubmatch(match); len(linkMatch) > 1 {
			art.URL = linkMatch[1]
			if strings.HasPrefix(art.URL, "/") {
				art.URL = "https://apnews.com" + art.URL
			}
		}
		if art.URL == "" || seenURLs[art.URL] {
			continue
		}

		if titleMatch := titleRegex.FindStringSubmatch(match); len(titleMatch) > 1 {
			art.Title = CleanHTMLText(titleMatch[1])
		} else if altMatch := altTitleRegex.FindStringSubmatch(match); len(altMatch) > 1 {
			art.Title = CleanHTMLText(altTitleRegex.FindStringSubmatch(match)[1])
		}
		if art.Title == "" {
			continue
		}

		if descMatch := descRegex.FindStringSubmatch(match); len(descMatch) > 1 {
			art.Description = CleanHTMLText(descMatch[1])
		}

		if imgMatch := imageSrcsetRegex.FindStringSubmatch(match); len(imgMatch) > 1 {
			art.ImageURL = html.UnescapeString(imgMatch[1])
		} else if srcMatch := imageSrcRegex.FindStringSubmatch(match); len(srcMatch) > 1 {
			art.ImageURL = html.UnescapeString(srcMatch[1])
		}

		seenURLs[art.URL] = true
		articles = append(articles, art)
	}

	return articles
}

func FetchAPNews(ctx context.Context, country string) ([]NewsArticle, error) {
	hubURL := fmt.Sprintf("https://apnews.com/hub/%s", country)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hubURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch news: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	articles := ParseAPNewsHTML(string(bodyBytes))
	return articles, nil
}

func FetchNewsImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	if imageURL == "" {
		return nil, "", fmt.Errorf("empty image url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil || len(imgData) == 0 {
		return nil, "", fmt.Errorf("empty image response")
	}

	mimetype := http.DetectContentType(imgData)
	return imgData, mimetype, nil
}
