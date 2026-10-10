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

	// 6. client.parseJID and jid helper
	resJID, err := executeJavaScriptEval(ctx, "return client.parseJID('999888')")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resJID != "999888@s.whatsapp.net" {
		t.Errorf("expected 999888@s.whatsapp.net, got %q", resJID)
	}

	// 7. toProto with Baileys-style text and mentions
	resProto, err := executeJavaScriptEval(ctx, "let m = toProto({ text: 'Hello @user', mentions: ['123@s.whatsapp.net'] }); return proto.toJSON(m)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resProto, "Hello @user") || !strings.Contains(resProto, "123@s.whatsapp.net") {
		t.Errorf("expected extendedTextMessage with mentions, got %q", resProto)
	}

	// 8. toProto with direct protobuf structure
	resProtoDirect, err := executeJavaScriptEval(ctx, "let m = toProto({ conversation: 'Direct text' }); return proto.toJSON(m)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resProtoDirect, "Direct text") {
		t.Errorf("expected direct conversation, got %q", resProtoDirect)
	}

	// 9. wa constants
	resWA, err := executeJavaScriptEval(ctx, "return wa.Presence.Available")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resWA != "available" {
		t.Errorf("expected available, got %q", resWA)
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

	resButterfly, err := executeGrolEval(ctx, "Butterfly()")
	if err != nil {
		t.Fatalf("unexpected error running Butterfly in Grol: %v", err)
	}
	if !strings.Contains(resButterfly, "Time elapsed:") {
		t.Errorf("expected Time elapsed in Butterfly output, got %q", resButterfly)
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
