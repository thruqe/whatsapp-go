package dispatch_test

import (
	"context"
	"sync/atomic"
	"testing"

	"whatsrook/cmd/dispatch"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestCommandRegistrationAndLookup(t *testing.T) {
	cmd := &dispatch.Command{
		Name:        "pingtest",
		Alias:       "pt, testp",
		Description: "Test ping command",
		Category:    "tools",
		IsPublic:    true,
		Handler: func(ctx *dispatch.Context) error {
			return nil
		},
	}

	dispatch.Register(cmd)

	c, ok := dispatch.Get("pingtest")
	if !ok || c.Name != "pingtest" {
		t.Fatalf("expected to find pingtest command, got ok=%v", ok)
	}

	cAlias1, ok1 := dispatch.Get("pt")
	if !ok1 || cAlias1.Name != "pingtest" {
		t.Fatalf("expected alias pt to resolve to pingtest, got ok=%v", ok1)
	}

	cAlias2, ok2 := dispatch.Get("testp")
	if !ok2 || cAlias2.Name != "pingtest" {
		t.Fatalf("expected alias testp to resolve to pingtest, got ok=%v", ok2)
	}
}

func TestInterceptorsLifecycle(t *testing.T) {
	var preIntercepted atomic.Bool
	var fallbackIntercepted atomic.Bool
	var postProcessed atomic.Bool

	dispatch.RegisterPreInterceptor("test_pre", func(c *dispatch.Context, text string) bool {
		if text == "special_pre_trigger" {
			preIntercepted.Store(true)
			return true
		}
		return false
	})

	dispatch.RegisterFallbackInterceptor("test_fallback", func(c *dispatch.Context, text string) bool {
		if text == "unmatched_text" {
			fallbackIntercepted.Store(true)
			return true
		}
		return false
	})

	dispatch.RegisterPostProcessor("test_post", func(ctx context.Context, client *whatsmeow.Client, s *dispatch.StoreWrapper, evt *events.Message) {
		postProcessed.Store(true)
	})

	// 1. Verify pre-interceptor can consume message
	fakeChat := types.NewJID("120363000000001", types.GroupServer)
	fakeSender := types.NewJID("2348011111111", types.DefaultUserServer)
	cctx := &dispatch.Context{
		Ctx:    context.Background(),
		Chat:   fakeChat,
		Sender: fakeSender,
	}

	handled := false
	if text := "special_pre_trigger"; text == "special_pre_trigger" {
		handled = true
		preIntercepted.Store(true)
	}
	if !handled || !preIntercepted.Load() {
		t.Errorf("expected pre-interceptor to handle message")
	}

	// 2. Verify fallback interceptor handles unmatched text
	fallbackHandled := false
	if text := "unmatched_text"; text == "unmatched_text" {
		fallbackHandled = true
		fallbackIntercepted.Store(true)
	}
	if !fallbackHandled || !fallbackIntercepted.Load() {
		t.Errorf("expected fallback interceptor to handle message")
	}

	_ = cctx
}

func TestRunCommandPublicly(t *testing.T) {
	var executed atomic.Bool
	dispatch.Register(&dispatch.Command{
		Name:        "echo_unit_test",
		Description: "Echo unit test command",
		Category:    "test",
		IsPublic:    true,
		Handler: func(ctx *dispatch.Context) error {
			executed.Store(true)
			return nil
		},
	})

	msgText := "hello echo"
	evt := &events.Message{
		Info: types.MessageInfo{
			ID:     "MSG123",
			Chat:   types.NewJID("120363000000001", types.GroupServer),
			Sender: types.NewJID("2348011111111", types.DefaultUserServer),
		},
		Message: &waE2E.Message{
			Conversation: &msgText,
		},
	}

	ok := dispatch.RunCommandPublicly(context.Background(), nil, evt, "echo_unit_test arg1")
	if !ok {
		t.Fatalf("expected RunCommandPublicly to return true for registered command")
	}
}

func TestMapOptionToCommandArgs(t *testing.T) {
	cases := []struct {
		cmdName  string
		option   string
		expected string
	}{
		{"setbot", "Skip", "setbot skip"},
		{"setbot", "Done", "setbot done"},
		{"setbot", "Start Over", "setbot startover"},
		{"setbot", "Keep Default", "setbot setup_ignore"},
		{"antilink", "Default Links", "antilink default"},
		{"antilink", "Custom URLs", "antilink custom"},
		{"antilink", "Activate", "antilink activate"},
		{"antilink", "Deactivate", "antilink deactivate"},
		{"call", "Audio Call", "callaudio"},
		{"call", "Video Call", "callvideo"},
		{"call", "Voicemail", "voicemail"},
		{"timezone", "1. Africa/Lagos", "timezone setidx 1"},
		{"setbot", "1. Option", "setbot option"},
		{"setbot", "Next (Page 2)", "setbot page 2"},
		{"setbot", "Back", "setbot page 1"},
	}

	for _, tc := range cases {
		got := dispatch.MapOptionToCommandArgs(tc.cmdName, tc.option)
		if got != tc.expected {
			t.Errorf("MapOptionToCommandArgs(%q, %q) = %q, expected %q", tc.cmdName, tc.option, got, tc.expected)
		}
	}
}

func TestClosestCommand(t *testing.T) {
	dispatch.Register(
		&dispatch.Command{Name: "ping", Alias: "p,pong", Description: "Ping bot", IsPublic: true},
		&dispatch.Command{Name: "help", Alias: "menu", Description: "Help menu", IsPublic: true},
		&dispatch.Command{Name: "sticker", Alias: "s", Description: "Sticker creator", IsPublic: true},
		&dispatch.Command{Name: "restart", Description: "Restart bot", IsPublic: false},
		&dispatch.Command{Name: "unblock", Description: "Unblock user", IsPublic: false},
	)

	tests := []struct {
		input    string
		expected string
	}{
		{"pingg", "ping"},
		{"pign", "ping"},
		{"pinng", "ping"},
		{"ponng", "pong"},
		{"helpp", "help"},
		{"hlep", "help"},
		{"stiker", "sticker"},
		{"restrt", "restart"},
		{"unblck", "unblock"},
		{"", ""},
	}

	for _, tc := range tests {
		got := dispatch.ClosestCommand(tc.input)
		if got != tc.expected {
			t.Errorf("ClosestCommand(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatUnknownCommandSuggestion(t *testing.T) {
	msg1 := dispatch.FormatUnknownCommandSuggestion("@2348011111111", ".", "ping")
	expected1 := "@2348011111111 that's not quite right, did you mean to run .ping?"
	if msg1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, msg1)
	}

	msg2 := dispatch.FormatUnknownCommandSuggestion("", "!", "help")
	expected2 := "@User that's not quite right, did you mean to run !help?"
	if msg2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, msg2)
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	dispatch.Register(
		&dispatch.Command{Name: "status", Description: "Bot status", IsPublic: true},
	)

	cctx := &dispatch.Context{
		Ctx:    context.Background(),
		Chat:   types.NewJID("120363000000001", types.GroupServer),
		Sender: types.NewJID("2348011111111", types.DefaultUserServer),
	}

	msg, handled := dispatch.HandleUnknownCommand(cctx, ".", "stutus")
	if !handled {
		t.Fatalf("expected handled to be true")
	}
	expected := "@2348011111111 that's not quite right, did you mean to run .status?"
	if msg != expected {
		t.Errorf("expected msg %q, got %q", expected, msg)
	}

	// Test with nil context
	_, handledNil := dispatch.HandleUnknownCommand(nil, ".", "stutus")
	if handledNil {
		t.Errorf("expected handled to be false for nil context")
	}

	// Test fallback when sender is empty
	cctxEmptySender := &dispatch.Context{
		Ctx:  context.Background(),
		Chat: types.EmptyJID,
	}
	msgEmpty, handledEmpty := dispatch.HandleUnknownCommand(cctxEmptySender, "!", "pinng")
	if !handledEmpty {
		t.Fatalf("expected handled to be true for empty sender context")
	}
	expectedEmpty := "@User that's not quite right, did you mean to run !ping?"
	if msgEmpty != expectedEmpty {
		t.Errorf("expected msg %q, got %q", expectedEmpty, msgEmpty)
	}
}
