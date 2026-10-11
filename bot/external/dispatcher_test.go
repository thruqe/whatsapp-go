package external

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginDir_SessionScoping(t *testing.T) {
	tempDir := t.TempDir()
	d := NewDispatcher(WithPluginDir(tempDir))

	// Global dir
	globalDir, err := d.PluginDir()
	if err != nil {
		t.Fatalf("PluginDir() error: %v", err)
	}
	if globalDir != tempDir {
		t.Errorf("expected global dir %q, got %q", tempDir, globalDir)
	}

	// Session dir
	sessDir, err := d.PluginDir("user123")
	if err != nil {
		t.Fatalf("PluginDir(user123) error: %v", err)
	}
	expectedSessDir := filepath.Join(tempDir, "sessions", "user123", "plugins")
	if sessDir != expectedSessDir {
		t.Errorf("expected session dir %q, got %q", expectedSessDir, sessDir)
	}
}

func TestPluginSessionIsolation(t *testing.T) {
	tempDir := t.TempDir()
	d := NewDispatcher(WithPluginDir(tempDir))
	ctx := context.Background()

	// Create dummy binary to install
	dummyBin := filepath.Join(tempDir, "dummy_bin")
	if err := os.WriteFile(dummyBin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("failed to create dummy binary: %v", err)
	}

	// Install plugin for userA
	if err := d.Install(ctx, "plugx", dummyBin, "userA"); err != nil {
		t.Fatalf("Install for userA failed: %v", err)
	}

	// Check IsInstalled
	if !d.IsInstalled("plugx", "userA") {
		t.Errorf("expected plugx to be installed for userA")
	}
	if d.IsInstalled("plugx", "userB") {
		t.Errorf("plugx should NOT be installed for userB")
	}
	if d.IsInstalled("plugx") {
		t.Errorf("plugx should NOT be installed globally")
	}

	// Check List
	listA, err := d.List("userA")
	if err != nil {
		t.Fatalf("List(userA) error: %v", err)
	}
	if len(listA) != 1 || listA[0].Name != "plugx" {
		t.Errorf("expected [plugx] for userA, got %+v", listA)
	}

	listB, err := d.List("userB")
	if err != nil {
		t.Fatalf("List(userB) error: %v", err)
	}
	if len(listB) != 0 {
		t.Errorf("expected empty list for userB, got %+v", listB)
	}

	// Uninstall for userA
	if err := d.Uninstall("plugx", "userA"); err != nil {
		t.Fatalf("Uninstall for userA failed: %v", err)
	}
	if d.IsInstalled("plugx", "userA") {
		t.Errorf("plugx should not be installed for userA after uninstall")
	}
}

func TestLiveSessionKey_Scoping(t *testing.T) {
	d := NewDispatcher()

	keyGlobal := d.sessionKey("chat1", "pluginA")
	if keyGlobal != "chat1:pluginA" {
		t.Errorf("unexpected global session key: %s", keyGlobal)
	}

	keyUserA := d.sessionKey("chat1", "pluginA", "userA")
	if keyUserA != "userA:chat1:pluginA" {
		t.Errorf("unexpected scoped session key: %s", keyUserA)
	}

	keyUserB := d.sessionKey("chat1", "pluginA", "userB")
	if keyUserB != "userB:chat1:pluginA" {
		t.Errorf("unexpected scoped session key: %s", keyUserB)
	}
}
