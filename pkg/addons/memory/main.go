package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

func getMemoryInfo() (totalGB, usedGB, availGB, usedPct float64, ok bool) {
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err != nil {
			return 0, 0, 0, 0, false
		}
		defer file.Close()

		memMap := make(map[string]uint64)
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 2 {
				key := strings.TrimSuffix(fields[0], ":")
				if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					memMap[key] = val
				}
			}
		}

		if totalKB, exists := memMap["MemTotal"]; exists {
			var availKB uint64
			if a, exists := memMap["MemAvailable"]; exists {
				availKB = a
			} else {
				free := memMap["MemFree"]
				buffers := memMap["Buffers"]
				cached := memMap["Cached"]
				availKB = free + buffers + cached
			}

			totalGB = float64(totalKB) / (1024.0 * 1024.0)
			availGB = float64(availKB) / (1024.0 * 1024.0)
			usedGB = totalGB - availGB
			if totalGB > 0 {
				usedPct = (usedGB / totalGB) * 100.0
			}
			return totalGB, usedGB, availGB, usedPct, true
		}
	}
	return 0, 0, 0, 0, false
}

func main() {
	_ = sdk.Load()

	total, used, avail, pct, ok := getMemoryInfo()
	if !ok {
		sdk.Respond("*System Memory Information*\n\nMemory statistics unavailable for current platform.")
		return
	}

	text := fmt.Sprintf(
		"*System Memory Information*\n\n"+
			"• *Total System Memory:* %.2f GB\n"+
			"• *Used System Memory:* %.2f GB (%.1f%%)\n"+
			"• *Available Memory:* %.2f GB",
		total, used, pct, avail,
	)

	sdk.Respond(text)
}
