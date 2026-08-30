package cliutils

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	utils "whatsrook/src"
)

type FFInstrumentResponse struct {
	Data []FFInstrumentData `json:"data"`
}

type FFInstrumentData struct {
	Instrument struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Decimals    int    `json:"decimals"`
		InHoliday   bool   `json:"is_in_holiday"`
	} `json:"instrument"`
	Metrics map[string]struct {
		Price  float64 `json:"price"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Spread float64 `json:"spread"`
	} `json:"metrics"`
	Quotes []struct {
		Instrument string  `json:"instrument"`
		Bid        float64 `json:"bid"`
		Ask        float64 `json:"ask"`
	} `json:"quotes"`
}

type FFBarsResponse struct {
	Data []struct {
		Timestamp    int64   `json:"timestamp"`
		DataSourceID string  `json:"data_source_id"`
		Interval     string  `json:"interval"`
		Instrument   string  `json:"instrument"`
		Open         float64 `json:"open"`
		High         float64 `json:"high"`
		Low          float64 `json:"low"`
		Close        float64 `json:"close"`
	} `json:"data"`
}

type FFListItem struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	Title               string `json:"title"`
	DisplayName         string `json:"display_name"`
	InstrumentClassName string `json:"instrument_class_name"`
	Rank                int    `json:"rank"`
}

type FFListResponse struct {
	Data []FFListItem `json:"data"`
}

type SelectListRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SelectListSection struct {
	Title string          `json:"title"`
	Rows  []SelectListRow `json:"rows"`
}

type SelectListParams struct {
	Title    string              `json:"title"`
	Sections []SelectListSection `json:"sections"`
}

func NormalizeMarketPair(queryArg string) string {
	switch strings.ToUpper(strings.TrimSpace(queryArg)) {
	case "GOLD", "XAUUSD", "GOLD/USD":
		return "Gold/USD"
	case "SILVER", "XAGUSD", "SILVER/USD":
		return "Silver/USD"
	case "OIL", "WTI", "WTI/USD", "CRUDE":
		return "WTI/USD"
	case "BRENT", "BRENT/USD":
		return "Brent/USD"
	case "NATGAS", "NATGAS/USD", "GAS":
		return "NatGas/USD"
	case "HEATOIL", "HEATOIL/USD":
		return "HeatOil/USD"
	case "DOW", "DOW/USD", "DOWJONES", "US30", "DJIA":
		return "Dow/USD"
	case "SPX", "SPX/USD", "SP500", "US500", "S&P500":
		return "SPX/USD"
	case "NDX", "NDX/USD", "NASDAQ", "US100", "NAS100":
		return "NDX/USD"
	case "NIKKEI", "NIKKEI225", "NIKKEI225/USD", "JP225":
		return "Nikkei225/USD"
	case "DAX", "DAX/USD", "GER30", "DE30", "GER40":
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
	case "ASX", "ASX200", "ASX/USD", "AUS200":
		return "ASX/USD"
	case "EURUSD":
		return "EUR/USD"
	case "GBPUSD":
		return "GBP/USD"
	case "USDJPY":
		return "USD/JPY"
	case "USDCHF":
		return "USD/CHF"
	case "USDCAD":
		return "USD/CAD"
	case "AUDUSD":
		return "AUD/USD"
	case "NZDUSD":
		return "NZD/USD"
	case "BTCUSD", "BTC":
		return "BTC/USD"
	case "ETHUSD", "ETH":
		return "ETH/USD"
	case "DOGEUSD", "DOGE":
		return "DOGE/USD"
	}
	return queryArg
}

func FetchForexFactoryInstrumentList(ctx context.Context) ([]FFListItem, error) {
	var allItems []FFListItem

	apiURL := "https://mds-api.forexfactory.com/instrument-list"
	var res FFListResponse
	if err := utils.FetchJSON(ctx, apiURL, &res); err == nil {
		allItems = append(allItems, res.Data...)
	}

	synthURL := "https://mds-api.forexfactory.com/synthetic-instrument-list"
	var synthRes FFListResponse
	if err := utils.FetchJSON(ctx, synthURL, &synthRes); err == nil {
		allItems = append(allItems, synthRes.Data...)
	}

	return allItems, nil
}

func FetchMarketBars(ctx context.Context, pair string) (*FFBarsResponse, error) {
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/bars?instrument=%s&interval=M5&per_page=1", url.QueryEscape(pair))
	var res FFBarsResponse
	if err := utils.FetchJSON(ctx, apiURL, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func FetchSingleMarket(ctx context.Context, pair string) (*FFInstrumentData, error) {
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", url.QueryEscape(pair))
	var res FFInstrumentResponse
	if err := utils.FetchJSON(ctx, apiURL, &res); err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("no data returned")
	}
	return &res.Data[0], nil
}

func FetchAllMarkets(ctx context.Context, pairs []string) (*FFInstrumentResponse, error) {
	apiURL := fmt.Sprintf("https://mds-api.forexfactory.com/instruments?instruments=%s", url.QueryEscape(strings.Join(pairs, ",")))
	var res FFInstrumentResponse
	if err := utils.FetchJSON(ctx, apiURL, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
