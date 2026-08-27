package plugins

import (
	"reflect"
	"testing"
)

func TestTagAllCommand_Alias(t *testing.T) {
	cmd, ok := Get("tag")
	if !ok || cmd == nil {
		t.Fatalf("Expected command 'tag' to be registered as an alias of tagall")
	}
	if cmd.Name != "tagall" {
		t.Errorf("Expected command name 'tagall', got: %s", cmd.Name)
	}

	cmdAll, okAll := Get("tagall")
	if !okAll || cmdAll == nil {
		t.Fatalf("Expected command 'tagall' to be registered")
	}
	if cmdAll != cmd {
		t.Errorf("Expected 'tag' and 'tagall' to resolve to same Command")
	}
}

func TestAutoReactCommand_Registration(t *testing.T) {
	cmd, ok := Get("autoreact")
	if !ok || cmd == nil {
		t.Fatalf("Expected command 'autoreact' to be registered")
	}
	if cmd.Name != "autoreact" {
		t.Errorf("Expected command name 'autoreact', got: %s", cmd.Name)
	}

	aliasCmd, okAlias := Get("reactauto")
	if !okAlias || aliasCmd == nil {
		t.Fatalf("Expected alias 'reactauto' to be registered")
	}
	if aliasCmd != cmd {
		t.Errorf("Expected 'reactauto' alias to resolve to 'autoreact'")
	}
}

func TestAutoReadCommand_Registration(t *testing.T) {
	cmd, ok := Get("autoread")
	if !ok || cmd == nil {
		t.Fatalf("Expected command 'autoread' to be registered")
	}
	if cmd.Name != "autoread" {
		t.Errorf("Expected command name 'autoread', got: %s", cmd.Name)
	}

	aliasCmd, okAlias := Get("readauto")
	if !okAlias || aliasCmd == nil {
		t.Fatalf("Expected alias 'readauto' to be registered")
	}
	if aliasCmd != cmd {
		t.Errorf("Expected 'readauto' alias to resolve to 'autoread'")
	}
}

func TestParseEmojiList(t *testing.T) {
	// Comma separated
	list1 := parseEmojiList("❤️, 🔥, 👍, 🚀")
	expected1 := []string{"❤️", "🔥", "👍", "🚀"}
	if !reflect.DeepEqual(list1, expected1) {
		t.Errorf("parseEmojiList comma-separated failed: got %v, want %v", list1, expected1)
	}

	// Whitespace separated
	list2 := parseEmojiList("❤️ 🔥 👍 ✨")
	expected2 := []string{"❤️", "🔥", "👍", "✨"}
	if !reflect.DeepEqual(list2, expected2) {
		t.Errorf("parseEmojiList space-separated failed: got %v, want %v", list2, expected2)
	}

	// Empty string
	if parseEmojiList("") != nil {
		t.Errorf("parseEmojiList empty string should return nil")
	}
}
