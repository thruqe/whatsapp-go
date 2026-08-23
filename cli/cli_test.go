package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
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

func TestParseCLIArgs_OverridesEnv(t *testing.T) {
	// Set env vars
	t.Setenv("PAIR", "true")
	t.Setenv("QRCODE", "false")
	t.Setenv("SESSION", "111111111111")
	t.Setenv("CLIENT", "android")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	// Case 1: -q overrides PAIR=true in env
	args := parseCLIArgsFrom([]string{"-s", "2348060598068", "-q"})
	if !args.QRCode {
		t.Errorf("expected QRCode=true when -q is passed, got false")
	}
	if args.Pair {
		t.Errorf("expected Pair=false when -q is passed, despite PAIR=true in env")
	}
	if args.Session != "2348060598068" {
		t.Errorf("expected Session=2348060598068, got %q", args.Session)
	}

	// Case 2: -p overrides QRCODE=true in env
	t.Setenv("PAIR", "false")
	t.Setenv("QRCODE", "true")
	args2 := parseCLIArgsFrom([]string{"-s", "2348060598068", "-p"})
	if !args2.Pair {
		t.Errorf("expected Pair=true when -p is passed, got false")
	}
	if args2.QRCode {
		t.Errorf("expected QRCode=false when -p is passed, despite QRCODE=true in env")
	}

	// Case 3: -c overrides CLIENT env var
	args3 := parseCLIArgsFrom([]string{"-c", "chrome"})
	if args3.Client != "chrome" {
		t.Errorf("expected Client=chrome when -c chrome passed, got %q", args3.Client)
	}

	// Case 4: -db overrides DATABASE_URL env var
	args4 := parseCLIArgsFrom([]string{"-db", "sqlite"})
	if args4.Database != "sqlite" {
		t.Errorf("expected Database=sqlite when -db sqlite passed, got %q", args4.Database)
	}

	// Case 5: Positional phone argument overrides SESSION env var
	args5 := parseCLIArgsFrom([]string{"2348099887766"})
	if args5.Session != "2348099887766" {
		t.Errorf("expected Session=2348099887766 from positional arg, got %q", args5.Session)
	}

	// Case 6: -redis overrides REDIS_URL env var
	t.Setenv("REDIS_URL", "redis://localhost:6379/1")
	args6 := parseCLIArgsFrom([]string{"-redis", "redis://remote-redis:6379/2"})
	if args6.RedisURL != "redis://remote-redis:6379/2" {
		t.Errorf("expected RedisURL from flag, got %q", args6.RedisURL)
	}

	// Case 7: REDIS_URL env var fallback when flag not set
	args7 := parseCLIArgsFrom([]string{})
	if args7.RedisURL != "redis://localhost:6379/1" {
		t.Errorf("expected RedisURL from env var, got %q", args7.RedisURL)
	}
}

func TestCaptchaCommand_Registered(t *testing.T) {
	cmd, ok := commands.Get("captcha")
	if !ok || cmd == nil {
		t.Fatalf("expected 'captcha' command to be registered")
	}
	if cmd.Category != "group" {
		t.Errorf("expected category to be 'group', got %s", cmd.Category)
	}
	if !cmd.GroupOnly {
		t.Errorf("expected GroupOnly to be true")
	}
}

func TestPendingCaptcha_Lifecycle(t *testing.T) {
	groupJID := types.NewJID("123456789", types.GroupServer)
	userJID := types.NewJID("987654321", types.DefaultUserServer)
	resolvedJID := types.NewJID("987654321", types.DefaultUserServer)

	var timeoutCalled atomic.Bool
	code := "4829"

	// 1. Register pending captcha with short duration
	commands.RegisterPendingCaptcha(
		groupJID,
		userJID,
		resolvedJID,
		"TestUser",
		code,
		100*time.Millisecond,
		func() {
			timeoutCalled.Store(true)
		},
	)

	// 2. Lookup pending captcha
	pending, ok := commands.GetPendingCaptcha(groupJID, userJID)
	if !ok || pending == nil {
		t.Fatalf("expected pending captcha to exist")
	}
	if pending.Code != code {
		t.Errorf("expected code %s, got %s", code, pending.Code)
	}
	if pending.Username != "TestUser" {
		t.Errorf("expected username 'TestUser', got %s", pending.Username)
	}

	// 3. Let timer expire
	time.Sleep(150 * time.Millisecond)

	if !timeoutCalled.Load() {
		t.Errorf("expected timeout callback to be invoked")
	}

	// 4. Verify it was deleted after timeout
	_, okAfter := commands.GetPendingCaptcha(groupJID, userJID)
	if okAfter {
		t.Errorf("expected pending captcha to be removed after timeout")
	}
}

func TestPendingCaptcha_ManualRemoval(t *testing.T) {
	groupJID := types.NewJID("123456789", types.GroupServer)
	userJID := types.NewJID("987654321", types.DefaultUserServer)
	resolvedJID := types.NewJID("987654321", types.DefaultUserServer)

	var timeoutCalled atomic.Bool
	code := "1234"

	commands.RegisterPendingCaptcha(
		groupJID,
		userJID,
		resolvedJID,
		"User2",
		code,
		200*time.Millisecond,
		func() {
			timeoutCalled.Store(true)
		},
	)

	removed, ok := commands.RemovePendingCaptcha(groupJID, userJID)
	if !ok || removed == nil {
		t.Fatalf("expected pending captcha to be removed successfully")
	}

	time.Sleep(250 * time.Millisecond)

	if timeoutCalled.Load() {
		t.Errorf("timeout callback should not have been called after manual removal")
	}
}
