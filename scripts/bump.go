package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	curr := wd
	for {
		if _, err := os.Stat(filepath.Join(curr, "version.txt")); err == nil {
			return curr, nil
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return wd, nil
}

func runBump(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to locate repo root: %w", err)
	}

	now := time.Now()
	var day, month, yearShort, yearFull int
	var versionStr string

	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		raw := strings.TrimPrefix(strings.TrimSpace(args[0]), "v")
		parts := strings.Split(raw, ".")
		if len(parts) != 3 {
			return fmt.Errorf("invalid version argument %q: expected format D.M.YY (e.g. 21.8.26)", args[0])
		}
		var errConv error
		day, errConv = strconv.Atoi(parts[0])
		if errConv != nil {
			return fmt.Errorf("invalid day segment %q: %w", parts[0], errConv)
		}
		month, errConv = strconv.Atoi(parts[1])
		if errConv != nil {
			return fmt.Errorf("invalid month segment %q: %w", parts[1], errConv)
		}
		yearShort, errConv = strconv.Atoi(parts[2])
		if errConv != nil {
			return fmt.Errorf("invalid year segment %q: %w", parts[2], errConv)
		}
		yearFull = 2000 + yearShort
		if yearShort > 100 {
			yearFull = yearShort
			yearShort = yearShort % 100
		}
		versionStr = fmt.Sprintf("%d.%d.%d", day, month, yearShort)
	} else {
		day = now.Day()
		month = int(now.Month())
		yearShort = now.Year() % 100
		yearFull = now.Year()
		versionStr = fmt.Sprintf("%d.%d.%d", day, month, yearShort)
	}

	fmt.Printf("Bumping release version to %s (Date: %04d-%02d-%02d)...\n", versionStr, yearFull, month, day)

	// 1. Update version.txt
	versionTxtPath := filepath.Join(rootDir, "version.txt")
	if err := os.WriteFile(versionTxtPath, []byte(versionStr+"\n"), 0o644); err != nil {
		return fmt.Errorf("failed writing %s: %w", versionTxtPath, err)
	}
	fmt.Printf("✓ %s -> %s\n", filepath.Base(versionTxtPath), versionStr)

	// 2. Update cli/updater/updater.go fallback string
	updaterPath := filepath.Join(rootDir, "cmd", "updater", "updater.go")
	if data, err := os.ReadFile(updaterPath); err == nil {
		re := regexp.MustCompile(`return "\d+\.\d+\.\d+"`)
		updated := re.ReplaceAllString(string(data), fmt.Sprintf(`return "%s"`, versionStr))
		if err := os.WriteFile(updaterPath, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("failed writing %s: %w", updaterPath, err)
		}
		fmt.Printf("✓ %s -> %s\n", filepath.Join("cmd", "updater", "updater.go"), versionStr)
	}

	// 3. Update cli/versioninfo.json
	versionInfoPath := filepath.Join(rootDir, "cmd", "versioninfo.json")
	if data, err := os.ReadFile(versionInfoPath); err == nil {
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			return fmt.Errorf("failed parsing %s: %w", versionInfoPath, err)
		}

		if ffi, ok := v["FixedFileInfo"].(map[string]any); ok {
			if fv, ok := ffi["FileVersion"].(map[string]any); ok {
				fv["Major"] = day
				fv["Minor"] = month
				fv["Patch"] = yearShort
				fv["Build"] = 0
			}
			if pv, ok := ffi["ProductVersion"].(map[string]any); ok {
				pv["Major"] = day
				pv["Minor"] = month
				pv["Patch"] = yearShort
				pv["Build"] = 0
			}
		}

		if sfi, ok := v["StringFileInfo"].(map[string]any); ok {
			sfi["FileVersion"] = fmt.Sprintf("%s.0", versionStr)
			sfi["ProductVersion"] = versionStr
			sfi["LegalCopyright"] = fmt.Sprintf("Copyright © %d Thruqe", yearFull)
		}

		indented, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("failed formatting %s: %w", versionInfoPath, err)
		}

		if err := os.WriteFile(versionInfoPath, append(indented, '\n'), 0o644); err != nil {
			return fmt.Errorf("failed writing %s: %w", versionInfoPath, err)
		}
		fmt.Printf("✓ %s -> %s\n", filepath.Join("cmd", "versioninfo.json"), versionStr)
	}

	fmt.Printf("Version successfully bumped to %s\n", versionStr)
	return nil
}
