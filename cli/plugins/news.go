package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
	slog.Debug("handleMarkets executing", "chat", ctx.Chat.String(), "sender", ctx.Sender.String(), "device", ctx.Sender.Device, "args", ctx.Args)

	if len(ctx.Args) == 0 {
		slog.Debug("handleMarkets: no args provided, routing menu", "device", ctx.Sender.Device)
		if ctx.Sender.Device != 0 {
			slog.Debug("handleMarkets: routing to sendMarketsButtonsMenu for non-mobile device", "device", ctx.Sender.Device)
			return sendMarketsButtonsMenu(ctx)
		}
		slog.Debug("handleMarkets: routing to sendMarketsSelectListMenu for mobile device", "device", ctx.Sender.Device)
		return sendMarketsSelectListMenu(ctx)
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
			fmt.Fprintf(&sb, "- %smarkets %s\n", p, name)
			count++
			if count >= 12 {
				break
			}
		}
	} else {
		fmt.Fprintf(&sb, "- %smarkets EUR/USD\n- %smarkets GBP/USD\n- %smarkets USD/JPY\n- %smarkets Gold/USD\n", p, p, p, p)
	}

	fmt.Fprintf(&sb, "\nView all summary:\n- %smarkets all", p)
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

		fmt.Fprintf(&sb, "- %s: %.*f\n", displayName, decimals, price)
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func sendMarketsButtonsMenu(ctx *Context) error {
	p := ctx.GetPrefix()
	slog.Debug("sendMarketsButtonsMenu: building normal buttons for Web/Desktop non-mobile device", "chat", ctx.Chat.String(), "device", ctx.Sender.Device)

	apiItems, _ := cliutils.FetchForexFactoryInstrumentList(ctx.Ctx)

	var btn1, btn2, btn3 string
	if len(apiItems) >= 3 {
		btn1 = apiItems[0].DisplayName
		btn2 = apiItems[1].DisplayName
		btn3 = apiItems[2].DisplayName
	} else {
		btn1 = "EUR/USD"
		btn2 = "GBP/USD"
		btn3 = "Gold/USD"
	}

	bodyText := "Forex Factory Markets\n\nSelect an instrument below to check real-time rates:\n\nExamples:\n- " + p + "markets EUR/USD\n- " + p + "markets Gold/USD\n- " + p + "markets all"

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("Forex Factory Rates"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(p + "markets " + btn1),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn1),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets " + btn2),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn2),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "markets " + btn3),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new(btn3),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"name": "quick_reply"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	slog.Debug("sendMarketsButtonsMenu: sending buttons message", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		slog.Error("sendMarketsButtonsMenu: failed to send buttons message", "err", err)
	}
	return err
}

