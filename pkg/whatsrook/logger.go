package whatsrook

import (
	waLog "go.mau.fi/whatsmeow/util/log"
)

var defaultLogger waLog.Logger = waLog.Stdout("whatsrook", "INFO", true)

// SetLogger overrides the package-level logger used by whatsrook.
func SetLogger(l waLog.Logger) {
	if l != nil {
		defaultLogger = l
	}
}

// GetLogger returns the current package-level logger.
func GetLogger() waLog.Logger {
	return defaultLogger
}
