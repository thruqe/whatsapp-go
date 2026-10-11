package owner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsrook/bot/dispatch"
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

func TestEvalFunctionMetadataAndCallTracking(t *testing.T) {
	ctx := &dispatch.Context{
		Chat:   types.NewJID("123456789", types.DefaultUserServer),
		Sender: types.NewJID("987654321", types.DefaultUserServer),
		Evt: &events.Message{
			Info: types.MessageInfo{
				ID: "MSG-12345",
			},
		},
	}

	// 1. Evaluating a function without calling it returns its type and return type metadata
	fnRes, err := executeJavaScriptEvalDetailed(ctx, "client.parseJID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fnRes.IsFunction {
		t.Errorf("expected IsFunction = true, got false")
	}
	if fnRes.FuncType == "" {
		t.Errorf("expected non-empty FuncType")
	}
	if fnRes.ReturnType == "" {
		t.Errorf("expected non-empty ReturnType")
	}
	replyText := fnRes.FormatForReply()
	if !strings.Contains(replyText, "• *Function:*") || !strings.Contains(replyText, "• *Type:*") || !strings.Contains(replyText, "• *Return Type:*") {
		t.Errorf("expected formatted reply to contain function metadata, got: %s", replyText)
	}

	// 2. Calling a function tracks the call and formats with function, type, return type, and result type
	callRes, err := executeJavaScriptEvalDetailed(ctx, "client.parseJID('5511999999999')")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callRes.CalledFunc != "parseJID" {
		t.Errorf("expected CalledFunc 'parseJID', got %q", callRes.CalledFunc)
	}
	if callRes.FuncType == "" {
		t.Errorf("expected non-empty FuncType")
	}
	if callRes.ReturnType == "" {
		t.Errorf("expected non-empty ReturnType")
	}
	if callRes.ResultType == "" {
		t.Errorf("expected non-empty ResultType")
	}
	callReply := callRes.FormatForReply()
	if !strings.Contains(callReply, "• *Function:* `parseJID`") {
		t.Errorf("expected call reply to contain function name, got: %s", callReply)
	}
	if !strings.Contains(callReply, "• *Function Type:*") {
		t.Errorf("expected call reply to contain function type, got: %s", callReply)
	}
	if !strings.Contains(callReply, "• *Return Type:*") {
		t.Errorf("expected call reply to contain return type, got: %s", callReply)
	}
	if !strings.Contains(callReply, "• *Result Type:*") {
		t.Errorf("expected call reply to contain result type, got: %s", callReply)
	}
	if !strings.Contains(callReply, "5511999999999@s.whatsapp.net") {
		t.Errorf("expected call reply to contain result value, got: %s", callReply)
	}
}

func TestEvalTypeAndIntrospectionHelpers(t *testing.T) {
	ctx := &dispatch.Context{}

	// 1. type() and typeOf()
	tNum, err := executeJavaScriptEval(ctx, "return type(123)")
	if err != nil || tNum != "int64" {
		t.Errorf("expected int64, got %q (err=%v)", tNum, err)
	}

	tStr, err := executeJavaScriptEval(ctx, "return type('hello world')")
	if err != nil || tStr != "string" {
		t.Errorf("expected string, got %q (err=%v)", tStr, err)
	}

	// 2. returnType() of a function
	retType, err := executeJavaScriptEval(ctx, "return returnType(client.parseJID)")
	if err != nil || retType == "" {
		t.Errorf("expected returnType string, got %q (err=%v)", retType, err)
	}

	// 3. methods() on client
	mList, err := executeJavaScriptEval(ctx, "return methods(client)")
	if err != nil || !strings.Contains(mList, "parseJID") {
		t.Errorf("expected methods(client) to contain parseJID, got %q (err=%v)", mList, err)
	}

	// 4. props() on an object
	pList, err := executeJavaScriptEval(ctx, "return props({ foo: 1, bar: 'baz' })")
	if err != nil || !strings.Contains(pList, "foo") || !strings.Contains(pList, "bar") {
		t.Errorf("expected props to contain foo and bar, got %q (err=%v)", pList, err)
	}

	// 5. inspect()
	insp, err := executeJavaScriptEval(ctx, "return inspect(client.parseJID)")
	if err != nil || !strings.Contains(insp, "parseJID") {
		t.Errorf("expected inspect to mention parseJID, got %q (err=%v)", insp, err)
	}
}

