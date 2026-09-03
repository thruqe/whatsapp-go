package games

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"whatsrook/builder"
	"whatsrook/httpx"
)

var funHTTPClient = httpx.NewClient(4 * time.Second)

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

var fallbackQuotes = []string{
	"\"The secret of getting ahead is getting started.\" – Mark Twain",
	"\"It always seems impossible until it's done.\" – Nelson Mandela",
	"\"Do what you can, with what you have, where you are.\" – Theodore Roosevelt",
	"\"In the middle of every difficulty lies opportunity.\" – Albert Einstein",
	"\"Success is not final, failure is not fatal: It is the courage to continue that counts.\" – Winston Churchill",
	"\"Chains of habit are too light to be felt until they are too heavy to be broken.\" – Warren Buffett",
	"\"The only way to do great work is to love what you do.\" – Steve Jobs",
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

var fallbackRizz = []string{
	"Are you a magician? Because whenever I look at you, everyone else disappears.",
	"Do you have a map? I keep getting lost in your eyes.",
	"Is your name Google? Because you have everything I’ve been searching for.",
	"Are you Wi-Fi? Because I'm feeling a really strong connection.",
	"If beauty were time, you’d be an eternity.",
	"Are you a campfire? Because you're hot and I want s'more.",
	"Do you believe in love at first sight, or should I walk by again?",
	"Is it bright in here, or is it just your smile?",
	"Are you an interior decorator? Because when I saw you, the whole room became beautiful.",
	"I must be a snowflake, because I've fallen for you.",
}

func GetRandomFact(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://uselessfacts.jsph.pl/api/v2/facts/random", nil)
	if err == nil {
		resp, errDo := funHTTPClient.Do(req)
		if errDo == nil {
			defer resp.Body.Close()
			var res struct {
				Text string `json:"text"`
			}
			if json.NewDecoder(resp.Body).Decode(&res) == nil && strings.TrimSpace(res.Text) != "" {
				return strings.TrimSpace(res.Text)
			}
		}
	}
	req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, "https://catfact.ninja/fact", nil)
	if err2 == nil {
		resp2, errDo2 := funHTTPClient.Do(req2)
		if errDo2 == nil {
			defer resp2.Body.Close()
			var res2 struct {
				Fact string `json:"fact"`
			}
			if json.NewDecoder(resp2.Body).Decode(&res2) == nil && strings.TrimSpace(res2.Fact) != "" {
				return strings.TrimSpace(res2.Fact)
			}
		}
	}
	return fallbackFacts[rand.Intn(len(fallbackFacts))]
}

func GetRandomQuote(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://dummyjson.com/quotes/random", nil)
	if err == nil {
		resp, errDo := funHTTPClient.Do(req)
		if errDo == nil {
			defer resp.Body.Close()
			var res struct {
				Quote  string `json:"quote"`
				Author string `json:"author"`
			}
			if json.NewDecoder(resp.Body).Decode(&res) == nil && strings.TrimSpace(res.Quote) != "" {
				q := strings.TrimSpace(res.Quote)
				if res.Author != "" {
					return builder.Sprintf("%q – %s", q, res.Author)
				}
				return builder.Sprintf("%q", q)
			}
		}
	}
	req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, "https://zenquotes.io/api/random", nil)
	if err2 == nil {
		resp2, errDo2 := funHTTPClient.Do(req2)
		if errDo2 == nil {
			defer resp2.Body.Close()
			var resList []struct {
				Q string `json:"q"`
				A string `json:"a"`
			}
			if json.NewDecoder(resp2.Body).Decode(&resList) == nil && len(resList) > 0 && strings.TrimSpace(resList[0].Q) != "" {
				q := strings.TrimSpace(resList[0].Q)
				a := strings.TrimSpace(resList[0].A)
				if a != "" {
					return builder.Sprintf("%q – %s", q, a)
				}
				return builder.Sprintf("%q", q)
			}
		}
	}
	return fallbackQuotes[rand.Intn(len(fallbackQuotes))]
}

func GetRandomJoke(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://icanhazdadjoke.com/", nil)
	if err == nil {
		req.Header.Set("Accept", "application/json")
		resp, errDo := funHTTPClient.Do(req)
		if errDo == nil {
			defer resp.Body.Close()
			var res struct {
				Joke string `json:"joke"`
			}
			if json.NewDecoder(resp.Body).Decode(&res) == nil && strings.TrimSpace(res.Joke) != "" {
				return strings.TrimSpace(res.Joke)
			}
		}
	}
	req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, "https://official-joke-api.appspot.com/random_joke", nil)
	if err2 == nil {
		resp2, errDo2 := funHTTPClient.Do(req2)
		if errDo2 == nil {
			defer resp2.Body.Close()
			var res2 struct {
				Setup     string `json:"setup"`
				Punchline string `json:"punchline"`
			}
			if json.NewDecoder(resp2.Body).Decode(&res2) == nil && strings.TrimSpace(res2.Setup) != "" {
				return builder.Sprintf("%s\n\n%s", strings.TrimSpace(res2.Setup), strings.TrimSpace(res2.Punchline))
			}
		}
	}
	return fallbackJokes[rand.Intn(len(fallbackJokes))]
}

func GetRandomRizz(ctx context.Context) string {
	return fallbackRizz[rand.Intn(len(fallbackRizz))]
}
