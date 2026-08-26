package src

import (
	Logger "whatsrook/src/logger"

	"github.com/rs/zerolog"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// InitLogger initializes the global logger with stdout and per-level log files in sessionDir/logs.
func InitLogger(sessionDir string, verbose bool) error {
	return Logger.InitLogger(sessionDir, verbose)
}

// SetVerbose sets the global log level to DebugLevel if verbose is true, otherwise InfoLevel.
func SetVerbose(verbose bool) {
	Logger.SetVerbose(verbose)
}

// CloseLogger flushes and closes all open log files.
func CloseLogger() {
	Logger.Close()
}

// WhatsmeowStyle creates a fast waLog.Logger adapter with module prefix.
func WhatsmeowStyle(module string, minLevel string, color bool) waLog.Logger {
	return Logger.WhatsmeowStyle(module, minLevel, color)
}

// ZerologStyle creates a zerolog.Logger adapter that routes all events through Zap.
func ZerologStyle(module string) zerolog.Logger {
	return Logger.ZerologStyle(module)
}
