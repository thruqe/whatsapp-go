// eval.go provides dynamic runtime code evaluation for bot owners and sudoers.
// It supports both JavaScript (via Goja) and Grol (via grol.io/grol) with live
// access to the incoming message payload, WhatsApp client, and execution context.
package owner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"grol.io/grol/eval"
	"grol.io/grol/repl"

	"whatsrook"
	"whatsrook/cmd/dispatch"
	"whatsrook/util/logger"
)

const (
	maxEvalTimeout     = 10 * time.Second
	maxEvalOutputChars = 3500
)

func init() {
	dispatch.RegisterPreInterceptor("owner_eval", HandleOwnerEvalIntercept)
	dispatch.Register(&dispatch.Command{
		Name:        "eval",
		Alias:       "ev",
		Description: "Evaluate JavaScript or Grol code with live bot context (Owner only)",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleEvalCommand,
	})
}

// HandleOwnerEvalIntercept intercepts messages starting with $, >, =>, $gr, or $$ for owner/sudoers.
func HandleOwnerEvalIntercept(c *dispatch.Context, text string) bool {
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
		_ = c.Reply("*Owner Eval (Baileys-style)*\n\n" +
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

func handleEvalCommand(ctx *dispatch.Context) error {
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

func executeAndReplyEval(ctx *dispatch.Context, code string, isGrol bool) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in eval execution", "panic", r)
				_ = ctx.Replyf("*Eval Panic:*\n```\n%v\n```", r)
			}
		}()

		var result string
		var err error

		if isGrol {
			result, err = executeGrolEval(ctx, code)
		} else {
			result, err = executeJavaScriptEval(ctx, code)
		}

		if err != nil {
			_ = ctx.Replyf("*Eval Error:*\n```\n%v\n```", err)
			return
		}

		if result == "" {
			_ = ctx.Reply("*Eval Output:* `(executed successfully, undefined)`")
			return
		}

		if runes := []rune(result); len(runes) > maxEvalOutputChars {
			result = string(runes[:maxEvalOutputChars]) + "\n... (truncated)"
		}

		_ = ctx.Replyf("*Eval Output:*\n```\n%s\n```", result)
	}()
}

// executeJavaScriptEval executes code in a sandboxed Goja JavaScript runtime.
func executeJavaScriptEval(ctx *dispatch.Context, code string) (string, error) {
	vm := goja.New()

	// Set execution deadline
	timer := time.AfterFunc(maxEvalTimeout, func() {
		vm.Interrupt("execution timeout exceeded (10s)")
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

	// Built-in reply helper
	_ = vm.Set("reply", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			msg := call.Arguments[0].String()
			_ = ctx.Reply(msg)
		}
		return goja.Undefined()
	})

	// JSON formatter helper
	_ = vm.Set("json", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		exported := call.Arguments[0].Export()
		data, err := json.MarshalIndent(exported, "", "  ")
		if err != nil {
			return vm.ToValue(fmt.Sprintf("<json error: %v>", err))
		}
		return vm.ToValue(string(data))
	})

	// Sleep helper
	_ = vm.Set("sleep", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			ms := call.Arguments[0].ToInteger()
			if ms > 0 && ms <= 10000 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		return goja.Undefined()
	})

	// Build message and client objects
	msgMap := buildEvalMessageMap(ctx)
	_ = vm.Set("message", msgMap)
	_ = vm.Set("m", msgMap)

	clientMap := buildEvalClientMap(ctx)
	_ = vm.Set("client", clientMap)
	_ = vm.Set("conn", clientMap)
	_ = vm.Set("ctx", ctx)

	trimmedCode := strings.TrimSpace(code)
	val, err := vm.RunString(trimmedCode)
	if err != nil && strings.Contains(err.Error(), "Illegal return statement") {
		// Bare return statement detected: execute wrapped in an IIFE function
		val, err = vm.RunString("(function() {\n" + trimmedCode + "\n})()")
	}
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(logBuf.String())
	if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		exported := val.Export()
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

		if output != "" {
			return output + "\n" + resStr, nil
		}
		return resStr, nil
	}

	return output, nil
}

// executeGrolEval executes code in the Grol runtime with message and client metadata.
func executeGrolEval(ctx *dispatch.Context, code string) (string, error) {
	evalCtx, cancel := context.WithTimeout(ctx.GetSendContext(), maxEvalTimeout)
	defer cancel()

	s := eval.NewState()
	outBuf := &strings.Builder{}
	s.Out = outBuf
	s.LogOut = outBuf
	s.NoLog = true

	opts := repl.EvalStringOptions()
	opts.MaxDepth = eval.DefaultMaxDepth - 1
	opts.MaxDuration = maxEvalTimeout

	_, panicked, errs, _ := repl.EvalOne(evalCtx, s, code, outBuf, opts)
	if panicked && len(errs) == 0 {
		return "", fmt.Errorf("execution panicked")
	}
	if len(errs) > 0 {
		return "", fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	return strings.TrimSpace(outBuf.String()), nil
}

func buildEvalMessageMap(c *dispatch.Context) map[string]any {
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
		m["quoted"] = quotedMap
	}

	return m
}

func buildEvalClientMap(c *dispatch.Context) map[string]any {
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

func extractQuotedEvalCode(ctx *dispatch.Context) string {
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
