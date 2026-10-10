// client_proxy.go provides a dynamic reflection proxy and Baileys-compatible
// bridge for the whatsmeow.Client instance, exposing all client methods
// with automatic argument conversion to Go equivalents.
package owner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dop251/goja"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"whatsrook/cmd/dispatch"
	"whatsrook/util/message"
	"whatsrook/util/send"
)

// SetupEvalJavaScriptEnvironment registers Baileys-style client proxies, protobuf builders,
// utility globals, and reflection methods into the Goja runtime.
func SetupEvalJavaScriptEnvironment(vm *goja.Runtime, ctx *dispatch.Context) (*goja.Object, error) {
	clientObj := vm.NewObject()

	// 1. Core metadata properties
	_ = clientObj.Set("raw", ctx.Client)
	_ = clientObj.Set("user", func() string {
		if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
			return ctx.Client.Store.ID.ToNonAD().String()
		}
		return ""
	}())
	_ = clientObj.Set("isConnected", func() bool {
		return ctx.Client != nil && ctx.Client.IsConnected()
	})
	_ = clientObj.Set("isLoggedIn", func() bool {
		return ctx.Client != nil && ctx.Client.IsLoggedIn()
	})

	// 2. High-level messaging shortcuts
	_ = clientObj.Set("reply", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		text := call.Arguments[0].String()
		_ = ctx.Reply(text)
		return vm.ToValue(true)
	})

	_ = clientObj.Set("react", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || ctx.Evt == nil {
			return goja.Undefined()
		}
		emoji := call.Arguments[0].String()
		reaction := ctx.Client.BuildReaction(ctx.Chat, ctx.Sender, ctx.Evt.Info.ID, emoji)
		_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, reaction)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendText", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendText requires (to, text)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		text := call.Arguments[1].String()
		err = send.Text(ctx.GetSendContext(), ctx.Client, toJID, text)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendImage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendImage requires (to, dataOrUrl, [caption])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		data, mime, err := resolveMediaData(call.Arguments[1].Export(), "image/jpeg")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		caption := ""
		if len(call.Arguments) > 2 {
			caption = call.Arguments[2].String()
		}
		err = send.Image(ctx.GetSendContext(), ctx.Client, toJID, data, mime, caption)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendVideo", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendVideo requires (to, dataOrUrl, [caption])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		data, mime, err := resolveMediaData(call.Arguments[1].Export(), "video/mp4")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		caption := ""
		if len(call.Arguments) > 2 {
			caption = call.Arguments[2].String()
		}
		err = send.Video(ctx.GetSendContext(), ctx.Client, toJID, data, mime, caption)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendAudio", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendAudio requires (to, dataOrUrl, [ptt])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		ptt := false
		if len(call.Arguments) > 2 {
			ptt = call.Arguments[2].ToBoolean()
		}
		defaultMime := "audio/mp4"
		if ptt {
			defaultMime = "audio/ogg; codecs=opus"
		}
		data, mime, err := resolveMediaData(call.Arguments[1].Export(), defaultMime)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		err = send.Audio(ctx.GetSendContext(), ctx.Client, toJID, data, mime)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendSticker", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendSticker requires (to, dataOrUrl)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		data, _, err := resolveMediaData(call.Arguments[1].Export(), "image/webp")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		err = send.Sticker(ctx.GetSendContext(), ctx.Client, toJID, data, ctx.GetStickerPack(), ctx.GetStickerAuthor())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("sendReaction", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendReaction requires (to, messageId, emoji)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		msgID := call.Arguments[1].String()
		emoji := call.Arguments[2].String()
		err = send.ReactMessage(ctx.GetSendContext(), ctx.Client, toJID, msgID, emoji)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = clientObj.Set("edit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("edit requires (messageId, newContent, [chatJid])"))
		}
		targetChat := ctx.Chat
		if len(call.Arguments) > 2 {
			if parsed, err := resolveJID(call.Arguments[2].Export(), ctx); err == nil {
				targetChat = parsed
			}
		}
		msgID := types.MessageID(call.Arguments[0].String())
		content := call.Arguments[1].Export()
		resp, err := send.Edit(ctx.GetSendContext(), ctx.Client, targetChat, msgID, content)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	})

	_ = clientObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("delete requires (messageId, [chatJid])"))
		}
		targetChat := ctx.Chat
		if len(call.Arguments) > 1 {
			if parsed, err := resolveJID(call.Arguments[1].Export(), ctx); err == nil {
				targetChat = parsed
			}
		}
		msgID := types.MessageID(call.Arguments[0].String())
		resp, err := send.Delete(ctx.GetSendContext(), ctx.Client, targetChat, msgID)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	})

	_ = clientObj.Set("download", func(call goja.FunctionCall) goja.Value {
		targetMsg := ctx.Evt.Message
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) && !goja.IsNull(call.Arguments[0]) {
			exported := call.Arguments[0].Export()
			if qMsg, ok := exported.(*waE2E.Message); ok {
				targetMsg = qMsg
			} else if qMap, ok := exported.(map[string]any); ok {
				if converted, err := ConvertToProtoMessage(qMap, ctx); err == nil {
					targetMsg = converted
				}
			}
		} else if quoted := ctx.GetQuotedMessage(); quoted != nil {
			targetMsg = quoted
		}
		if targetMsg == nil {
			panic(vm.ToValue("no message available to download"))
		}
		data, mime, err := message.Download(ctx.GetSendContext(), ctx.Client, targetMsg)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		resObj := vm.NewObject()
		_ = resObj.Set("data", vm.ToValue(data))
		_ = resObj.Set("mimetype", mime)
		_ = resObj.Set("size", len(data))
		_ = resObj.Set("base64", base64.StdEncoding.EncodeToString(data))
		return resObj
	})

	_ = clientObj.Set("upload", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("upload requires (dataOrUrl, [mediaType])"))
		}
		mediaType := whatsmeow.MediaImage
		if len(call.Arguments) > 1 {
			tStr := strings.ToLower(call.Arguments[1].String())
			switch tStr {
			case "video":
				mediaType = whatsmeow.MediaVideo
			case "audio":
				mediaType = whatsmeow.MediaAudio
			case "document", "doc":
				mediaType = whatsmeow.MediaDocument
			}
		}
		data, _, err := resolveMediaData(call.Arguments[0].Export(), "application/octet-stream")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		upResp, err := ctx.Client.Upload(ctx.GetSendContext(), data, mediaType)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{
			"url":           upResp.URL,
			"directPath":    upResp.DirectPath,
			"mediaKey":      upResp.MediaKey,
			"fileLength":    upResp.FileLength,
			"fileSha256":    upResp.FileSHA256,
			"fileEncSha256": upResp.FileEncSHA256,
		})
	})

	_ = clientObj.Set("parseJID", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		parsed, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(parsed.String())
	})

	_ = clientObj.Set("buildMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	})

	_ = clientObj.Set("toProto", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	})

	// 3. Register dynamic reflection proxy for ALL exported methods of whatsmeow.Client
	registerReflectedClientMethods(vm, clientObj, ctx)

	// Aliases
	_ = clientObj.Set("send", clientObj.Get("sendMessage"))

	return clientObj, nil
}