func TestEvalExtendedTools(t *testing.T) {
	ctx := &dispatch.Context{}

	// 1. Crypto utilities
	sha, err := executeJavaScriptEval(ctx, "return crypto.sha256('hello')")
	if err != nil || sha != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Errorf("crypto.sha256 mismatch: got %q (err=%v)", sha, err)
	}

	md5Val, err := executeJavaScriptEval(ctx, "return crypto.md5('hello')")
	if err != nil || md5Val != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("crypto.md5 mismatch: got %q (err=%v)", md5Val, err)
	}

	randBytes, err := executeJavaScriptEval(ctx, "return crypto.randomBytes(8).length")
	if err != nil || randBytes != "8" {
		t.Errorf("crypto.randomBytes mismatch: got %q (err=%v)", randBytes, err)
	}

	// 2. Base64 & Hex
	b64, err := executeJavaScriptEval(ctx, "return base64.encode('whatsrook')")
	if err != nil || b64 != "d2hhdHNyb29r" {
		t.Errorf("base64.encode mismatch: got %q (err=%v)", b64, err)
	}

	b64Dec, err := executeJavaScriptEval(ctx, "return base64.decode('d2hhdHNyb29r')")
	if err != nil || b64Dec != "whatsrook" {
		t.Errorf("base64.decode mismatch: got %q (err=%v)", b64Dec, err)
	}

	hx, err := executeJavaScriptEval(ctx, "return hex.encode('abc')")
	if err != nil || hx != "616263" {
		t.Errorf("hex.encode mismatch: got %q (err=%v)", hx, err)
	}

	// 3. System & Runtime
	osPlat, err := executeJavaScriptEval(ctx, "return os.platform()")
	if err != nil || osPlat == "" {
		t.Errorf("os.platform failed: got %q (err=%v)", osPlat, err)
	}

	numCpu, err := executeJavaScriptEval(ctx, "return runtime.numCPU() > 0")
	if err != nil || numCpu != "true" {
		t.Errorf("runtime.numCPU failed: got %q (err=%v)", numCpu, err)
	}

	// 4. File system
	tmpFile := filepath.Join(os.TempDir(), "whatsrook_eval_test.txt")
	defer os.Remove(tmpFile)

	codeFs := `
		fs.writeFile('` + tmpFile + `', 'hello from eval');
		let exists = fs.exists('` + tmpFile + `');
		let content = fs.readFile('` + tmpFile + `');
		fs.unlink('` + tmpFile + `');
		let existsAfter = fs.exists('` + tmpFile + `');
		return JSON.stringify({ exists: exists, content: content, existsAfter: existsAfter });
	`
	fsRes, err := executeJavaScriptEval(ctx, codeFs)
	if err != nil {
		t.Fatalf("fs execution failed: %v", err)
	}
	if !strings.Contains(fsRes, `"exists":true`) || !strings.Contains(fsRes, `"content":"hello from eval"`) || !strings.Contains(fsRes, `"existsAfter":false`) {
		t.Errorf("unexpected fs result: %q", fsRes)
	}

	// 5. Protobuf builders
	locProto, err := executeJavaScriptEval(ctx, "let loc = proto.location(37.7749, -122.4194, 'San Francisco'); return proto.toJSON(loc)")
	if err != nil || !strings.Contains(locProto, "San Francisco") {
		t.Errorf("proto.location failed: got %q (err=%v)", locProto, err)
	}

	contactProto, err := executeJavaScriptEval(ctx, "let c = proto.contact('Alice', '12345678'); return proto.toJSON(c)")
	if err != nil || !strings.Contains(contactProto, "Alice") {
		t.Errorf("proto.contact failed: got %q (err=%v)", contactProto, err)
	}

	pollProto, err := executeJavaScriptEval(ctx, "let p = proto.poll('Choose:', ['A', 'B']); return proto.toJSON(p)")
	if err != nil || !strings.Contains(pollProto, "Choose:") {
		t.Errorf("proto.poll failed: got %q (err=%v)", pollProto, err)
	}

	// 6. Subprocess execution
	shRes, err := executeJavaScriptEval(ctx, "return exec('echo', ['hello_eval']).stdout.trim()")
	if err != nil || shRes != "hello_eval" {
		t.Errorf("exec failed: got %q (err=%v)", shRes, err)
	}
}
