// system package provides runtime environment inspection and host hardware metrics.
//
// it provides cross-platform helpers to query memory statistics, CPU core counts,
// active goroutines, and operating system metadata without invoking heavy external shell commands.
package system

import (
	"fmt"
	"runtime"
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
