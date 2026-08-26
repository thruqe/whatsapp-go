package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	cliutils "whatsrook/cmd/utils"
	"whatsrook/logger"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "btc",
		Alias:       "bitcoin",
		Description: "Live real-time Bitcoin price and halving countdown tracker",
		Category:    "news",
		IsPublic:    true,
		Handler:     handleBTC,
	})

	Register(&Command{
		Name:        "markets",
		Alias:       "fx",
		Description: "View real-time Forex Factory currency & market rates",
		Category:    "news",
		IsPublic:    true,
		Handler:     handleMarkets,
	})

	Register(&Command{
		Name:        "news",
		Alias:       "apnews",
		Description: "Fetch latest news headlines for a country from AP News",
		Category:    "news",
		IsPublic:    true,
		Handler:     handleNews,
	})

	Register(&Command{
		Name:        "wabeta",
		Alias:       "wbi",
		Description: "Fetch the latest WhatsApp beta news and feature breakdown from WABetaInfo",
		Category:    "news",
		IsPublic:    true,
		Handler:     handleWABeta,
	})
}

type btcLiveSession struct {
	chat      string
	msgID     types.MessageID
	cancel    context.CancelFunc
	startedAt time.Time
}

var (
	activeBTCSessionsMu sync.Mutex
	activeBTCSessions   = make(map[string]*btcLiveSession)
)

func handleBTC(ctx *Context) error {
	chatKey := ctx.Chat.String()
	p := ctx.GetPrefix()
	Logger.Debug("handleBTC executing", "chat", chatKey, "sender", ctx.Sender.String(), "args", ctx.Args)

	if len(ctx.Args) > 0 {
		arg := strings.ToLower(ctx.Args[0])
		if arg == "cancel" || arg == "stop" || arg == "end" || arg == "off" {
			activeBTCSessionsMu.Lock()
			sess, exists := activeBTCSessions[chatKey]
			if exists && sess != nil {
				if sess.cancel != nil {
					sess.cancel()
				}
				delete(activeBTCSessions, chatKey)
				activeBTCSessionsMu.Unlock()
				return ctx.Reply("🛑 Live bitcoin price tracking stopped.")
			}
			activeBTCSessionsMu.Unlock()
			return ctx.Reply("No active Bitcoin tracking session running in this chat.")
		}
	}

	// Cancel any active BTC session in this chat
	activeBTCSessionsMu.Lock()
	if prev, exists := activeBTCSessions[chatKey]; exists && prev != nil {
		if prev.cancel != nil {
			prev.cancel()
		}
		delete(activeBTCSessions, chatKey)
	}
	activeBTCSessionsMu.Unlock()

	// Initial fetch
	data, err := cliutils.FetchBTCPredictions(ctx.Ctx)
	if err != nil {
		Logger.Error("handleBTC: initial fetch failed", "err", err)
		return ctx.Replyf("Failed to fetch Bitcoin data: %v", err)
	}

	statusLine := fmt.Sprintf("Use %sbitcoin stop to end live bitcoin price", p)
	bodyText := cliutils.FormatBTCMessage(data, statusLine)

	btcCtx, cancel := context.WithCancel(context.Background())

	msgID, err := ctx.ReplyWithID(bodyText)
	if err != nil {
		cancel()
		Logger.Error("handleBTC: failed to send initial message", "err", err)
		return ctx.Replyf("Failed to start Bitcoin tracker: %v", err)
	}

	session := &btcLiveSession{
		chat:      chatKey,
		msgID:     msgID,
		cancel:    cancel,
		startedAt: time.Now(),
	}

	activeBTCSessionsMu.Lock()
	activeBTCSessions[chatKey] = session
	activeBTCSessionsMu.Unlock()

	go runBTCLiveLoop(ctx, session, btcCtx, p)

	return nil
}

