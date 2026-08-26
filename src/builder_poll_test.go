package src

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestPollBuilder_Basic(t *testing.T) {
	pb := NewPoll(nil, "Choose an option:")
	if pb.question != "Choose an option:" {
		t.Errorf("question = %q, want 'Choose an option:'", pb.question)
	}
	if !pb.single {
		t.Errorf("expected single choice by default")
	}
	if !pb.autoDelete {
		t.Errorf("expected autoDelete = true by default")
	}
	if pb.timeout != DefaultPollTimeout {
		t.Errorf("expected timeout = %v, got %v", DefaultPollTimeout, pb.timeout)
	}

	pb.AddOption("Option 1").
		AddOptions("Option 2", "Option 3").
		MultiChoice().
		Timeout(10 * time.Second)

	if len(pb.options) != 3 {
		t.Errorf("len(options) = %d, want 3", len(pb.options))
	}
	if pb.single {
		t.Errorf("expected multi-choice")
	}
	if pb.timeout != 10*time.Second {
		t.Errorf("expected timeout = 10s, got %v", pb.timeout)
	}

	pb.SingleChoice().AutoDelete(false)
	if !pb.single {
		t.Errorf("expected single choice after SingleChoice()")
	}
	if pb.autoDelete {
		t.Errorf("expected autoDelete = false after AutoDelete(false)")
	}
}

func TestPollRoute_Lifecycle(t *testing.T) {
	msgID := types.MessageID("TEST_POLL_MSG_ID_1")
	RegisterPollRoute(PollRouteConfig{
		PollMsgID:  msgID,
		Options:    []string{"Opt A", "Opt B"},
		Once:       true,
		AutoDelete: true,
		Timeout:    100 * time.Millisecond,
	})

	reactorsMu.RLock()
	route, ok := pollRoutes[msgID]
	reactorsMu.RUnlock()

	if !ok {
		t.Fatalf("expected route to be registered")
	}
	if len(route.options) != 2 {
		t.Errorf("expected 2 options in route, got %d", len(route.options))
	}

	// Wait for the timeout to fire and auto-deregister
	time.Sleep(150 * time.Millisecond)

	reactorsMu.RLock()
	_, okAfter := pollRoutes[msgID]
	reactorsMu.RUnlock()

	if okAfter {
		t.Errorf("expected route to be removed after timeout expiry")
	}
}
