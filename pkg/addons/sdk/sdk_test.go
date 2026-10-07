package sdk

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRequestMethods(t *testing.T) {
	req := &Request{
		Command:  "weather",
		Args:     []string{"forecast", "London", "--days", "5", "-v"},
		RawArgs:  "forecast London --days 5 -v",
		Chat:     "123456789-987654@g.us",
		Sender:   "44712345678@s.whatsapp.net",
		Prefix:   ".",
		BotName:  "WhatsRook",
		PushName: "Alice",
		IsGroup:  true,
		IsAdmin:  true,
		QuotedMessage: &QuotedMessage{
			ID:     "MSG999",
			Sender: "44700000000@s.whatsapp.net",
			Text:   "Original question",
		},
	}

	if req.Subcommand() != "forecast" {
		t.Fatalf("expected subcommand 'forecast', got %q", req.Subcommand())
	}
	if len(req.SubcommandArgs()) != 4 || req.SubcommandArgs()[0] != "London" {
		t.Fatalf("unexpected subcommand args: %v", req.SubcommandArgs())
	}
	if req.Arg(1) != "London" {
		t.Fatalf("expected arg 1 'London', got %q", req.Arg(1))
	}
	if days, err := req.ArgAsInt(3); err != nil || days != 5 {
		t.Fatalf("expected days 5, got %d (err: %v)", days, err)
	}
	if !req.Flag("v") {
		t.Fatalf("expected flag 'v' to be true")
	}
	if req.FlagValue("days") != "5" {
		t.Fatalf("expected flag value '5', got %q", req.FlagValue("days"))
	}
	if req.SenderPhone() != "44712345678" {
		t.Fatalf("expected phone '44712345678', got %q", req.SenderPhone())
	}
	if req.ChatTarget() != "123456789-987654" {
		t.Fatalf("expected chat target '123456789-987654', got %q", req.ChatTarget())
	}
	if req.QuotedText() != "Original question" {
		t.Fatalf("expected quoted text 'Original question', got %q", req.QuotedText())
	}
	if req.QuotedID() != "MSG999" {
		t.Fatalf("expected quoted id 'MSG999', got %q", req.QuotedID())
	}
	if req.IsDM() {
		t.Fatalf("expected IsDM() to be false for group")
	}
	if !req.IsGroup {
		t.Fatalf("expected IsGroup to be true")
	}
}

func TestActionSerialization(t *testing.T) {
	tests := []struct {
		name     string
		action   *Action
		expected string
	}{
		{
			name:     "reply",
			action:   NewReplyAction("test message"),
			expected: `{"action":"reply","text":"test message"}`,
		},
		{
			name:     "edit",
			action:   NewEditAction("MSG123", "updated"),
			expected: `{"action":"edit","text":"updated","msg_id":"MSG123"}`,
		},
		{
			name:     "react",
			action:   NewReactAction("👍"),
			expected: `{"action":"react","emoji":"👍"}`,
		},
		{
			name:     "react_to",
			action:   NewReactToAction("MSG456", "🔥"),
			expected: `{"action":"react","msg_id":"MSG456","emoji":"🔥"}`,
		},
		{
			name:     "image_with_caption",
			action:   NewImageAction("https://example.com/pic.png").WithCaption("A photo"),
			expected: `{"action":"send_image","data":"https://example.com/pic.png","caption":"A photo"}`,
		},
		{
			name:     "video_as_gif",
			action:   NewVideoAction("https://example.com/vid.mp4").AsGIF(true),
			expected: `{"action":"send_video","data":"https://example.com/vid.mp4","gif_playback":true}`,
		},
		{
			name:     "poll",
			action:   NewPollAction("Choose:", []string{"A", "B", "C"}).WithSelectable(2),
			expected: `{"action":"poll","question":"Choose:","options":["A","B","C"],"selectable":2}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.action)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			// compare parsed JSON to ignore key ordering
			var gotMap, expMap map[string]any
			if err := json.Unmarshal(data, &gotMap); err != nil {
				t.Fatalf("unmarshal got error: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.expected), &expMap); err != nil {
				t.Fatalf("unmarshal exp error: %v", err)
			}
			gotStr, _ := json.Marshal(gotMap)
			expStr, _ := json.Marshal(expMap)
			if string(gotStr) != string(expStr) {
				t.Errorf("mismatch:\ngot: %s\nexp: %s", gotStr, expStr)
			}
		})
	}
}

func TestFormatting(t *testing.T) {
	if Bold("hello") != "*hello*" {
		t.Errorf("expected *hello*, got %s", Bold("hello"))
	}
	if Italic("hello") != "_hello_" {
		t.Errorf("expected _hello_, got %s", Italic("hello"))
	}
	if Strikethrough("hello") != "~hello~" {
		t.Errorf("expected ~hello~, got %s", Strikethrough("hello"))
	}
	if Monospace("code") != "`code`" {
		t.Errorf("expected `code`, got %s", Monospace("code"))
	}
	if CodeBlock("code") != "```\ncode\n```" {
		t.Errorf("unexpected CodeBlock: %s", CodeBlock("code"))
	}
	if Quote("a\nb") != "> a\n> b" {
		t.Errorf("unexpected Quote: %s", Quote("a\nb"))
	}
	if BulletList([]string{"1", "2"}) != "• 1\n• 2" {
		t.Errorf("unexpected BulletList: %s", BulletList([]string{"1", "2"}))
	}
	if NumberedList([]string{"a", "b"}) != "1. a\n2. b" {
		t.Errorf("unexpected NumberedList: %s", NumberedList([]string{"a", "b"}))
	}
}

func TestMessageBuilder(t *testing.T) {
	msg := NewMessageBuilder().
		Header("Title").
		Line("Intro").
		Bold("Bold").
		Text(" ").
		Italic("Italic").
		Newline().
		Bullet("Item 1").
		Numbered(2, "Item 2").
		Quote("Quote").
		CodeBlock("func main() {}").
		Build()

	expectedSubstrings := []string{
		"*Title*\n\n",
		"Intro\n",
		"*Bold* _Italic_\n",
		"• Item 1\n",
		"2. Item 2\n",
		"> Quote\n",
		"```\nfunc main() {}\n```",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(msg, sub) {
			t.Errorf("missing substring %q in built message:\n%s", sub, msg)
		}
	}
}

func TestBase64Roundtrip(t *testing.T) {
	cases := [][]byte{
		[]byte(""),
		[]byte("f"),
		[]byte("fo"),
		[]byte("foo"),
		[]byte("foob"),
		[]byte("fooba"),
		[]byte("foobar"),
		[]byte("Go in WhatsApp plugins is awesome!"),
	}

	for _, c := range cases {
		enc := EncodeBase64(c)
		dec, err := DecodeBase64(enc)
		if err != nil {
			t.Fatalf("decode failed for %s: %v", string(c), err)
		}
		if string(dec) != string(c) {
			t.Fatalf("expected %q, got %q", string(c), string(dec))
		}
	}
}

func TestDataURL(t *testing.T) {
	got := ToDataURL("image/png", "aGVsbG8=")
	expected := "data:image/png;base64,aGVsbG8="
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestCreateHTTPClient(t *testing.T) {
	client := CreateHTTPClient(5)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout.Seconds() != 5 {
		t.Errorf("expected timeout 5s, got %v", client.Timeout)
	}
}

func TestCLIArgsFallback(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"mybot", "hello", "world"}
	req := Load()
	if req.Command != "mybot" {
		t.Errorf("expected command 'mybot', got %q", req.Command)
	}
	if req.Query() != "hello world" {
		t.Errorf("expected query 'hello world', got %q", req.Query())
	}
}
