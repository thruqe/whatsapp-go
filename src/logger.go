package src

import (
	Logger "whatsrook/src/logger"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// InitLogger initializes the global logger with stdout and per-level log files in sessionDir/logs.
func InitLogger(sessionDir string, verbose bool) error {
	return Logger.InitLogger(sessionDir, verbose)
}

// CloseLogger flushes and closes all open log files.
func CloseLogger() {
	Logger.Close()
}

// WhatsmeowStyle creates a fast waLog.Logger adapter with module prefix.
func WhatsmeowStyle(module string, minLevel string, color bool) waLog.Logger {
	return Logger.WhatsmeowStyle(module, minLevel, color)
}