func runBTCLiveLoop(ctx *Context, sess *btcLiveSession, btcCtx context.Context, p string) {
	ticker := time.NewTicker(2800 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		activeBTCSessionsMu.Lock()
		if curr, ok := activeBTCSessions[sess.chat]; ok && curr == sess {
			delete(activeBTCSessions, sess.chat)
		}
		activeBTCSessionsMu.Unlock()
	}()

	maxDuration := 5 * time.Minute
	startTime := time.Now()

	for {
		select {
		case <-btcCtx.Done():
			Logger.Debug("runBTCLiveLoop: session canceled", "chat", sess.chat)
			if latestData, err := cliutils.FetchBTCPredictions(context.Background()); err == nil {
				finalMsg := cliutils.FormatBTCMessage(latestData, "🛑 Live bitcoin price tracking stopped.")
				_, _ = ctx.Edit(sess.msgID, finalMsg)
			}
			return

		case <-ticker.C:
			if time.Since(startTime) > maxDuration {
				Logger.Debug("runBTCLiveLoop: session reached max duration", "chat", sess.chat)
				if latestData, err := cliutils.FetchBTCPredictions(context.Background()); err == nil {
					finalMsg := cliutils.FormatBTCMessage(latestData, "⏱️ Live bitcoin price tracking ended (5m timeout).")
					_, _ = ctx.Edit(sess.msgID, finalMsg)
				}
				return
			}

			latestData, err := cliutils.FetchBTCPredictions(btcCtx)
			if err != nil {
				Logger.Debug("runBTCLiveLoop: fetch tick failed", "err", err)
				continue
			}

			statusLine := fmt.Sprintf("Use %sbitcoin stop to end live bitcoin price", p)
			updatedText := cliutils.FormatBTCMessage(latestData, statusLine)

			_, editErr := ctx.Edit(sess.msgID, updatedText)
			if editErr != nil {
				Logger.Debug("runBTCLiveLoop: edit failed", "err", editErr)
			}
		}
	}
}

func handleMarkets(ctx *Context) error {
	Logger.Debug("handleMarkets executing", "chat", ctx.Chat.String(), "sender", ctx.Sender.String(), "args", ctx.Args)

	if len(ctx.Args) == 0 {
		return sendMarketsHelp(ctx)
	}

	queryArg := strings.ToUpper(strings.TrimSpace(strings.Join(ctx.Args, "")))
	queryArg = strings.ReplaceAll(queryArg, "-", "/")
	queryArg = strings.ReplaceAll(queryArg, " ", "")
	Logger.Debug("handleMarkets: parsed query argument", "raw_args", ctx.Args, "parsed_query", queryArg)

	if queryArg == "MENU" || queryArg == "LIST" || queryArg == "ALL" {
		Logger.Debug("handleMarkets: requested market summary overview", "query", queryArg)
		return fetchAndSendAllMarkets(ctx)
	}

	queryArg = cliutils.NormalizeMarketPair(queryArg)

	Logger.Debug("handleMarkets: querying single instrument", "pair", queryArg)
	return fetchAndSendSingleMarket(ctx, queryArg)
}

func sendMarketsHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("Forex Factory Market Rates").
		Section("Usage:").
		Bulletf("%smarkets <pair> (e.g. %smarkets EUR/USD, %smarkets Gold/USD, %smarkets BTC/USD)", p, p, p, p).
		Bulletf("%smarkets all (overview of major currency & commodity pairs)", p).
		Blank().
		Line("Type the currency pair, metal, or cryptocurrency you would like to view.").
		Reply()
}

func fetchAndSendSingleMarket(ctx *Context, pair string) error {
	Logger.Debug("fetchAndSendSingleMarket: requesting market metrics from primary API", "pair", pair)

	if item, err := cliutils.FetchSingleMarket(ctx.Ctx, pair); err == nil && item != nil {
		return formatAndSendInstrumentResponse(ctx, pair, *item)
	}

	Logger.Debug("fetchAndSendSingleMarket: primary API empty or unavailable, querying bars fallback API", "pair", pair)
	barsRes, err := cliutils.FetchMarketBars(ctx.Ctx, pair)
	if err == nil && len(barsRes.Data) > 0 {
		latest := barsRes.Data[0]
		Logger.Debug("fetchAndSendSingleMarket: successfully retrieved bar metrics from fallback API", "pair", pair, "close", latest.Close)

		return ctx.Text().
			Headerf("Forex Factory Rates - %s", pair).
			Fieldf("Price", "%.2f", latest.Close).
			Fieldf("Open", "%.2f", latest.Open).
			Fieldf("High / Low", "%.2f | %.2f", latest.High, latest.Low).
			Field("Market Status", "Active").
			Reply()
	}

	Logger.Warn("fetchAndSendSingleMarket: both primary and bars APIs failed", "pair", pair)
	return sendAvailableInstrumentsList(ctx, pair)
}

