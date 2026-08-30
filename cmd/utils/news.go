package cliutils

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	utils "whatsrook/src"
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

	htmlCommentRegex    = regexp.MustCompile(`(?s)<!--.*?-->`)
	apArticleTitleRegex = regexp.MustCompile(`(?s)<h1[^>]*class=["'][^"']*(?:Page-headline|headline)[^"']*["'][^>]*>(.*?)</h1>`)
	apArticleH1Regex    = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	apArticleOgTitle    = regexp.MustCompile(`<meta[^>]*property=["']og:title["'][^>]*content=["']([^"']+)["']`)
	apArticleOgImage    = regexp.MustCompile(`<meta[^>]*property=["']og:image["'][^>]*content=["']([^"']+)["']`)
	apArticleTwImage    = regexp.MustCompile(`<meta[^>]*name=["']twitter:image["'][^>]*content=["']([^"']+)["']`)
	apRichTextBodyRegex = regexp.MustCompile(`(?i)<div[^>]*class=["'][^"']*RichTextStoryBody[^"']*["'][^>]*>`)
	apStoryBodyFallback = regexp.MustCompile(`(?i)<div[^>]*class=["'][^"']*(?:StoryBody|RichTextBody)[^"']*["'][^>]*>`)
	apAsideRegex        = regexp.MustCompile(`(?s)<aside[^>]*>.*?</aside>`)
	apFigureRegex       = regexp.MustCompile(`(?s)<figure[^>]*>.*?</figure>`)
	apNavRegex          = regexp.MustCompile(`(?s)<nav[^>]*>.*?</nav>`)
	apEnhancementRegex  = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*(?:Enhancement|PagePromo|InlinePromo|RelatedStory|RelatedStories|PageListPromo|Page-storyPromos|Page-related|Page-readMore|CardContent)[^"']*["'][^>]*>.*?</div>`)
	apAdBlockRegex      = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*(?:Advertisement|ad-slot|FreeStar|fs-feed-ad|optimizely|Page-ad)[^"']*["'][^>]*>.*?</div>`)
	apNoiseDivRegex     = regexp.MustCompile(`(?s)<div[^>]*class=["'][^"']*(?:Byline|Page-byline|Page-authors|Page-dateModified|Page-published|GoogleFollow|SocialShare|Page-socialShare|ShareBar|ShareList|Page-breadcrumbs|Page-storyFeedback)[^"']*["'][^>]*>.*?</div>`)
	apScriptRegex       = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	apStyleRegex        = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	apOptimizelyRegex   = regexp.MustCompile(`(?s)<div[^>]*id=["'][^"']*(?:optimizely|ad-|freestar)[^"']*["'][^>]*>.*?</div>`)
	apParagraphRegex    = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	apArticleURLRegex   = regexp.MustCompile(`https?://apnews\.com/article/[a-zA-Z0-9_-]+`)
)

type NewsArticle struct {
	Title       string
	Description string
	URL         string
	ImageURL    string
}

type APNewsArticleDetail struct {
	Title    string
	ImageURL string
	Content  string
	URL      string
}

type NewsSession struct {
	Country   string
	Articles  []NewsArticle
	UpdatedAt time.Time
}

var (
	recentNewsSessionsMu sync.RWMutex
	recentNewsSessions   = make(map[string]NewsSession)
)

func SetRecentNewsSession(chat string, country string, articles []NewsArticle) {
	recentNewsSessionsMu.Lock()
	defer recentNewsSessionsMu.Unlock()
	recentNewsSessions[chat] = NewsSession{
		Country:   country,
		Articles:  articles,
		UpdatedAt: time.Now(),
	}
}

func GetRecentNewsSession(chat string) (NewsSession, bool) {
	recentNewsSessionsMu.RLock()
	defer recentNewsSessionsMu.RUnlock()
	sess, ok := recentNewsSessions[chat]
	if !ok || time.Since(sess.UpdatedAt) > 1*time.Hour {
		return NewsSession{}, false
	}
	return sess, true
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
	bodyBytes, err := utils.FetchURLBytes(ctx, hubURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch news: %w", err)
	}

	articles := ParseAPNewsHTML(string(bodyBytes))
	return articles, nil
}

func FetchNewsImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	if imageURL == "" {
		return nil, "", fmt.Errorf("empty image url")
	}

	imgData, err := utils.FetchURLBytes(ctx, imageURL)
	if err != nil {
		return nil, "", err
	}
	if len(imgData) == 0 {
		return nil, "", fmt.Errorf("empty image response")
	}

	mimetype := http.DetectContentType(imgData)
	return imgData, mimetype, nil
}

