package waLog

import (
	"fmt"

	"go.uber.org/zap"
)

type zapLogger struct {
	mod   string
	sugar *zap.SugaredLogger
	raw   *zap.Logger
}

// Zap wraps a [*zap.Logger] to implement the [Logger] interface.
func Zap(log *zap.Logger) Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return &zapLogger{sugar: log.Sugar(), raw: log}
}

// ZapSugared wraps a [*zap.SugaredLogger] to implement the [Logger] interface.
func ZapSugared(s *zap.SugaredLogger) Logger {
	if s == nil {
		s = zap.NewNop().Sugar()
	}
	return &zapLogger{sugar: s, raw: s.Desugar()}
}

func (z *zapLogger) Warnf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Warn(msg)
	} else {
		z.sugar.Warnf(msg, args...)
	}
}

func (z *zapLogger) Errorf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Error(msg)
	} else {
		z.sugar.Errorf(msg, args...)
	}
}

func (z *zapLogger) Infof(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Info(msg)
	} else {
		z.sugar.Infof(msg, args...)
	}
}

func (z *zapLogger) Debugf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Debug(msg)
	} else {
		z.sugar.Debugf(msg, args...)
	}
}

func (z *zapLogger) Sub(module string) Logger {
	modName := module
	if z.mod != "" {
		modName = fmt.Sprintf("%s/%s", z.mod, module)
	}
	subRaw := z.raw.Named(module)
	return &zapLogger{
		mod:   modName,
		sugar: subRaw.Sugar(),
		raw:   subRaw,
	}
}

var _ Logger = &zapLogger{}