func formatAndSendInstrumentResponse(ctx *Context, pair string, item cliutils.FFInstrumentData) error {
	displayName := item.Instrument.DisplayName
	if displayName == "" {
		displayName = pair
	}

	var price, high, low, spread float64
	var bid, ask float64

	if d1, ok := item.Metrics["D1"]; ok {
		price = d1.Price
		high = d1.High
		low = d1.Low
		spread = d1.Spread
	} else if h1, ok := item.Metrics["H1"]; ok {
		price = h1.Price
		high = h1.High
		low = h1.Low
		spread = h1.Spread
	}

	if len(item.Quotes) > 0 {
		bid = item.Quotes[0].Bid
		ask = item.Quotes[0].Ask
		if price == 0 {
			price = (bid + ask) / 2
		}
	}

	decimals := item.Instrument.Decimals
	if decimals == 0 {
		decimals = 4
	}

	marketStatus := "Open"
	if item.Instrument.InHoliday {
		marketStatus = "Holiday / Closed"
	}

	Logger.Debug("formatAndSendInstrumentResponse: parsed market data", "pair", displayName, "price", price, "bid", bid, "ask", ask, "high", high, "low", low, "spread", spread, "status", marketStatus)

	tb := ctx.Text().Headerf("Forex Factory Rates - %s", displayName)
	if price > 0 {
		tb.Fieldf("Price", "%.*f", decimals, price)
	}
	if bid > 0 && ask > 0 {
		tb.Fieldf("Bid / Ask", "%.*f | %.*f", decimals, bid, decimals, ask)
	}
	if high > 0 && low > 0 {
		tb.Fieldf("24h High / Low", "%.*f | %.*f", decimals, high, decimals, low)
	}
	if spread > 0 {
		tb.Fieldf("Spread", "%.1f pips", spread)
	}
	tb.Field("Market Status", marketStatus)

	return tb.Reply()
}

