package Logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// LogEntry encapsulates a structured log event dispatched to subscribers.
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Caller    string         `json:"caller,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// LogHook defines a callback receiving live structured log entries.
type LogHook func(entry LogEntry)

var (
	mu          sync.RWMutex
	atomicLevel zap.AtomicLevel
	rawLogger   *zap.Logger
	sugarLogger *zap.SugaredLogger
	isInit      atomic.Bool

	hooksMu    sync.RWMutex
	nextHookID atomic.Uint32
	hooks      = make(map[uint32]LogHook)
)

func init() {
	atomicLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	rawLogger = newDefaultLogger(atomicLevel, os.Stdout)
	sugarLogger = rawLogger.Sugar()
	setupSlogBridge(rawLogger)
}

// L returns the underlying *zap.Logger.
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return rawLogger
}

// S returns the underlying *zap.SugaredLogger.
func S() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return sugarLogger
}

// SetLevel sets the global minimum logging level dynamically.
func SetLevel(lvl zapcore.Level) {
	atomicLevel.SetLevel(lvl)
}

// SetVerbose sets the global log level to DebugLevel if verbose is true, otherwise InfoLevel.
func SetVerbose(verbose bool) {
	if verbose {
		atomicLevel.SetLevel(zapcore.DebugLevel)
	} else {
		atomicLevel.SetLevel(zapcore.InfoLevel)
	}
}

// GetLevel returns the current global logging level.
func GetLevel() zapcore.Level {
	return atomicLevel.Level()
}

// AddHook registers a callback that receives structured log entries in real-time.
// It returns an unsubscribe closure to deregister the hook.
func AddHook(fn LogHook) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	id := nextHookID.Add(1)
	hooksMu.Lock()
	hooks[id] = fn
	hooksMu.Unlock()

	return func() {
		hooksMu.Lock()
		delete(hooks, id)
		hooksMu.Unlock()
	}
}

// ClearHooks removes all currently registered log hooks.
func ClearHooks() {
	hooksMu.Lock()
	hooks = make(map[uint32]LogHook)
	hooksMu.Unlock()
}

// InitLogger initializes the global logger with console stdout and event hook streaming (no disk file creation).
func InitLogger(_ string, verbose bool) error {
	mu.Lock()
	defer mu.Unlock()

	if verbose {
		atomicLevel.SetLevel(zapcore.DebugLevel)
	} else {
		atomicLevel.SetLevel(zapcore.InfoLevel)
	}

	consoleEncoder := newConsoleEncoder(true)
	stdoutCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.Lock(os.Stdout),
		atomicLevel,
	)

	hCore := newHookCore(atomicLevel)

	teeCore := zapcore.NewTee(stdoutCore, hCore)
	rawLogger = zap.New(teeCore, zap.AddCaller(), zap.AddCallerSkip(1))
	sugarLogger = rawLogger.Sugar()
	isInit.Store(true)

	setupSlogBridge(rawLogger)
	return nil
}

// Close flushes buffered log entries and resets the logger.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if rawLogger != nil {
		_ = rawLogger.Sync()
	}

	ClearHooks()

	rawLogger = newDefaultLogger(atomicLevel, os.Stdout)
	sugarLogger = rawLogger.Sugar()
	setupSlogBridge(rawLogger)
	isInit.Store(false)
}

// CloseLogger is an alias for Close.
func CloseLogger() {
	Close()
}

// Sync flushes any buffered log entries.
func Sync() error {
	mu.RLock()
	defer mu.RUnlock()
	if rawLogger != nil {
		return rawLogger.Sync()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Stream & Hook Zap Core
// ─────────────────────────────────────────────────────────────

type hookCore struct {
	zapcore.LevelEnabler
	fields map[string]any
}

func newHookCore(enabler zapcore.LevelEnabler) *hookCore {
	return &hookCore{
		LevelEnabler: enabler,
		fields:       make(map[string]any),
	}
}

func (c *hookCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &hookCore{
		LevelEnabler: c.LevelEnabler,
		fields:       make(map[string]any, len(c.fields)+len(fields)),
	}
	maps.Copy(clone.fields, c.fields)
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	maps.Copy(clone.fields, enc.Fields)
	return clone
}

func (c *hookCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *hookCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	hooksMu.RLock()
	count := len(hooks)
	if count == 0 {
		hooksMu.RUnlock()
		return nil
	}
	activeHooks := make([]LogHook, 0, count)
	for _, h := range hooks {
		activeHooks = append(activeHooks, h)
	}
	hooksMu.RUnlock()

	mergedFields := make(map[string]any, len(c.fields)+len(fields))
	maps.Copy(mergedFields, c.fields)
	if len(fields) > 0 {
		enc := zapcore.NewMapObjectEncoder()
		for _, f := range fields {
			f.AddTo(enc)
		}
		maps.Copy(mergedFields, enc.Fields)
	}

	callerStr := ""
	if entry.Caller.Defined {
		callerStr = entry.Caller.TrimmedPath()
	}

	logEntry := LogEntry{
		Timestamp: entry.Time,
		Level:     entry.Level.CapitalString(),
		Message:   entry.Message,
		Caller:    callerStr,
		Fields:    mergedFields,
	}

	for _, h := range activeHooks {
		func() {
			defer func() {
				_ = recover()
			}()
			h(logEntry)
		}()
	}

	return nil
}

func (c *hookCore) Sync() error {
	return nil
}

// ─────────────────────────────────────────────────────────────
// Encoders & Default Logger
// ─────────────────────────────────────────────────────────────

func newConsoleEncoder(color bool) zapcore.Encoder {
	cfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "",
		FunctionKey:    "",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	if color {
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return zapcore.NewConsoleEncoder(cfg)
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05.000"))
}

func newDefaultLogger(lvl zapcore.LevelEnabler, w io.Writer) *zap.Logger {
	encoder := newConsoleEncoder(true)
	stdoutCore := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(w)), lvl)
	hCore := newHookCore(lvl)
	teeCore := zapcore.NewTee(stdoutCore, hCore)
	return zap.New(teeCore, zap.AddCaller(), zap.AddCallerSkip(1))
}

// ─────────────────────────────────────────────────────────────
// Slog Bridge
// ─────────────────────────────────────────────────────────────

func setupSlogBridge(l *zap.Logger) {
	slog.SetDefault(slog.New(&slogZapHandler{logger: l.WithOptions(zap.AddCallerSkip(1))}))
}

type slogZapHandler struct {
	logger *zap.Logger
	attrs  []zapcore.Field
	group  string
}

func (h *slogZapHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.Core().Enabled(slogToZapLevel(level))
}

func (h *slogZapHandler) Handle(_ context.Context, r slog.Record) error {
	fields := make([]zapcore.Field, 0, r.NumAttrs()+len(h.attrs))
	fields = append(fields, h.attrs...)

	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, slogAttrToZapField(h.group, a))
		return true
	})

	lvl := slogToZapLevel(r.Level)
	if ce := h.logger.Check(lvl, r.Message); ce != nil {
		ce.Write(fields...)
	}
	return nil
}

func (h *slogZapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newFields := make([]zapcore.Field, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newFields, h.attrs)
	for _, a := range attrs {
		newFields = append(newFields, slogAttrToZapField(h.group, a))
	}
	return &slogZapHandler{logger: h.logger, attrs: newFields, group: h.group}
}

func (h *slogZapHandler) WithGroup(name string) slog.Handler {
	group := name
	if h.group != "" {
		group = h.group + "." + name
	}
	return &slogZapHandler{logger: h.logger, attrs: h.attrs, group: group}
}

func slogToZapLevel(l slog.Level) zapcore.Level {
	switch {
	case l < slog.LevelInfo:
		return zapcore.DebugLevel
	case l < slog.LevelWarn:
		return zapcore.InfoLevel
	case l < slog.LevelError:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}

func slogAttrToZapField(group string, a slog.Attr) zapcore.Field {
	key := a.Key
	if group != "" {
		key = group + "." + key
	}
	val := a.Value.Resolve()
	switch val.Kind() {
	case slog.KindString:
		return zap.String(key, val.String())
	case slog.KindInt64:
		return zap.Int64(key, val.Int64())
	case slog.KindUint64:
		return zap.Uint64(key, val.Uint64())
	case slog.KindFloat64:
		return zap.Float64(key, val.Float64())
	case slog.KindBool:
		return zap.Bool(key, val.Bool())
	case slog.KindDuration:
		return zap.Duration(key, val.Duration())
	case slog.KindTime:
		return zap.Time(key, val.Time())
	case slog.KindAny:
		if err, ok := val.Any().(error); ok && key == "err" {
			return zap.NamedError(key, err)
		}
		return zap.Any(key, val.Any())
	default:
		return zap.Any(key, val.Any())
	}
}

type zapWaLogger struct {
	sugar *zap.SugaredLogger
	raw   *zap.Logger
}

// WhatsmeowStyle creates a fast waLog.Logger adapter with module prefix.
func WhatsmeowStyle(module string, minLevel string, _ bool) waLog.Logger {
	mu.RLock()
	base := rawLogger
	mu.RUnlock()

	sub := base.Named(module).WithOptions(zap.AddCallerSkip(1))
	return &zapWaLogger{
		sugar: sub.Sugar(),
		raw:   sub,
	}
}

// NewWaLogger creates a waLog.Logger with the given module name.
func NewWaLogger(module string) waLog.Logger {
	return WhatsmeowStyle(module, "INFO", true)
}

func (z *zapWaLogger) Warnf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Warn(msg)
	} else {
		z.sugar.Warnf(msg, args...)
	}
}

func (z *zapWaLogger) Errorf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Error(msg)
	} else {
		z.sugar.Errorf(msg, args...)
	}
}

func (z *zapWaLogger) Infof(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Info(msg)
	} else {
		z.sugar.Infof(msg, args...)
	}
}

func (z *zapWaLogger) Debugf(msg string, args ...any) {
	if len(args) == 0 {
		z.sugar.Debug(msg)
	} else {
		formatted := fmt.Sprintf(msg, args...)
		z.sugar.Debug(formatted)
	}
}

func (z *zapWaLogger) Sub(module string) waLog.Logger {
	sub := z.raw.Named(module)
	return &zapWaLogger{
		sugar: sub.Sugar(),
		raw:   sub,
	}
}

var _ waLog.Logger = (*zapWaLogger)(nil)

// ─────────────────────────────────────────────────────────────
// zerolog Adapter
// ─────────────────────────────────────────────────────────────

type zerologToZapWriter struct {
	logger *zap.Logger
}

func (w *zerologToZapWriter) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) == 0 {
		return n, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(p, &raw); err != nil {
		w.logger.Info(strings.TrimSpace(string(p)))
		return n, nil
	}

	var msg string
	if m, ok := raw[zerolog.MessageFieldName].(string); ok {
		msg = m
		delete(raw, zerolog.MessageFieldName)
	} else if m, ok := raw["msg"].(string); ok {
		msg = m
		delete(raw, "msg")
	}

	lvl := zapcore.InfoLevel
	if l, ok := raw[zerolog.LevelFieldName].(string); ok {
		delete(raw, zerolog.LevelFieldName)
		switch strings.ToLower(l) {
		case "trace", "debug":
			lvl = zapcore.DebugLevel
		case "info":
			lvl = zapcore.InfoLevel
		case "warn", "warning":
			lvl = zapcore.WarnLevel
		case "error":
			lvl = zapcore.ErrorLevel
		case "fatal", "panic":
			lvl = zapcore.DPanicLevel
		}
	}

	delete(raw, zerolog.TimestampFieldName)
	delete(raw, "timestamp")
	delete(raw, "subsystem")

	if lvl == zapcore.DebugLevel {
		return n, nil
	}

	if ce := w.logger.Check(lvl, msg); ce != nil {
		if len(raw) == 0 {
			ce.Write()
			return n, nil
		}

		keys := make([]string, 0, len(raw))
		for k := range raw {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		fields := make([]zapcore.Field, 0, len(keys))
		for _, k := range keys {
			fields = append(fields, zap.Any(k, raw[k]))
		}
		ce.Write(fields...)
	}

	return n, nil
}

// ZerologStyle creates a zerolog.Logger adapter that routes all log entries through Zap with structured fields and level routing.
func ZerologStyle(module string) zerolog.Logger {
	mu.RLock()
	base := rawLogger
	mu.RUnlock()

	var sub *zap.Logger
	if module != "" {
		sub = base.Named(module).WithOptions(zap.AddCallerSkip(1))
	} else {
		sub = base.WithOptions(zap.AddCallerSkip(1))
	}

	writer := &zerologToZapWriter{logger: sub}
	return zerolog.New(writer).Level(zerolog.DebugLevel)
}

// ─────────────────────────────────────────────────────────────
// Direct Logging Functions (Universal & Ergonomic)
// ─────────────────────────────────────────────────────────────

// Named returns a new sub-logger with the specified name.
func Named(name string) *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return rawLogger.Named(name).Sugar()
}

// With creates a child logger with additional structured context.
func With(args ...any) *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return sugarLogger.With(args...)
}

func logMessage(lvl zapcore.Level, msg string, args ...any) {
	mu.RLock()
	s := sugarLogger
	r := rawLogger
	mu.RUnlock()

	if len(args) == 0 {
		switch lvl {
		case zapcore.DebugLevel:
			r.Debug(msg)
		case zapcore.InfoLevel:
			r.Info(msg)
		case zapcore.WarnLevel:
			r.Warn(msg)
		case zapcore.ErrorLevel:
			r.Error(msg)
		case zapcore.DPanicLevel:
			r.DPanic(msg)
		case zapcore.PanicLevel:
			r.Panic(msg)
		case zapcore.FatalLevel:
			r.Fatal(msg)
		}
		return
	}

	// If arguments are zap.Field instances, pass them directly to raw logger for zero-alloc
	allFields := true
	fields := make([]zapcore.Field, len(args))
	for i, a := range args {
		if f, ok := a.(zapcore.Field); ok {
			fields[i] = f
		} else {
			allFields = false
			break
		}
	}
	if allFields {
		switch lvl {
		case zapcore.DebugLevel:
			r.Debug(msg, fields...)
		case zapcore.InfoLevel:
			r.Info(msg, fields...)
		case zapcore.WarnLevel:
			r.Warn(msg, fields...)
		case zapcore.ErrorLevel:
			r.Error(msg, fields...)
		case zapcore.DPanicLevel:
			r.DPanic(msg, fields...)
		case zapcore.PanicLevel:
			r.Panic(msg, fields...)
		case zapcore.FatalLevel:
			r.Fatal(msg, fields...)
		}
		return
	}

	// Key-value pairs (slog / zap.SugaredLogger style)
	switch lvl {
	case zapcore.DebugLevel:
		s.Debugw(msg, args...)
	case zapcore.InfoLevel:
		s.Infow(msg, args...)
	case zapcore.WarnLevel:
		s.Warnw(msg, args...)
	case zapcore.ErrorLevel:
		s.Errorw(msg, args...)
	case zapcore.DPanicLevel:
		s.DPanicw(msg, args...)
	case zapcore.PanicLevel:
		s.Panicw(msg, args...)
	case zapcore.FatalLevel:
		s.Fatalw(msg, args...)
	}
}

// Info logs at InfoLevel. Accepts key-value pairs or zap.Field arguments.
func Info(msg string, args ...any) {
	logMessage(zapcore.InfoLevel, msg, args...)
}

// Infof formats message according to format specifier and logs at InfoLevel.
func Infof(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Infof(template, args...)
}

// Infow logs at InfoLevel with structured context (key-value pairs).
func Infow(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Infow(msg, keysAndValues...)
}

// Debug logs at DebugLevel. Accepts key-value pairs or zap.Field arguments.
func Debug(msg string, args ...any) {
	logMessage(zapcore.DebugLevel, msg, args...)
}

// Debugf formats message according to format specifier and logs at DebugLevel.
func Debugf(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Debugf(template, args...)
}

// Debugw logs at DebugLevel with structured context (key-value pairs).
func Debugw(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Debugw(msg, keysAndValues...)
}

// Warn logs at WarnLevel. Accepts key-value pairs or zap.Field arguments.
func Warn(msg string, args ...any) {
	logMessage(zapcore.WarnLevel, msg, args...)
}

// Warnf formats message according to format specifier and logs at WarnLevel.
func Warnf(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Warnf(template, args...)
}

// Warnw logs at WarnLevel with structured context (key-value pairs).
func Warnw(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Warnw(msg, keysAndValues...)
}

// Error logs at ErrorLevel. Accepts key-value pairs or zap.Field arguments.
func Error(msg string, args ...any) {
	logMessage(zapcore.ErrorLevel, msg, args...)
}

// Errorf formats message according to format specifier and logs at ErrorLevel.
func Errorf(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Errorf(template, args...)
}

// Errorw logs at ErrorLevel with structured context (key-value pairs).
func Errorw(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Errorw(msg, keysAndValues...)
}

// Fatal logs at FatalLevel then calls os.Exit(1).
func Fatal(msg string, args ...any) {
	logMessage(zapcore.FatalLevel, msg, args...)
}

// Fatalf formats message and logs at FatalLevel then calls os.Exit(1).
func Fatalf(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Fatalf(template, args...)
}

// Fatalw logs at FatalLevel with structured context then calls os.Exit(1).
func Fatalw(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Fatalw(msg, keysAndValues...)
}

// Panic logs at PanicLevel then panics.
func Panic(msg string, args ...any) {
	logMessage(zapcore.PanicLevel, msg, args...)
}

// Panicf formats message and logs at PanicLevel then panics.
func Panicf(template string, args ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Panicf(template, args...)
}

// Panicw logs at PanicLevel with structured context then panics.
func Panicw(msg string, keysAndValues ...any) {
	mu.RLock()
	s := sugarLogger
	mu.RUnlock()
	s.Panicw(msg, keysAndValues...)
}
