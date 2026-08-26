package src

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestFormattingHelpers(t *testing.T) {
	if got := Bold("hello"); got != "hello" {
		t.Errorf("Bold() = %q, want hello", got)
	}
	if got := Bold(""); got != "" {
		t.Errorf("Bold(\"\") = %q, want empty string", got)
	}
	if got := Boldf("count: %d", 5); got != "count: 5" {
		t.Errorf("Boldf() = %q, want count: 5", got)
	}

	if got := Italic("world"); got != "world" {
		t.Errorf("Italic() = %q, want world", got)
	}
	if got := Italicf("val: %s", "x"); got != "val: x" {
		t.Errorf("Italicf() = %q, want val: x", got)
	}

	if got := Code("go test"); got != "go test" {
		t.Errorf("Code() = %q, want go test", got)
	}
	if got := Codef("exit: %d", 0); got != "exit: 0" {
		t.Errorf("Codef() = %q, want exit: 0", got)
	}

	if got := CodeBlock("fmt.Println()", "go"); got != "fmt.Println()" {
		t.Errorf("CodeBlock() = %q, want fmt.Println()", got)
	}

	if got := Strike("deleted"); got != "deleted" {
		t.Errorf("Strike() = %q, want deleted", got)
	}
	if got := Strikef("num: %d", 1); got != "num: 1" {
		t.Errorf("Strikef() = %q, want num: 1", got)
	}

	if got := Quote("line1\nline2"); got != "line1\nline2" {
		t.Errorf("Quote() = %q, want line1\\nline2", got)
	}
	if got := Quotef("val: %d", 42); got != "val: 42" {
		t.Errorf("Quotef() = %q, want val: 42", got)
	}
}

func TestTextBuilder_Basic(t *testing.T) {
	b := NewText()
	if !b.IsEmpty() {
		t.Errorf("NewText() should be empty")
	}

	b.Header("GUIDE").
		Line("First line").
		Linef("Formatted: %d", 10).
		Blank().
		Bullet("Item 1").
		Bulletf("Item %d", 2).
		Numbered(1, "Step 1").
		Field("Key", "Value").
		Fieldf("Count", "%d items", 5)

	out := b.String()
	if !strings.Contains(out, "GUIDE\n\n") {
		t.Errorf("Expected Header in output, got: %q", out)
	}
	if strings.Contains(out, "*") || strings.Contains(out, "`") || strings.Contains(out, "•") {
		t.Errorf("Output should not contain formatting symbols: %q", out)
	}
	if !strings.Contains(out, "First line\n") {
		t.Errorf("Expected 'First line\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "Formatted: 10\n") {
		t.Errorf("Expected 'Formatted: 10\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "- Item 1\n") {
		t.Errorf("Expected '- Item 1\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "- Item 2\n") {
		t.Errorf("Expected '- Item 2\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "1. Step 1\n") {
		t.Errorf("Expected '1. Step 1\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "Key: Value\n") {
		t.Errorf("Expected 'Key: Value\\n' in output, got: %q", out)
	}
	if !strings.Contains(out, "Count: 5 items\n") {
		t.Errorf("Expected 'Count: 5 items\\n' in output, got: %q", out)
	}
}

func TestTextBuilder_InlineAndStyles(t *testing.T) {
	b := NewText().
		Text("Hello").Space().Bold("World").Text("!").NewLine().
		Italic("italic").Space().Code("inline").Space().Strike("strike").NewLine().
		Quote("quoted text")

	out := b.String()
	expected := "Hello World!\nitalic inline strike\nquoted text\n"
	if out != expected {
		t.Errorf("Got %q, want %q", out, expected)
	}
}

func TestTextBuilder_Conditionals(t *testing.T) {
	b := NewText().
		LineIf(true, "Included line").
		LineIf(false, "Excluded line").
		LinefIf(true, "Included %s", "linef").
		LinefIf(false, "Excluded %s", "linef").
		BulletIf(true, "Included bullet").
		BulletIf(false, "Excluded bullet").
		FieldIf(true, "Key1", "Val1").
		FieldIf(false, "Key2", "Val2").
		FieldIf(true, "Key3", ""). // empty val shouldn't print
		If(true, func(tb *TextBuilder) {
			tb.Line("If true branch")
		}).
		If(false, func(tb *TextBuilder) {
			tb.Line("If false branch")
		}).
		IfElse(false, func(tb *TextBuilder) {
			tb.Line("Then branch")
		}, func(tb *TextBuilder) {
			tb.Line("Else branch")
		})

	out := b.String()
	if strings.Contains(out, "Excluded") {
		t.Errorf("Output contains excluded elements: %q", out)
	}
	if !strings.Contains(out, "Included line\n") {
		t.Errorf("Missing 'Included line': %q", out)
	}
	if !strings.Contains(out, "Included linef\n") {
		t.Errorf("Missing 'Included linef': %q", out)
	}
	if !strings.Contains(out, "- Included bullet\n") {
		t.Errorf("Missing '- Included bullet': %q", out)
	}
	if !strings.Contains(out, "Key1: Val1\n") {
		t.Errorf("Missing 'Key1: Val1': %q", out)
	}
	if strings.Contains(out, "Key3") {
		t.Errorf("Empty field Key3 should not be in output: %q", out)
	}
	if !strings.Contains(out, "If true branch\n") {
		t.Errorf("Missing 'If true branch': %q", out)
	}
	if !strings.Contains(out, "Else branch\n") {
		t.Errorf("Missing 'Else branch': %q", out)
	}
}

func TestTextBuilder_Mentions(t *testing.T) {
	jid1 := types.NewJID("123456789", types.DefaultUserServer)
	jid2 := types.NewJID("987654321", types.DefaultUserServer)

	b := NewText().
		Text("Hello ").Mention(jid1).Text(" and ").MentionUser("Friend", jid2)

	out := b.String()
	if out != "Hello @123456789 and @Friend" {
		t.Errorf("Got %q, want 'Hello @123456789 and @Friend'", out)
	}

	mentions := b.GetMentions()
	if len(mentions) != 2 {
		t.Fatalf("Expected 2 mentions, got %d", len(mentions))
	}
	if mentions[0] != jid1 || mentions[1] != jid2 {
		t.Errorf("Mentions do not match expected JIDs")
	}
}

func TestTextBuilder_Collections(t *testing.T) {
	b := NewText().
		Bullets("Apple", "Banana", "Cherry").
		Blank().
		NumberedList("First", "Second")

	out := b.String()
	expected := "- Apple\n- Banana\n- Cherry\n\n1. First\n2. Second\n"
	if out != expected {
		t.Errorf("Got %q, want %q", out, expected)
	}
}
