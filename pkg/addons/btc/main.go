package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"whatsrook/pkg/addons/sdk"
)

const (
	tickInterval = 1500 * time.Millisecond
	maxDuration  = 5 * time.Minute
)

type watcherGuruResponse struct {
	BitcoinPrice struct {
		PriceUSD       float64 `json:"price_usd"`
		PriceChange24h float64 `json:"price_change_24h"`
	} `json:"bitcoin_price"`
	Current struct {
		BlockNumber int64 `json:"block_number"`
	} `json:"current"`
	Target struct {
		BlockNumber int64 `json:"block_number"`
	} `json:"target"`
}

type binancePriceResponse struct {
	Price string `json:"price"`
}

func formatCommas(n int64) string {
	in := strconv.FormatInt(int64(math.Abs(float64(n))), 10)
	var out []byte
	l := len(in)
	for i := range l {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, in[i])
	}
	if n < 0 {
		return "-" + string(out)
	}
	return string(out)
}

func formatUSD(price float64) string {
	intPart := int64(price)
	dec := int64(math.Round((price - float64(intPart)) * 100))
	if dec >= 100 {
		dec = 99
	} else if dec < 0 {
		dec = 0
	}
	return fmt.Sprintf("$%s.%02d", formatCommas(intPart), dec)
}

func formatChange(change float64) string {
	if change > 0 {
		return fmt.Sprintf("📈 +%.2f%%", change)
	} else if change < 0 {
		return fmt.Sprintf("📉 %.2f%%", change)
	}
	return "➡️ 0.00%"
}

func buildMessage(data *watcherGuruResponse, prefix, status string) string {
	priceStr := formatUSD(data.BitcoinPrice.PriceUSD)
	changeStr := ""
	if data.BitcoinPrice.PriceChange24h != 0 {
		changeStr = fmt.Sprintf(" (%s)", formatChange(data.BitcoinPrice.PriceChange24h))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "₿ *Bitcoin (BTC)*\n\n*Price:* %s%s", priceStr, changeStr)

	if data.Current.BlockNumber > 0 {
		fmt.Fprintf(&b, "\n*Current Block:* %s", formatCommas(data.Current.BlockNumber))
	}
	if data.Target.BlockNumber > 0 {
		fmt.Fprintf(&b, "\n*Target Block:* %s", formatCommas(data.Target.BlockNumber))
	}
	if data.Target.BlockNumber > 0 && data.Current.BlockNumber > 0 {
		remaining := data.Target.BlockNumber - data.Current.BlockNumber
		if remaining > 0 {
			fmt.Fprintf(&b, "\n*Blocks Remaining:* %s", formatCommas(remaining))
		}
	}

	if status != "" {
		fmt.Fprintf(&b, "\n\n_%s_", status)
	}
	return b.String()
}

func fetchData(client *http.Client) *watcherGuruResponse {
	resp, err := client.Get("https://watcher.guru/api/v1/bitcoin-halving")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var data watcherGuruResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || data.BitcoinPrice.PriceUSD <= 0 {
		return nil
	}
	return &data
}

func fetchFallbackPrice(client *http.Client) (float64, error) {
	resp, err := client.Get("https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status: %s", resp.Status)
	}
	var data binancePriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(data.Price, 64)
}

func main() {
	req := sdk.Load()

	if strings.EqualFold(req.Query(), "stop") || req.IsCancel() {
		sdk.Respond("🛑 Bitcoin live tracker stopped.")
		return
	}

	prefix := req.EffectivePrefix()
	client := sdk.CreateHTTPClient(10)

	initialData := fetchData(client)
	if initialData == nil {
		price, err := fetchFallbackPrice(client)
		if err != nil {
			sdk.RespondErr("Failed to fetch Bitcoin market data.")
		}
		sdk.Respond(fmt.Sprintf("₿ *Bitcoin (BTC)*\n\n*Price:* %s\n\n_Powered by Binance · %smarkets for more_", formatUSD(price), prefix))
		return
	}

	status := fmt.Sprintf("Use %sbtc stop to end live tracking", prefix)
	initialText := buildMessage(initialData, prefix, status)

	msgID, err := sdk.SendReplyLive(initialText)
	if err != nil || msgID == "" {
		sdk.Respond(initialText)
		return
	}

	start := time.Now()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for range ticker.C {
		if time.Since(start) >= maxDuration {
			if data := fetchData(client); data != nil {
				finalText := buildMessage(data, prefix, "⏱️ Live tracking ended (5m timeout).")
				_ = sdk.SendEditLive(msgID, finalText)
			}
			break
		}

		if data := fetchData(client); data != nil {
			updated := buildMessage(data, prefix, status)
			_ = sdk.SendEditLive(msgID, updated)
		}
	}

	_ = sdk.SendDone()
}
