package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// Log level constants for fast array indexing and zero-alloc comparisons.
const (
	LevelDebug = 0
	LevelInfo  = 1
	LevelWarn  = 2
	LevelError = 3
)

var (
	levelNames = [4]string{"DEBUG", "INFO", "WARN", "ERROR"}
	colorGray  = []byte("\033[90m")
	colorReset = []byte("\033[0m")
)

// Global state for per-level log files.
type fileStore struct {
	mu    sync.RWMutex
	files [4]*os.File
}

var globalFiles fileStore

// Buffer pool to eliminate heap allocations on log formatting.
var bufPool = sync.Pool{
	New: func() any {
		// Pre-allocate 512 bytes capacity, typical for log entries.
		b := make([]byte, 0, 512)
		return &b
	},
}

func getBuf() *[]byte {
	return bufPool.Get().(*[]byte)
}

func putBuf(b *[]byte) {
	*b = (*b)[:0]
	// Cap buffer reuse to prevent unbounded memory growth on rare giant log lines.
	if cap(*b) <= 65536 {
		bufPool.Put(b)
	}
}

// InitLogger initializes logging for the specified session directory, routing each
// log level into its own level.log file inside sessionDir/logs/
func InitLogger(sessionDir string, verbose bool) error {
	logDir := filepath.Join(sessionDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	globalFiles.mu.Lock()
	defer globalFiles.mu.Unlock()

	// Close existing files if any
	for i, f := range globalFiles.files {
		if f != nil {
			_ = f.Close()
			globalFiles.files[i] = nil
		}
	}

	fileNames := [4]string{"debug.log", "info.log", "warn.log", "error.log"}
	for i, name := range fileNames {
		f, err := os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			// Cleanup previously opened files on error
			for j := range i {
				if globalFiles.files[j] != nil {
					_ = globalFiles.files[j].Close()
					globalFiles.files[j] = nil
				}
			}
			return err
		}
		globalFiles.files[i] = f
	}

	minLevel := slog.LevelInfo
	if verbose {
		minLevel = slog.LevelDebug
	}

	slog.SetDefault(slog.New(newWMSlogHandler(os.Stdout, minLevel, "App", true)))
	return nil
}

// CloseLogger closes all open per-level log files.
func CloseLogger() {
	globalFiles.mu.Lock()
	defer globalFiles.mu.Unlock()

	for i, f := range globalFiles.files {
		if f != nil {
			_ = f.Close()
			globalFiles.files[i] = nil
		}
	}
}

// writeToLevelLog writes raw log bytes to the corresponding level log file safely.
func writeToLevelLog(level int, p []byte) {
	globalFiles.mu.RLock()
	f := globalFiles.files[level]
	if f != nil {
		_, _ = f.Write(p)
	}
	globalFiles.mu.RUnlock()
}

// ─────────────────────────────────────────────────────────────
// whatsmeow-style formatting
// ─────────────────────────────────────────────────────────────

func parseLevel(lvl string) int {
	switch strings.ToUpper(lvl) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return -1
	}
}

// formatLogEntry formats a complete log entry into a byte slice buffer.
// Returns two slices from the buffer: one formatted for stdout (with optional color)
// and one clean slice suitable for log files (without ANSI codes).
func formatLogEntry(buf *[]byte, mod string, level int, color bool, formatMsg func(b *[]byte)) (stdoutBytes []byte, fileBytes []byte) {
	startLen := len(*buf)

	// 1. Timestamp (e.g. 15:04:05.000) using zero-alloc AppendFormat
	*buf = time.Now().AppendFormat(*buf, "15:04:05.000")

	// 2. Color prefix (gray only for DEBUG if color enabled)
	useColor := color && level == LevelDebug
	if useColor {
		*buf = append(*buf, colorGray...)
	}

	// 3. Header: [Module LEVEL]
	*buf = append(*buf, " ["...)
	*buf = append(*buf, mod...)
	*buf = append(*buf, ' ')
	if level >= 0 && level < len(levelNames) {
		*buf = append(*buf, levelNames[level]...)
	}
	*buf = append(*buf, "] "...)

	// 4. Message content
	formatMsg(buf)

	// 5. Reset color if applied
	if useColor {
		*buf = append(*buf, colorReset...)
	}
	*buf = append(*buf, '\n')

	stdoutBytes = (*buf)[startLen:]

	// If color was used, create a clean version for the log file
	if useColor {
		// Clean version strips colorGray and colorReset
		clean := make([]byte, 0, len(stdoutBytes)-len(colorGray)-len(colorReset))
		clean = append(clean, stdoutBytes[:12]...)                                                  // Timestamp
		clean = append(clean, stdoutBytes[12+len(colorGray):len(stdoutBytes)-len(colorReset)-1]...) // Body
		clean = append(clean, '\n')
		fileBytes = clean
	} else {
		fileBytes = stdoutBytes
	}

	return stdoutBytes, fileBytes
}

