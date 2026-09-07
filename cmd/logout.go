package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"whatsrook"
	"whatsrook/logger"
)

// handleLogoutCLI manages the command-line session logout and unpairing flow.
func handleLogoutCLI(ctx context.Context, args CLIArgs) {
	dataDir := whatsrook.DefaultDataDir()
	session := args.Session

	if session == "" {
		sessions, err := whatsrook.ListStoredSessions(ctx, dataDir, args.Database)
		if err != nil {
			logger.Error("failed to list stored sessions for logout", "err", err)
			fmt.Printf("Error: failed to query stored sessions: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No active stored sessions found to logout.")
			return
		}
		if len(sessions) == 1 {
			session = sessions[0].User
			fmt.Printf("Found single stored session +%s (%s). Initiating logout...\n", session, sessions[0].Platform)
		} else {
			if !isTerminalInteractive() {
				fmt.Println("Multiple stored sessions found. Please specify phone number to logout:")
				for _, s := range sessions {
					fmt.Printf("  +%s (%s)\n", s.User, s.Platform)
				}
				os.Exit(1)
			}
			fmt.Println("\nStored sessions:")
			for i, s := range sessions {
				fmt.Printf("  [%d] +%s (%s)\n", i+1, s.User, s.Platform)
			}
			fmt.Print("\nEnter session number or phone to logout (or Ctrl+C to exit): ")
			var input string
			if _, err := fmt.Scan(&input); err != nil {
				return
			}
			input = strings.TrimSpace(input)
			if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(sessions) {
				session = sessions[idx-1].User
			} else {
				cleanInput := strings.TrimPrefix(input, "+")
				for _, s := range sessions {
					if s.User == cleanInput || s.User == input {
						session = s.User
						break
					}
				}
				if session == "" {
					session = cleanInput
				}
			}
		}
	}

	cleanSession := strings.TrimPrefix(session, "+")
	logger.Info("initiating logout for session", "session", cleanSession)
	fmt.Printf("Logging out session +%s...\n", cleanSession)

	if err := whatsrook.DeleteStoredSession(ctx, dataDir, args.Database, cleanSession); err != nil {
		logger.Error("failed to complete session logout", "session", cleanSession, "err", err)
		fmt.Printf("Error: failed to complete logout for session +%s: %v\n", cleanSession, err)
		os.Exit(1)
	}

	logger.Info("session logged out and credentials removed", "session", cleanSession)
	fmt.Printf("Session +%s logged out and credentials purged successfully.\n", cleanSession)
}
