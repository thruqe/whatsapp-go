package plugins

import (
	"testing"
)

func TestTTTCommand_NameAndAlias(t *testing.T) {
	cmd, ok := Get("ttt")
	if !ok || cmd == nil {
		t.Fatalf("Expected command 'ttt' to be registered as primary command name")
	}
	if cmd.Name != "ttt" {
		t.Errorf("Expected command Name to be 'ttt', got: %s", cmd.Name)
	}
	if cmd.Alias != "tictactoe" {
		t.Errorf("Expected command Alias to be 'tictactoe', got: %s", cmd.Alias)
	}

	aliasCmd, okAlias := Get("tictactoe")
	if !okAlias || aliasCmd == nil {
		t.Fatalf("Expected alias 'tictactoe' to be registered")
	}
	if aliasCmd != cmd {
		t.Errorf("Expected 'tictactoe' alias to resolve to same Command as 'ttt'")
	}
}

func TestUnscrambleCommand_Registration(t *testing.T) {
	cmd, ok := Get("unscramble")
	if !ok || cmd == nil {
		t.Fatalf("Expected command 'unscramble' to be registered")
	}
	if cmd.Name != "unscramble" {
		t.Errorf("Expected command Name 'unscramble', got: %s", cmd.Name)
	}

	aliasCmd, okAlias := Get("wordunscramble")
	if !okAlias || aliasCmd == nil {
		t.Fatalf("Expected alias 'wordunscramble' to be registered")
	}
	if aliasCmd != cmd {
		t.Errorf("Expected 'wordunscramble' alias to resolve to 'unscramble'")
	}
}
