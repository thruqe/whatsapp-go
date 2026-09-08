package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLoggerFunctions(t *testing.T) {
	// Test basic logging functions before InitLogger
	Info("test info message", "key", "val")
	Infof("test info formatted: %d", 42)
	Infow("test infow", "status", "ok")

	Debug("test debug message", "debug_key", 123)
	Debugf("test debug formatted: %s", "hello")
	Debugw("test debugw", "flag", true)

	Warn("test warn message", "warn_key", "alert")
	Warnf("test warn formatted: %v", true)
	Warnw("test warnw", "code", 404)

	Error("test error message", "err", os.ErrNotExist)
	Errorf("test error formatted: %s", "not found")
	Errorw("test errorw", "detail", "some failure")

	// Test typed zap fields
	Info("zap fields info", zap.String("module", "test"), zap.Int("count", 10))
	Debug("zap fields debug", zap.Bool("active", true))
	Warn("zap fields warn", zap.Float64("duration", 1.25))
	Error("zap fields error", zap.Error(os.ErrPermission))
}

func TestInitAndHookStreaming(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var receivedEntries []LogEntry
	var mu sync.Mutex

	unsub := AddHook(func(entry LogEntry) {
		mu.Lock()
		receivedEntries = append(receivedEntries, entry)
		mu.Unlock()
	})

	if err := InitLogger(tmpDir, true); err != nil {
		t.Fatalf("InitLogger failed: %v", err)
	}
	defer Close()

	if GetLevel() != zapcore.DebugLevel {
		t.Fatalf("expected DebugLevel, got %v", GetLevel())
	}

	Debug("stream debug entry", "field", "d1")
	Info("stream info entry", "field", "i1")
	Warn("stream warn entry", "field", "w1")
	Error("stream error entry", "field", "e1")

	_ = Sync()

	mu.Lock()
	count := len(receivedEntries)
	mu.Unlock()

	if count < 4 {
		t.Fatalf("expected at least 4 log entries received by hook, got %d", count)
	}

	// Verify no files or logs directory created in tmpDir
	logDirPath := filepath.Join(tmpDir, "logs")
	if _, err := os.Stat(logDirPath); !os.IsNotExist(err) {
		t.Fatalf("expected logs directory NOT to exist, but it was found")
	}

	// Test unsubscribe
	unsub()
	mu.Lock()
	beforeCount := len(receivedEntries)
	mu.Unlock()

	Info("after unsub message")
	_ = Sync()

	mu.Lock()
	afterCount := len(receivedEntries)
	mu.Unlock()

	if afterCount != beforeCount {
		t.Fatalf("expected hook not to receive logs after unsubscribe: before %d, after %d", beforeCount, afterCount)
	}
}

func TestNamedAndWith(t *testing.T) {
	sub := Named("SubModule")
	if sub == nil {
		t.Fatal("expected named logger, got nil")
	}
	sub.Infow("sub module log", "worker", 1)

	withLogger := With("req_id", "xyz-123")
	if withLogger == nil {
		t.Fatal("expected with logger, got nil")
	}
	withLogger.Info("with logger message")
}

func TestWaLoggerAdapter(t *testing.T) {
	wa := WhatsmeowStyle("WaClient", "DEBUG", true)
	if wa == nil {
		t.Fatal("expected waLog.Logger, got nil")
	}
	wa.Debugf("wa debug %s", "arg")
	wa.Infof("wa info %d", 10)
	wa.Warnf("wa warn %v", true)
	wa.Errorf("wa error %s", "err")

	subWa := wa.Sub("SubArea")
	if subWa == nil {
		t.Fatal("expected sub waLog.Logger, got nil")
	}
	subWa.Infof("sub wa log")
}

func TestSlogBridge(t *testing.T) {
	slog.Info("testing slog info through zap bridge", "bridge_key", "bridge_val")
	slog.Debug("testing slog debug through zap bridge", "num", 42)
	slog.Warn("testing slog warn through zap bridge", "issue", "minor")
	slog.Error("testing slog error through zap bridge", "err", os.ErrClosed)
}

func TestZerologAdapter(t *testing.T) {
	zlog := ZerologStyle("wacaller")
	zlog.Info().Str("call_id", "ABC-123").Bool("video", false).Msg("offer sent")
	zlog.Debug().Str("relay", "pmo1c01").Int("bytes", 352).Msg("relay connected")
	zlog.Warn().Str("call_id", "ABC-123").Msg("call ended early")
	zlog.Error().Err(os.ErrPermission).Msg("media failed")
}

func TestConcurrentLogging(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 20 {
				Info("concurrent log", "worker", id, "iter", j)
				Debug("concurrent debug", "worker", id)
			}
		}(i)
	}
	wg.Wait()
}

func TestCleanConsoleFormatting(t *testing.T) {
	enc := newConsoleEncoder(false)

	entry := zapcore.Entry{
		Level:      zapcore.InfoLevel,
		Time:       time.Date(2026, 9, 8, 12, 0, 0, 123000000, time.UTC),
		LoggerName: "client",
		Message:    "incoming message received",
	}
	fields := []zapcore.Field{
		zap.String("chat", "270613692313713@lid"),
		zap.Bool("isFromMe", true),
		zap.Int("count", 42),
	}

	buf, err := enc.EncodeEntry(entry, fields)
	if err != nil {
		t.Fatalf("EncodeEntry failed: %v", err)
	}
	output := buf.String()

	// Verify no tab characters
	if strings.Contains(output, "\t") {
		t.Errorf("output should not contain tab characters: %q", output)
	}

	// Verify bracketed logger name
	if !strings.Contains(output, "[client]") {
		t.Errorf("output should contain bracketed logger name [client]: %q", output)
	}

	// Verify clean key=value formatting without JSON braces
	if strings.Contains(output, `{"chat":`) {
		t.Errorf("output should not contain raw JSON braces: %q", output)
	}
	if !strings.Contains(output, "chat=270613692313713@lid") {
		t.Errorf("output should contain clean field chat=270613692313713@lid: %q", output)
	}
	if !strings.Contains(output, "isFromMe=true") {
		t.Errorf("output should contain clean field isFromMe=true: %q", output)
	}
	if !strings.Contains(output, "count=42") {
		t.Errorf("output should contain clean field count=42: %q", output)
	}
}
