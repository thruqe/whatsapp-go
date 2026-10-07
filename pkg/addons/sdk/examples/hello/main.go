package main

import (
	"fmt"
	"whatsrook/pkg/addons/sdk"
)

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond(fmt.Sprintf("Usage: %shello <name>", req.EffectivePrefix()))
		return
	}

	sdk.Respond(fmt.Sprintf("Hello, %s! 👋", query))
}
