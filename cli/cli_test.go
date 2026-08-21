package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types/events"
	commands "whatsrook/cli/plugins"
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

func TestRunIdleMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use an ephemeral port
	port := 38472
	errChan := make(chan error, 1)

	go func() {
		errChan <- runIdleMode(ctx, port)
	}()

	// Wait for server to bind
	var resp *http.Response
	var err error
	for range 20 {
		time.Sleep(50 * time.Millisecond)
		resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to query idle server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("expected nil error on cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runIdleMode to shutdown")
	}
}

func TestBot_LoggedOut_EventHandling(t *testing.T) {
	bot := NewBot(BotConfig{
		Session: "1234567890",
		DataDir: t.TempDir(),
	})

	called := false
	bot.onLoggedOut = func() {
		called = true
	}

	bot.WAEventHandler(&events.LoggedOut{})

	if !bot.loggedOut.Load() {
		t.Errorf("expected loggedOut atomic boolean to be true")
	}
	if !called {
		t.Errorf("expected onLoggedOut callback to be invoked")
	}
}

func TestAutoMuteScheduler_Lifecycle(t *testing.T) {
	ctx := t.Context()

	// Should not panic or emit errors with nil client or nil store
	commands.StartAutoMuteScheduler(ctx, nil)
	time.Sleep(50 * time.Millisecond)
	commands.StopAutoMuteScheduler()

	// Restart and cancel via context
	ctx2, cancel2 := context.WithCancel(context.Background())
	commands.StartAutoMuteScheduler(ctx2, nil)
	cancel2()
	time.Sleep(50 * time.Millisecond)
	commands.StopAutoMuteScheduler()
}

func TestAutoBioScheduler_Lifecycle(t *testing.T) {
	ctx := t.Context()

	// Should not panic or emit errors with nil client or nil store
	commands.StartAutoBioScheduler(ctx, nil)
	time.Sleep(50 * time.Millisecond)
	commands.StopAutoBioScheduler()

	// Restart and cancel via context
	ctx2, cancel2 := context.WithCancel(context.Background())
	commands.StartAutoBioScheduler(ctx2, nil)
	cancel2()
	time.Sleep(50 * time.Millisecond)
	commands.StopAutoBioScheduler()
}
