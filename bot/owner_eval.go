// eval.go provides dynamic runtime code evaluation for bot owners and sudoers.
// It supports both JavaScript (via Goja) and Grol (via grol.io/grol) with live
// access to the incoming message payload, WhatsApp client, and execution context.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/thruqe/whatsrook"
)

const (
	maxEvalTimeout = 60 * time.Second
)

var (
	reEvalTemplateURL = regexp.MustCompile(`(?m)(:\s*|=\s*|return\s+|\(\s*)(https?://\$\{[^\n;,\)]+\})`)
	reEvalTemplateStr = regexp.MustCompile(`(?m)(:\s*|=\s*|return\s+|\(\s*)([A-Za-z0-9_-]+\s+\$\{[^}]+\})`)
	reEvalUnquotedIP  = regexp.MustCompile(`([\(,:\s\[=])([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})([,\)\s;\]])`)
)

// preprocessEvalCode normalizes common mobile typing quirks, fixes unquoted IPs and URLs,
// strips markdown code fences, and prepares code for JavaScript execution.
func preprocessEvalCode(code string) string {
	c := strings.TrimSpace(code)
	c = strings.TrimPrefix(c, "$")
	c = strings.TrimPrefix(c, ">")
	c = strings.TrimSpace(c)

	if strings.HasPrefix(c, "```") {
		lines := strings.Split(c, "\n")
		if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			c = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}

	c = reEvalTemplateURL.ReplaceAllString(c, "${1}`${2}`")
	c = reEvalTemplateStr.ReplaceAllString(c, "${1}`${2}`")
	c = reEvalUnquotedIP.ReplaceAllString(c, `${1}"${2}"${3}`)
	c = reEvalUnquotedIP.ReplaceAllString(c, `${1}"${2}"${3}`)
	return c
}

func init() {
	RegisterPreInterceptor("owner_eval", HandleOwnerEvalIntercept)
	Register(&Command{
		Name:        "eval",
		Alias:       "ev",
		Description: "Evaluate JavaScript or Grol code with live bot context (Owner only)",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleEvalCommand,
	})
}

// HandleOwnerEvalIntercept intercepts messages starting with $, >, =>, $gr, or $$ for owner/sudoers.
func HandleOwnerEvalIntercept(c *Context, text string) bool {
	if c == nil || text == "" {
		return false
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	// Check if the message matches an eval trigger
	isGrol := false
	var code string

	switch {
	case strings.HasPrefix(trimmed, "$gr ") || strings.HasPrefix(trimmed, ">gr "):
		isGrol = true
		code = strings.TrimSpace(trimmed[4:])
	case strings.HasPrefix(trimmed, "$$"):
		isGrol = true
		code = strings.TrimSpace(trimmed[2:])
	case strings.HasPrefix(trimmed, "=>"):
		code = strings.TrimSpace(trimmed[2:])
	case strings.HasPrefix(trimmed, "$"):
		code = strings.TrimSpace(trimmed[1:])
	case strings.HasPrefix(trimmed, ">"):
		code = strings.TrimSpace(trimmed[1:])
	default:
		return false
	}

	// Only owner or sudoers can execute dynamic evaluation
	if !c.IsOwner() && !c.IsSudo() {
		return false
	}

	// Fallback to quoted message if code is empty
	if code == "" {
		code = extractQuotedEvalCode(c)
	}

	if code == "" {
		_ = c.Reply("*Owner Eval*\n\n" +
			"*Usage:*\n" +
			"• `$ <javascript>` or `> <javascript>`\n" +
			"• `$gr <grol>` or `$$ <grol>`\n" +
			"• Example: `$ return message`\n" +
			"• Example: `$ return client.user`\n" +
			"• Example: `$ reply(\"Hello!\"); return 42`")
		return true
	}

	executeAndReplyEval(c, code, isGrol)
	return true
}

func handleEvalCommand(ctx *Context) error {
	raw := strings.TrimSpace(ctx.RawArgs)
	isGrol := false

	if strings.HasPrefix(raw, "-gr ") || strings.HasPrefix(raw, "--grol ") {
		isGrol = true
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(raw, "-gr "), "--grol "))
	} else if strings.HasPrefix(raw, "-js ") || strings.HasPrefix(raw, "--js ") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(raw, "-js "), "--js "))
	}

	if raw == "" {
		raw = extractQuotedEvalCode(ctx)
	}

	if raw == "" {
		return ctx.Replyf("Usage: %seval <javascript code> or %seval -gr <grol code>", ctx.GetPrefix(), ctx.GetPrefix())
	}

	executeAndReplyEval(ctx, raw, isGrol)
	return nil
}