func sendMarketsSelectListMenu(ctx *Context) error {
	msgVersion := int32(1)
	p := ctx.GetPrefix()
	slog.Debug("sendMarketsSelectListMenu: building single_select native flow list for mobile device", "chat", ctx.Chat.String(), "device", ctx.Sender.Device)

	apiItems, err := cliutils.FetchForexFactoryInstrumentList(ctx.Ctx)
	if err != nil || len(apiItems) == 0 {
		slog.Error("sendMarketsSelectListMenu: failed to fetch live instruments from Forex Factory API", "err", err)
		return ctx.Reply("Failed to fetch market instruments from Forex Factory. Please try again later.")
	}

	slog.Debug("sendMarketsSelectListMenu: fetched live instruments from Forex Factory API", "total_items", len(apiItems))

	slices.SortFunc(apiItems, func(a, b cliutils.FFListItem) int {
		if a.Rank == 0 && b.Rank == 0 {
			return a.ID - b.ID
		}
		if a.Rank == 0 {
			return 1
		}
		if b.Rank == 0 {
			return -1
		}
		return a.Rank - b.Rank
	})

	var forexRows []cliutils.SelectListRow
	var cryptoRows []cliutils.SelectListRow
	var commodityRows []cliutils.SelectListRow
	var otherRows []cliutils.SelectListRow

	seenNames := make(map[string]bool)

	for _, item := range apiItems {
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" || seenNames[strings.ToUpper(name)] {
			continue
		}
		seenNames[strings.ToUpper(name)] = true

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = name
		}

		row := cliutils.SelectListRow{
			ID:          fmt.Sprintf("%smarkets %s", p, name),
			Title:       name,
			Description: title,
		}

		className := strings.ToUpper(strings.TrimSpace(item.InstrumentClassName))
		upperName := strings.ToUpper(name)

		if className == "CRYPTO" || strings.HasPrefix(upperName, "BTC") || strings.HasPrefix(upperName, "ETH") || strings.HasPrefix(upperName, "DOGE") {
			if len(cryptoRows) < 10 {
				cryptoRows = append(cryptoRows, row)
			}
		} else if className == "METALS" || className == "COMMODITIES" || className == "ENERGY" ||
			strings.Contains(upperName, "GOLD") || strings.Contains(upperName, "SILVER") ||
			strings.Contains(upperName, "OIL") || strings.Contains(upperName, "GAS") {
			if len(commodityRows) < 10 {
				commodityRows = append(commodityRows, row)
			}
		} else if className == "EQUITIES" || className == "INDICES" || className == "INDEX" ||
			upperName == "DOW" || upperName == "SPX" || upperName == "NDX" || upperName == "NIKKEI225" ||
			upperName == "DAX" || upperName == "FTSE100" || upperName == "STOXX50" || upperName == "US2000" ||
			upperName == "VIX" || upperName == "DXY" || upperName == "CAC" || upperName == "ASX" {
			if len(otherRows) < 10 {
				otherRows = append(otherRows, row)
			}
		} else if className == "FOREX" || (strings.Contains(upperName, "/") && len(upperName) == 7) {
			if len(forexRows) < 10 {
				forexRows = append(forexRows, row)
			}
		} else {
			if len(otherRows) < 10 {
				otherRows = append(otherRows, row)
			}
		}
	}

	var sections []cliutils.SelectListSection

	if len(forexRows) > 0 {
		sections = append(sections, cliutils.SelectListSection{
			Title: "Forex Currency Pairs",
			Rows:  forexRows,
		})
	}
	if len(commodityRows) > 0 {
		sections = append(sections, cliutils.SelectListSection{
			Title: "Metals & Commodities",
			Rows:  commodityRows,
		})
	}
	if len(cryptoRows) > 0 {
		sections = append(sections, cliutils.SelectListSection{
			Title: "Cryptocurrencies",
			Rows:  cryptoRows,
		})
	}
	if len(otherRows) > 0 {
		sections = append(sections, cliutils.SelectListSection{
			Title: "Other Instruments",
			Rows:  otherRows,
		})
	}

	sections = append(sections, cliutils.SelectListSection{
		Title: "Market Summary",
		Rows: []cliutils.SelectListRow{
			{
				ID:          p + "markets all",
				Title:       "All Major Markets",
				Description: "View live summary for all major market pairs",
			},
		},
	})

	paramsObj := cliutils.SelectListParams{
		Title:    "Select Market Instrument",
		Sections: sections,
	}

	paramsJSON, err := json.Marshal(paramsObj)
	if err != nil {
		slog.Error("sendMarketsSelectListMenu: failed to marshal params JSON", "err", err)
		return ctx.Reply("Failed to construct market menu.")
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Forex Factory Market Rates\n\nClick the button below to choose an active instrument:"),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Live Forex Factory Data"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             new("single_select"),
									ButtonParamsJSON: new(string(paramsJSON)),
								},
							},
							MessageVersion: &msgVersion,
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"v": "9", "name": "mixed"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	slog.Debug("sendMarketsSelectListMenu: sending live API single_select list message", "chat", ctx.Chat.String())
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		slog.Error("sendMarketsSelectListMenu: failed to send list message", "err", err)
	}
	return err
}

func handleNews(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return sendNewsCountryMenu(ctx)
	}

	country := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, "-")))
	articles, err := cliutils.FetchAPNews(ctx.Ctx, country)
	if err != nil {
		if err.Error() == "not found" {
			return ctx.Reply(fmt.Sprintf("No news topic hub found for %q. Usage:\n- %snews <country_name>", country, p))
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

func sendNewsCountryMenu(ctx *Context) error {
	p := ctx.GetPrefix()
	bodyText := "AP News Country Selector\n\nPlease select a country below to view top news headlines, or type a country name:\n\nExamples:\n- " + p + "news nigeria\n- " + p + "news japan\n- " + p + "news united-states\n- " + p + "news kenya"

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook AP News"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{ButtonID: new(p + "news afghanistan"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("AFGHANISTAN")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news algeria"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("ALGERIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news argentina"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("ARGENTINA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news australia"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("AUSTRALIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news austria"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("AUSTRIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news bangladesh"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("BANGLADESH")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news belgium"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("BELGIUM")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news brazil"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("BRAZIL")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news canada"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("CANADA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news chile"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("CHILE")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news china"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("CHINA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news colombia"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("COLOMBIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news denmark"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("DENMARK")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news egypt"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("EGYPT")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news finland"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("FINLAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news france"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("FRANCE")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news germany"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("GERMANY")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news ghana"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("GHANA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news greece"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("GREECE")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news india"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("INDIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news indonesia"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("INDONESIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news ireland"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("IRELAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news israel"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("ISRAEL")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news italy"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("ITALY")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news jamaica"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("JAMAICA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news japan"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("JAPAN")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news kenya"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("KENYA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news malaysia"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("MALAYSIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news mexico"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("MEXICO")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news morocco"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("MOROCCO")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news netherlands"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("NETHERLANDS")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news new-zealand"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("NEW ZEALAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news nigeria"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("NIGERIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news norway"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("NORWAY")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news pakistan"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("PAKISTAN")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news peru"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("PERU")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news philippines"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("PHILIPPINES")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news poland"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("POLAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news portugal"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("PORTUGAL")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news saudi-arabia"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SAUDI ARABIA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news singapore"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SINGAPORE")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news south-africa"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SOUTH AFRICA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news south-korea"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SOUTH KOREA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news spain"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SPAIN")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news sweden"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SWEDEN")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news switzerland"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("SWITZERLAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news thailand"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("THAILAND")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news turkey"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("TURKEY")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news united-kingdom"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("UK")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
						{ButtonID: new(p + "news united-states"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("USA")}, Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum()},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"name": "quick_reply"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}
