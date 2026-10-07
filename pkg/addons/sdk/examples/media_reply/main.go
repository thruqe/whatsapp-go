package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"whatsrook/pkg/addons/sdk"
)

type dogResponse struct {
	Message string `json:"message"`
}

func main() {
	_ = sdk.Load()

	client := sdk.CreateHTTPClient(10)
	resp, err := client.Get("https://dog.ceo/api/breeds/image/random")
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("API returned status: %s", resp.Status))
	}

	var data dogResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		sdk.RespondErr(fmt.Sprintf("Parse error: %v", err))
	}

	// React first so the user knows we're responding.
	_ = sdk.SendReact("🐶")

	// Send fetched image URL with caption.
	_ = sdk.SendImage(data.Message, "Here's your random dog! 🐕")
}
