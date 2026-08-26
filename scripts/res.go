package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runRes(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to locate repo root: %w", err)
	}

	// 1. Read version from version.txt
	versionTxtPath := filepath.Join(rootDir, "version.txt")
	verBytes, err := os.ReadFile(versionTxtPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", versionTxtPath, err)
	}
	productVersion := strings.TrimSpace(string(verBytes))
	if productVersion == "" {
		productVersion = "0.0.1"
	}
	fileVersion := fmt.Sprintf("%s.0", productVersion)

	iconPath := filepath.Join(rootDir, "assets", "logo.png")
	if _, err := os.Stat(iconPath); err != nil {
		return fmt.Errorf("icon not found at %s: %w", iconPath, err)
	}

	cmdDir := filepath.Join(rootDir, "cmd")
	year := time.Now().Year()

	fmt.Printf("Generating binary resources (Product Version: %s, File Version: %s, Icon: %s)...\n", productVersion, fileVersion, filepath.Base(iconPath))

	cmd := exec.Command("go", "run", "github.com/tc-hib/go-winres@latest", "simply",
		"--icon", iconPath,
		"--manifest", "cli",
		"--product-name", "WhatsRook",
		"--file-description", "WhatsRook WhatsApp Bot & Client CLI",
		"--copyright", fmt.Sprintf("Copyright © %d Thruqe", year),
		"--original-filename", "whatsrook.exe",
		"--file-version", fileVersion,
		"--product-version", productVersion,
		"--arch", "amd64,arm64,386,arm",
	)
	cmd.Dir = cmdDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go-winres failed: %w", err)
	}

	fmt.Println("✓ Windows PE resources generated successfully.")
	return nil
}
