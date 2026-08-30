package cliutils

import (
	"context"
	"fmt"

	utils "whatsrook/src"
)

// BTCPredictionItem holds individual target milestone prices and predicted dates.
type BTCPredictionItem struct {
	Price    string `json:"price"`
	Date     string `json:"date"`
	Accuracy string `json:"accuracy"`
}

// BTCPredictionsResponse mirrors the Watcher Guru prediction endpoint payload.
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
	Status       bool                `json:"status"`
	Predictions  []BTCPredictionItem `json:"predictions"`
	BitcoinPrice struct {
		PriceUSD float64 `json:"price_usd"`
		Time     int64   `json:"time"`
	} `json:"bitcoin_price"`
}

// FetchBTCPredictions queries the Watcher Guru Bitcoin Halving and Price Predictions API.
func FetchBTCPredictions(ctx context.Context) (*BTCPredictionsResponse, error) {
	apiURL := "https://api.watcher.guru/bitcoinhalving/predictions"
	var res BTCPredictionsResponse
	if err := utils.FetchJSON(ctx, apiURL, &res, utils.WithHeader("Accept", "application/json")); err != nil {
		return nil, fmt.Errorf("failed to fetch BTC data: %w", err)
	}
	return &res, nil
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
