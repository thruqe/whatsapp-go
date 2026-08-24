package waLog

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestZapLogger(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	zl := zap.New(core)

	logger := Zap(zl)
	logger.Debugf("debug message: %d", 1)
	logger.Infof("info message: %s", "hello")
	logger.Warnf("warn message: %v", true)
	logger.Errorf("error message: %s", "fail")

	if logs.Len() != 4 {
		t.Fatalf("expected 4 logs, got %d", logs.Len())
	}

	sub := logger.Sub("submod")
	sub.Infof("sub message")

	if logs.Len() != 5 {
		t.Fatalf("expected 5 logs, got %d", logs.Len())
	}
	last := logs.All()[4]
	if last.LoggerName != "submod" {
		t.Fatalf("expected LoggerName submod, got %s", last.LoggerName)
	}
}
