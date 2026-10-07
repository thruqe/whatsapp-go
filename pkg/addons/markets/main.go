package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type ffInstrumentResponse struct {
	Data []ffInstrumentData `json:"data"`
}

type ffInstrumentData struct {
	Instrument ffInstrumentMeta    `json:"instrument"`
	Metrics    map[string]ffMetric `json:"metrics"`
	Quotes     []ffQuote           `json:"quotes"`
}

type ffInstrumentMeta struct {
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Decimals    int    `json:"decimals"`
	IsInHoliday bool   `json:"is_in_holiday"`
}

type ffMetric struct {
	Price  float64 `json:"price"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Spread float64 `json:"spread"`
}

type ffQuote struct {
	Bid float64 `json:"bid"`
	Ask float64 `json:"ask"`
}

type ffBarsResponse struct {
	Data []ffBarItem `json:"data"`
}

type ffBarItem struct {
	Close float64 `json:"close"`
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
}

func normalizePair(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	switch s {
	case "GOLD", "XAUUSD", "XAU/USD":
		return "Gold/USD"
	case "SILVER", "XAGUSD", "XAG/USD":
		return "Silver/USD"
	case "OIL", "BRENT", "CRUDE", "WTI", "USOIL":
		return "WTI/USD"
	case "SPX", "SP500", "US500", "S&P500":
		return "SPX500/USD"
	case "NAS", "NAS100", "US100", "NASDAQ", "NDX":
		return "NAS100/USD"
	case "DOW", "DJI", "US30", "DJ30":
		return "US30/USD"
	case "NIKKEI", "JP225", "N225":
		return "Nikkei225/USD"
	case "DAX", "DAX/USD", "GER30", "DE30", "GER40", "DE40":
		return "DAX/USD"
	case "FTSE", "FTSE100", "FTSE100/USD", "UK100":
		return "FTSE100/USD"
	case "STOXX50", "STOXX50/USD", "EU50":
		return "STOXX50/USD"
	case "US2000", "US2000/USD", "RUSSELL2000", "RUSSELL":
		return "US2000/USD"
	case "VIX", "VIX/USD":
		return "VIX/USD"
	case "DXY", "DXY/USD", "USDX":
		return "DXY/USD"
	case "CAC", "CAC40", "CAC/USD", "FRA40":
		return "CAC/USD"
	case "EURUSD", "EUR/USD":
		return "EUR/USD"
	case "GBPUSD", "GBP/USD":
		return "GBP/USD"
	case "USDJPY", "USD/JPY":
		return "USD/JPY"
	case "USDCHF", "USD/CHF":
		return "USD/CHF"
	case "USDCAD", "USD/CAD":
		return "USD/CAD"
	case "AUDUSD", "AUD/USD":
		return "AUD/USD"
	case "NZDUSD", "NZD/USD":
		return "NZD/USD"
	case "BTCUSD", "BTC/USD", "BTC", "BITCOIN":
		return "BTC/USD"
	case "ETHUSD", "ETH/USD", "ETH", "ETHEREUM":
		return "ETH/USD"
	case "DOGEUSD", "DOGE/USD", "DOGE":
		return "DOGE/USD"
	default:
		if strings.Contains(s, "/") {
			return s
		}
		if len(s) == 6 {
			return s[:3] + "/" + s[3:]
		}
		return s
	}
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond("*Forex Factory Market Rates*\n\nUsage:\n• markets <pair> (e.g. EUR/USD, Gold/USD, BTC/USD)\n• markets all (overview of major currency & commodity pairs)")
		return
	}

	upper := strings.ToUpper(strings.TrimSpace(query))
	if upper == "ALL" || upper == "LIST" || upper == "MENU" || upper == "OVERVIEW" {
		fetchOverview()
		return
	}

	pair := normalizePair(query)
	fetchSingleMarket(pair)
}

func fetchOverview() {
	pairs := []string{
		"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "USD/CAD", "AUD/USD", "NZD/USD", "Gold/USD",
	}
	client := sdk.CreateHTTPClient(10)
	encoded := url.QueryEscape(strings.Join(pairs, ","))
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", encoded)

	resp, err := client.Get(apiURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data ffInstrumentResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && len(data.Data) > 0 {
				var out strings.Builder
				out.WriteString("*Forex Factory Market Overview*\n\n")
				for _, item := range data.Data {
					name := item.Instrument.DisplayName
					if name == "" {
						name = item.Instrument.Name
					}
					var price float64
					if len(item.Quotes) > 0 {
						price = (item.Quotes[0].Bid + item.Quotes[0].Ask) / 2.0
					}
					if price == 0 {
						if d1, ok := item.Metrics["D1"]; ok {
							price = d1.Price
						}
					}
					decimals := item.Instrument.Decimals
					if decimals <= 0 {
						decimals = 4
					}
					fmt.Fprintf(&out, "• *%s*: %.*f\n", name, decimals, price)
				}
				sdk.Respond(strings.TrimSpace(out.String()))
				return
			}
		}
	}

	sdk.RespondErr("Failed to fetch market rates overview.")
}

func fetchSingleMarket(pair string) {
	client := sdk.CreateHTTPClient(8)
	encoded := url.QueryEscape(pair)
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", encoded)

	resp, err := client.Get(apiURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data ffInstrumentResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && len(data.Data) > 0 {
				item := data.Data[0]
				name := item.Instrument.DisplayName
				if name == "" {
					name = item.Instrument.Name
				}

				var price, high, low, spread float64
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

				var bid, ask float64
				if len(item.Quotes) > 0 {
					if price == 0 {
						price = (item.Quotes[0].Bid + item.Quotes[0].Ask) / 2.0
					}
					bid = item.Quotes[0].Bid
					ask = item.Quotes[0].Ask
				}

				decimals := item.Instrument.Decimals
				if decimals <= 0 {
					decimals = 4
				}
				status := "Open"
				if item.Instrument.IsInHoliday {
					status = "Holiday / Closed"
				}

				var out strings.Builder
				fmt.Fprintf(&out, "*Forex Factory Rates - %s*\n", name)
				if price > 0 {
					fmt.Fprintf(&out, "\n*Price:* %.*f", decimals, price)
				}
				if bid > 0 && ask > 0 {
					fmt.Fprintf(&out, "\n*Bid / Ask:* %.*f | %.*f", decimals, bid, decimals, ask)
				}
				if high > 0 && low > 0 {
					fmt.Fprintf(&out, "\n*24h High / Low:* %.*f | %.*f", decimals, high, decimals, low)
				}
				if spread > 0 {
					fmt.Fprintf(&out, "\n*Spread:* %.1f pips", spread)
				}
				fmt.Fprintf(&out, "\n*Market Status:* %s", status)

				sdk.Respond(out.String())
				return
			}
		}
	}

	// Fallback to Bars API
	barsURL := fmt.Sprintf("https://mds-api.forexfactory.com/bars?instrument=%s&interval=M5&per_page=1", encoded)
	if resp, err := client.Get(barsURL); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var bars ffBarsResponse
			if err := json.NewDecoder(resp.Body).Decode(&bars); err == nil && len(bars.Data) > 0 {
				bar := bars.Data[0]
				out := fmt.Sprintf(
					"*Forex Factory Rates - %s*\n\n*Price:* %.2f\n*Open:* %.2f\n*High / Low:* %.2f | %.2f\n*Market Status:* Active",
					pair, bar.Close, bar.Open, bar.High, bar.Low,
				)
				sdk.Respond(out)
				return
			}
		}
	}

	sdk.Respond(fmt.Sprintf("Could not find market rates for %q. Use `markets all` to view active instruments.", pair))
}
