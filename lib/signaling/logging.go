package signaling

import "github.com/rs/zerolog"

// pickLog returns the first logger from a variadic logger argument, or a silent
// no-op logger when none was supplied. Stateless builders accept a trailing
// variadic logger and resolve it here so callers that pass nothing stay silent.
func pickLog(log []zerolog.Logger) zerolog.Logger {
	if len(log) > 0 {
		return log[0]
	}
	return zerolog.Nop()
}
