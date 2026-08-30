package cliutils

import (
	"testing"
)

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
