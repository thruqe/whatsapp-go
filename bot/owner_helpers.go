package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook"
)

var (
	ActiveShellSessions   = make(map[string]*ShellSession)
	ActiveShellSessionsMu sync.Mutex
	ShellWorkingDirs      = make(map[string]string)
	ShellWorkingDirsMu    sync.Mutex
	AnsiEscapeRegex       = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([a-zA-Z]|\x1b\][0-9];[^\a\x1b]*(?:\a|\x1b\\)`)
)

type ShellSession struct {
	Chat           types.JID
	Sender         types.JID
	MsgID          types.MessageID
	Cmd            *exec.Cmd
	Stdin          io.WriteCloser
	Cancel         context.CancelFunc
	Buf            *bytes.Buffer
	Mu             sync.Mutex
	StartTime      time.Time
	CommandStr     string
	InitialDir     string
	FinalDir       string
	UpdateCh       chan struct{}
	Done           chan struct{}
	UserTerminated bool
}

func getShellWorkingDir(chatKey string) string {
	ShellWorkingDirsMu.Lock()
	defer ShellWorkingDirsMu.Unlock()

	dir, exists := ShellWorkingDirs[chatKey]
	if !exists || dir == "" {
		if defaultDir, err := os.Getwd(); err == nil {
			dir = defaultDir
		} else {
			dir = "."
		}
		ShellWorkingDirs[chatKey] = dir
	}

	// Validate directory still exists
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if defaultDir, err := os.Getwd(); err == nil {
			dir = defaultDir
		} else {
			dir = "."
		}
		ShellWorkingDirs[chatKey] = dir
	}

	return dir
}

func setShellWorkingDir(chatKey, newDir string) {
	newDir = strings.TrimSpace(newDir)
	if newDir == "" {
		return
	}
	if info, err := os.Stat(newDir); err == nil && info.IsDir() {
		ShellWorkingDirsMu.Lock()
		ShellWorkingDirs[chatKey] = newDir
		ShellWorkingDirsMu.Unlock()
	}
}

func getShellSessionRCPath(chatKey string) string {
	dir := filepath.Join(whatsrook.DefaultDataDir(), "shell_sessions")
	_ = os.MkdirAll(dir, 0700)
	safeKey := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, chatKey)
	return filepath.Join(dir, safeKey+".shrc")
}

func CleanShellOutput(raw string) string {
	cleaned := AnsiEscapeRegex.ReplaceAllString(raw, "")
	lines := strings.Split(cleaned, "\n")
	var resultLines []string
	for _, line := range lines {
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			var last string
			for _, part := range slices.Backward(parts) {
				trimmed := strings.TrimRight(part, " \t")
				if trimmed != "" {
					last = trimmed
					break
				}
			}
			if last != "" {
				resultLines = append(resultLines, last)
			}
		} else {
			resultLines = append(resultLines, line)
		}
	}
	return strings.Join(resultLines, "\n")
}
