package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

func getCPUModel() string {
	if runtime.GOOS == "linux" {
		if file, err := os.Open("/proc/cpuinfo"); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "model name") ||
					strings.HasPrefix(line, "Hardware") ||
					strings.HasPrefix(line, "Processor") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return fmt.Sprintf("%s (%s)", runtime.GOARCH, runtime.GOOS)
}

func getLoadAvg() string {
	if runtime.GOOS == "linux" {
		if content, err := os.ReadFile("/proc/loadavg"); err == nil {
			parts := strings.Fields(string(content))
			if len(parts) >= 3 {
				return strings.Join(parts[:3], ", ")
			}
		}
	}
	return "N/A"
}

func main() {
	_ = sdk.Load()

	model := getCPUModel()
	cores := runtime.NumCPU()
	loadAvg := getLoadAvg()
	arch := runtime.GOARCH
	goos := runtime.GOOS

	text := fmt.Sprintf(
		"*CPU Information*\n\n"+
			"• *Model:* %s\n"+
			"• *Architecture:* %s\n"+
			"• *OS:* %s\n"+
			"• *Cores/Threads:* %d\n"+
			"• *Load Average (1m, 5m, 15m):* %s",
		model, arch, goos, cores, loadAvg,
	)

	sdk.Respond(text)
}
