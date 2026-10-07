package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type urbanResponse struct {
	List []urbanEntry `json:"list"`
}

type urbanEntry struct {
	Word       string `json:"word"`
	Definition string `json:"definition"`
	Example    string `json:"example"`
	Author     string `json:"author"`
}

func cleanBrackets(s string) string {
	r := strings.NewReplacer("[", "", "]", "")
	return r.Replace(s)
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond("Usage: urban [term]")
		return
	}

	apiURL := fmt.Sprintf("https://api.urbandictionary.com/v0/define?term=%s", url.QueryEscape(query))
	client := sdk.CreateHTTPClient(10)
	resp, err := client.Get(apiURL)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("Urban Dictionary returned status: %s", resp.Status))
	}

	var data urbanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to parse Urban Dictionary response: %v", err))
	}

	if len(data.List) == 0 {
		sdk.Respond(fmt.Sprintf("Could not find Urban Dictionary definition for %q.", query))
		return
	}

	first := data.List[0]
	cleanDef := strings.TrimSpace(cleanBrackets(first.Definition))
	cleanExample := strings.TrimSpace(cleanBrackets(first.Example))

	var b strings.Builder
	fmt.Fprintf(&b, "*Urban Dictionary: %s*\n\n*Definition:*\n%s", first.Word, cleanDef)
	if cleanExample != "" {
		fmt.Fprintf(&b, "\n\n*Example:*\n%s", cleanExample)
	}
	if author := strings.TrimSpace(first.Author); author != "" {
		fmt.Fprintf(&b, "\n\nAuthor: %s", author)
	}

	sdk.Respond(b.String())
}
