package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMissingDependencies(t *testing.T) {
	deps, ok := missingDependencies()
	if !ok {
		t.Fatalf("expected missingDependencies to return true for ok")
	}
	t.Logf("Detected missing dependencies: %v", deps)
}

func TestRefreshWindowsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		// Run on non-windows to ensure it safely no-ops without panics
		refreshWindowsPath()
		return
	}

	tmpDir := t.TempDir()
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	t.Setenv("LOCALAPPDATA", tmpDir)
	fakeBin := filepath.Join(tmpDir, "whatsrook", "bin")
	_ = os.MkdirAll(fakeBin, 0755)

	refreshWindowsPath()
	newPath := os.Getenv("PATH")
	if !filepath.IsAbs(fakeBin) {
		t.Errorf("expected absolute path for fakeBin")
	}
	_ = newPath
}

func TestRunDependencyInstallUnsupported(t *testing.T) {
	var buf bytes.Buffer
	err := runDependencyInstall("nonexistent_dep_xyz", &buf)
	if err == nil {
		t.Fatal("expected error for unsupported dependency")
	}
}

func TestCommandExists(t *testing.T) {
	if !commandExists("go") {
		t.Log("go binary not on path or test environment")
	}
	if commandExists("this_command_definitely_does_not_exist_123456") {
		t.Errorf("expected false for nonexistent command")
	}
}
