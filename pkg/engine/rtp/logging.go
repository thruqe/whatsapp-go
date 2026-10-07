package rtp

import "github.com/rs/zerolog"

// Option configures optional, non-behavioral aspects (currently the diagnostic logger).
type Option func(*config)

type config struct {
	log zerolog.Logger
}

func resolveConfig(opts []Option) config {
	c := config{log: zerolog.Nop()}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// WithLogger sets the zerolog logger for debug/trace diagnostics; default is silent.
func WithLogger(l zerolog.Logger) Option {
	return func(c *config) { c.log = l }
}

// pickLog returns the first supplied logger, or a silent Nop logger when none is given.
func pickLog(log []zerolog.Logger) zerolog.Logger {
	if len(log) > 0 {
		return log[0]
	}
	return zerolog.Nop()
}
