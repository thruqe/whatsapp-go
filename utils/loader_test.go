package utils

import (
	"context"
	"testing"
	"time"
)

func TestLoader_StopAndIdempotentDelete(t *testing.T) {
	ctx := &PluginContext{
		Ctx: context.Background(),
	}

	l := &Loader{
		ctx:         ctx,
		id:          nextLoaderID(),
		initialText: "Please wait",
		stopChan:    make(chan struct{}),
		msgID:       "TEST_MSG_ID",
	}

	// Calling Delete on a loader without a client should be safe and not panic or deadlock
	l.Delete()

	if l.msgID != "" {
		t.Errorf("expected msgID to be cleared after Delete, got %q", l.msgID)
	}
	if !l.stopped {
		t.Errorf("expected stopped to be true")
	}

	// Calling Delete multiple times must be idempotent and not panic
	l.Delete()
}

func TestPluginContext_StopAutoLoaderNoDeadlock(t *testing.T) {
	ctx := &PluginContext{
		Ctx: context.Background(),
	}

	// Start auto loader with delay
	ctx.StartAutoLoader(50 * time.Millisecond)

	// Stop before timer fires
	ctx.StopAutoLoader()

	if ctx.loaderTimer != nil {
		t.Errorf("expected loaderTimer to be nil after StopAutoLoader")
	}
	if !ctx.loaderStopped {
		t.Errorf("expected loaderStopped to be true")
	}

	// Repeated StopAutoLoader calls must not deadlock
	done := make(chan bool)
	go func() {
		ctx.StopAutoLoader()
		ctx.StopAutoLoader()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("StopAutoLoader deadlocked on repeated calls")
	}
}

func TestPluginContext_AutoLoaderTimerFiresAndStops(t *testing.T) {
	ctx := &PluginContext{
		Ctx: context.Background(),
	}

	ctx.StartAutoLoader(10 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	// Even if client is nil, StopAutoLoader must complete without deadlock
	done := make(chan bool)
	go func() {
		ctx.StopAutoLoader()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("StopAutoLoader deadlocked after timer fired")
	}
}
