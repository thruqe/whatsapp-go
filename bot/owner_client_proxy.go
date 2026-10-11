// client_proxy.go provides a dynamic reflection proxy and Baileys-compatible
// bridge for the whatsmeow.Client instance, exposing all client methods
// with automatic argument conversion to Go equivalents.
package main

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

	"github.com/thruqe/whatsrook"
)

// SetupEvalJavaScriptEnvironment registers Baileys-style client proxies, protobuf builders,
// utility globals, reflection methods, and extended tools into the Goja runtime.
func SetupEvalJavaScriptEnvironment(vm *goja.Runtime, ctx *Context, trackerOpt ...*EvalContextTracker) (*goja.Object, error) {
	var tracker *EvalContextTracker
	if len(trackerOpt) > 0 && trackerOpt[0] != nil {
		tracker = trackerOpt[0]
	}

	// Register extended tools (fetch, exec, fs, crypto, store, system, type inspection)
	RegisterEvalExtendedTools(vm, ctx, tracker)

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

	// 2. High-level messaging shortcuts with function metadata
	tagAndSet(vm, clientObj, "reply", FunctionMetadata{
		Name:       "reply",
		Signature:  "(text string)",
		Type:       "func(text string) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		text := call.Arguments[0].String()
		_ = ctx.Reply(text)
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "react", FunctionMetadata{
		Name:       "react",
		Signature:  "(emoji string, [msgID string])",
		Type:       "func(emoji string, [msgID string]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || ctx.Evt == nil {
			return goja.Undefined()
		}
		emoji := call.Arguments[0].String()
		targetMsgID := ctx.Evt.Info.ID
		if len(call.Arguments) > 1 && call.Arguments[1].String() != "" {
			targetMsgID = types.MessageID(call.Arguments[1].String())
		}
		reaction := ctx.Client.BuildReaction(ctx.Chat, ctx.Sender, targetMsgID, emoji)
		_, err := ctx.Client.SendMessage(ctx.GetSendContext(), ctx.Chat, reaction)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendText", FunctionMetadata{
		Name:       "sendText",
		Signature:  "(to string|JID, text string)",
		Type:       "func(to string|JID, text string) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("sendText requires (to, text)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		text := call.Arguments[1].String()
		err = whatsrook.SendText(ctx.GetSendContext(), ctx.Client, toJID, text)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendImage", FunctionMetadata{
		Name:       "sendImage",
		Signature:  "(to string|JID, dataOrUrl any, [caption string])",
		Type:       "func(to string|JID, dataOrUrl any, [caption string]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "any", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		err = whatsrook.SendImage(ctx.GetSendContext(), ctx.Client, toJID, data, mime, caption)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendVideo", FunctionMetadata{
		Name:       "sendVideo",
		Signature:  "(to string|JID, dataOrUrl any, [caption string])",
		Type:       "func(to string|JID, dataOrUrl any, [caption string]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "any", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		err = whatsrook.SendVideo(ctx.GetSendContext(), ctx.Client, toJID, data, mime, caption)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendAudio", FunctionMetadata{
		Name:       "sendAudio",
		Signature:  "(to string|JID, dataOrUrl any, [ptt bool])",
		Type:       "func(to string|JID, dataOrUrl any, [ptt bool]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "any", "bool?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		err = whatsrook.SendAudio(ctx.GetSendContext(), ctx.Client, toJID, data, mime)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendSticker", FunctionMetadata{
		Name:       "sendSticker",
		Signature:  "(to string|JID, dataOrUrl any)",
		Type:       "func(to string|JID, dataOrUrl any) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "any"},
	}, func(call goja.FunctionCall) goja.Value {
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
		err = whatsrook.SendSticker(ctx.GetSendContext(), ctx.Client, toJID, data, ctx.GetStickerPack(), ctx.GetStickerAuthor())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendReaction", FunctionMetadata{
		Name:       "sendReaction",
		Signature:  "(to string|JID, messageId string, emoji string)",
		Type:       "func(to string|JID, messageId string, emoji string) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "string", "string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendReaction requires (to, messageId, emoji)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		msgID := call.Arguments[1].String()
		emoji := call.Arguments[2].String()
		err = whatsrook.SendReactMessage(ctx.GetSendContext(), ctx.Client, toJID, msgID, emoji)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendPoll", FunctionMetadata{
		Name:       "sendPoll",
		Signature:  "(to string|JID, question string, options []string, [selectableCount int]) SendResponse",
		Type:       "func(to string|JID, question string, options []string, [selectableCount int]) whatsmeow.SendResponse",
		ReturnType: "whatsmeow.SendResponse",
		ParamTypes: []string{"string|JID", "string", "[]string", "int?"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendPoll requires (to, question, options, [selectableCount])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		question := call.Arguments[1].String()
		var opts []string
		if arr, ok := call.Arguments[2].Export().([]any); ok {
			for _, item := range arr {
				opts = append(opts, fmt.Sprintf("%v", item))
			}
		}
		selCount := 1
		if len(call.Arguments) > 3 {
			if sc := call.Arguments[3].ToInteger(); sc > 0 {
				selCount = int(sc)
			}
		}
		pollMsg := ctx.Client.BuildPollCreation(question, opts, selCount)
		resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), toJID, pollMsg)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	}, tracker)

	tagAndSet(vm, clientObj, "sendDocument", FunctionMetadata{
		Name:       "sendDocument",
		Signature:  "(to string|JID, dataOrUrl any, filename string, [mimetype string], [caption string]) bool",
		Type:       "func(to string|JID, dataOrUrl any, filename string, [mimetype string], [caption string]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string|JID", "any", "string", "string?", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendDocument requires (to, dataOrUrl, filename, [mimetype], [caption])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		mimetype := "application/octet-stream"
		if len(call.Arguments) > 3 && call.Arguments[3].String() != "" {
			mimetype = call.Arguments[3].String()
		}
		data, mime, err := resolveMediaData(call.Arguments[1].Export(), mimetype)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		filename := call.Arguments[2].String()
		caption := ""
		if len(call.Arguments) > 4 {
			caption = call.Arguments[4].String()
		}
		err = whatsrook.SendDocument(ctx.GetSendContext(), ctx.Client, toJID, data, mime, filename, caption)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "sendLocation", FunctionMetadata{
		Name:       "sendLocation",
		Signature:  "(to string|JID, lat float64, long float64, [name string], [address string]) SendResponse",
		Type:       "func(to string|JID, lat float64, long float64, [name string], [address string]) whatsmeow.SendResponse",
		ReturnType: "whatsmeow.SendResponse",
		ParamTypes: []string{"string|JID", "float64", "float64", "string?", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendLocation requires (to, lat, long, [name], [address])"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		lat := call.Arguments[1].ToFloat()
		long := call.Arguments[2].ToFloat()
		name := ""
		addr := ""
		if len(call.Arguments) > 3 {
			name = call.Arguments[3].String()
		}
		if len(call.Arguments) > 4 {
			addr = call.Arguments[4].String()
		}
		locMsg := &waE2E.LocationMessage{
			DegreesLatitude:  new(lat),
			DegreesLongitude: new(long),
		}
		if name != "" {
			locMsg.Name = new(name)
		}
		if addr != "" {
			locMsg.Address = new(addr)
		}
		resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), toJID, &waE2E.Message{LocationMessage: locMsg})
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	}, tracker)

	tagAndSet(vm, clientObj, "sendContact", FunctionMetadata{
		Name:       "sendContact",
		Signature:  "(to string|JID, displayName string, vcardOrPhone string) SendResponse",
		Type:       "func(to string|JID, displayName string, vcardOrPhone string) whatsmeow.SendResponse",
		ReturnType: "whatsmeow.SendResponse",
		ParamTypes: []string{"string|JID", "string", "string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("sendContact requires (to, displayName, vcardOrPhone)"))
		}
		toJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		name := call.Arguments[1].String()
		vcardOrPhone := call.Arguments[2].String()
		vcard := vcardOrPhone
		if !strings.HasPrefix(vcard, "BEGIN:VCARD") {
			vcard = fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", name, vcardOrPhone, vcardOrPhone)
		}
		conMsg := &waE2E.ContactMessage{
			DisplayName: new(name),
			Vcard:       new(vcard),
		}
		resp, err := ctx.Client.SendMessage(ctx.GetSendContext(), toJID, &waE2E.Message{ContactMessage: conMsg})
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	}, tracker)

	tagAndSet(vm, clientObj, "sendPresence", FunctionMetadata{
		Name:       "sendPresence",
		Signature:  "(presence string, [chat string|JID]) bool",
		Type:       "func(presence string, [chat string|JID]) bool",
		ReturnType: "bool",
		ParamTypes: []string{"string", "string|JID?"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("sendPresence requires a presence state"))
		}
		pStr := strings.ToLower(call.Arguments[0].String())
		targetChat := ctx.Chat
		if len(call.Arguments) > 1 {
			if parsed, err := resolveJID(call.Arguments[1].Export(), ctx); err == nil {
				targetChat = parsed
			}
		}
		switch pStr {
		case "available", "online":
			_ = ctx.Client.SendPresence(ctx.GetSendContext(), types.PresenceAvailable)
		case "unavailable", "offline":
			_ = ctx.Client.SendPresence(ctx.GetSendContext(), types.PresenceUnavailable)
		case "composing", "typing":
			_ = ctx.Client.SendChatPresence(ctx.GetSendContext(), targetChat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		case "recording":
			_ = ctx.Client.SendChatPresence(ctx.GetSendContext(), targetChat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
		case "paused":
			_ = ctx.Client.SendChatPresence(ctx.GetSendContext(), targetChat, types.ChatPresencePaused, types.ChatPresenceMediaText)
		default:
			_ = ctx.Client.SendPresence(ctx.GetSendContext(), types.Presence(pStr))
		}
		return vm.ToValue(true)
	}, tracker)

	tagAndSet(vm, clientObj, "groupMetadata", FunctionMetadata{
		Name:       "groupMetadata",
		Signature:  "(jid string|JID) *types.GroupInfo",
		Type:       "func(jid string|JID) (*types.GroupInfo, error)",
		ReturnType: "*types.GroupInfo",
		ParamTypes: []string{"string|JID"},
	}, func(call goja.FunctionCall) goja.Value {
		targetJID := ctx.Chat
		if len(call.Arguments) > 0 {
			if parsed, err := resolveJID(call.Arguments[0].Export(), ctx); err == nil {
				targetJID = parsed
			}
		}
		info, err := ctx.Client.GetGroupInfo(ctx.GetSendContext(), targetJID)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(info)
	}, tracker)
	_ = clientObj.Set("groupInfo", clientObj.Get("groupMetadata"))

	tagAndSet(vm, clientObj, "groupParticipantsUpdate", FunctionMetadata{
		Name:       "groupParticipantsUpdate",
		Signature:  "(jid string|JID, participants []string|JID, action string) []types.GroupParticipant",
		Type:       "func(jid string|JID, participants []string|JID, action string) ([]types.GroupParticipant, error)",
		ReturnType: "[]types.GroupParticipant",
		ParamTypes: []string{"string|JID", "[]string|JID", "string"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.ToValue("groupParticipantsUpdate requires (jid, participants, action)"))
		}
		gJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		pJIDs, err := resolveJIDSlice(call.Arguments[1].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		actionStr := strings.ToLower(call.Arguments[2].String())
		var act whatsmeow.ParticipantChange
		switch actionStr {
		case "add":
			act = whatsmeow.ParticipantChangeAdd
		case "remove", "kick":
			act = whatsmeow.ParticipantChangeRemove
		case "promote", "admin":
			act = whatsmeow.ParticipantChangePromote
		case "demote":
			act = whatsmeow.ParticipantChangeDemote
		default:
			panic(vm.ToValue(fmt.Sprintf("unknown participant action: %s (expected add, remove, promote, demote)", actionStr)))
		}
		res, err := ctx.Client.UpdateGroupParticipants(ctx.GetSendContext(), gJID, pJIDs, act)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(res)
	}, tracker)

	tagAndSet(vm, clientObj, "edit", FunctionMetadata{
		Name:       "edit",
		Signature:  "(messageId string, newContent any, [chatJid string|JID])",
		Type:       "func(messageId string, newContent any, [chatJid string|JID]) { id: string, timestamp: int64 }",
		ReturnType: "{ id: string, timestamp: int64 }",
		ParamTypes: []string{"string", "any", "string|JID?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		resp, err := whatsrook.SendEdit(ctx.GetSendContext(), ctx.Client, targetChat, msgID, content)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	}, tracker)

	tagAndSet(vm, clientObj, "delete", FunctionMetadata{
		Name:       "delete",
		Signature:  "(messageId string, [chatJid string|JID])",
		Type:       "func(messageId string, [chatJid string|JID]) { id: string, timestamp: int64 }",
		ReturnType: "{ id: string, timestamp: int64 }",
		ParamTypes: []string{"string", "string|JID?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		resp, err := whatsrook.SendDelete(ctx.GetSendContext(), ctx.Client, targetChat, msgID)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(map[string]any{"id": resp.ID, "timestamp": resp.Timestamp.Unix()})
	}, tracker)

	tagAndSet(vm, clientObj, "download", FunctionMetadata{
		Name:       "download",
		Signature:  "([msg any])",
		Type:       "func([msg any]) { data: []byte, mimetype: string, size: int, base64: string }",
		ReturnType: "{ data: []byte, mimetype: string, size: int, base64: string }",
		ParamTypes: []string{"any?"},
	}, func(call goja.FunctionCall) goja.Value {
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
		data, mime, err := whatsrook.Download(ctx.GetSendContext(), ctx.Client, targetMsg)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		resObj := vm.NewObject()
		_ = resObj.Set("data", vm.ToValue(data))
		_ = resObj.Set("mimetype", mime)
		_ = resObj.Set("size", len(data))
		_ = resObj.Set("base64", base64.StdEncoding.EncodeToString(data))
		return resObj
	}, tracker)

	tagAndSet(vm, clientObj, "upload", FunctionMetadata{
		Name:       "upload",
		Signature:  "(dataOrUrl any, [mediaType string])",
		Type:       "func(dataOrUrl any, [mediaType string]) whatsmeow.UploadResponse",
		ReturnType: "whatsmeow.UploadResponse",
		ParamTypes: []string{"any", "string?"},
	}, func(call goja.FunctionCall) goja.Value {
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
	}, tracker)

	tagAndSet(vm, clientObj, "parseJID", FunctionMetadata{
		Name:       "parseJID",
		Signature:  "(val any)",
		Type:       "func(val any) string",
		ReturnType: "string",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		parsed, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(parsed.String())
	}, tracker)

	tagAndSet(vm, clientObj, "buildMessage", FunctionMetadata{
		Name:       "buildMessage",
		Signature:  "(val any)",
		Type:       "func(val any) *waE2E.Message",
		ReturnType: "*waE2E.Message",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	}, tracker)

	tagAndSet(vm, clientObj, "toProto", FunctionMetadata{
		Name:       "toProto",
		Signature:  "(val any)",
		Type:       "func(val any) *waE2E.Message",
		ReturnType: "*waE2E.Message",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	}, tracker)

	// 3. Register dynamic reflection proxy for ALL exported methods of whatsmeow.Client
	registerReflectedClientMethods(vm, clientObj, ctx, tracker)

	// Aliases
	_ = clientObj.Set("send", clientObj.Get("sendMessage"))

	return clientObj, nil
}

// registerReflectedClientMethods registers every exported method on whatsmeow.Client into clientObj.
func registerReflectedClientMethods(vm *goja.Runtime, clientObj *goja.Object, ctx *Context, tracker *EvalContextTracker) {
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

		meta := BuildFunctionMetadataFromType(methodName, mType)
		fnVal := createMethodWrapper(vm, methodName, mVal, mType, ctx, meta, tracker)

		// Set PascalCase name (e.g. SendMessage, GetUserInfo)
		_ = clientObj.Set(methodName, fnVal)

		// Set camelCase name (e.g. sendMessage, getUserInfo)
		lowerName := strings.ToLower(methodName[:1]) + methodName[1:]
		if lowerName != methodName {
			lowerMeta := meta
			lowerMeta.Name = lowerName
			lowerFnVal := createMethodWrapper(vm, lowerName, mVal, mType, ctx, lowerMeta, tracker)
			_ = clientObj.Set(lowerName, lowerFnVal)
		}
	}
}

// createMethodWrapper wraps an arbitrary Go method into a Goja function with automatic parameter coercion,
// metadata tagging, and call tracking.
func createMethodWrapper(vm *goja.Runtime, name string, mVal reflect.Value, mType reflect.Type, ctx *Context, meta FunctionMetadata, tracker *EvalContextTracker) goja.Value {
	rawFn := func(call goja.FunctionCall) goja.Value {
		if tracker != nil {
			var argsSummary []string
			for _, arg := range call.Arguments {
				argsSummary = append(argsSummary, truncateString(arg.String(), 40))
			}
			tracker.RecordCall(CallRecord{
				Name:        name,
				Type:        meta.Type,
				ReturnType:  meta.ReturnType,
				ParamTypes:  meta.ParamTypes,
				ArgsSummary: argsSummary,
			})
		}

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
		firstIsContext := numIn > 0 && mType.In(0) == reflect.TypeFor[context.Context]()
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
			if tracker != nil && tracker.lastCall != nil && tracker.lastCall.Name == name {
				tracker.lastCall.ResultType = "void"
			}
			return goja.Undefined()
		}

		// Check if last return value is error
		lastResult := results[len(results)-1]
		if lastResult.Type().Implements(reflect.TypeFor[error]()) {
			if !lastResult.IsNil() {
				err := lastResult.Interface().(error)
				panic(vm.NewGoError(err))
			}
			// If method returned only error (and error is nil), return true
			if len(results) == 1 {
				if tracker != nil && tracker.lastCall != nil && tracker.lastCall.Name == name {
					tracker.lastCall.ResultType = "bool"
				}
				return vm.ToValue(true)
			}
			// Exclude the nil error from the results
			results = results[:len(results)-1]
		}

		// Return single non-error value
		if len(results) == 1 {
			val := results[0].Interface()
			if tracker != nil && tracker.lastCall != nil && tracker.lastCall.Name == name {
				tracker.lastCall.ResultType = GetValueTypeName(val)
			}
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
		if tracker != nil && tracker.lastCall != nil && tracker.lastCall.Name == name {
			tracker.lastCall.ResultType = GetValueTypeName(arr)
		}
		return vm.ToValue(arr)
	}

	fnVal := vm.ToValue(rawFn)
	TagFunction(vm, fnVal, meta)
	return fnVal
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
func convertJSArgToGo(vm *goja.Runtime, jsVal goja.Value, targetType reflect.Type, ctx *Context) (reflect.Value, error) {
	if jsVal == nil || goja.IsUndefined(jsVal) || goja.IsNull(jsVal) {
		return reflect.Zero(targetType), nil
	}

	exported := jsVal.Export()

	// 1. types.JID
	if targetType == reflect.TypeFor[types.JID]() {
		jid, err := resolveJID(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(jid), nil
	}

	// 2. []types.JID
	if targetType == reflect.TypeFor[[]types.JID]() {
		jids, err := resolveJIDSlice(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(jids), nil
	}

	// 3. *waE2E.Message
	if targetType == reflect.TypeFor[*waE2E.Message]() {
		msg, err := ConvertToProtoMessage(exported, ctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(msg), nil
	}

	// 4. Protobuf Message pointer (*waE2E.ContextInfo, etc.)
	if targetType.Implements(reflect.TypeFor[proto.Message]()) {
		if targetType.Kind() == reflect.Pointer {
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
	if targetType == reflect.TypeFor[types.Presence]() {
		return reflect.ValueOf(types.Presence(jsVal.String())), nil
	}
	if targetType == reflect.TypeFor[types.ChatPresence]() {
		return reflect.ValueOf(types.ChatPresence(jsVal.String())), nil
	}
	if targetType == reflect.TypeFor[types.ChatPresenceMedia]() {
		return reflect.ValueOf(types.ChatPresenceMedia(jsVal.String())), nil
	}
	if targetType == reflect.TypeFor[types.MessageID]() {
		return reflect.ValueOf(types.MessageID(jsVal.String())), nil
	}

	// 6. []byte
	if targetType == reflect.TypeFor[[]byte]() {
		data, _, err := resolveMediaData(exported, "application/octet-stream")
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(data), nil
	}

	// 7. time.Duration
	if targetType == reflect.TypeFor[time.Duration]() {
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
	if targetType == reflect.TypeFor[time.Time]() {
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
	if targetType.Kind() == reflect.Struct || (targetType.Kind() == reflect.Pointer && targetType.Elem().Kind() == reflect.Struct) {
		jsonBytes, err := json.Marshal(exported)
		if err == nil {
			var newStruct reflect.Value
			if targetType.Kind() == reflect.Pointer {
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

func resolveJID(val any, ctx *Context) (types.JID, error) {
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

func resolveJIDSlice(val any, ctx *Context) ([]types.JID, error) {
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

// SetupGlobalConstants registers convenient helper objects (proto, wa, jid, toProto) into Goja with metadata.
func SetupGlobalConstants(vm *goja.Runtime, ctx *Context, trackerOpt ...*EvalContextTracker) {
	var tracker *EvalContextTracker
	if len(trackerOpt) > 0 && trackerOpt[0] != nil {
		tracker = trackerOpt[0]
	}

	// Top-level JID helper
	tagAndSet(vm, vm.GlobalObject(), "jid", FunctionMetadata{
		Name:       "jid",
		Signature:  "(val any)",
		Type:       "func(val any) string",
		ReturnType: "string",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		parsed, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(parsed.String())
	}, tracker)

	// Top-level toProto helper
	tagAndSet(vm, vm.GlobalObject(), "toProto", FunctionMetadata{
		Name:       "toProto",
		Signature:  "(payload any)",
		Type:       "func(payload any) *waE2E.Message",
		ReturnType: "*waE2E.Message",
		ParamTypes: []string{"any"},
	}, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		converted, err := ConvertToProtoMessage(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(converted)
	}, tracker)

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
		return vm.ToValue(&waE2E.Message{Conversation: new(text)})
	})
	_ = protoObj.Set("location", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("proto.location requires (latitude, longitude, [name], [address])"))
		}
		lat := call.Arguments[0].ToFloat()
		long := call.Arguments[1].ToFloat()
		name := ""
		addr := ""
		if len(call.Arguments) > 2 {
			name = call.Arguments[2].String()
		}
		if len(call.Arguments) > 3 {
			addr = call.Arguments[3].String()
		}
		locMsg := &waE2E.LocationMessage{
			DegreesLatitude:  new(lat),
			DegreesLongitude: new(long),
		}
		if name != "" {
			locMsg.Name = new(name)
		}
		if addr != "" {
			locMsg.Address = new(addr)
		}
		return vm.ToValue(&waE2E.Message{LocationMessage: locMsg})
	})
	_ = protoObj.Set("contact", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("proto.contact requires (displayName, vcardOrPhone)"))
		}
		name := call.Arguments[0].String()
		vcard := call.Arguments[1].String()
		if !strings.HasPrefix(vcard, "BEGIN:VCARD") {
			vcard = fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", name, vcard, vcard)
		}
		conMsg := &waE2E.ContactMessage{
			DisplayName: new(name),
			Vcard:       new(vcard),
		}
		return vm.ToValue(&waE2E.Message{ContactMessage: conMsg})
	})
	_ = protoObj.Set("poll", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("proto.poll requires (question, options, [selectableCount])"))
		}
		q := call.Arguments[0].String()
		var opts []string
		if arr, ok := call.Arguments[1].Export().([]any); ok {
			for _, item := range arr {
				opts = append(opts, fmt.Sprintf("%v", item))
			}
		}
		sc := 1
		if len(call.Arguments) > 2 {
			if count := call.Arguments[2].ToInteger(); count > 0 {
				sc = int(count)
			}
		}
		if ctx != nil && ctx.Client != nil {
			return vm.ToValue(ctx.Client.BuildPollCreation(q, opts, sc))
		}
		var dummyCli *whatsmeow.Client
		return vm.ToValue(dummyCli.BuildPollCreation(q, opts, sc))
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
	_ = protoObj.Set("clone", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		if pm, ok := call.Arguments[0].Export().(proto.Message); ok {
			return vm.ToValue(proto.Clone(pm))
		}
		return call.Arguments[0]
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
