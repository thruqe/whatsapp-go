package external

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDispatcherInstallAndList(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDispatcher(WithPluginDir(tmpDir))

	// Create a dummy source executable
	dummySrc := filepath.Join(tmpDir, "dummy_source")
	if err := os.WriteFile(dummySrc, []byte("#!/bin/sh\necho test\n"), 0o755); err != nil {
		t.Fatalf("failed to write dummy source: %v", err)
	}

	ctx := context.Background()
	if err := d.Install(ctx, "testplug", dummySrc); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !d.IsInstalled("testplug") {
		t.Errorf("expected testplug to be installed")
	}

	plugins, err := d.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	found := false
	for _, p := range plugins {
		if p.Name == "testplug" {
			found = true
			if !strings.HasPrefix(p.Path, tmpDir) {
				t.Errorf("expected path to start with %s, got %s", tmpDir, p.Path)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected testplug in List(), got %+v", plugins)
	}

	if err := d.Uninstall("testplug"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if d.IsInstalled("testplug") {
		t.Errorf("expected testplug to no longer be installed")
	}
}

func TestDispatcherZipArchiveInstall(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDispatcher(WithPluginDir(tmpDir))

	// Create a zip archive containing binary named "mytool" (or "mytool.exe")
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	entryName := "mytool"
	if runtime.GOOS == "windows" {
		entryName = "mytool.exe"
	}
	w, err := zw.Create(entryName)
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("binary-payload-data")); err != nil {
		t.Fatalf("failed to write zip entry content: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	zipSrc := filepath.Join(tmpDir, "mytool.zip")
	if err := os.WriteFile(zipSrc, zipBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	ctx := context.Background()
	if err := d.Install(ctx, "mytool", zipSrc); err != nil {
		t.Fatalf("Install from zip failed: %v", err)
	}

	if !d.IsInstalled("mytool") {
		t.Errorf("expected mytool to be installed from zip archive")
	}

	plugins, err := d.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, p := range plugins {
		if p.Name == "mytool" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected mytool in List(), got %+v", plugins)
	}
}

func TestDispatcherPluginPathResolution(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDispatcher(WithPluginDir(tmpDir))

	// Invalid name
	if _, err := d.PluginPath("invalid/name"); err == nil {
		t.Errorf("expected error for invalid plugin name")
	}

	// Normal resolution
	path, err := d.PluginPath("testcmd")
	if err != nil {
		t.Fatalf("PluginPath failed: %v", err)
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(path, ".exe") {
			t.Errorf("expected .exe on windows, got %s", path)
		}
	} else {
		if strings.HasSuffix(path, ".exe") {
			t.Errorf("did not expect .exe on non-windows, got %s", path)
		}
	}
}
