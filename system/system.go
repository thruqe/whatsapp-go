// Provides runtime environment inspection and host hardware metrics.
//
// It provides cross-platform helpers to query memory statistics, CPU core counts,
// active goroutines, and operating system metadata without invoking heavy external
// shell commands.
package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// BootTime records the process startup timestamp.
var BootTime = time.Now()

// Stats captures a snapshot of current host and process runtime metrics.
type Stats struct {
	// OS is the host operating system (e.g. linux, darwin, windows).
	OS string
	// Arch is the compilation architecture (e.g. amd64, arm64).
	Arch string
	// GoVersion is the Go compiler release version.
	GoVersion string
	// NumCPU is the number of logical CPU cores available to the process.
	NumCPU int
	// NumGoroutine is the number of currently executing goroutines.
	NumGoroutine int
	// MemAlloc is the bytes of allocated heap objects currently in use.
	MemAlloc uint64
	// MemTotalAlloc is the cumulative bytes allocated for heap objects since process start.
	MemTotalAlloc uint64
	// MemSys is the total bytes of memory obtained from the host OS.
	MemSys uint64
	// NumGC is the completed number of garbage collection cycles.
	NumGC uint32
}

// String renders a Stats snapshot as a compact, human-readable summary.
func (s Stats) String() string {
	return fmt.Sprintf(
		"%s/%s %s | cpus=%d goroutines=%d alloc=%s sys=%s gc=%d",
		s.OS, s.Arch, s.GoVersion, s.NumCPU, s.NumGoroutine,
		FormatBytes(s.MemAlloc), FormatBytes(s.MemSys), s.NumGC,
	)
}

// GetStats returns a point-in-time snapshot of runtime memory and CPU statistics.
func GetStats() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return Stats{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		GoVersion:     runtime.Version(),
		NumCPU:        runtime.NumCPU(),
		NumGoroutine:  runtime.NumGoroutine(),
		MemAlloc:      m.Alloc,
		MemTotalAlloc: m.TotalAlloc,
		MemSys:        m.Sys,
		NumGC:         m.NumGC,
	}
}

// FormatBytes formats a raw byte count into human-readable units (B, KB, MB, GB, TB).
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatDuration formats a duration into a human-readable string, e.g. "1d 2h 3m 4s".
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	parts := make([]string, 0, 4)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// crashLogName is the filename used for on-disk crash reports.
const crashLogName = "whatsrook_crash.log"

// crashLogPerm is the file permission used when creating the crash log directory and file.
const (
	crashDirPerm  os.FileMode = 0o700
	crashFilePerm os.FileMode = 0o600
)

// CrashReport holds the structured detail captured for a recovered panic
// or runtime error, before it is formatted and persisted.
type CrashReport struct {
	// Time is when the crash was recorded.
	Time time.Time
	// Value is the recovered panic value (from recover()).
	Value any
	// Context holds any extra caller-supplied context strings.
	Context []string
	// Stack is the captured goroutine stack trace.
	Stack []byte
	// Stats is a runtime snapshot taken at crash time.
	Stats Stats
}

// crashLogDir resolves the directory used for crash log storage, honoring
// the WHATSDATA_DIR environment variable override.
func crashLogDir() string {
	if dir := os.Getenv("WHATSDATA_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".whatsrook")
	}
	return "."
}

// Format renders the crash report as the on-disk log entry text.
func (r CrashReport) Format() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "CRASH REPORT — %s\n", r.Time.Format("2006-01-02 15:04:05.000 MST"))
	fmt.Fprintf(&buf, "Runtime Panic/Error: %v\n", r.Value)
	if len(r.Context) > 0 {
		fmt.Fprintf(&buf, "Execution Context:   %s\n", strings.Join(r.Context, " | "))
	}
	fmt.Fprintf(&buf, "Host OS/Arch:        %s/%s\n", r.Stats.OS, r.Stats.Arch)
	fmt.Fprintf(&buf, "Compiler:         %s\n", r.Stats.GoVersion)
	fmt.Fprintf(&buf, "Goroutines:   %d\n", r.Stats.NumGoroutine)
	fmt.Fprintf(&buf, "Memory In-Use/Sys:   %s / %s\n", FormatBytes(r.Stats.MemAlloc), FormatBytes(r.Stats.MemSys))
	buf.WriteString("STACK TRACE:\n")
	buf.Write(r.Stack)
	return buf.String()
}

// RecordCrash captures panic/runtime-error metadata, a stack trace, and a
// runtime.Stats snapshot, appends it to the crash log on disk, and returns
// the absolute log path.
//
// If r is nil, RecordCrash is a no-op and returns "".
//
// Unlike a naive implementation, disk errors are surfaced (not silently
// swallowed): if the log directory or file cannot be written, an error
// noting the underlying cause is written to stderr in addition to the
// original crash summary, so operators are not left without any signal.
func RecordCrash(r any, extraContext ...string) string {
	if r == nil {
		return ""
	}

	report := CrashReport{
		Time:    time.Now(),
		Value:   r,
		Context: extraContext,
		Stack:   debug.Stack(),
		Stats:   GetStats(),
	}

	dir := crashLogDir()
	path := filepath.Join(dir, crashLogName)

	if err := writeCrashLog(path, dir, report); err != nil {
		fmt.Fprintf(os.Stderr, "\n🚨 runtime error occurred, but crash log could not be written: %v\n", err)
		fmt.Fprintf(os.Stderr, "%s\n", report.Format())
		return ""
	}

	fmt.Fprintf(os.Stderr, "\n🚨 runtime error, written to %s\n", path)
	return path
}

// writeCrashLog appends the formatted report to path, creating dir if needed.
func writeCrashLog(path, dir string, report CrashReport) error {
	if err := os.MkdirAll(dir, crashDirPerm); err != nil {
		return fmt.Errorf("creating crash log directory %q: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, crashFilePerm)
	if err != nil {
		return fmt.Errorf("opening crash log %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(report.Format()); err != nil {
		return fmt.Errorf("writing crash log %q: %w", path, err)
	}

	if err := f.Sync(); err != nil {
		// Non-fatal: the write likely succeeded even if the fsync failed
		// (e.g. on filesystems that don't support it). Surface it as a
		// wrapped error so callers can decide whether it matters to them,
		// but don't treat it as a hard failure of the log write itself.
		return fmt.Errorf("syncing crash log %q: %w", path, errors.Join(errSyncFailed, err))
	}

	return nil
}

// errSyncFailed marks sentinel wrapping for fsync failures so callers can
// distinguish "log probably written, fsync failed" from a genuine write error
// using errors.Is.
var errSyncFailed = errors.New("crash log sync failed")
