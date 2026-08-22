package plugins

import (
	"fmt"
	"log/slog"
	"strings"

	cliutils "whatsrook/cli/utils"
)

func init() {
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
}

func handleMarkets(ctx *Context) error {
	slog.Debug("handleMarkets executing", "chat", ctx.Chat.String(), "sender", ctx.Sender.String(), "args", ctx.Args)

	if len(ctx.Args) == 0 {
		return sendMarketsHelp(ctx)
	}

	queryArg := strings.ToUpper(strings.TrimSpace(strings.Join(ctx.Args, "")))
	queryArg = strings.ReplaceAll(queryArg, "-", "/")
	queryArg = strings.ReplaceAll(queryArg, " ", "")
	slog.Debug("handleMarkets: parsed query argument", "raw_args", ctx.Args, "parsed_query", queryArg)

	if queryArg == "MENU" || queryArg == "LIST" || queryArg == "ALL" {
		slog.Debug("handleMarkets: requested market summary overview", "query", queryArg)
		return fetchAndSendAllMarkets(ctx)
	}

	queryArg = cliutils.NormalizeMarketPair(queryArg)

	slog.Debug("handleMarkets: querying single instrument", "pair", queryArg)
	return fetchAndSendSingleMarket(ctx, queryArg)
}

func sendMarketsHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("Forex Factory Market Rates\n\n")
	sb.WriteString("Usage:\n")
	fmt.Fprintf(&sb, "• %smarkets <pair> (e.g. %smarkets EUR/USD, %smarkets Gold/USD, %smarkets BTC/USD)\n", p, p, p, p)
	fmt.Fprintf(&sb, "• %smarkets all (overview of major currency & commodity pairs)\n\n", p)
	sb.WriteString("Type the currency pair, metal, or cryptocurrency you would like to view.")
	return ctx.Reply(sb.String())
}

func fetchAndSendSingleMarket(ctx *Context, pair string) error {
	slog.Debug("fetchAndSendSingleMarket: requesting market metrics from primary API", "pair", pair)

	if item, err := cliutils.FetchSingleMarket(ctx.Ctx, pair); err == nil && item != nil {
		return formatAndSendInstrumentResponse(ctx, pair, *item)
	}

	slog.Debug("fetchAndSendSingleMarket: primary API empty or unavailable, querying bars fallback API", "pair", pair)
	barsRes, err := cliutils.FetchMarketBars(ctx.Ctx, pair)
	if err == nil && len(barsRes.Data) > 0 {
		latest := barsRes.Data[0]
		slog.Debug("fetchAndSendSingleMarket: successfully retrieved bar metrics from fallback API", "pair", pair, "close", latest.Close)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Forex Factory Rates - %s\n\n", pair))
		sb.WriteString(fmt.Sprintf("Price: %.2f\n", latest.Close))
		sb.WriteString(fmt.Sprintf("Open: %.2f\n", latest.Open))
		sb.WriteString(fmt.Sprintf("High: %.2f | Low: %.2f\n", latest.High, latest.Low))
		sb.WriteString("Market Status: Active\n")

		return ctx.Reply(sb.String())
	}

	slog.Warn("fetchAndSendSingleMarket: both primary and bars APIs failed", "pair", pair)
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

	slog.Debug("formatAndSendInstrumentResponse: parsed market data", "pair", displayName, "price", price, "bid", bid, "ask", ask, "high", high, "low", low, "spread", spread, "status", marketStatus)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Forex Factory Rates - %s\n\n", displayName)
	if price > 0 {
		fmt.Fprintf(&sb, "Price: %.*f\n", decimals, price)
	}
	if bid > 0 && ask > 0 {
		fmt.Fprintf(&sb, "Bid: %.*f | Ask: %.*f\n", decimals, bid, decimals, ask)
	}
	if high > 0 && low > 0 {
		fmt.Fprintf(&sb, "24h High: %.*f | 24h Low: %.*f\n", decimals, high, decimals, low)
	}
	if spread > 0 {
		fmt.Fprintf(&sb, "Spread: %.1f pips\n", spread)
	}
	fmt.Fprintf(&sb, "Market Status: %s\n", marketStatus)

	return ctx.Reply(sb.String())
}