// ParseAPNewsArticle extracts title, lead image URL, and formatted body paragraphs from AP News article HTML.
func ParseAPNewsArticle(articleHTML, articleURL string) (*APNewsArticleDetail, error) {
	art := &APNewsArticleDetail{
		URL: articleURL,
	}

	art.Title = extractAPArticleTitle(articleHTML)
	art.ImageURL = extractAPArticleImage(articleHTML)
	art.Content = extractAPArticleBody(articleHTML)

	if art.Title == "" && art.Content == "" {
		return nil, fmt.Errorf("failed to extract article content")
	}

	return art, nil
}

func extractAPArticleTitle(htmlContent string) string {
	if m := apArticleTitleRegex.FindStringSubmatch(htmlContent); len(m) > 1 {
		return CleanHTMLText(m[1])
	}
	if m := apArticleH1Regex.FindStringSubmatch(htmlContent); len(m) > 1 {
		return CleanHTMLText(m[1])
	}
	if m := apArticleOgTitle.FindStringSubmatch(htmlContent); len(m) > 1 {
		title := CleanHTMLText(m[1])
		title = strings.TrimSuffix(title, " | AP News")
		title = strings.TrimSuffix(title, " - AP News")
		return title
	}
	return ""
}

func extractAPArticleImage(htmlContent string) string {
	if m := apArticleOgImage.FindStringSubmatch(htmlContent); len(m) > 1 {
		img := html.UnescapeString(m[1])
		if isValidNewsImage(img) {
			return img
		}
	}
	if m := apArticleTwImage.FindStringSubmatch(htmlContent); len(m) > 1 {
		img := html.UnescapeString(m[1])
		if isValidNewsImage(img) {
			return img
		}
	}
	return ""
}

