package whatsmeow

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestServer401ErrorSkipping(t *testing.T) {
	cli := &Client{}
	reqID := types.MessageID("test-msg-123")

	cli.server401ErrorsLock.Lock()
	count, _ := incrementBoundedCounter(
		ensureMap(&cli.server401Errors),
		reqID,
		maxServer401Entries,
		&cli.server401ErrorsReset,
		time.Now(),
	)
	cli.server401ErrorsLock.Unlock()

	if count != 1 {
		t.Fatalf("expected count 1 on first attempt, got %d", count)
	}

	cli.server401ErrorsLock.Lock()
	count2, _ := incrementBoundedCounter(
		ensureMap(&cli.server401Errors),
		reqID,
		maxServer401Entries,
		&cli.server401ErrorsReset,
		time.Now(),
	)
	cli.server401ErrorsLock.Unlock()

	if count2 != 2 {
		t.Fatalf("expected count 2 on second attempt, got %d", count2)
	}

	// Successful ACK clears tracked 401 error
	cli.server401ErrorsLock.Lock()
	delete(cli.server401Errors, reqID)
	cli.server401ErrorsLock.Unlock()

	cli.server401ErrorsLock.Lock()
	count3, _ := incrementBoundedCounter(
		ensureMap(&cli.server401Errors),
		reqID,
		maxServer401Entries,
		&cli.server401ErrorsReset,
		time.Now(),
	)
	cli.server401ErrorsLock.Unlock()

	if count3 != 1 {
		t.Fatalf("expected count 1 after reset, got %d", count3)
	}
}
