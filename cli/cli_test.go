package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `
# Test env file
SESSION=2348061234567
CLIENT=android
PAIR=true
VERBOSE=1
QUOTED_VAL="hello world"
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	_ = os.Unsetenv("SESSION")
	_ = os.Unsetenv("CLIENT")
	_ = os.Unsetenv("PAIR")

	loadDotEnv(envPath)

	if got := os.Getenv("SESSION"); got != "2348061234567" {
		t.Errorf("expected SESSION=2348061234567, got %q", got)
	}
	if got := os.Getenv("CLIENT"); got != "android" {
		t.Errorf("expected CLIENT=android, got %q", got)
	}
	if got := os.Getenv("PAIR"); got != "true" {
		t.Errorf("expected PAIR=true, got %q", got)
	}
	if got := os.Getenv("QUOTED_VAL"); got != "hello world" {
		t.Errorf("expected QUOTED_VAL='hello world', got %q", got)
	}
}