func isValidNewsImage(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	if strings.Contains(lower, "ap_logo") || strings.Contains(lower, "placeholder") || strings.Contains(lower, "favicon") {
		return false
	}
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func stripTagPattern(input string, re *regexp.Regexp) string {
	for range 5 {
		next := re.ReplaceAllString(input, "")
		if next == input {
			return next
		}
		input = next
	}
	return input
}

func extractAPArticleBody(htmlContent string) string {
	cleaned := htmlCommentRegex.ReplaceAllString(htmlContent, "")
	cleaned = apScriptRegex.ReplaceAllString(cleaned, "")
	cleaned = apStyleRegex.ReplaceAllString(cleaned, "")
	cleaned = apAsideRegex.ReplaceAllString(cleaned, "")
	cleaned = apFigureRegex.ReplaceAllString(cleaned, "")
	cleaned = apNavRegex.ReplaceAllString(cleaned, "")
	cleaned = stripTagPattern(cleaned, apEnhancementRegex)
	cleaned = stripTagPattern(cleaned, apAdBlockRegex)
	cleaned = stripTagPattern(cleaned, apNoiseDivRegex)
	cleaned = stripTagPattern(cleaned, apOptimizelyRegex)

	bodyHTML := cleaned
	if loc := apRichTextBodyRegex.FindStringIndex(cleaned); len(loc) >= 2 {
		start := loc[0]
		end := len(cleaned)
		endMarkers := []string{"</article>", "<footer", "class=\"Page-footer", "class=\"Page-breadcrumbs", "class=\"Page-storyFeedback", "<section id=\"search"}
		for _, marker := range endMarkers {
			if idx := strings.Index(cleaned[start:], marker); idx != -1 {
				if start+idx < end {
					end = start + idx
				}
			}
		}
		bodyHTML = cleaned[start:end]
	} else if loc := apStoryBodyFallback.FindStringIndex(cleaned); len(loc) >= 2 {
		start := loc[0]
		end := len(cleaned)
		endMarkers := []string{"</article>", "<footer", "class=\"Page-footer", "class=\"Page-breadcrumbs", "class=\"Page-storyFeedback", "<section id=\"search"}
		for _, marker := range endMarkers {
			if idx := strings.Index(cleaned[start:], marker); idx != -1 {
				if start+idx < end {
					end = start + idx
				}
			}
		}
		bodyHTML = cleaned[start:end]
	}

	matches := apParagraphRegex.FindAllStringSubmatch(bodyHTML, -1)
	var paras []string
	for _, m := range matches {
		if len(m) > 1 {
			txt := CleanHTMLText(m[1])
			txt = strings.ReplaceAll(txt, "\u00a0", " ")
			txt = strings.TrimSpace(txt)
			if isAPNoiseParagraph(txt) {
				continue
			}
			paras = append(paras, txt)
		}
	}

	joined := strings.TrimSpace(strings.Join(paras, "\n\n"))
	joined = regexp.MustCompile(`\n{3,}`).ReplaceAllString(joined, "\n\n")
	return strings.TrimSpace(joined)
}

func isAPNoiseParagraph(txt string) bool {
	txt = strings.TrimSpace(txt)
	if txt == "" {
		return true
	}
	if txt == "____" || txt == "___" || txt == "--" || txt == "---" || txt == "–" || txt == "—" || txt == "-->" || txt == "<!--" {
		return true
	}
	lower := strings.ToLower(txt)
	if strings.EqualFold(txt, "ADVERTISEMENT") || strings.EqualFold(txt, "Share") {
		return true
	}
	if strings.HasPrefix(txt, "By ") && len(txt) < 80 {
		return true
	}
	if strings.HasPrefix(lower, "updated ") && (strings.Contains(lower, "gmt") || strings.Contains(lower, "est") || strings.Contains(lower, "edt") || strings.Contains(lower, "pst") || strings.Contains(lower, "pdt") || strings.Contains(lower, "timezone") || strings.Contains(lower, "am") || strings.Contains(lower, "pm") || strings.Contains(lower, "[hour]")) {
		return true
	}
	if strings.Contains(lower, "add ap news on google") || strings.Contains(lower, "add ap news as your preferred source") {
		return true
	}
	if strings.HasPrefix(lower, "ap decision notes:") || strings.HasPrefix(lower, "ap decision notes") {
		return true
	}
	if strings.HasPrefix(lower, "related:") || strings.HasPrefix(lower, "read more:") || strings.HasPrefix(lower, "related stories:") || strings.HasPrefix(lower, "related story:") || strings.HasPrefix(lower, "related coverage:") || strings.HasPrefix(lower, "more on this topic:") || strings.HasPrefix(lower, "see more:") {
		return true
	}
	if strings.HasPrefix(txt, "Follow AP’s ") || strings.HasPrefix(txt, "Follow AP's ") || strings.HasPrefix(txt, "For more AP ") {
		return true
	}
	if (strings.HasPrefix(txt, "AP’s ") || strings.HasPrefix(txt, "AP's ")) && (strings.Contains(lower, "coverage at:") || strings.Contains(lower, "coverage of")) {
		return true
	}
	return false
}

// FetchAPNewsArticle fetches and parses the full story for an AP News article URL.
func FetchAPNewsArticle(ctx context.Context, articleURL string) (*APNewsArticleDetail, error) {
	if articleURL == "" {
		return nil, fmt.Errorf("empty article url")
	}

	bodyBytes, err := utils.FetchURLBytes(ctx, articleURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch article: %w", err)
	}

	return ParseAPNewsArticle(string(bodyBytes), articleURL)
}

// ExtractAPNewsArticleURLs extracts all unique AP News article URLs from a message text in the order they appear.
func ExtractAPNewsArticleURLs(text string) []string {
	if text == "" {
		return nil
	}
	matches := apArticleURLRegex.FindAllString(text, -1)
	var out []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// ParseNewsSelectionNumber parses article numbers from user input (e.g. "1", "#2", "article 3", ".news 1", etc.).
func ParseNewsSelectionNumber(text string) (int, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, false
	}

	lower := strings.ToLower(trimmed)
	for _, p := range []string{".news", "!news", "/news", "news", "article", "read", "show", "open", "no", "num", "#"} {
		if strings.HasPrefix(lower, p) {
			trimmed = strings.TrimSpace(trimmed[len(p):])
			lower = strings.ToLower(trimmed)
		}
	}

	trimmed = strings.TrimPrefix(trimmed, "#")
	trimmed = strings.TrimSuffix(trimmed, ".")
	trimmed = strings.TrimSpace(trimmed)

	num, err := strconv.Atoi(trimmed)
	if err != nil || num <= 0 || num > 50 {
		return 0, false
	}
	return num, true
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
	// 1. Fetch homepage
	bodyHome, err := utils.FetchURLBytes(ctx, "https://wabetainfo.com/")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch wabetainfo.com homepage: %w", err)
	}

	articleURL := ParseRecentWABetaLink(string(bodyHome))
	if articleURL == "" {
		return nil, fmt.Errorf("failed to find latest article link on wabetainfo.com")
	}

	// 2. Fetch article page
	bodyArt, err := utils.FetchURLBytes(ctx, articleURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch article page: %w", err)
	}

	return ParseWABetaArticle(string(bodyArt), articleURL)
}
