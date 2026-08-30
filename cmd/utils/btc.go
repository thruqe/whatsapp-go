package cliutils

import (
	"fmt"
)

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
