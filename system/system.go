// system package provides runtime environment inspection and host hardware metrics.
//
// it provides cross-platform helpers to query memory statistics, CPU core counts,
// active goroutines, and operating system metadata without invoking heavy external shell commands.
package system

import (
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

// FormatDuration formats a duration into a human-readable string.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	var parts []string
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

// RecordCrash persists detailed panic or runtime error metadata, stack traces, and system diagnostics
// to whatsrook_crash.log, outputs a notification to stderr, and returns the absolute log path.
func RecordCrash(r any, extraContext ...string) string {
	if r == nil {
		return ""
	}

	stack := debug.Stack()
	stats := GetStats()

	crashDir := os.Getenv("WHATSDATA_DIR")
	if crashDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			crashDir = filepath.Join(home, ".whatsrook")
		} else {
			crashDir = "."
		}
	}
	_ = os.MkdirAll(crashDir, 0700)
	crashPath := filepath.Join(crashDir, "whatsrook_crash.log")

	var contextInfo string
	if len(extraContext) > 0 {
		contextInfo = strings.Join(extraContext, " | ")
	}

	var buf strings.Builder
	buf.WriteString("\n================================================================================\n")
	buf.WriteString(fmt.Sprintf("WHATSRROK RUNTIME CRASH REPORT — %s\n", time.Now().Format("2006-01-02 15:04:05.000 MST")))
	buf.WriteString("================================================================================\n")
	buf.WriteString(fmt.Sprintf("Runtime Panic/Error: %v\n", r))
	if contextInfo != "" {
		buf.WriteString(fmt.Sprintf("Execution Context:   %s\n", contextInfo))
	}
	buf.WriteString(fmt.Sprintf("Host OS/Arch:        %s/%s\n", stats.OS, stats.Arch))
	buf.WriteString(fmt.Sprintf("Go Compiler:         %s\n", stats.GoVersion))
	buf.WriteString(fmt.Sprintf("Active Goroutines:   %d\n", stats.NumGoroutine))
	buf.WriteString(fmt.Sprintf("Memory In-Use/Sys:   %s / %s\n", FormatBytes(stats.MemAlloc), FormatBytes(stats.MemSys)))
	buf.WriteString("--------------------------------------------------------------------------------\n")
	buf.WriteString("GOROUTINE STACK TRACE:\n")
	buf.Write(stack)
	buf.WriteString("================================================================================\n\n")

	f, err := os.OpenFile(crashPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		_, _ = f.WriteString(buf.String())
		_ = f.Close()
	}

	fmt.Fprintf(os.Stderr, "\n🚨 runtime error, written to %s\n", crashPath)
	return crashPath
}
