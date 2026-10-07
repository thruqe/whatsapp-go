package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type dadJoke struct {
	Joke string `json:"joke"`
}

type officialJoke struct {
	Setup     string `json:"setup"`
	Punchline string `json:"punchline"`
}

var fallbackJokes = []string{
	"Why don't scientists trust atoms? Because they make up everything!",
	"Why did the scarecrow win an award? Because he was outstanding in his field!",
	"What do you call fake spaghetti? An impasta!",
	"Why do programmers prefer dark mode? Because light attracts bugs!",
	"How do you organize a space party? You planet!",
	"What do you call a pig that knows karate? A pork chop!",
	"Why don't eggs tell jokes? They'd crack each other up!",
}

func main() {
	_ = sdk.Load()
	client := sdk.CreateHTTPClient(4)

	// Try ICanHazDadJoke API
	req, err := http.NewRequest(http.MethodGet, "https://icanhazdadjoke.com/", nil)
	if err == nil {
		req.Header.Set("Accept", "application/json")
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var data dadJoke
				if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
					if j := strings.TrimSpace(data.Joke); j != "" {
						sdk.Respond(fmt.Sprintf("😂 %s", j))
						return
					}
				}
			}
		}
	}

	// Try Official Joke API
	if resp, err := client.Get("https://official-joke-api.appspot.com/random_joke"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data officialJoke
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				setup := strings.TrimSpace(data.Setup)
				punch := strings.TrimSpace(data.Punchline)
				if setup != "" && punch != "" {
					sdk.Respond(fmt.Sprintf("😂 %s\n\n%s", setup, punch))
					return
				}
			}
		}
	}

	// Fallback
	choice := fallbackJokes[rand.IntN(len(fallbackJokes))]
	sdk.Respond(fmt.Sprintf("😂 %s", choice))
}
