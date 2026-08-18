package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func tmpCron() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cleanOldTempFiles()
	}
}

func cleanOldTempFiles() {
	tmpDir := os.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "whatsrook_") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(tmpDir, entry.Name()))
		}
	}
}