// registerReflectedClientMethods registers every exported method on whatsmeow.Client into clientObj.
func registerReflectedClientMethods(vm *goja.Runtime, clientObj *goja.Object, ctx *dispatch.Context) {
	if ctx.Client == nil {
		return
	}

	cliVal := reflect.ValueOf(ctx.Client)
	cliType := cliVal.Type()

	for i := 0; i < cliType.NumMethod(); i++ {
		method := cliType.Method(i)
		methodName := method.Name

		// Skip unexported methods
		if !method.IsExported() {
			continue
		}

		mVal := cliVal.Method(i)
		mType := mVal.Type()

		fn := createMethodWrapper(vm, mVal, mType, ctx)

		// Set PascalCase name (e.g. SendMessage, GetUserInfo)
		_ = clientObj.Set(methodName, fn)

		// Set camelCase name (e.g. sendMessage, getUserInfo)
		lowerName := strings.ToLower(methodName[:1]) + methodName[1:]
		if lowerName != methodName {
			_ = clientObj.Set(lowerName, fn)
		}
	}
}

// createMethodWrapper wraps an arbitrary Go method into a Goja function with automatic parameter coercion.
func createMethodWrapper(vm *goja.Runtime, mVal reflect.Value, mType reflect.Type, ctx *dispatch.Context) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		defer func() {
			if r := recover(); r != nil {
				// Re-throw panic with context
				if ge, ok := r.(*goja.Exception); ok {
					panic(ge)
				}
				panic(vm.ToValue(fmt.Sprintf("%v", r)))
			}
		}()

		numIn := mType.NumIn()
		isVariadic := mType.IsVariadic()

		var inArgs []reflect.Value
		jsArgIdx := 0

		// Context auto-injection: if method's 1st param is context.Context
		firstIsContext := numIn > 0 && mType.In(0) == reflect.TypeOf((*context.Context)(nil)).Elem()
		if firstIsContext {
			// Check if caller supplied their own context
			if len(call.Arguments) > 0 && isContextArg(call.Arguments[0]) {
				inArgs = append(inArgs, reflect.ValueOf(call.Arguments[0].Export().(context.Context)))
				jsArgIdx++
			} else {
				// Automatically supply execution send context
				inArgs = append(inArgs, reflect.ValueOf(ctx.GetSendContext()))
			}
		}

		// Convert required parameters
		reqCount := numIn
		if isVariadic {
			reqCount = numIn - 1
		}

		for paramIdx := len(inArgs); paramIdx < reqCount; paramIdx++ {
			expectedType := mType.In(paramIdx)
			var jsVal goja.Value
			if jsArgIdx < len(call.Arguments) {
				jsVal = call.Arguments[jsArgIdx]
				jsArgIdx++
			} else {
				jsVal = goja.Undefined()
			}

			converted, err := convertJSArgToGo(vm, jsVal, expectedType, ctx)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("argument %d conversion failed: %w", paramIdx, err)))
			}
			inArgs = append(inArgs, converted)
		}

		// Convert variadic parameters if present
		if isVariadic {
			varElemType := mType.In(numIn - 1).Elem()
			for jsArgIdx < len(call.Arguments) {
				converted, err := convertJSArgToGo(vm, call.Arguments[jsArgIdx], varElemType, ctx)
				if err != nil {
					panic(vm.NewGoError(fmt.Errorf("variadic argument conversion failed: %w", err)))
				}
				inArgs = append(inArgs, converted)
				jsArgIdx++
			}
		}

		// Invoke method
		var results []reflect.Value
		if isVariadic {
			results = mVal.CallSlice(packVariadicArgs(inArgs, numIn))
		} else {
			results = mVal.Call(inArgs)
		}

		// Process return values
		if len(results) == 0 {
			return goja.Undefined()
		}

		// Check if last return value is error
		lastResult := results[len(results)-1]
		if lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if !lastResult.IsNil() {
				err := lastResult.Interface().(error)
				panic(vm.NewGoError(err))
			}
			// If method returned only error (and error is nil), return true
			if len(results) == 1 {
				return vm.ToValue(true)
			}
			// Exclude the nil error from the results
			results = results[:len(results)-1]
		}

		// Return single non-error value
		if len(results) == 1 {
			val := results[0].Interface()
			if sr, ok := val.(whatsmeow.SendResponse); ok {
				return vm.ToValue(map[string]any{
					"id":        sr.ID,
					"timestamp": sr.Timestamp.Unix(),
				})
			}
			return vm.ToValue(val)
		}

		// Return multiple values as an array
		var arr []any
		for _, r := range results {
			arr = append(arr, r.Interface())
		}
		return vm.ToValue(arr)
	}
}

