// dispatch package provides structured error handling and friendly error responses for commands.
package dispatch

import (
	"fmt"
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
		return fmt.Sprintf("%s: %v", e.UserMessage, e.Cause)
	}
	return e.UserMessage
}

// Unwrap returns the underlying error cause.
func (e *PluginError) Unwrap() error {
	return e.Cause
}

// Fail constructs a simple user-facing PluginError.
func Fail(msg string) error {
	return &PluginError{UserMessage: msg}
}

// Failf constructs a formatted user-facing PluginError.
func Failf(format string, a ...any) error {
	return &PluginError{UserMessage: fmt.Sprintf(format, a...)}
}

// ErrUsage returns a standardized usage error message.
func ErrUsage(usage string) error {
	return Failf("Usage: %s", usage)
}

// ErrPermission returns a standardized permission denied error message.
func ErrPermission(msg string) error {
	if msg == "" {
		msg = "This command is restricted to sudoers/owners only."
	}
	return Fail(msg)
}

// ErrAdminOnly returns an admin-only restriction error.
func ErrAdminOnly() error {
	return Fail("This command can only be executed by group admins.")
}

// ErrBotAdminOnly returns a bot admin requirement error.
func ErrBotAdminOnly() error {
	return Fail("WhatsRook must be an admin in this group to execute this command.")
}

// ErrGroupOnly returns a group-only restriction error.
func ErrGroupOnly() error {
	return Fail("This command can only be used in group chats.")
}

// ErrOwnerOnly returns an owner-only restriction error.
func ErrOwnerOnly() error {
	return Fail("This command can only be executed by the bot owner.")
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
