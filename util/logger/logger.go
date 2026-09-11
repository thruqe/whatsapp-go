// Package logger provides WhatsRook's structured logging facade.
//
// It is backed by Zap for performance, and bridges to the various logging
// APIs WhatsRook's dependencies expect: stdlib log/slog, whatsmeow's
// waLog.Logger, and zerolog.Logger. Callers can also register hooks to
// receive live structured LogEntry events (e.g. to stream logs to a UI).
package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/exp/zapslog"
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

// state bundles everything that changes together when the logger is
// (re)configured, so it can be swapped atomically without a mutex on the
// hot logging path.
type state struct {
	level zap.AtomicLevel
	raw   *zap.Logger
	sugar *zap.SugaredLogger
}

var (
	current    atomic.Pointer[state]
	isInit     atomic.Bool
	nextHookID atomic.Uint32
	hooks      sync.Map // uint32 -> LogHook
)

func init() {
	lvl := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	setState(lvl, newDefaultLogger(lvl, os.Stdout))
}

// setState builds and installs a new logger built on raw, updating the
// slog default in the same step so the two never drift out of sync.
func setState(lvl zap.AtomicLevel, raw *zap.Logger) {
	current.Store(&state{level: lvl, raw: raw, sugar: raw.Sugar()})
	slog.SetDefault(slog.New(zapslog.NewHandler(raw.WithOptions(zap.AddCallerSkip(1)).Core())))
}

func cur() *state { return current.Load() }

// L returns the underlying *zap.Logger.
func L() *zap.Logger { return cur().raw }

// S returns the underlying *zap.SugaredLogger.
func S() *zap.SugaredLogger { return cur().sugar }

// SetLevel sets the global minimum logging level dynamically.
func SetLevel(lvl zapcore.Level) { cur().level.SetLevel(lvl) }

// SetVerbose sets the global log level to DebugLevel if verbose is true, otherwise InfoLevel.
func SetVerbose(verbose bool) {
	if verbose {
		SetLevel(zapcore.DebugLevel)
	} else {
		SetLevel(zapcore.InfoLevel)
	}
}

// GetLevel returns the current global logging level.
func GetLevel() zapcore.Level { return cur().level.Level() }

// AddHook registers a callback that receives structured log entries in real-time.
// It returns an unsubscribe closure to deregister the hook.
func AddHook(fn LogHook) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	id := nextHookID.Add(1)
	hooks.Store(id, fn)
	return func() { hooks.Delete(id) }
}

// ClearHooks removes all currently registered log hooks.
func ClearHooks() {
	hooks.Range(func(k, _ any) bool {
		hooks.Delete(k)
		return true
	})
}

// InitLogger initializes the global logger with console stdout and event hook streaming (no disk file creation).
func InitLogger(_ string, verbose bool) error {
	lvl := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if verbose {
		lvl.SetLevel(zapcore.DebugLevel)
	}

	core := zapcore.NewTee(
		zapcore.NewCore(newConsoleEncoder(true), zapcore.Lock(os.Stdout), lvl),
		newHookCore(lvl),
	)
	setState(lvl, zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)))
	isInit.Store(true)
	return nil
}

// Close flushes buffered log entries and resets the logger to its default configuration.
func Close() {
	_ = cur().raw.Sync()
	ClearHooks()
	lvl := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	setState(lvl, newDefaultLogger(lvl, os.Stdout))
	isInit.Store(false)
}

// CloseLogger is an alias for Close.
func CloseLogger() { Close() }

// Sync flushes any buffered log entries.
func Sync() error { return cur().raw.Sync() }

// ─────────────────────────────────────────────────────────────
// Hook Zap Core — fans out log entries to registered LogHooks
// ─────────────────────────────────────────────────────────────

type hookCore struct {
	zapcore.LevelEnabler
	fields map[string]any
}

func newHookCore(enabler zapcore.LevelEnabler) *hookCore {
	return &hookCore{LevelEnabler: enabler, fields: make(map[string]any)}
}

