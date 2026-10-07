package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type dummyQuote struct {
	Quote  string `json:"quote"`
	Author string `json:"author"`
}

type zenQuote struct {
	Q string `json:"q"`
	A string `json:"a"`
}

var fallbackQuotes = []string{
	`"The secret of getting ahead is getting started." – Mark Twain`,
	`"It always seems impossible until it's done." – Nelson Mandela`,
	`"Do what you can, with what you have, where you are." – Theodore Roosevelt`,
	`"In the middle of every difficulty lies opportunity." – Albert Einstein`,
	`"Success is not final, failure is not fatal: It is the courage to continue that counts." – Winston Churchill`,
	`"Chains of habit are too light to be felt until they are too heavy to be broken." – Warren Buffett`,
	`"The only way to do great work is to love what you do." – Steve Jobs`,
}

func main() {
	_ = sdk.Load()
	client := sdk.CreateHTTPClient(4)

	// Try DummyJSON API
	if resp, err := client.Get("https://dummyjson.com/quotes/random"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data dummyQuote
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				q := strings.TrimSpace(data.Quote)
				a := strings.TrimSpace(data.Author)
				if q != "" {
					if a != "" {
						sdk.Respond(fmt.Sprintf("💬 %q – %s", q, a))
					} else {
						sdk.Respond(fmt.Sprintf("💬 %q", q))
					}
					return
				}
			}
		}
	}

	// Try ZenQuotes API
	if resp, err := client.Get("https://zenquotes.io/api/random"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var list []zenQuote
			if err := json.NewDecoder(resp.Body).Decode(&list); err == nil && len(list) > 0 {
				q := strings.TrimSpace(list[0].Q)
				a := strings.TrimSpace(list[0].A)
				if q != "" {
					if a != "" {
						sdk.Respond(fmt.Sprintf("💬 %q – %s", q, a))
					} else {
						sdk.Respond(fmt.Sprintf("💬 %q", q))
					}
					return
				}
			}
		}
	}

	// Fallback
	choice := fallbackQuotes[rand.IntN(len(fallbackQuotes))]
	sdk.Respond(fmt.Sprintf("💬 %s", choice))
}
