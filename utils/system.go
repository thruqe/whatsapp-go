package utils

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// GetGitCommit returns the short commit hash if running inside a Git repository.
func GetGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(out))
}

// SystemMetadata contains runtime system and environment details.
type SystemMetadata struct {
	Version   string
	Commit    string
	OS        string
	Arch      string
	NumCPU    int
	GoVersion string
}

// GetSystemMetadata gathers system metadata for diagnostics and status reporting.
func GetSystemMetadata(version string) SystemMetadata {
	commit := "N/A"
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	if out, err := cmd.Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	return SystemMetadata{
		Version:   version,
		Commit:    commit,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
}

// FormatBytes converts a byte count into a human-readable string (KB, MB, GB).
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

// FormatUptime converts seconds into a human-readable duration string (e.g. 1d 2h 3m 4s).
func FormatUptime(seconds float64) string {
	totalSec := int(seconds)
	days := totalSec / (24 * 3600)
	totalSec %= (24 * 3600)
	hours := totalSec / 3600
	totalSec %= 3600
	mins := totalSec / 60
	secs := totalSec % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}
