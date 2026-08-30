package cliutils

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type CPUStats struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

type SystemMemory struct {
	Total     uint64
	Free      uint64
	Available uint64
}

func GetCPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "Unknown"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "Unknown"
	}
	return "Generic CPU"
}

func GetLoadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "Unknown"
	}
	parts := strings.Fields(string(data))
	if len(parts) >= 3 {
		return strings.Join(parts[:3], ", ")
	}
	return strings.TrimSpace(string(data))
}

func GetCPUStats() (CPUStats, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return CPUStats{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "cpu" {
			var s CPUStats
			var err error
			s.User, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return CPUStats{}, err
			}
			s.Nice, _ = strconv.ParseUint(fields[2], 10, 64)
			s.System, _ = strconv.ParseUint(fields[3], 10, 64)
			s.Idle, _ = strconv.ParseUint(fields[4], 10, 64)
			if len(fields) > 5 {
				s.IOWait, _ = strconv.ParseUint(fields[5], 10, 64)
			}
			if len(fields) > 6 {
				s.IRQ, _ = strconv.ParseUint(fields[6], 10, 64)
			}
			if len(fields) > 7 {
				s.SoftIRQ, _ = strconv.ParseUint(fields[7], 10, 64)
			}
			if len(fields) > 8 {
				s.Steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
			return s, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return CPUStats{}, err
	}
	return CPUStats{}, fmt.Errorf("could not parse /proc/stat")
}

func GetCPUUsage() (float64, error) {
	s1, err := GetCPUStats()
	if err != nil {
		return 0, err
	}
	time.Sleep(200 * time.Millisecond)
	s2, err := GetCPUStats()
	if err != nil {
		return 0, err
	}

	idle1 := s1.Idle + s1.IOWait
	idle2 := s2.Idle + s2.IOWait

	nonIdle1 := s1.User + s1.Nice + s1.System + s1.IRQ + s1.SoftIRQ + s1.Steal
	nonIdle2 := s2.User + s2.Nice + s2.System + s2.IRQ + s2.SoftIRQ + s2.Steal

	total1 := idle1 + nonIdle1
	total2 := idle2 + nonIdle2

	totalDiff := total2 - total1
	idleDiff := idle2 - idle1

	if totalDiff == 0 {
		return 0, nil
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100, nil
}

func GetSystemMemory() (SystemMemory, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return SystemMemory{}, err
	}
	defer f.Close()

	var mem SystemMemory
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			mem.Total = val
		case "MemFree":
			mem.Free = val
		case "MemAvailable":
			mem.Available = val
		}
	}

	if err := scanner.Err(); err != nil {
		return SystemMemory{}, err
	}

	if mem.Total == 0 {
		return SystemMemory{}, fmt.Errorf("could not parse system memory total")
	}
	if mem.Available == 0 {
		mem.Available = mem.Free
	}

	return mem, nil
}

func FormatBytes(b uint64) string {
	if b == 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	parts := []string{}
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
