// dispatch package provides structured error handling and friendly error responses for commands.
package dispatch

import (
	Logger "whatsrook/logger"
)

// PluginError wraps user-facing command error messages with an optional underlying root cause.
type PluginError struct {
	// UserMessage is the friendly explanation displayed back to the user in chat.
	UserMessage string
	// Cause is the underlying system or network error.
	Cause error
}

// Error implements the standard error interface.
func (e *PluginError) Error() string {
	if e.Cause != nil {
		return Sprintf("%s: %v", e.UserMessage, e.Cause)
	}
	return e.UserMessage
}

// Unwrap returns the underlying error cause.
func (e *PluginError) Unwrap() error {
	return e.Cause
}

// Failf constructs a formatted user-facing PluginError.
func Failf(format string, a ...any) error {
	return &PluginError{UserMessage: Sprintf(format, a...)}
}

// ErrUsage returns a standardized usage error message.
func ErrUsage(usage string) error {
	return Failf("Usage: %s", usage)
}

// LogHandlerErrWithContext logs an execution error along with the caller context.
func LogHandlerErrWithContext(cctx *Context, name string, err error) {
	if err == nil {
		return
	}
	chatStr := ""
	senderStr := ""
	if cctx != nil {
		chatStr = cctx.Chat.String()
		senderStr = cctx.Sender.String()
	}
	Logger.Error("command handler failed", "command", name, "chat", chatStr, "sender", senderStr, "err", err)
}
