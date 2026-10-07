package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type uselessFactResponse struct {
	Text string `json:"text"`
}

type catFactResponse struct {
	Fact string `json:"fact"`
}

var fallbackFacts = []string{
	"Honey never spoils; archaeologists have found 3,000-year-old edible honey in Egyptian tombs.",
	"Octopuses have three hearts and blue blood.",
	"Bananas are naturally slightly radioactive because they are rich in potassium.",
	"Venus is the only planet in our solar system that rotates clockwise.",
	"A day on Venus is longer than a year on Venus.",
	"Wombat poop is cube-shaped to keep it from rolling away.",
	"Sharks existed before trees.",
	"The U.S. bought Alaska for 2 cents an acre from Russia.",
	"A flock of crows is known as a murder.",
	"There are more trees on Earth than stars in the Milky Way galaxy.",
}

func main() {
	_ = sdk.Load()
	client := sdk.CreateHTTPClient(4)

	// Try UselessFacts API
	if resp, err := client.Get("https://uselessfacts.jsph.pl/api/v2/facts/random"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data uselessFactResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				if t := strings.TrimSpace(data.Text); t != "" {
					sdk.Respond(fmt.Sprintf("💡 *Fact:* %s", t))
					return
				}
			}
		}
	}

	// Try CatFact API
	if resp, err := client.Get("https://catfact.ninja/fact"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data catFactResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				if t := strings.TrimSpace(data.Fact); t != "" {
					sdk.Respond(fmt.Sprintf("💡 *Fact:* %s", t))
					return
				}
			}
		}
	}

	// Fallback
	choice := fallbackFacts[rand.IntN(len(fallbackFacts))]
	sdk.Respond(fmt.Sprintf("💡 *Fact:* %s", choice))
}
