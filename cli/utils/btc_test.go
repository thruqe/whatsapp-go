package cliutils

import (
	"encoding/json"
	"testing"
)

const sampleBTCJSON = `{
    "meta": {
        "is_complete": false,
        "average_sec_between_blocks": 603.476,
        "previous_halving_block_number": 840000,
        "minus_blocks": 50000
    },
    "current": {
        "block_number": 963853,
        "timestamp": 1787570907
    },
    "target": {
        "block_number": 1050000,
        "predicted_timestamp": 1839558554
    },
    "bitcoin_price": {
        "price_usd": 78269.77,
        "time": 1787572126783
    }
}`

func TestBTCPredictionsUnmarshal(t *testing.T) {
	var resp BTCPredictionsResponse
	if err := json.Unmarshal([]byte(sampleBTCJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal BTC JSON: %v", err)
	}

	if resp.Current.BlockNumber != 963853 {
		t.Errorf("expected current block 963853, got %d", resp.Current.BlockNumber)
	}
	if resp.Target.BlockNumber != 1050000 {
		t.Errorf("expected target block 1050000, got %d", resp.Target.BlockNumber)
	}
	if resp.BitcoinPrice.PriceUSD != 78269.77 {
		t.Errorf("expected price 78269.77, got %f", resp.BitcoinPrice.PriceUSD)
	}
}

func TestFormatNumberWithCommas(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{9, "9"},
		{100, "100"},
		{1000, "1,000"},
		{840000, "840,000"},
		{963853, "963,853"},
		{1050000, "1,050,000"},
	}

	for _, tt := range tests {
		res := FormatNumberWithCommas(tt.input)
		if res != tt.expected {
			t.Errorf("FormatNumberWithCommas(%d) = %q, expected %q", tt.input, res, tt.expected)
		}
	}
}

func TestFormatPriceUSD(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{78269.77, "$78,269.77"},
		{100000.00, "$100,000.00"},
		{123.45, "$123.45"},
		{0.50, "$0.50"},
	}

	for _, tt := range tests {
		res := FormatPriceUSD(tt.input)
		if res != tt.expected {
			t.Errorf("FormatPriceUSD(%f) = %q, expected %q", tt.input, res, tt.expected)
		}
	}
}

func TestFormatBTCMessage(t *testing.T) {
	var resp BTCPredictionsResponse
	_ = json.Unmarshal([]byte(sampleBTCJSON), &resp)

	msg := FormatBTCMessage(&resp, "Use .bitcoin stop to end live bitcoin price")
	expected := "Bitcoin Price: $78,269.77\n\nUse .bitcoin stop to end live bitcoin price"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}