// fieldsToMap flattens zap fields into a plain map via zapcore's map encoder.
func fieldsToMap(fields []zapcore.Field) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	return enc.Fields
}

func (c *hookCore) With(fields []zapcore.Field) zapcore.Core {
	merged := make(map[string]any, len(c.fields)+len(fields))
	maps.Copy(merged, c.fields)
	maps.Copy(merged, fieldsToMap(fields))
	return &hookCore{LevelEnabler: c.LevelEnabler, fields: merged}
}

func (c *hookCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *hookCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	var active []LogHook
	hooks.Range(func(_, v any) bool {
		active = append(active, v.(LogHook))
		return true
	})
	if len(active) == 0 {
		return nil
	}

	merged := make(map[string]any, len(c.fields)+len(fields))
	maps.Copy(merged, c.fields)
	maps.Copy(merged, fieldsToMap(fields))

	var caller string
	if entry.Caller.Defined {
		caller = entry.Caller.TrimmedPath()
	}

	logEntry := LogEntry{
		Timestamp: entry.Time,
		Level:     entry.Level.CapitalString(),
		Message:   entry.Message,
		Caller:    caller,
		Fields:    merged,
	}

	for _, h := range active {
		callHookSafely(h, logEntry)
	}
	return nil
}

// callHookSafely invokes a hook, isolating the logger from a panicking subscriber.
func callHookSafely(h LogHook, entry LogEntry) {
	defer func() { _ = recover() }()
	h(entry)
}

func (c *hookCore) Sync() error { return nil }

// ─────────────────────────────────────────────────────────────
// Console Encoder — writes "key=value" fields directly, no JSON round-trip
// ─────────────────────────────────────────────────────────────

func shouldColorize() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func customLevelEncoder(color bool) zapcore.LevelEncoder {
	return func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(levelLabel(l, color))
	}
}

func levelLabel(l zapcore.Level, color bool) string {
	label, code := "?????", ""
	switch l {
	case zapcore.DebugLevel:
		label, code = "DEBUG", "35"
	case zapcore.InfoLevel:
		label, code = "INFO ", "32"
	case zapcore.WarnLevel:
		label, code = "WARN ", "33"
	case zapcore.ErrorLevel:
		label, code = "ERROR", "31"
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		label, code = "FATAL", "1;31"
	default:
		return fmt.Sprintf("%-5s", l.CapitalString())
	}
	if color {
		return "\x1b[" + code + "m" + label + "\x1b[0m"
	}
	return label
}

func customTimeEncoder(color bool) zapcore.TimeEncoder {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		formatted := t.Format("15:04:05.000")
		if color {
			formatted = "\x1b[90m" + formatted + "\x1b[0m"
		}
		enc.AppendString(formatted)
	}
}

func customNameEncoder(color bool) zapcore.NameEncoder {
	return func(s string, enc zapcore.PrimitiveArrayEncoder) {
		if s == "" {
			return
		}
		if color {
			enc.AppendString("\x1b[36m[" + s + "]\x1b[0m")
		} else {
			enc.AppendString("[" + s + "]")
		}
	}
}

// cleanConsoleEncoder wraps zapcore's console encoder but replaces its
// trailing JSON field blob with a sorted "key=value key2=value2" tail,
// formatted directly from the already-decoded field map — no
// encode-to-JSON-then-decode-then-reformat round-trip.
type cleanConsoleEncoder struct {
	zapcore.Encoder
	color bool
}

func (c *cleanConsoleEncoder) Clone() zapcore.Encoder {
	return &cleanConsoleEncoder{Encoder: c.Encoder.Clone(), color: c.color}
}