func sendAvailableInstrumentsList(ctx *Context, requestedPair string) error {
	p := ctx.GetPrefix()
	apiItems, err := cliutils.FetchForexFactoryInstrumentList(ctx.Ctx)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Instrument %q is not available on Forex Factory.\n\nAvailable Active Markets:\n", requestedPair)

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
			fmt.Fprintf(&sb, "• %smarkets %s\n", p, name)
			count++
			if count >= 12 {
				break
			}
		}
	} else {
		fmt.Fprintf(&sb, "• %smarkets EUR/USD\n• %smarkets GBP/USD\n• %smarkets USD/JPY\n• %smarkets Gold/USD\n", p, p, p, p)
	}

	fmt.Fprintf(&sb, "\nView all summary:\n• %smarkets all", p)
	return ctx.Reply(sb.String())
}

func fetchAndSendAllMarkets(ctx *Context) error {
	pairs := []string{"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "USD/CAD", "AUD/USD", "NZD/USD", "Gold/USD"}
	res, err := cliutils.FetchAllMarkets(ctx.Ctx, pairs)
	if err != nil {
		slog.Error("fetchAndSendAllMarkets: HTTP request failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to fetch market rates: %v", err))
	}
	if len(res.Data) == 0 {
		slog.Warn("fetchAndSendAllMarkets: no data returned from API")
		return ctx.Reply("No market rates available at this time.")
	}

	slog.Debug("fetchAndSendAllMarkets: successfully parsed market overview", "item_count", len(res.Data))

	var sb strings.Builder
	fmt.Fprintf(&sb, "Forex Factory Market Overview\n\n")

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

		fmt.Fprintf(&sb, "• %s: %.*f\n", displayName, decimals, price)
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func handleNews(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return sendNewsHelp(ctx)
	}

	country := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, "-")))
	articles, err := cliutils.FetchAPNews(ctx.Ctx, country)
	if err != nil {
		if err.Error() == "not found" {
			return ctx.Reply(fmt.Sprintf("No news topic hub found for %q. Usage:\n• %snews <country_name>", country, p))
		}
		return ctx.Reply(fmt.Sprintf("Failed to fetch news for %q: %v", country, err))
	}

	if len(articles) == 0 {
		return ctx.Reply(fmt.Sprintf("No recent news articles found for %q.", country))
	}

	var firstImageURL string
	var sb strings.Builder
	fmt.Fprintf(&sb, "AP News - %s\n\n", titleCase(strings.ReplaceAll(country, "-", " ")))

	count := 0
	for _, art := range articles {
		if count >= 5 {
			break
		}
		count++
		if firstImageURL == "" && art.ImageURL != "" {
			firstImageURL = art.ImageURL
		}

		fmt.Fprintf(&sb, "%d. %s\n", count, art.Title)
		if art.Description != "" {
			fmt.Fprintf(&sb, "   %s\n", art.Description)
		}
		if art.URL != "" {
			fmt.Fprintf(&sb, "   %s\n", art.URL)
		}
		sb.WriteString("\n")
	}

	responseText := strings.TrimSpace(sb.String())

	if firstImageURL != "" {
		if imgData, mimetype, errImg := cliutils.FetchNewsImage(ctx.Ctx, firstImageURL); errImg == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, mimetype, responseText)
		}
	}

	return ctx.Reply(responseText)
}

func sendNewsHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("AP News Country Headlines\n\n")
	sb.WriteString("Usage:\n")
	fmt.Fprintf(&sb, "• %snews <country> (e.g. %snews nigeria, %snews japan, %snews usa, %snews uk)\n\n", p, p, p, p, p)
	sb.WriteString("Type a country name to fetch the latest top headlines.")
	return ctx.Reply(sb.String())
}