func sendAvailableInstrumentsList(ctx *Context, requestedPair string) error {
	p := ctx.GetPrefix()
	apiItems, err := cliutils.FetchForexFactoryInstrumentList(ctx.Ctx)

	tb := ctx.Text().
		Headerf("Instrument %q is not available on Forex Factory.", requestedPair).
		Section("Available Active Markets:")

	if err == nil && len(apiItems) > 0 {
		seen := make(map[string]bool)
		count := 0
		for _, item := range apiItems {
			name := item.DisplayName
			if name == "" {
				name = item.Name
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			tb.Bulletf("%smarkets %s", p, name)
			count++
			if count >= 12 {
				break
			}
		}
	} else {
		tb.Bulletf("%smarkets EUR/USD", p).
			Bulletf("%smarkets GBP/USD", p).
			Bulletf("%smarkets USD/JPY", p).
			Bulletf("%smarkets Gold/USD", p)
	}

	tb.Blank().
		Section("View all summary:").
		Bulletf("%smarkets all", p)

	return tb.Reply()
}

func fetchAndSendAllMarkets(ctx *Context) error {
	pairs := []string{"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "USD/CAD", "AUD/USD", "NZD/USD", "Gold/USD"}
	res, err := cliutils.FetchAllMarkets(ctx.Ctx, pairs)
	if err != nil {
		Logger.Error("fetchAndSendAllMarkets: HTTP request failed", "err", err)
		return ctx.Replyf("Failed to fetch market rates: %v", err)
	}
	if len(res.Data) == 0 {
		Logger.Warn("fetchAndSendAllMarkets: no data returned from API")
		return ctx.Reply("No market rates available at this time.")
	}

	Logger.Debug("fetchAndSendAllMarkets: successfully parsed market overview", "item_count", len(res.Data))

	tb := ctx.Text().Header("Forex Factory Market Overview")

	for _, item := range res.Data {
		displayName := item.Instrument.DisplayName
		var price float64
		if len(item.Quotes) > 0 {
			price = (item.Quotes[0].Bid + item.Quotes[0].Ask) / 2
		}
		if price == 0 {
			if d1, ok := item.Metrics["D1"]; ok {
				price = d1.Price
			}
		}

		decimals := item.Instrument.Decimals
		if decimals == 0 {
			decimals = 4
		}

		tb.Bulletf("%s: %.*f", displayName, decimals, price)
	}

	return tb.Reply()
}

func handleNews(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return sendNewsHelp(ctx)
	}

	firstArg := ctx.Args[0]
	if num, isNum := cliutils.ParseNewsSelectionNumber(firstArg); isNum {
		quotedMsg := ctx.GetQuotedMessage()
		if quotedMsg == nil {
			quotedMsg = getQuotedMessageFromEvent(ctx.Evt)
		}
		if quotedMsg != nil {
			quotedText := utils.ExtractTextFromProto(quotedMsg)
			urls := cliutils.ExtractAPNewsArticleURLs(quotedText)
			if len(urls) > 0 {
				if num >= 1 && num <= len(urls) {
					return SendAPNewsArticleDetail(ctx, urls[num-1])
				}
				return ctx.Replyf("Article #%d is out of range. Please choose a number between 1 and %d.", num, len(urls))
			}
		}

		if sess, okSess := cliutils.GetRecentNewsSession(ctx.Chat.String()); okSess && len(sess.Articles) > 0 {
			if num >= 1 && num <= len(sess.Articles) {
				return SendAPNewsArticleDetail(ctx, sess.Articles[num-1].URL)
			}
			return ctx.Replyf("Article #%d is out of range. Please choose a number between 1 and %d.", num, len(sess.Articles))
		}

		return ctx.Replyf("No recent news list found to select article #%d from. Usage:\n• %snews <country_name>", num, p)
	}

	country := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, "-")))
	articles, err := cliutils.FetchAPNews(ctx.Ctx, country)
	if err != nil {
		if err.Error() == "not found" {
			return ctx.Replyf("No news topic hub found for %q. Usage:\n• %snews <country_name>", country, p)
		}
		return ctx.Replyf("Failed to fetch news for %q: %v", country, err)
	}

	if len(articles) == 0 {
		return ctx.Replyf("No recent news articles found for %q.", country)
	}

	// Cache recent news articles for this chat
	cliutils.SetRecentNewsSession(ctx.Chat.String(), country, articles)

	var firstImageURL string
	tb := ctx.Text().Headerf("AP News - %s", titleCase(strings.ReplaceAll(country, "-", " ")))

	count := 0
	for _, art := range articles {
		if count >= 5 {
			break
		}
		count++
		if firstImageURL == "" && art.ImageURL != "" {
			firstImageURL = art.ImageURL
		}

		tb.Numberedf(count, "%s", art.Title)
		if art.Description != "" {
			tb.Line("   " + art.Description)
		}
		if art.URL != "" {
			tb.Line("   " + art.URL)
		}
		tb.Blank()
	}

	tb.Line("💡 _Reply/quote this message with a number (1-" + strconv.Itoa(count) + ") to read the full article._")

	responseText := tb.Trimmed()

	if firstImageURL != "" {
		if imgData, mimetype, errImg := cliutils.FetchNewsImage(ctx.Ctx, firstImageURL); errImg == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, mimetype, responseText)
		}
	}

	return ctx.Reply(responseText)
}

// SendAPNewsArticleDetail fetches and sends the full story content for an AP News article.
func SendAPNewsArticleDetail(ctx *Context, articleURL string) error {
	Logger.Debug("SendAPNewsArticleDetail: fetching AP news article", "url", articleURL)

	art, err := cliutils.FetchAPNewsArticle(ctx.Ctx, articleURL)
	if err != nil {
		Logger.Error("SendAPNewsArticleDetail: failed to fetch article", "url", articleURL, "err", err)
		return ctx.Replyf("Failed to load news article: %v", err)
	}

	content := strings.TrimSpace(art.Content)
	if content == "" {
		return ctx.Reply("No article content found.")
	}

	return ctx.Reply(content)
}

