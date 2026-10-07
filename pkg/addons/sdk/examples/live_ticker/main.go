package main

import (
	"fmt"
	"time"
	"whatsrook/pkg/addons/sdk"
)

func main() {
	req := sdk.Load()

	// Only group admins may use this command in groups.
	if req.IsGroup && !req.IsAdmin {
		_ = sdk.SendReact("❌")
		sdk.Respond("This feature is for group admins only.")
		return
	}

	// Show loader while setting up.
	_ = sdk.SendLoader("Initializing live tracker...")

	// React to acknowledge command.
	_ = sdk.SendReact("🚀")

	// Ask which asset to track via an interactive poll.
	_ = sdk.SendPoll("Which asset to track?", []string{"BTC", "ETH", "Gold"})

	// Send initial live message and capture msgID for in-place edits.
	msgID, err := sdk.SendReplyLive("⏳ Initializing live tracker...")
	if err != nil || msgID == "" {
		sdk.Respond("Failed to start live session.")
		return
	}

	// Simulate 5 ticks of a live data feed.
	for tick := 1; tick <= 5; tick++ {
		time.Sleep(1500 * time.Millisecond)
		_ = sdk.SendEditLive(msgID, fmt.Sprintf("📈 Tracker tick #%d...", tick))
	}

	// Final edit to show completion.
	_ = sdk.SendEditLive(msgID, "✅ Live session complete.")
	_ = sdk.SendDone()
}