// EvalResult contains the result, output, and full type and reflection metadata of an eval run.
type EvalResult struct {
	Value       string
	Type        string
	ReturnType  string
	ResultType  string
	CalledFunc  string
	FuncType    string
	FuncParams  []string
	IsFunction  bool
	ConsoleLogs string
}

// FormatForReply formats the evaluation result into a Markdown reply with type, return type,
// and function metadata for the WhatsApp "$" command.
func (r *EvalResult) FormatForReply() string {
	var sb strings.Builder

	if r.ConsoleLogs != "" {
		sb.WriteString("*Console Log:*\n```\n")
		sb.WriteString(r.ConsoleLogs)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("*Eval Output:*\n")

	if r.IsFunction {
		if r.CalledFunc != "" {
			sb.WriteString(fmt.Sprintf("• *Function:* `%s`\n", r.CalledFunc))
		}
		if r.FuncType != "" {
			sb.WriteString(fmt.Sprintf("• *Type:* `%s`\n", r.FuncType))
		}
		if r.ReturnType != "" {
			sb.WriteString(fmt.Sprintf("• *Return Type:* `%s`\n", r.ReturnType))
		}
		if len(r.FuncParams) > 0 {
			sb.WriteString(fmt.Sprintf("• *Parameters:* `(%s)`\n", strings.Join(r.FuncParams, ", ")))
		}
		return strings.TrimSpace(sb.String())
	}

	if r.CalledFunc != "" {
		sb.WriteString(fmt.Sprintf("• *Function:* `%s`\n", r.CalledFunc))
		if r.FuncType != "" {
			sb.WriteString(fmt.Sprintf("• *Function Type:* `%s`\n", r.FuncType))
		}
		if r.ReturnType != "" {
			sb.WriteString(fmt.Sprintf("• *Return Type:* `%s`\n", r.ReturnType))
		}
		if r.ResultType != "" {
			sb.WriteString(fmt.Sprintf("• *Result Type:* `%s`\n", r.ResultType))
		}
	} else if r.ResultType != "" && r.ResultType != "nil" {
		sb.WriteString(fmt.Sprintf("• *Type:* `%s`\n", r.ResultType))
	}

	val := r.Value
	if val == "" {
		sb.WriteString("0")
	} else {
		sb.WriteString(fmt.Sprintf("```\n%s\n```", val))
	}

	return strings.TrimSpace(sb.String())
}

// replyEvalResult sends the evaluation output. If the response fits within WhatsApp's
// message limit (<= 55,000 runes), it is sent directly in full. If it exceeds message size limits,
// the full, un-truncated output is delivered as a document file so 100% of the content is preserved.
func replyEvalResult(ctx *Context, replyText, rawData string) {
	if len([]rune(replyText)) <= 55000 {
		_ = ctx.Reply(replyText)
		return
	}

	filename := "eval_output.txt"
	trimmed := strings.TrimSpace(rawData)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		filename = "eval_output.json"
	}

	payload := []byte(rawData)
	if len(payload) == 0 {
		payload = []byte(replyText)
	}

	caption := "*Eval Output:* `(Full output attached as document)`"
	if err := ctx.ReplyWithDocument(payload, "text/plain; charset=utf-8", filename, caption); err != nil {
		chunkRunes := 45000
		allRunes := []rune(replyText)
		for i := 0; i < len(allRunes); i += chunkRunes {
			end := min(i+chunkRunes, len(allRunes))
			_ = ctx.Reply(string(allRunes[i:end]))
		}
	}
}

func executeAndReplyEval(ctx *Context, code string, isGrol bool) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Error("panic in eval execution", "panic", r)
				_ = ctx.Replyf("*Eval Panic:*\n```\n%v\n```", r)
			}
		}()

		if isGrol {
			result, err := executeGrolEval(ctx, code)
			if err != nil {
				_ = ctx.Replyf("*Eval Error:*\n```\n%v\n```", err)
				return
			}
			if result == "" {
				_ = ctx.Reply("*Eval Output:* 0")
				return
			}
			replyText := fmt.Sprintf("*Eval Output:*\n```\n%s\n```", result)
			replyEvalResult(ctx, replyText, result)
			return
		}

		res, err := executeJavaScriptEvalDetailed(ctx, code)
		if err != nil {
			_ = ctx.Replyf("*Eval Error:*\n```\n%v\n```", err)
			return
		}

		replyText := res.FormatForReply()
		replyEvalResult(ctx, replyText, res.Value)
	}()
}

