package plugins

import (
	"log/slog"
)

type PluginError struct {
	UserMessage string
	Cause       error
}

func (e *PluginError) Error() string {
	if e.Cause != nil {
		return Sprintf("%s: %v", e.UserMessage, e.Cause)
	}
	return e.UserMessage
}

func (e *PluginError) Unwrap() error {
	return e.Cause
}

func Fail(msg string) error {
	return &PluginError{UserMessage: msg}
}

func Failf(format string, a ...any) error {
	return &PluginError{UserMessage: Sprintf(format, a...)}
}

func WrapError(userMsg string, cause error) error {
	return &PluginError{UserMessage: userMsg, Cause: cause}
}

func ErrUsage(usage string) error {
	return Failf("Usage: %s", usage)
}

func ErrPermission(msg string) error {
	if msg == "" {
		msg = "This command is restricted to sudoers/owners only."
	}
	return Fail(msg)
}

func ErrMediaRequired() error {
	return Fail("No media found in this message or the replied message.")
}

func ErrGroupOnly() error {
	return Fail("This command can only be used in a group chat.")
}

func logHandlerErr(name string, err error) {
	if err == nil {
		return
	}
	slog.Error("command handler failed", "command", name, "err", err)
}

func LogHandlerErrWithContext(cctx *Context, name string, err error) {
	if err == nil {
		return
	}

	slog.Error("command handler failed", "command", name, "err", err)

	if cctx == nil {
		return
	}

	if pErr, ok := err.(*PluginError); ok {
		_ = cctx.Reply(pErr.UserMessage)
		return
	}

	_ = cctx.Replyf("Command execution failed: %v", err)
}
