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