// executeJavaScriptEvalDetailed executes code and captures full type, return type, function metadata, and console logs.
func executeJavaScriptEvalDetailed(ctx *Context, code string) (*EvalResult, error) {
	vm := goja.New()
	tracker := NewEvalTracker()

	// Set execution deadline
	timer := time.AfterFunc(maxEvalTimeout, func() {
		vm.Interrupt(fmt.Sprintf("execution timeout exceeded (%v)", maxEvalTimeout))
	})
	defer timer.Stop()

	var logBuf bytes.Buffer
	logFn := func(call goja.FunctionCall) goja.Value {
		for i, arg := range call.Arguments {
			if i > 0 {
				logBuf.WriteString(" ")
			}
			logBuf.WriteString(arg.String())
		}
		logBuf.WriteString("\n")
		return goja.Undefined()
	}

	// Console object
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", logFn)
	_ = consoleObj.Set("info", logFn)
	_ = consoleObj.Set("warn", logFn)
	_ = consoleObj.Set("error", logFn)
	_ = vm.Set("console", consoleObj)
	_ = vm.Set("print", logFn)

	// Built-in reply helper with metadata
	tagAndSet(vm, vm.GlobalObject(), "reply", FunctionMetadata{
		Name:       "reply",
		Signature:  "(msg string)",
		Type:       "func(msg string) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			msg := call.Arguments[0].String()
			_ = ctx.Reply(msg)
		}
		return vm.ToValue(true)
	}, tracker)

	// JSON formatter helper with metadata
	tagAndSet(vm, vm.GlobalObject(), "json", FunctionMetadata{
		Name:       "json",
		Signature:  "(val any)",
		Type:       "func(val any) string",
		ReturnType: "string",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		exported := call.Arguments[0].Export()
		data, err := json.MarshalIndent(exported, "", "  ")
		if err != nil {
			return vm.ToValue(fmt.Sprintf("<json error: %v>", err))
		}
		return vm.ToValue(string(data))
	}, tracker)

	// Sleep helper with metadata
	tagAndSet(vm, vm.GlobalObject(), "sleep", FunctionMetadata{
		Name:       "sleep",
		Signature:  "(ms number)",
		Type:       "func(ms number) void",
		ReturnType: "void",
		ParamTypes: []string{"number"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			ms := call.Arguments[0].ToInteger()
			if ms > 0 && ms <= 10000 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		return goja.Undefined()
	}, tracker)

	// Build message and client objects
	msgMap := buildEvalMessageMap(ctx)
	_ = vm.Set("message", msgMap)
	_ = vm.Set("m", msgMap)

	clientObj, err := SetupEvalJavaScriptEnvironment(vm, ctx, tracker)
	if err == nil && clientObj != nil {
		_ = vm.Set("client", clientObj)
		_ = vm.Set("conn", clientObj)
	} else {
		clientMap := buildEvalClientMap(ctx)
		_ = vm.Set("client", clientMap)
		_ = vm.Set("conn", clientMap)
	}
	SetupGlobalConstants(vm, ctx, tracker)
	_ = vm.Set("ctx", ctx)

	cleanedCode := preprocessEvalCode(code)
	val, err := vm.RunString(cleanedCode)
	if err != nil && strings.Contains(err.Error(), "Illegal return statement") {
		// Bare return statement detected: execute wrapped in an IIFE function
		val, err = vm.RunString("(function() {\n" + cleanedCode + "\n})()")
	}
	if err != nil {
		return nil, err
	}

	evalRes := &EvalResult{
		ConsoleLogs: strings.TrimSpace(logBuf.String()),
	}

	// Check if evaluated value is a function
	if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		if meta := ExtractFunctionMetadata(val); meta != nil && (meta.Type != "" || meta.Name != "") {
			evalRes.IsFunction = true
			evalRes.CalledFunc = meta.Name
			evalRes.FuncType = meta.Type
			evalRes.ReturnType = meta.ReturnType
			evalRes.FuncParams = meta.ParamTypes
			evalRes.Value = meta.Type
			return evalRes, nil
		}
	}

	// Check if a function was invoked during execution
	lastCall := tracker.LastCall()
	if lastCall != nil {
		evalRes.CalledFunc = lastCall.Name
		evalRes.FuncType = lastCall.Type
		evalRes.ReturnType = lastCall.ReturnType
		evalRes.ResultType = lastCall.ResultType
	}

	if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		exported := val.Export()
		if evalRes.ResultType == "" {
			evalRes.ResultType = GetValueTypeName(exported)
		}

		var resStr string
		switch v := exported.(type) {
		case string:
			resStr = v
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			resStr = fmt.Sprintf("%v", v)
		default:
			if jsonBytes, errJson := json.MarshalIndent(v, "", "  "); errJson == nil {
				resStr = string(jsonBytes)
			} else {
				resStr = fmt.Sprintf("%+v", v)
			}
		}
		evalRes.Value = resStr
	}

	return evalRes, nil
}