// HandleNewsQuoteInput checks if an incoming message is quoting a news article list with a number selection.
func HandleNewsQuoteInput(ctx *Context, text string) bool {
	if text == "" || ctx == nil || ctx.Evt == nil {
		return false
	}

	quotedMsg := ctx.GetQuotedMessage()
	if quotedMsg == nil {
		quotedMsg = getQuotedMessageFromEvent(ctx.Evt)
	}
	if quotedMsg == nil {
		return false
	}

	quotedText := utils.ExtractTextFromProto(quotedMsg)
	if quotedText == "" {
		return false
	}

	isNewsQuote := strings.Contains(quotedText, "apnews.com/article/") ||
		strings.Contains(quotedText, "AP News - ") ||
		strings.Contains(quotedText, "AP News")

	if !isNewsQuote {
		return false
	}

	num, ok := cliutils.ParseNewsSelectionNumber(text)
	if !ok {
		return false
	}

	urls := cliutils.ExtractAPNewsArticleURLs(quotedText)
	if len(urls) > 0 {
		if num >= 1 && num <= len(urls) {
			targetURL := urls[num-1]
			go func() {
				reqCtx, cancel := context.WithCancel(ctx.Ctx)
				defer cancel()

				cctx := &Context{
					Ctx:        reqCtx,
					CancelFunc: cancel,
					Client:     ctx.Client,
					Evt:        ctx.Evt,
					Chat:       ctx.Chat,
					Sender:     ctx.Sender,
				}
				cctx.StartAutoLoader()
				defer cctx.StopAutoLoader()

				if err := SendAPNewsArticleDetail(cctx, targetURL); err != nil {
					Logger.Error("HandleNewsQuoteInput: failed to send article detail", "url", targetURL, "err", err)
				}
			}()
			return true
		}
		_ = ctx.Replyf("Article #%d is out of range. Please choose a number between 1 and %d.", num, len(urls))
		return true
	}

	if sess, okSess := cliutils.GetRecentNewsSession(ctx.Chat.String()); okSess && len(sess.Articles) > 0 {
		if num >= 1 && num <= len(sess.Articles) {
			targetURL := sess.Articles[num-1].URL
			go func() {
				reqCtx, cancel := context.WithCancel(ctx.Ctx)
				defer cancel()

				cctx := &Context{
					Ctx:        reqCtx,
					CancelFunc: cancel,
					Client:     ctx.Client,
					Evt:        ctx.Evt,
					Chat:       ctx.Chat,
					Sender:     ctx.Sender,
				}
				cctx.StartAutoLoader()
				defer cctx.StopAutoLoader()

				if err := SendAPNewsArticleDetail(cctx, targetURL); err != nil {
					Logger.Error("HandleNewsQuoteInput: failed to send article detail from session", "url", targetURL, "err", err)
				}
			}()
			return true
		}
		_ = ctx.Replyf("Article #%d is out of range. Please choose a number between 1 and %d.", num, len(sess.Articles))
		return true
	}

	return false
}

func sendNewsHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	return ctx.Text().
		Header("AP News Country Headlines").
		Section("Usage:").
		Bulletf("%snews <country> (e.g. %snews nigeria, %snews japan, %snews usa, %snews uk)", p, p, p, p, p).
		Blank().
		Line("Type a country name to fetch the latest top headlines.").
		Reply()
}

func handleWABeta(ctx *Context) error {
	Logger.Debug("handleWABeta executing", "chat", ctx.Chat.String(), "sender", ctx.Sender.String())

	article, err := cliutils.FetchWABetaLatest(ctx.Ctx)
	if err != nil {
		Logger.Error("handleWABeta: failed to fetch WABetaInfo article", "err", err)
		return ctx.Replyf("Failed to fetch latest WABetaInfo updates: %v", err)
	}

	tb := ctx.Text()
	if article.Title != "" {
		tb.Header(article.Title)
	}
	if article.Content != "" {
		tb.Line(article.Content)
	}

	caption := tb.Trimmed()

	if article.ImageURL != "" {
		if imgData, mimetype, errImg := cliutils.FetchNewsImage(ctx.Ctx, article.ImageURL); errImg == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, mimetype, caption)
		}
	}

	return ctx.Reply(caption)
}