// ─────────────────────────────────────────────────────────────
// waLog.Logger adapter
// ─────────────────────────────────────────────────────────────

// WhatsmeowStyle creates a fast waLog.Logger adapter with module prefix.
func WhatsmeowStyle(module string, minLevel string, color bool) waLog.Logger {
	return &wmLogger{
		mod:   module,
		color: color,
		min:   parseLevel(minLevel),
	}
}

type wmLogger struct {
	mod   string
	color bool
	min   int
}

func (w *wmLogger) outputf(level int, msg string, args ...any) {
	if level < w.min {
		return
	}

	buf := getBuf()
	defer putBuf(buf)

	stdoutBytes, fileBytes := formatLogEntry(buf, w.mod, level, w.color, func(b *[]byte) {
		if len(args) == 0 {
			*b = append(*b, msg...)
		} else {
			// Format message directly into buffer
			tmp := bytes.NewBuffer(*b)
			_, _ = fmt.Fprintf(tmp, msg, args...)
			*b = tmp.Bytes()
		}
	})

	_, _ = os.Stdout.Write(stdoutBytes)
	writeToLevelLog(level, fileBytes)
}

func (w *wmLogger) Errorf(msg string, args ...any) { w.outputf(LevelError, msg, args...) }
func (w *wmLogger) Warnf(msg string, args ...any)  { w.outputf(LevelWarn, msg, args...) }
func (w *wmLogger) Infof(msg string, args ...any)  { w.outputf(LevelInfo, msg, args...) }
func (w *wmLogger) Debugf(msg string, args ...any) { w.outputf(LevelDebug, msg, args...) }

func (w *wmLogger) Sub(mod string) waLog.Logger {
	return &wmLogger{mod: w.mod + "/" + mod, color: w.color, min: w.min}
}

// ─────────────────────────────────────────────────────────────
// slog.Handler adapter
// ─────────────────────────────────────────────────────────────

type wmSlogHandler struct {
	w     io.Writer
	level slog.Level
	mod   string
	color bool
	attrs []slog.Attr
}

func newWMSlogHandler(w io.Writer, level slog.Level, mod string, color bool) *wmSlogHandler {
	return &wmSlogHandler{w: w, level: level, mod: mod, color: color}
}

func (h *wmSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *wmSlogHandler) Handle(_ context.Context, r slog.Record) error {
	level := slogLevelToInt(r.Level)

	buf := getBuf()
	defer putBuf(buf)

	stdoutBytes, fileBytes := formatLogEntry(buf, h.mod, level, h.color, func(b *[]byte) {
		*b = append(*b, r.Message...)

		// Append record attributes directly without intermediate string concatenations
		r.Attrs(func(a slog.Attr) bool {
			appendAttr(b, a)
			return true
		})
		for _, a := range h.attrs {
			appendAttr(b, a)
		}
	})

	_, err := h.w.Write(stdoutBytes)
	writeToLevelLog(level, fileBytes)
	return err
}

func appendAttr(b *[]byte, a slog.Attr) {
	*b = append(*b, ' ')
	*b = append(*b, a.Key...)
	*b = append(*b, '=')
	val := a.Value.Resolve()
	switch val.Kind() {
	case slog.KindString:
		*b = append(*b, val.String()...)
	case slog.KindInt64:
		*b = fmt.Appendf(*b, "%d", val.Int64())
	case slog.KindUint64:
		*b = fmt.Appendf(*b, "%d", val.Uint64())
	case slog.KindFloat64:
		*b = fmt.Appendf(*b, "%g", val.Float64())
	case slog.KindBool:
		*b = fmt.Appendf(*b, "%t", val.Bool())
	case slog.KindDuration:
		*b = append(*b, val.Duration().String()...)
	case slog.KindTime:
		*b = val.Time().AppendFormat(*b, time.RFC3339)
	default:
		*b = fmt.Appendf(*b, "%v", val.Any())
	}
}

func (h *wmSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &wmSlogHandler{w: h.w, level: h.level, mod: h.mod, color: h.color, attrs: newAttrs}
}

func (h *wmSlogHandler) WithGroup(_ string) slog.Handler {
	return h
}

func slogLevelToInt(l slog.Level) int {
	switch {
	case l < slog.LevelInfo:
		return LevelDebug
	case l < slog.LevelWarn:
		return LevelInfo
	case l < slog.LevelError:
		return LevelWarn
	default:
		return LevelError
	}
}