// executeGrolEval executes code in the Grol runtime with message and client metadata.
func executeGrolEval(ctx *Context, code string) (string, error) {
	return RunGrolScript(ctx, code)
}

func buildEvalMessageMap(c *Context) map[string]any {
	m := map[string]any{
		"chat":      c.Chat.String(),
		"sender":    c.Sender.String(),
		"isGroup":   c.IsGroup(),
		"isOwner":   c.IsOwner(),
		"isSudo":    c.IsSudo(),
		"command":   c.Command,
		"args":      c.Args,
		"rawArgs":   c.RawArgs,
		"timestamp": time.Now().Unix(),
	}

	// Direct message action shortcuts
	m["reply"] = func(text string) error {
		return c.Reply(text)
	}
	m["react"] = func(emoji string) error {
		if c.Evt == nil || c.Client == nil {
			return nil
		}
		reaction := c.Client.BuildReaction(c.Chat, c.Sender, c.Evt.Info.ID, emoji)
		_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reaction)
		return err
	}
	m["delete"] = func() error {
		if c.Evt == nil || c.Client == nil {
			return nil
		}
		_, err := whatsrook.SendDelete(c.GetSendContext(), c.Client, c.Chat, c.Evt.Info.ID)
		return err
	}
	m["download"] = func() (map[string]any, error) {
		if c.Evt == nil || c.Evt.Message == nil || c.Client == nil {
			return nil, fmt.Errorf("no message to download")
		}
		data, mime, err := whatsrook.Download(c.GetSendContext(), c.Client, c.Evt.Message)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"data":     data,
			"mimetype": mime,
			"size":     len(data),
			"base64":   base64.StdEncoding.EncodeToString(data),
		}, nil
	}

	if c.Evt != nil {
		m["id"] = c.Evt.Info.ID
		m["isFromMe"] = c.Evt.Info.IsFromMe
		m["pushName"] = c.Evt.Info.PushName
		m["type"] = c.Evt.Info.Type
		m["timestamp"] = c.Evt.Info.Timestamp.Unix()
		m["text"] = whatsrook.ExtractMessageText(c.Evt)
		m["raw"] = c.Evt
	}

	if quoted := c.GetQuotedMessage(); quoted != nil {
		quotedMap := map[string]any{
			"text": extractTextFromMessage(quoted),
			"raw":  quoted,
		}
		if sender, ok := c.GetQuotedSender(); ok {
			quotedMap["sender"] = sender.String()
		}
		quotedMap["reply"] = func(text string) error {
			return c.Reply(text)
		}
		quotedMap["download"] = func() (map[string]any, error) {
			if c.Client == nil {
				return nil, fmt.Errorf("client unavailable")
			}
			data, mime, err := whatsrook.Download(c.GetSendContext(), c.Client, quoted)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"data":     data,
				"mimetype": mime,
				"size":     len(data),
				"base64":   base64.StdEncoding.EncodeToString(data),
			}, nil
		}
		m["quoted"] = quotedMap
	}

	return m
}

func buildEvalClientMap(c *Context) map[string]any {
	m := map[string]any{
		"raw": c.Client,
		"user": func() string {
			if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
				return c.Client.Store.ID.ToNonAD().String()
			}
			return ""
		}(),
		"reply": func(text string) error {
			return c.Reply(text)
		},
		"send": func(target string, text string) error {
			jid, err := types.ParseJID(target)
			if err != nil {
				return err
			}
			_, err = c.Client.SendMessage(c.GetSendContext(), jid, &waE2E.Message{Conversation: &text})
			return err
		},
		"react": func(emoji string) error {
			if c.Evt == nil {
				return nil
			}
			reaction := c.Client.BuildReaction(c.Chat, c.Sender, c.Evt.Info.ID, emoji)
			_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reaction)
			return err
		},
	}
	return m
}

func extractQuotedEvalCode(ctx *Context) string {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ""
	}
	return strings.TrimSpace(extractTextFromMessage(quoted))
}

func extractTextFromMessage(msg *waE2E.Message) string {
	msg = whatsrook.UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	switch {
	case msg.GetConversation() != "":
		return msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetCaption()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}
