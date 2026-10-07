package whatsmeow

import (
	"testing"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestParseDisappearingModeNotification_WithChild(t *testing.T) {
	cli := &Client{
		Log: waLog.Stdout("Test", "DEBUG", true),
	}

	chatJID := types.NewJID("120363000000000000", types.GroupServer)
	senderJID := types.NewJID("15550001111", types.DefaultUserServer)

	node := &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"type":        "disappearing_mode",
			"from":        chatJID,
			"participant": senderJID,
			"t":           "1700000000",
		},
		Content: []waBinary.Node{
			{
				Tag: "disappearing_mode",
				Attrs: waBinary.Attrs{
					"duration":  "86400",
					"trigger":   "chat_setting",
					"initiator": "changed_in_chat",
					"t":         "1700000001",
				},
			},
		},
	}

	evt := cli.parseDisappearingModeNotification(node)
	if evt == nil {
		t.Fatalf("expected DisappearingMode event to be parsed, got nil")
	}

	if evt.Chat != chatJID {
		t.Errorf("expected Chat %s, got %s", chatJID, evt.Chat)
	}
	if evt.Sender == nil || *evt.Sender != senderJID {
		t.Errorf("expected Sender %s, got %v", senderJID, evt.Sender)
	}
	if !evt.IsEphemeral {
		t.Errorf("expected IsEphemeral true, got false")
	}
	if evt.Timer != 24*time.Hour {
		t.Errorf("expected Timer 24h (86400s), got %s", evt.Timer)
	}
	if evt.Trigger != "chat_setting" {
		t.Errorf("expected Trigger 'chat_setting', got %q", evt.Trigger)
	}
	if evt.Initiator != "changed_in_chat" {
		t.Errorf("expected Initiator 'changed_in_chat', got %q", evt.Initiator)
	}
	if evt.SettingTimestamp.Unix() != 1700000001 {
		t.Errorf("expected SettingTimestamp 1700000001, got %d", evt.SettingTimestamp.Unix())
	}
}

func TestParseDisappearingModeNotification_Disabled(t *testing.T) {
	cli := &Client{
		Log: waLog.Stdout("Test", "DEBUG", true),
	}

	userJID := types.NewJID("15550002222", types.DefaultUserServer)

	node := &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"type": "disappearing_mode",
			"from": userJID,
			"t":    "1700000000",
		},
		Content: []waBinary.Node{
			{
				Tag: "disappearing_mode",
				Attrs: waBinary.Attrs{
					"duration": "0",
				},
			},
		},
	}

	evt := cli.parseDisappearingModeNotification(node)
	if evt == nil {
		t.Fatalf("expected DisappearingMode event to be parsed, got nil")
	}
	if evt.IsEphemeral {
		t.Errorf("expected IsEphemeral false, got true")
	}
	if evt.Timer != 0 {
		t.Errorf("expected Timer 0, got %s", evt.Timer)
	}
}
