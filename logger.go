package whatsrook

import (
	Logger "whatsrook/logger"

	"github.com/rs/zerolog"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// LogEntry encapsulates a structured log event dispatched to subscribers.
type LogEntry = Logger.LogEntry

// LogHook defines a callback receiving live structured log entries.
type LogHook = Logger.LogHook

// InitLogger initializes the global logger with stdout and in-memory event stream/hooks.
func InitLogger(sessionDir string, verbose bool) error {
	return Logger.InitLogger(sessionDir, verbose)
}

// AddHook registers a callback that receives structured log entries in real-time.
func AddHook(fn LogHook) func() {
	return Logger.AddHook(fn)
}

// ClearHooks removes all currently registered log hooks.
func ClearHooks() {
	Logger.ClearHooks()
}

// SetVerbose sets the global log level to DebugLevel if verbose is true, otherwise InfoLevel.
func SetVerbose(verbose bool) {
	Logger.SetVerbose(verbose)
}

// CloseLogger flushes buffered log entries and resets the logger.
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
