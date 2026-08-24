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

var (
	recentPostsSectionRegex = regexp.MustCompile(`(?s)<section[^>]*id=["']recent-posts-2["'][^>]*>(.*?)</section>`)
	recentPostLinkRegex     = regexp.MustCompile(`<a[^>]*href=["'](https?://wabetainfo\.com/[^"'/]+/)["']`)
	fallbackPostLinkRegex   = regexp.MustCompile(`(?s)<h2[^>]*class=["'][^"']*entry-title[^"']*["'][^>]*>.*?<a[^>]*href=["'](https?://wabetainfo\.com/[^"'/]+/)["']`)

	articleTitleRegex = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*entry-title[^"']*["'][^>]*>.*?<h1[^>]*>(.*?)</h1>`)
	h1FallbackRegex   = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)

	metaImageRegex  = regexp.MustCompile(`<meta[^>]*itemprop=["']image["'][^>]*content=["']([^"']+)["']`)
	ogImageRegex    = regexp.MustCompile(`<meta[^>]*property=["']og:image["'][^>]*content=["']([^"']+)["']`)
	contentImgRegex = regexp.MustCompile(`<img[^>]*src=["'](https?://wabetainfo\.com/wp-content/uploads/[^"']+)["']`)

	quadsAdRegex        = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*quads-location[^"']*["'][^>]*>.*?</div>`)
	jetpackWidgetRegex  = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*jetpack_subscription_widget[^"']*["'][^>]*>.*?</div>`)
	scriptTagRegex      = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	styleTagRegex       = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	tableRegex          = regexp.MustCompile(`(?s)<table[^>]*>(.*?)</table>`)
	h2Regex             = regexp.MustCompile(`(?s)<h2[^>]*>(.*?)</h2>`)
	pRegex              = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	moreSpanRegex       = regexp.MustCompile(`<span[^>]*id=["']more-\d+["'][^>]*></span>`)
	connectSectionRegex = regexp.MustCompile(`(?s)<h2>\s*Connect with WABetaInfo\s*</h2>.*$`)
	adTextRegex         = regexp.MustCompile(`(?i)\bADVERTISEMENT\b`)
)

type WABetaArticle struct {
	Title    string
	ImageURL string
	Content  string
	URL      string
}

// ParseRecentWABetaLink extracts the latest article URL from wabetainfo.com homepage HTML.
func ParseRecentWABetaLink(htmlContent string) string {
	if section := recentPostsSectionRegex.FindStringSubmatch(htmlContent); len(section) > 1 {
		links := recentPostLinkRegex.FindAllStringSubmatch(section[1], -1)
		for _, link := range links {
			if len(link) > 1 && isValidWABetaArticleURL(link[1]) {
				return link[1]
			}
		}
	}

	fallbackLinks := fallbackPostLinkRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, link := range fallbackLinks {
		if len(link) > 1 && isValidWABetaArticleURL(link[1]) {
			return link[1]
		}
	}

	return ""
}

func isValidWABetaArticleURL(url string) bool {
	if url == "" {
		return false
	}
	ignored := []string{
		"/android/", "/ios/", "/news/", "/about/", "/disclaimer/",
		"/privacy-policy/", "/contact-us/", "/page/",
	}
	for _, ign := range ignored {
		if strings.Contains(url, ign) {
			return false
		}
	}
	return strings.HasPrefix(url, "https://wabetainfo.com/")
}

// ParseWABetaArticle extracts title, featured/content image URL, and formatted content from article HTML.
func ParseWABetaArticle(articleHTML, articleURL string) (*WABetaArticle, error) {
	art := &WABetaArticle{
		URL: articleURL,
	}

	// 1. Extract Title
	if titleMatch := articleTitleRegex.FindStringSubmatch(articleHTML); len(titleMatch) > 1 {
		art.Title = CleanHTMLText(titleMatch[1])
	} else if h1Match := h1FallbackRegex.FindStringSubmatch(articleHTML); len(h1Match) > 1 {
		art.Title = CleanHTMLText(h1Match[1])
	}

	// 2. Extract Image URL
	if imgMatch := contentImgRegex.FindStringSubmatch(articleHTML); len(imgMatch) > 1 {
		art.ImageURL = html.UnescapeString(imgMatch[1])
	} else if metaMatch := metaImageRegex.FindStringSubmatch(articleHTML); len(metaMatch) > 1 {
		img := html.UnescapeString(metaMatch[1])
		if !strings.Contains(img, "WBI_LOGO") && !strings.Contains(img, "WA_WBI_LOGO") {
			art.ImageURL = img
		}
	} else if ogMatch := ogImageRegex.FindStringSubmatch(articleHTML); len(ogMatch) > 1 {
		img := html.UnescapeString(ogMatch[1])
		if !strings.Contains(img, "WBI_LOGO") && !strings.Contains(img, "WA_WBI_LOGO") {
			art.ImageURL = img
		}
	}

	// 3. Extract Content from article body
	rawContent := extractKentaArticleContent(articleHTML)
	art.Content = formatWABetaContent(rawContent, art.Title)

	if art.Title == "" && art.Content == "" {
		return nil, fmt.Errorf("failed to extract article content")
	}

	return art, nil
}

var articleDivRegex = regexp.MustCompile(`(?i)<div[^>]*class=["'][^"']*(?:kenta-article-content|entry-content)[^"']*["'][^>]*>`)

func extractKentaArticleContent(articleHTML string) string {
	loc := articleDivRegex.FindStringIndex(articleHTML)
	if len(loc) < 2 {
		return articleHTML
	}
	start := loc[1]

	endMarkers := []string{"</article>", "class=\"kenta-sidebar", "<footer", "id=\"sidebar", "<section id=\"search"}
	end := len(articleHTML)
	for _, marker := range endMarkers {
		if mIdx := strings.Index(articleHTML[start:], marker); mIdx != -1 {
			if start+mIdx < end {
				end = start + mIdx
			}
		}
	}
	return articleHTML[start:end]
}

func formatWABetaContent(rawHTML string, title string) string {
	// Strip ads, forms, tables, scripts, widgets, and social footers
	cleaned := tableRegex.ReplaceAllString(rawHTML, "")
	cleaned = quadsAdRegex.ReplaceAllString(cleaned, "")
	cleaned = jetpackWidgetRegex.ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`(?s)<form[^>]*>.*?</form>`).ReplaceAllString(cleaned, "")
	cleaned = scriptTagRegex.ReplaceAllString(cleaned, "")
	cleaned = styleTagRegex.ReplaceAllString(cleaned, "")
	cleaned = moreSpanRegex.ReplaceAllString(cleaned, "")
	cleaned = connectSectionRegex.ReplaceAllString(cleaned, "")
	cleaned = adTextRegex.ReplaceAllString(cleaned, "")

	isWeeklyRoundup := strings.Contains(rawHTML, "wa-weekly-roundup") ||
		strings.Contains(rawHTML, "Weekly WhatsApp beta updates") ||
		strings.Contains(strings.ToLower(title), "roundup")

	var paragraphs []string

	if isWeeklyRoundup {
		// In a weekly roundup, extract:
		// 1) The main feature intro at top (before any <h2> or widget)
		// 2) The specific <h2> section matching the article title
		// Skip all other weekly roundup summaries
		h2Matches := h2Regex.FindAllStringIndex(cleaned, -1)
		if len(h2Matches) > 0 {
			topIntroHTML := cleaned[:h2Matches[0][0]]
			for _, p := range extractParagraphsFromHTML(topIntroHTML) {
				if len(p) > 20 && !strings.HasPrefix(p, "ADVERTISEMENT") {
					paragraphs = append(paragraphs, p)
				}
			}

			// Search matching <h2> section
			for i, match := range h2Matches {
				secStart := match[0]
				var secEnd int
				if i+1 < len(h2Matches) {
					secEnd = h2Matches[i+1][0]
				} else {
					secEnd = len(cleaned)
				}

				secHTML := cleaned[secStart:secEnd]
				h2Sub := h2Regex.FindStringSubmatch(secHTML)
				if len(h2Sub) > 1 {
					headingText := CleanHTMLText(h2Sub[1])
					if isMatchingHeading(headingText, title) {
						for _, p := range extractParagraphsFromHTML(secHTML) {
							if len(p) > 20 && !strings.HasPrefix(p, "ADVERTISEMENT") {
								// Avoid duplicate intro paragraph
								if !containsParagraph(paragraphs, p) {
									paragraphs = append(paragraphs, p)
								}
							}
						}
					}
				}
			}
		} else {
			paragraphs = extractParagraphsFromHTML(cleaned)
		}
	} else {
		// Single-topic article: format headings and paragraphs
		h2Matches := h2Regex.FindAllStringIndex(cleaned, -1)
		if len(h2Matches) > 0 {
			topIntroHTML := cleaned[:h2Matches[0][0]]
			for _, p := range extractParagraphsFromHTML(topIntroHTML) {
				if len(p) > 20 && !strings.HasPrefix(p, "ADVERTISEMENT") {
					paragraphs = append(paragraphs, p)
				}
			}

			for i, match := range h2Matches {
				secStart := match[0]
				var secEnd int
				if i+1 < len(h2Matches) {
					secEnd = h2Matches[i+1][0]
				} else {
					secEnd = len(cleaned)
				}

				secHTML := cleaned[secStart:secEnd]
				h2Sub := h2Regex.FindStringSubmatch(secHTML)
				if len(h2Sub) > 1 {
					headingText := CleanHTMLText(h2Sub[1])
					if !strings.EqualFold(headingText, "Connect with WABetaInfo") {
						paragraphs = append(paragraphs, "*"+headingText+"*")
						for _, p := range extractParagraphsFromHTML(secHTML) {
							if len(p) > 20 && !strings.HasPrefix(p, "ADVERTISEMENT") {
								paragraphs = append(paragraphs, p)
							}
						}
					}
				}
			}
		} else {
			paragraphs = extractParagraphsFromHTML(cleaned)
		}
	}

	return strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
}

func extractParagraphsFromHTML(htmlSnippet string) []string {
	var paras []string
	matches := pRegex.FindAllStringSubmatch(htmlSnippet, -1)
	for _, m := range matches {
		if len(m) > 1 {
			txt := CleanHTMLText(m[1])
			if txt != "" && !strings.EqualFold(txt, "ADVERTISEMENT") {
				paras = append(paras, txt)
			}
		}
	}
	return paras
}

func containsParagraph(paras []string, target string) bool {
	for _, p := range paras {
		if p == target || strings.Contains(p, target) || strings.Contains(target, p) {
			return true
		}
	}
	return false
}

func isMatchingHeading(heading, title string) bool {
	hNorm := strings.ToLower(CleanHTMLText(heading))
	tNorm := strings.ToLower(CleanHTMLText(title))

	if hNorm == tNorm {
		return true
	}
	if strings.Contains(hNorm, "weekly") || strings.Contains(hNorm, "roundup") || strings.Contains(hNorm, "connect with") {
		return false
	}

	tWords := strings.Fields(tNorm)
	matchCount := 0
	meaningfulWords := 0
	for _, w := range tWords {
		wClean := strings.Trim(w, ".,!?:;\"'")
		if len(wClean) >= 4 && !isWABetaStopWord(wClean) {
			meaningfulWords++
			if strings.Contains(hNorm, wClean) {
				matchCount++
			}
		}
	}

	if meaningfulWords > 0 && float64(matchCount)/float64(meaningfulWords) >= 0.5 {
		return true
	}
	return false
}

func isWABetaStopWord(w string) bool {
	switch w {
	case "whatsapp", "feature", "with", "that", "this", "from", "into", "over", "more", "also", "have", "been", "beta":
		return true
	default:
		return false
	}
}

// FetchWABetaLatest fetches the wabetainfo.com homepage, extracts the latest article URL, and parses the article.
func FetchWABetaLatest(ctx context.Context) (*WABetaArticle, error) {
	client := &http.Client{Timeout: 20 * time.Second}

	// 1. Fetch homepage
	reqHome, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://wabetainfo.com/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create homepage request: %w", err)
	}
	reqHome.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	respHome, err := client.Do(reqHome)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch wabetainfo.com homepage: %w", err)
	}
	defer respHome.Body.Close()

	if respHome.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("homepage returned http %d", respHome.StatusCode)
	}

	bodyHome, err := io.ReadAll(respHome.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read homepage body: %w", err)
	}

	articleURL := ParseRecentWABetaLink(string(bodyHome))
	if articleURL == "" {
		return nil, fmt.Errorf("failed to find latest article link on wabetainfo.com")
	}

	// 2. Fetch article page
	reqArt, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create article request: %w", err)
	}
	reqArt.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	respArt, err := client.Do(reqArt)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch article page: %w", err)
	}
	defer respArt.Body.Close()

	if respArt.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("article page returned http %d", respArt.StatusCode)
	}

	bodyArt, err := io.ReadAll(respArt.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read article body: %w", err)
	}

	return ParseWABetaArticle(string(bodyArt), articleURL)
}
