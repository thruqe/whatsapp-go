package chats

import (
	"testing"

	"whatsrook/cmd/dispatch"
)

func TestDltCommandRegistration(t *testing.T) {
	cmd, ok := dispatch.Get("dlt")
	if !ok || cmd == nil {
		t.Fatalf("expected 'dlt' command to be registered in dispatch")
	}
	if cmd.Name != "dlt" {
		t.Fatalf("expected primary command name 'dlt', got %q", cmd.Name)
	}

	delCmd, delOk := dispatch.Get("del")
	if !delOk || delCmd != cmd {
		t.Fatalf("expected 'del' alias to resolve to dlt command")
	}

	deleteCmd, deleteOk := dispatch.Get("delete")
	if !deleteOk || deleteCmd != cmd {
		t.Fatalf("expected 'delete' alias to resolve to dlt command")
	}
}
