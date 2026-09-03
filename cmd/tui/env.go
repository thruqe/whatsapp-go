package tui

import (
	"fmt"
	"os"
	"strings"
)

// SaveDotEnv writes or updates the .env configuration file, preserving existing custom environment variables.
func SaveDotEnv(session string, client string, verbose bool, dbURL string) error {
	envPath := ".env"

	updates := map[string]string{
		"SESSION": session,
	}

	if client != "" && client != "default" && client != "chrome" {
		updates["CLIENT"] = client
	}
	if verbose {
		updates["VERBOSE"] = "true"
	}
	if dbURL != "" && dbURL != "default" && dbURL != "sqlite" && dbURL != "postgres" && dbURL != "postgresql" {
		updates["DATABASE_URL"] = dbURL
	}

	var lines []string
	seen := make(map[string]bool)

	if data, err := os.ReadFile(envPath); err == nil {
		rawLines := strings.SplitSeq(string(data), "\n")
		for line := range rawLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}

			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				lines = append(lines, line)
				continue
			}

			key := strings.TrimSpace(parts[0])
			if newVal, exists := updates[key]; exists {
				lines = append(lines, fmt.Sprintf("%s=%s", key, newVal))
				seen[key] = true
			} else {
				lines = append(lines, line)
			}
		}
	}

	// Append any new keys not already present in .env
	for key, val := range updates {
		if !seen[key] && val != "" {
			lines = append(lines, fmt.Sprintf("%s=%s", key, val))
		}
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return os.WriteFile(envPath, []byte(output), 0o600)
}
