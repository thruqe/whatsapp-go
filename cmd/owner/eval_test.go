package owner

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/cmd/dispatch"
)

func TestExecuteJavaScriptEval(t *testing.T) {
	ctx := &dispatch.Context{
		Chat:   types.NewJID("123456789", types.DefaultUserServer),
		Sender: types.NewJID("987654321", types.DefaultUserServer),
		Evt: &events.Message{
			Info: types.MessageInfo{
				ID: "MSG-12345",
			},
		},
	}

	// 1. Math expression
	res, err := executeJavaScriptEval(ctx, "2 + 2 * 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "22" {
		t.Errorf("expected 22, got %q", res)
	}

	// 2. Return statement (Baileys style)
	resReturn, err := executeJavaScriptEval(ctx, "return 40 + 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resReturn != "42" {
		t.Errorf("expected 42, got %q", resReturn)
	}

	// 3. Return message object
	resMsg, err := executeJavaScriptEval(ctx, "return message.id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resMsg != "MSG-12345" {
		t.Errorf("expected MSG-12345, got %q", resMsg)
	}

	// 4. Return complex object formatted as JSON
	resObj, err := executeJavaScriptEval(ctx, "return { a: 1, b: 'test' }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resObj, `"a": 1`) || !strings.Contains(resObj, `"b": "test"`) {
		t.Errorf("expected JSON object string, got %q", resObj)
	}

	// 5. Console.log
	resLog, err := executeJavaScriptEval(ctx, "console.log('logged text'); return 'done'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resLog, "logged text") || !strings.Contains(resLog, "done") {
		t.Errorf("expected log and return value, got %q", resLog)
	}
}

func TestExecuteGrolEval(t *testing.T) {
	ctx := &dispatch.Context{}
	res, err := executeGrolEval(ctx, "10 * 5 + 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "53" {
		t.Errorf("expected 53, got %q", res)
	}
}

func TestHandleOwnerEvalIntercept_Unauthorized(t *testing.T) {
	// Context where user is not owner or sudo
	ctx := &dispatch.Context{
		Chat:   types.NewJID("123456789", types.DefaultUserServer),
		Sender: types.NewJID("987654321", types.DefaultUserServer),
	}

	handled := HandleOwnerEvalIntercept(ctx, "$ return message")
	if handled {
		t.Errorf("expected non-owner/sudo message not to be handled")
	}
}