func (c *cleanConsoleEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	buf, err := c.Encoder.EncodeEntry(ent, fields)
	if err != nil || buf == nil || buf.Len() == 0 {
		return buf, err
	}

	b := buf.Bytes()
	var stack string
	mainLine := b
	if ent.Stack != "" {
		if idx := bytes.LastIndex(b, []byte("\n"+ent.Stack)); idx != -1 {
			stack = ent.Stack
			mainLine = b[:idx]
		}
	} else {
		mainLine = bytes.TrimRight(mainLine, "\r\n")
	}

	lastBrace := bytes.LastIndexByte(mainLine, '}')
	if lastBrace == -1 {
		return buf, nil
	}
	sepBrace := bytes.LastIndex(mainLine[:lastBrace+1], []byte("  {"))
	if sepBrace == -1 {
		return buf, nil
	}

	var fieldMap map[string]any
	if err := json.Unmarshal(mainLine[sepBrace+2:lastBrace+1], &fieldMap); err != nil || len(fieldMap) == 0 {
		return buf, nil
	}

	prefix := append([]byte(nil), mainLine[:sepBrace]...)
	formatted := formatCleanFields(fieldMap, c.color)

	buf.Reset()
	buf.Write(prefix)
	if formatted != "" {
		buf.AppendString("  ")
		buf.AppendString(formatted)
	}
	if stack != "" {
		buf.AppendString(zapcore.DefaultLineEnding)
		buf.AppendString(stack)
	}
	buf.AppendString(zapcore.DefaultLineEnding)
	return buf, nil
}

func formatCleanFields(fields map[string]any, color bool) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(' ')
		}
		writeFieldKey(&sb, k, color)
		sb.WriteString(formatFieldValue(fields[k], color))
	}
	return sb.String()
}

func writeFieldKey(sb *strings.Builder, k string, color bool) {
	if !color {
		sb.WriteString(k)
		sb.WriteByte('=')
		return
	}
	if k == "err" || k == "error" {
		sb.WriteString("\x1b[31m")
	} else {
		sb.WriteString("\x1b[36m")
	}
	sb.WriteString(k)
	sb.WriteString("\x1b[0m=")
}