func packVariadicArgs(args []reflect.Value, numIn int) []reflect.Value {
	fixed := args[:numIn-1]
	variadic := args[numIn-1:]

	sliceType := reflect.SliceOf(variadic[0].Type())
	sliceVal := reflect.MakeSlice(sliceType, len(variadic), len(variadic))
	for i, v := range variadic {
		sliceVal.Index(i).Set(v)
	}
	return append(fixed, sliceVal)
}

func isContextArg(val goja.Value) bool {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return false
	}
	exported := val.Export()
	_, ok := exported.(context.Context)
	return ok
}

// convertJSArgToGo converts a single JavaScript argument to the expected Go reflect.Type.
func convertJSArgToGo(vm *goja.Runtime, jsVal goja.Value, targetType reflect.Type, ctx *dispatch.Context) (reflect.Value, error) {
	if jsVal == nil || goja.IsUndefined(jsVal) || goja.IsNull(jsVal) {
		return reflect.Zero(targetType), nil
	}

	exported := jsVal.Export()

	// 1. types.JID
	if targetType == reflect.TypeOf(types.JID{}) {
		jid, err := resolveJID(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(jid), nil
	}

	// 2. []types.JID
	if targetType == reflect.TypeOf([]types.JID{}) {
		jids, err := resolveJIDSlice(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(jids), nil
	}

	// 3. *waE2E.Message
	if targetType == reflect.TypeOf((*waE2E.Message)(nil)) {
		msg, err := ConvertToProtoMessage(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(msg), nil
	}

	// 4. Protobuf Message pointer (*waE2E.ContextInfo, etc.)
	if targetType.Implements(reflect.TypeOf((*proto.Message)(nil)).Elem()) {
		if targetType.Kind() == reflect.Ptr {
			elemType := targetType.Elem()
			newProto := reflect.New(elemType).Interface().(proto.Message)
			if m, ok := exported.(map[string]any); ok {
				jsonBytes, err := json.Marshal(m)
				if err == nil {
					if errProto := protojson.Unmarshal(jsonBytes, newProto); errProto == nil {
						return reflect.ValueOf(newProto), nil
					}
					if errJson := json.Unmarshal(jsonBytes, newProto); errJson == nil {
						return reflect.ValueOf(newProto), nil
					}
				}
			}
		}
	}

	// 5. types.Presence, ChatPresence, ChatPresenceMedia, MessageID
	if targetType == reflect.TypeOf(types.Presence("")) {
		return reflect.ValueOf(types.Presence(jsVal.String())), nil
	}
	if targetType == reflect.TypeOf(types.ChatPresence("")) {
		return reflect.ValueOf(types.ChatPresence(jsVal.String())), nil
	}
	if targetType == reflect.TypeOf(types.ChatPresenceMedia("")) {
		return reflect.ValueOf(types.ChatPresenceMedia(jsVal.String())), nil
	}
	if targetType == reflect.TypeOf(types.MessageID("")) {
		return reflect.ValueOf(types.MessageID(jsVal.String())), nil
	}

	// 6. []byte
	if targetType == reflect.TypeOf([]byte{}) {
		data, _, err := resolveMediaData(exported, "application/octet-stream")
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(data), nil
	}

	// 7. time.Duration
	if targetType == reflect.TypeOf(time.Duration(0)) {
		switch v := exported.(type) {
		case int64:
			return reflect.ValueOf(time.Duration(v) * time.Millisecond), nil
		case float64:
			return reflect.ValueOf(time.Duration(v) * time.Millisecond), nil
		case string:
			dur, err := time.ParseDuration(v)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(dur), nil
		}
	}

	// 8. time.Time
	if targetType == reflect.TypeOf(time.Time{}) {
		switch v := exported.(type) {
		case int64:
			return reflect.ValueOf(time.Unix(v, 0)), nil
		case float64:
			return reflect.ValueOf(time.Unix(int64(v), 0)), nil
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(t), nil
		}
	}

	// 9. Primitive types
	switch targetType.Kind() {
	case reflect.String:
		return reflect.ValueOf(jsVal.String()), nil
	case reflect.Bool:
		return reflect.ValueOf(jsVal.ToBoolean()), nil
	case reflect.Int:
		return reflect.ValueOf(int(jsVal.ToInteger())), nil
	case reflect.Int8:
		return reflect.ValueOf(int8(jsVal.ToInteger())), nil
	case reflect.Int16:
		return reflect.ValueOf(int16(jsVal.ToInteger())), nil
	case reflect.Int32:
		return reflect.ValueOf(int32(jsVal.ToInteger())), nil
	case reflect.Int64:
		return reflect.ValueOf(jsVal.ToInteger()), nil
	case reflect.Uint:
		return reflect.ValueOf(uint(jsVal.ToInteger())), nil
	case reflect.Uint8:
		return reflect.ValueOf(uint8(jsVal.ToInteger())), nil
	case reflect.Uint16:
		return reflect.ValueOf(uint16(jsVal.ToInteger())), nil
	case reflect.Uint32:
		return reflect.ValueOf(uint32(jsVal.ToInteger())), nil
	case reflect.Uint64:
		return reflect.ValueOf(uint64(jsVal.ToInteger())), nil
	case reflect.Float32:
		return reflect.ValueOf(float32(jsVal.ToFloat())), nil
	case reflect.Float64:
		return reflect.ValueOf(jsVal.ToFloat()), nil
	}

	// 10. Slice conversion ([]string, []types.MessageID, etc.)
	if targetType.Kind() == reflect.Slice {
		elemType := targetType.Elem()
		if arr, ok := exported.([]any); ok {
			sliceVal := reflect.MakeSlice(targetType, len(arr), len(arr))
			for i, elem := range arr {
				convElem, err := convertJSArgToGo(vm, vm.ToValue(elem), elemType, ctx)
				if err != nil {
					return reflect.Value{}, err
				}
				sliceVal.Index(i).Set(convElem)
			}
			return sliceVal, nil
		}
		// Wrap single element in slice if caller passed a scalar
		singleElem, err := convertJSArgToGo(vm, jsVal, elemType, ctx)
		if err == nil {
			sliceVal := reflect.MakeSlice(targetType, 1, 1)
			sliceVal.Index(0).Set(singleElem)
			return sliceVal, nil
		}
	}

	// 11. Generic Struct unmarshaling via JSON
	if targetType.Kind() == reflect.Struct || (targetType.Kind() == reflect.Ptr && targetType.Elem().Kind() == reflect.Struct) {
		jsonBytes, err := json.Marshal(exported)
		if err == nil {
			var newStruct reflect.Value
			if targetType.Kind() == reflect.Ptr {
				newStruct = reflect.New(targetType.Elem())
				if err := json.Unmarshal(jsonBytes, newStruct.Interface()); err == nil {
					return newStruct, nil
				}
			} else {
				newStruct = reflect.New(targetType)
				if err := json.Unmarshal(jsonBytes, newStruct.Interface()); err == nil {
					return newStruct.Elem(), nil
				}
			}
		}
	}

	// Fallback to direct reflect.ValueOf if types match
	val := reflect.ValueOf(exported)
	if val.IsValid() && val.Type().AssignableTo(targetType) {
		return val, nil
	}

	return reflect.Zero(targetType), fmt.Errorf("cannot convert JS value %v (%T) to %v", exported, exported, targetType)
}

func resolveJID(val any, ctx *dispatch.Context) (types.JID, error) {
	switch v := val.(type) {
	case types.JID:
		return v, nil
	case *types.JID:
		if v != nil {
			return *v, nil
		}
		return types.EmptyJID, nil
	case string:
		clean := strings.TrimSpace(v)
		switch strings.ToLower(clean) {
		case "me", "self", "bot":
			if ctx != nil && ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
				return ctx.Client.Store.ID.ToNonAD(), nil
			}
			return types.EmptyJID, fmt.Errorf("bot user JID not available")
		case "chat":
			if ctx != nil {
				return ctx.Chat, nil
			}
			return types.EmptyJID, fmt.Errorf("chat JID not available")
		case "sender":
			if ctx != nil {
				return ctx.Sender, nil
			}
			return types.EmptyJID, fmt.Errorf("sender JID not available")
		}

		if !strings.Contains(clean, "@") {
			clean += "@s.whatsapp.net"
		}
		return types.ParseJID(clean)
	case map[string]any:
		user, _ := v["user"].(string)
		server, _ := v["server"].(string)
		if server == "" {
			server = types.DefaultUserServer
		}
		return types.NewJID(user, server), nil
	default:
		return types.EmptyJID, fmt.Errorf("invalid JID value: %v", val)
	}
}

func resolveJIDSlice(val any, ctx *dispatch.Context) ([]types.JID, error) {
	if arr, ok := val.([]any); ok {
		var res []types.JID
		for _, item := range arr {
			jid, err := resolveJID(item, ctx)
			if err != nil {
				return nil, err
			}
			res = append(res, jid)
		}
		return res, nil
	}
	// Single element fallback
	jid, err := resolveJID(val, ctx)
	if err != nil {
		return nil, err
	}
	return []types.JID{jid}, nil
}

// SetupGlobalConstants registers convenient helper objects (proto, wa, jid, toProto) into Goja.
func SetupGlobalConstants(vm *goja.Runtime, ctx *dispatch.Context) {
	// Top-level JID helper
	_ = vm.Set("jid", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		parsed, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(parsed.String())
	})

	// Top-level toProto helper
	_ = vm.Set("toProto", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	})

	// Top-level proto namespace
	protoObj := vm.NewObject()
	_ = protoObj.Set("message", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(&waE2E.Message{})
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	})
	_ = protoObj.Set("text", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(&waE2E.Message{})
		}
		text := call.Arguments[0].String()
		return vm.ToValue(&waE2E.Message{Conversation: proto.String(text)})
	})
	_ = protoObj.Set("toJSON", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("{}")
		}
		exported := call.Arguments[0].Export()
		if pm, ok := exported.(proto.Message); ok {
			b, err := protojson.Marshal(pm)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(string(b))
		}
		b, err := json.Marshal(exported)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(string(b))
	})
	_ = vm.Set("proto", protoObj)

	// Top-level wa constants namespace
	waObj := vm.NewObject()
	_ = waObj.Set("Presence", map[string]string{
		"Available":   string(types.PresenceAvailable),
		"Unavailable": string(types.PresenceUnavailable),
	})
	_ = waObj.Set("ChatPresence", map[string]string{
		"Composing": string(types.ChatPresenceComposing),
		"Paused":    string(types.ChatPresencePaused),
	})
	_ = waObj.Set("ChatPresenceMedia", map[string]string{
		"Audio": string(types.ChatPresenceMediaAudio),
	})
	_ = waObj.Set("MediaType", map[string]string{
		"Image":    string(whatsmeow.MediaImage),
		"Video":    string(whatsmeow.MediaVideo),
		"Audio":    string(whatsmeow.MediaAudio),
		"Document": string(whatsmeow.MediaDocument),
	})
	_ = vm.Set("wa", waObj)

	// Context shortcuts
	if ctx != nil {
		_ = vm.Set("chat", ctx.Chat.String())
		_ = vm.Set("sender", ctx.Sender.String())
		if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
			_ = vm.Set("me", ctx.Client.Store.ID.ToNonAD().String())
		}
	}
}
