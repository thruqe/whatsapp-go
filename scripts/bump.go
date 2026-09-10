package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		if _, err := os.Stat(filepath.Join(curr, "Taskfile.yml")); err == nil {
			return curr, nil
		}
		if _, err := os.Stat(filepath.Join(curr, ".git")); err == nil {
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

func getGitShortSHA(rootDir string) string {
	cmd := exec.Command("git", "rev-parse", "--short=7", "HEAD")
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err == nil {
		sha := strings.TrimSpace(string(out))
		if sha != "" {
			return sha
		}
	}
	return "dev"
}

func runBump(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to locate repo root: %w", err)
	}

	now := time.Now()
	var versionStr string

	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		raw := strings.TrimPrefix(strings.TrimSpace(args[0]), "v")
		parts := strings.Split(raw, ".")
		if len(parts) < 2 {
			return fmt.Errorf("invalid version format %q: expected YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO (e.g. 26.09.7a3b4c1)", args[0])
		}
		if _, err := strconv.Atoi(parts[0]); err != nil {
			return fmt.Errorf("invalid year segment %q: %w", parts[0], err)
		}
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return fmt.Errorf("invalid month segment %q: %w", parts[1], err)
		}
		versionStr = raw
	} else {
		yy := now.Year() % 100
		mm := int(now.Month())
		sha := getGitShortSHA(rootDir)
		versionStr = fmt.Sprintf("%02d.%02d.%s", yy, mm, sha)
	}

	fmt.Printf("Bumping monthly release version to %s (Format: YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO)...\n", versionStr)

	// Refresh binary resources with new version
	if err := runRes([]string{versionStr}); err != nil {
		fmt.Printf("⚠️ Warning: failed to regenerate binary resources: %v\n", err)
	}

	fmt.Printf("Version successfully bumped to %s\n", versionStr)
	fmt.Printf("To create an annotated release tag:\n  git tag -a v%s -m \"Release v%s\"\n", versionStr, versionStr)
	return nil
}