func formatFieldValue(val any, color bool) string {
	switch v := val.(type) {
	case nil:
		if color {
			return "\x1b[90mnull\x1b[0m"
		}
		return "null"
	case string:
		if needsQuotes(v) {
			return strconv.Quote(v)
		}
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

func needsQuotes(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r <= ' ' || r == '"' || r == '\\' || r == '=' {
			return true
		}
	}
	return false
}

func newConsoleEncoder(color bool) zapcore.Encoder {
	useColor := color && shouldColorize()
	cfg := zapcore.EncoderConfig{
		TimeKey:          "time",
		LevelKey:         "level",
		NameKey:          "logger",
		MessageKey:       "msg",
		StacktraceKey:    "stacktrace",
		LineEnding:       zapcore.DefaultLineEnding,
		ConsoleSeparator: "  ",
		EncodeLevel:      customLevelEncoder(useColor),
		EncodeTime:       customTimeEncoder(useColor),
		EncodeName:       customNameEncoder(useColor),
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
	}
	return &cleanConsoleEncoder{Encoder: zapcore.NewConsoleEncoder(cfg), color: useColor}
}

func newDefaultLogger(lvl zapcore.LevelEnabler, w io.Writer) *zap.Logger {
	core := zapcore.NewTee(
		zapcore.NewCore(newConsoleEncoder(true), zapcore.Lock(zapcore.AddSync(w)), lvl),
		newHookCore(lvl),
	)
	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
}

// ─────────────────────────────────────────────────────────────
// whatsmeow adapter
// ─────────────────────────────────────────────────────────────

type zapWaLogger struct {
	sugar *zap.SugaredLogger
	raw   *zap.Logger
}

var _ waLog.Logger = (*zapWaLogger)(nil)

// WhatsmeowStyle creates a fast waLog.Logger adapter with module prefix.
func WhatsmeowStyle(module string, _ string, _ bool) waLog.Logger {
	sub := cur().raw.Named(module).WithOptions(zap.AddCallerSkip(1))
	return &zapWaLogger{sugar: sub.Sugar(), raw: sub}
}

// NewWaLogger creates a waLog.Logger with the given module name.
func NewWaLogger(module string) waLog.Logger {
	return WhatsmeowStyle(module, "INFO", true)
}

func (z *zapWaLogger) format(msg string, args []any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

func (z *zapWaLogger) Warnf(msg string, args ...any)  { z.sugar.Warn(z.format(msg, args)) }
func (z *zapWaLogger) Errorf(msg string, args ...any) { z.sugar.Error(z.format(msg, args)) }
func (z *zapWaLogger) Infof(msg string, args ...any)  { z.sugar.Info(z.format(msg, args)) }
func (z *zapWaLogger) Debugf(msg string, args ...any) { z.sugar.Debug(z.format(msg, args)) }

func (z *zapWaLogger) Sub(module string) waLog.Logger {
	sub := z.raw.Named(module)
	return &zapWaLogger{sugar: sub.Sugar(), raw: sub}
}

// ─────────────────────────────────────────────────────────────
// zerolog adapter
// ─────────────────────────────────────────────────────────────

type zerologToZapWriter struct{ logger *zap.Logger }

func (w *zerologToZapWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return n, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(p, &raw); err != nil {
		w.logger.Info(strings.TrimSpace(string(p)))
		return n, nil
	}

	msg, _ := raw[zerolog.MessageFieldName].(string)
	delete(raw, zerolog.MessageFieldName)
	if msg == "" {
		msg, _ = raw["msg"].(string)
		delete(raw, "msg")
	}

	lvl := zapcore.InfoLevel
	if l, ok := raw[zerolog.LevelFieldName].(string); ok {
		delete(raw, zerolog.LevelFieldName)
		lvl = zerologLevelToZap(l)
	}
	delete(raw, zerolog.TimestampFieldName)
	delete(raw, "timestamp")
	delete(raw, "subsystem")

	ce := w.logger.Check(lvl, msg)
	if ce == nil {
		return n, nil
	}
	if len(raw) == 0 {
		ce.Write()
		return n, nil
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]zapcore.Field, len(keys))
	for i, k := range keys {
		fields[i] = zap.Any(k, raw[k])
	}
	ce.Write(fields...)
	return n, nil
}

func zerologLevelToZap(l string) zapcore.Level {
	switch strings.ToLower(l) {
	case "trace", "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal", "panic":
		return zapcore.DPanicLevel
	default:
		return zapcore.InfoLevel
	}
}

// ZerologStyle creates a zerolog.Logger adapter that routes all log entries
// through Zap with structured fields and level routing.
func ZerologStyle(module string) zerolog.Logger {
	sub := cur().raw.WithOptions(zap.AddCallerSkip(1))
	if module != "" {
		sub = sub.Named(module)
	}
	return zerolog.New(&zerologToZapWriter{logger: sub}).Level(zerolog.DebugLevel)
}

// ─────────────────────────────────────────────────────────────
// Direct logging functions
// ─────────────────────────────────────────────────────────────

// Named returns a new sub-logger with the specified name.
func Named(name string) *zap.SugaredLogger { return cur().raw.Named(name).Sugar() }

// With creates a child logger with additional structured context.
func With(args ...any) *zap.SugaredLogger { return cur().sugar.With(args...) }

// asFields reports whether every arg is a pre-built zapcore.Field, returning
// them typed if so. Lets callers pass zap.Field for the zero-alloc path
// while still supporting Infow-style key/value pairs.
func asFields(args []any) ([]zapcore.Field, bool) {
	fields := make([]zapcore.Field, len(args))
	for i, a := range args {
		f, ok := a.(zapcore.Field)
		if !ok {
			return nil, false
		}
		fields[i] = f
	}
	return fields, true
}

func logMessage(lvl zapcore.Level, msg string, args ...any) {
	st := cur()

	if len(args) == 0 {
		st.raw.Check(lvl, msg).Write()
		return
	}

	if fields, ok := asFields(args); ok {
		st.raw.Check(lvl, msg).Write(fields...)
		return
	}

	switch lvl {
	case zapcore.DebugLevel:
		st.sugar.Debugw(msg, args...)
	case zapcore.InfoLevel:
		st.sugar.Infow(msg, args...)
	case zapcore.WarnLevel:
		st.sugar.Warnw(msg, args...)
	case zapcore.ErrorLevel:
		st.sugar.Errorw(msg, args...)
	case zapcore.DPanicLevel:
		st.sugar.DPanicw(msg, args...)
	case zapcore.PanicLevel:
		st.sugar.Panicw(msg, args...)
	case zapcore.FatalLevel:
		st.sugar.Fatalw(msg, args...)
	}
}

// Info logs at InfoLevel. Accepts key-value pairs or zap.Field arguments.
func Info(msg string, args ...any) { logMessage(zapcore.InfoLevel, msg, args...) }

// Infof formats message according to format specifier and logs at InfoLevel.
func Infof(template string, args ...any) { cur().sugar.Infof(template, args...) }

// Infow logs at InfoLevel with structured context (key-value pairs).
func Infow(msg string, keysAndValues ...any) { cur().sugar.Infow(msg, keysAndValues...) }

// Debug logs at DebugLevel. Accepts key-value pairs or zap.Field arguments.
func Debug(msg string, args ...any) { logMessage(zapcore.DebugLevel, msg, args...) }

// Debugf formats message according to format specifier and logs at DebugLevel.
func Debugf(template string, args ...any) { cur().sugar.Debugf(template, args...) }

// Debugw logs at DebugLevel with structured context (key-value pairs).
func Debugw(msg string, keysAndValues ...any) { cur().sugar.Debugw(msg, keysAndValues...) }

// Warn logs at WarnLevel. Accepts key-value pairs or zap.Field arguments.
func Warn(msg string, args ...any) { logMessage(zapcore.WarnLevel, msg, args...) }

// Warnf formats message according to format specifier and logs at WarnLevel.
func Warnf(template string, args ...any) { cur().sugar.Warnf(template, args...) }

// Warnw logs at WarnLevel with structured context (key-value pairs).
func Warnw(msg string, keysAndValues ...any) { cur().sugar.Warnw(msg, keysAndValues...) }

// Error logs at ErrorLevel. Accepts key-value pairs or zap.Field arguments.
func Error(msg string, args ...any) { logMessage(zapcore.ErrorLevel, msg, args...) }

// Errorf formats message according to format specifier and logs at ErrorLevel.
func Errorf(template string, args ...any) { cur().sugar.Errorf(template, args...) }

// Errorw logs at ErrorLevel with structured context (key-value pairs).
func Errorw(msg string, keysAndValues ...any) { cur().sugar.Errorw(msg, keysAndValues...) }

// Fatal logs at FatalLevel then calls os.Exit(1).
func Fatal(msg string, args ...any) { logMessage(zapcore.FatalLevel, msg, args...) }

// Fatalf formats message and logs at FatalLevel then calls os.Exit(1).
func Fatalf(template string, args ...any) { cur().sugar.Fatalf(template, args...) }

// Fatalw logs at FatalLevel with structured context then calls os.Exit(1).
func Fatalw(msg string, keysAndValues ...any) { cur().sugar.Fatalw(msg, keysAndValues...) }

// Panic logs at PanicLevel then panics.
func Panic(msg string, args ...any) { logMessage(zapcore.PanicLevel, msg, args...) }

// Panicf formats message and logs at PanicLevel then panics.
func Panicf(template string, args ...any) { cur().sugar.Panicf(template, args...) }

// Panicw logs at PanicLevel with structured context then panics.
func Panicw(msg string, keysAndValues ...any) { cur().sugar.Panicw(msg, keysAndValues...) }
