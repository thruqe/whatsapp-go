package whatsmeow

import (
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestGhostMessageTracking(t *testing.T) {
	cli := &Client{}

	target := types.NewJID("1234567890", types.DefaultUserServer)
	msgID := types.MessageID("3EB0TEST123456")

	if cli.isGhostMessage(msgID) {
		t.Fatalf("expected msgID to not be ghost message")
	}
	if targets := cli.getGhostMessageTargets(msgID); targets != nil {
		t.Fatalf("expected targets to be nil")
	}

	cli.addGhostMessage(msgID, []types.JID{target})

	if !cli.isGhostMessage(msgID) {
		t.Fatalf("expected msgID to be recognized as ghost message")
	}
	targets := cli.getGhostMessageTargets(msgID)
	if len(targets) != 1 || targets[0] != target {
		t.Fatalf("expected target %v, got %v", target, targets)
	}

	// Test capacity rotation
	for i := range ghostMessagesCapacity+50 {
		id := types.MessageID(fmt.Sprintf("3EB0TEST%06d", i))
		cli.addGhostMessage(id, []types.JID{target})
		if !cli.isGhostMessage(id) {
			t.Fatalf("expected id %s to be recognized", id)
		}
	}

	// First one should have been evicted
	if cli.isGhostMessage(msgID) {
		t.Fatalf("expected old msgID to be evicted")
	}
}
