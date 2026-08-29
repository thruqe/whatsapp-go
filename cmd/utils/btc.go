package cliutils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	utils "whatsrook/src"
)

// BTCPredictionsResponse matches the response of https://api.watcher.guru/bitcoinhalving/predictions
type BTCPredictionsResponse struct {
	Meta struct {
		IsComplete                 bool    `json:"is_complete"`
		AverageSecBetweenBlocks    float64 `json:"average_sec_between_blocks"`
		PreviousHalvingBlockNumber int64   `json:"previous_halving_block_number"`
		MinusBlocks                int64   `json:"minus_blocks"`
	} `json:"meta"`
	Current struct {
		BlockNumber int64 `json:"block_number"`
		Timestamp   int64 `json:"timestamp"`
	} `json:"current"`
	Target struct {
		BlockNumber        int64 `json:"block_number"`
		PredictedTimestamp int64 `json:"predicted_timestamp"`
	} `json:"target"`
	BitcoinPrice struct {
		PriceUSD float64 `json:"price_usd"`
		Time     int64   `json:"time"`
	} `json:"bitcoin_price"`
}

// FetchBTCPredictions queries the Watcher Guru Bitcoin Halving and Price Predictions API.
func FetchBTCPredictions(ctx context.Context) (*BTCPredictionsResponse, error) {
	apiURL := "https://api.watcher.guru/bitcoinhalving/predictions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTC API request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch BTC data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BTC API returned http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read BTC response: %w", err)
	}

	var data BTCPredictionsResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse BTC JSON: %w", err)
	}

	return &data, nil
}

// FormatNumberWithCommas formats an integer with thousands separator commas (e.g. 1050000 -> "1,050,000").
func FormatNumberWithCommas(n int64) string {
	in := fmt.Sprintf("%d", n)
	if n < 0 {
		in = in[1:]
	}
	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	if n < 0 {
		return "-" + string(out)
	}
	return string(out)
}

// FormatPriceUSD formats a float USD price with commas and two decimals (e.g. 78269.77 -> "$78,269.77").
func FormatPriceUSD(price float64) string {
	intPart := int64(price)
	decPart := int64((price-float64(intPart))*100 + 0.5)
	if decPart >= 100 {
		decPart = 99
	}
	return fmt.Sprintf("$%s.%02d", FormatNumberWithCommas(intPart), decPart)
}

// FormatBTCMessage formats the live Bitcoin price message.
func FormatBTCMessage(data *BTCPredictionsResponse, statusLine string) string {
	if data == nil {
		return "Bitcoin data unavailable."
	}

	tb := utils.NewText().Linef("Bitcoin Price: %s", FormatPriceUSD(data.BitcoinPrice.PriceUSD))
	if statusLine != "" {
		tb.Blank().Line(statusLine)
	}

	return tb.Trimmed()
}
