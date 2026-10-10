// proto_builder.go provides automatic payload conversion between JavaScript/dynamic objects
// and WhatsApp Go protobuf equivalents (*waE2E.Message and sub-messages).
package owner

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"whatsrook/cmd/dispatch"
)

// ConvertToProtoMessage converts an arbitrary JavaScript or Go object into a *waE2E.Message.
// It supports:
// 1. Direct *waE2E.Message pointers
// 2. Plain text strings (converted to Conversation or ExtendedTextMessage)
// 3. Baileys-style payloads ({ text, image, video, audio, document, sticker, poll, react, delete, edit })
// 4. Direct protobuf JSON/maps with camelCase, PascalCase, or snake_case field names.
func ConvertToProtoMessage(val any, ctx *dispatch.Context) (*waE2E.Message, error) {
	if val == nil {
		return nil, fmt.Errorf("nil message payload")
	}

	// 1. Direct *waE2E.Message
	if msg, ok := val.(*waE2E.Message); ok {
		return msg, nil
	}

	// 2. String payload
	if str, ok := val.(string); ok {
		return &waE2E.Message{Conversation: proto.String(str)}, nil
	}

	// 3. Raw byte payload
	if b, ok := val.([]byte); ok {
		return &waE2E.Message{Conversation: proto.String(string(b))}, nil
	}

	// 4. Map payload (from JavaScript object)
	mapVal, ok := val.(map[string]any)
	if !ok {
		// Try JSON marshaling/unmarshaling to map
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("cannot serialize payload of type %T: %w", val, err)
		}
		if err := json.Unmarshal(jsonBytes, &mapVal); err != nil {
			return nil, fmt.Errorf("cannot convert payload to map: %w", err)
		}
	}

	// Check for Baileys-style shorthand properties
	if msg, handled, err := convertBaileysPayload(mapVal, ctx); handled {
		return msg, err
	}

	// Check for direct protobuf message properties
	return convertProtobufMapToMessage(mapVal)
}

// convertBaileysPayload handles Baileys-style dictionary syntax.
func convertBaileysPayload(m map[string]any, ctx *dispatch.Context) (*waE2E.Message, bool, error) {
	// Text message with optional mentions / quoted
	if textVal, exists := getMapString(m, "text", "caption"); exists {
		// Check if it's text-only (not a media caption)
		hasMedia := hasAnyKey(m, "image", "video", "audio", "document", "sticker")
		if !hasMedia {
			ci := buildContextInfo(m, ctx)
			if ci != nil || hasAnyKey(m, "mentions", "quoted") {
				return &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text:        proto.String(textVal),
						ContextInfo: ci,
					},
				}, true, nil
			}
			return &waE2E.Message{Conversation: proto.String(textVal)}, true, nil
		}
	}

	// Image payload
	if imgVal, exists := m["image"]; exists {
		data, mime, err := resolveMediaData(imgVal, "image/jpeg")
		if err != nil {
			return nil, true, fmt.Errorf("resolving image data: %w", err)
		}
		uploaded, err := uploadMedia(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return nil, true, fmt.Errorf("uploading image: %w", err)
		}
		imgMsg := &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mime),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   buildContextInfo(m, ctx),
		}
		if cap, ok := getMapString(m, "caption", "text"); ok {
			imgMsg.Caption = proto.String(cap)
		}
		if viewOnce, _ := getMapBool(m, "viewOnce"); viewOnce {
			imgMsg.ViewOnce = proto.Bool(true)
		}
		return &waE2E.Message{ImageMessage: imgMsg}, true, nil
	}

	// Video payload
	if vidVal, exists := m["video"]; exists {
		data, mime, err := resolveMediaData(vidVal, "video/mp4")
		if err != nil {
			return nil, true, fmt.Errorf("resolving video data: %w", err)
		}
		uploaded, err := uploadMedia(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return nil, true, fmt.Errorf("uploading video: %w", err)
		}
		vidMsg := &waE2E.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mime),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   buildContextInfo(m, ctx),
		}
		if cap, ok := getMapString(m, "caption", "text"); ok {
			vidMsg.Caption = proto.String(cap)
		}
		if gif, _ := getMapBool(m, "gifPlayback", "gif"); gif {
			vidMsg.GifPlayback = proto.Bool(true)
		}
		if viewOnce, _ := getMapBool(m, "viewOnce"); viewOnce {
			vidMsg.ViewOnce = proto.Bool(true)
		}
		return &waE2E.Message{VideoMessage: vidMsg}, true, nil
	}

	// Audio payload
	if audVal, exists := m["audio"]; exists {
		ptt, _ := getMapBool(m, "ptt")
		defaultMime := "audio/mp4"
		if ptt {
			defaultMime = "audio/ogg; codecs=opus"
		}
		data, mime, err := resolveMediaData(audVal, defaultMime)
		if err != nil {
			return nil, true, fmt.Errorf("resolving audio data: %w", err)
		}
		uploaded, err := uploadMedia(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return nil, true, fmt.Errorf("uploading audio: %w", err)
		}
		audMsg := &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mime),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			PTT:           proto.Bool(ptt),
			ContextInfo:   buildContextInfo(m, ctx),
		}
		return &waE2E.Message{AudioMessage: audMsg}, true, nil
	}

	// Document payload
	if docVal, exists := m["document"]; exists {
		mime := "application/octet-stream"
		if mVal, ok := getMapString(m, "mimetype", "mime"); ok {
			mime = mVal
		}
		data, mime, err := resolveMediaData(docVal, mime)
		if err != nil {
			return nil, true, fmt.Errorf("resolving document data: %w", err)
		}
		uploaded, err := uploadMedia(ctx, data, whatsmeow.MediaDocument)
		if err != nil {
			return nil, true, fmt.Errorf("uploading document: %w", err)
		}
		docMsg := &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mime),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   buildContextInfo(m, ctx),
		}
		if fn, ok := getMapString(m, "fileName", "filename", "name"); ok {
			docMsg.FileName = proto.String(fn)
		}
		if cap, ok := getMapString(m, "caption", "text"); ok {
			docMsg.Caption = proto.String(cap)
		}
		return &waE2E.Message{DocumentMessage: docMsg}, true, nil
	}

	// Sticker payload
	if stkVal, exists := m["sticker"]; exists {
		data, _, err := resolveMediaData(stkVal, "image/webp")
		if err != nil {
			return nil, true, fmt.Errorf("resolving sticker data: %w", err)
		}
		uploaded, err := uploadMedia(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return nil, true, fmt.Errorf("uploading sticker: %w", err)
		}
		stkMsg := &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("image/webp"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   buildContextInfo(m, ctx),
		}
		return &waE2E.Message{StickerMessage: stkMsg}, true, nil
	}

	// Reaction payload: { react: { text: "❤️", key: { id: "...", ... } } }
	if reactVal, exists := m["react"]; exists {
		if rMap, ok := reactVal.(map[string]any); ok {
			emoji, _ := getMapString(rMap, "text", "emoji")
			keyMap, _ := rMap["key"].(map[string]any)
			id, _ := getMapString(keyMap, "id", "stanzaId")
			fromMe, _ := getMapBool(keyMap, "fromMe")
			remoteJID, _ := getMapString(keyMap, "remoteJid", "chat")
			participant, _ := getMapString(keyMap, "participant", "sender")

			reactMsg := &waE2E.ReactionMessage{
				Text: proto.String(emoji),
				Key: &waCommon.MessageKey{
					ID:        proto.String(id),
					FromMe:    proto.Bool(fromMe),
					RemoteJID: proto.String(remoteJID),
				},
				SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
			}
			if participant != "" {
				reactMsg.Key.Participant = proto.String(participant)
			}
			return &waE2E.Message{ReactionMessage: reactMsg}, true, nil
		}
	}

	// Poll creation: { poll: { name: "...", values: ["..."], selectableCount: 1 } }
	if pollVal, exists := m["poll"]; exists {
		if pMap, ok := pollVal.(map[string]any); ok {
			name, _ := getMapString(pMap, "name", "title", "question")
			var values []string
			if rawOpts, ok := pMap["values"].([]any); ok {
				for _, o := range rawOpts {
					values = append(values, fmt.Sprintf("%v", o))
				}
			} else if rawOpts, ok := pMap["options"].([]any); ok {
				for _, o := range rawOpts {
					values = append(values, fmt.Sprintf("%v", o))
				}
			}
			selCount := 1
			if sc, ok := pMap["selectableCount"].(float64); ok && sc > 0 {
				selCount = int(sc)
			}
			if ctx != nil && ctx.Client != nil {
				pollMsg := ctx.Client.BuildPollCreation(name, values, selCount)
				return pollMsg, true, nil
			}
		}
	}

	// Delete / Revoke payload: { delete: { id: "...", remoteJid: "...", fromMe: true } }
	if delVal, exists := m["delete"]; exists {
		if dMap, ok := delVal.(map[string]any); ok {
			id, _ := getMapString(dMap, "id", "stanzaId")
			remoteJID, _ := getMapString(dMap, "remoteJid", "chat")
			fromMe, _ := getMapBool(dMap, "fromMe")
			participant, _ := getMapString(dMap, "participant", "sender")

			key := &waCommon.MessageKey{
				ID:        proto.String(id),
				RemoteJID: proto.String(remoteJID),
				FromMe:    proto.Bool(fromMe),
			}
			if participant != "" {
				key.Participant = proto.String(participant)
			}
			revokeType := waE2E.ProtocolMessage_REVOKE
			return &waE2E.Message{
				ProtocolMessage: &waE2E.ProtocolMessage{
					Key:  key,
					Type: &revokeType,
				},
			}, true, nil
		}
	}

	return nil, false, nil
}

// convertProtobufMapToMessage unmarshals map to *waE2E.Message using JSON or protojson.
func convertProtobufMapToMessage(m map[string]any) (*waE2E.Message, error) {
	// Normalize keys to camelCase for protojson compliance
	normalized := make(map[string]any)
	for k, v := range m {
		camelKey := toLowerCamelCase(k)
		normalized[camelKey] = v
	}

	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshaling map to JSON: %w", err)
	}

	msg := &waE2E.Message{}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	if err := unmarshaler.Unmarshal(jsonBytes, msg); err == nil {
		return msg, nil
	}

	// Fallback to standard json unmarshaling
	if err := json.Unmarshal(jsonBytes, msg); err == nil {
		return msg, nil
	}

	return nil, fmt.Errorf("unable to decode payload into protobuf Message: %w", err)
}

func buildContextInfo(m map[string]any, ctx *dispatch.Context) *waE2E.ContextInfo {
	var ci *waE2E.ContextInfo

	// Check for explicit contextInfo in map
	if rawCI, ok := m["contextInfo"].(map[string]any); ok {
		if jsonBytes, err := json.Marshal(rawCI); err == nil {
			ci = &waE2E.ContextInfo{}
			_ = protojson.Unmarshal(jsonBytes, ci)
		}
	}

	// Mentions: mentions: ["123@s.whatsapp.net"] or mentions: [jid1, jid2]
	if mentions, ok := m["mentions"].([]any); ok && len(mentions) > 0 {
		if ci == nil {
			ci = &waE2E.ContextInfo{}
		}
		for _, men := range mentions {
			menStr := fmt.Sprintf("%v", men)
			if parsed, err := types.ParseJID(menStr); err == nil {
				ci.MentionedJID = append(ci.MentionedJID, parsed.ToNonAD().String())
			} else if menStr != "" {
				ci.MentionedJID = append(ci.MentionedJID, menStr)
			}
		}
	}

	// Quoted message: quoted: m or quoted: ctx.GetQuotedMessage() or quoted: { text: "..." }
	if quoted, ok := m["quoted"]; ok && quoted != nil {
		if ci == nil {
			ci = &waE2E.ContextInfo{}
		}
		if qMsg, ok := quoted.(*waE2E.Message); ok {
			ci.QuotedMessage = qMsg
		} else if ctx != nil && ctx.Evt != nil {
			// If quoted: true or quoted: m, inherit trigger message context
			ci.QuotedMessage = ctx.Evt.Message
			stanzaID := ctx.Evt.Info.ID
			ci.StanzaID = &stanzaID
			participant := ctx.Evt.Info.Sender.ToNonAD().String()
			ci.Participant = &participant
		}
	}

	return ci
}

func resolveMediaData(val any, defaultMime string) ([]byte, string, error) {
	switch v := val.(type) {
	case []byte:
		return v, defaultMime, nil
	case string:
		// Base64 data URL: data:image/png;base64,....
		if strings.HasPrefix(v, "data:") {
			parts := strings.SplitN(v, ";base64,", 2)
			if len(parts) == 2 {
				mime := strings.TrimPrefix(parts[0], "data:")
				data, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					return nil, "", fmt.Errorf("decoding base64 data url: %w", err)
				}
				return data, mime, nil
			}
		}
		// Remote URL: http:// or https://
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Get(v)
			if err != nil {
				return nil, "", fmt.Errorf("fetching remote media URL: %w", err)
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, "", fmt.Errorf("reading remote media stream: %w", err)
			}
			mime := resp.Header.Get("Content-Type")
			if mime == "" {
				mime = defaultMime
			}
			return data, mime, nil
		}
		// Local file path
		if fileInfo, err := os.Stat(v); err == nil && !fileInfo.IsDir() {
			data, err := os.ReadFile(v)
			if err != nil {
				return nil, "", fmt.Errorf("reading file %s: %w", v, err)
			}
			return data, defaultMime, nil
		}
		// Pure base64 string
		if data, err := base64.StdEncoding.DecodeString(v); err == nil && len(data) > 0 {
			return data, defaultMime, nil
		}
		// Hex string
		if data, err := hex.DecodeString(v); err == nil && len(data) > 0 {
			return data, defaultMime, nil
		}
		// Fallback to raw string bytes
		return []byte(v), defaultMime, nil
	default:
		return nil, "", fmt.Errorf("unsupported media source type: %T", val)
	}
}

func uploadMedia(ctx *dispatch.Context, data []byte, mediaType whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	if ctx == nil || ctx.Client == nil {
		return whatsmeow.UploadResponse{}, fmt.Errorf("whatsapp client unavailable")
	}
	sendCtx := ctx.GetSendContext()
	return ctx.Client.Upload(sendCtx, data, mediaType)
}

func getMapString(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			if s, ok := val.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

func getMapBool(m map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			if b, ok := val.(bool); ok {
				return b, true
			}
		}
	}
	return false, false
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, exists := m[k]; exists {
			return true
		}
	}
	return false
}

func toLowerCamelCase(s string) string {
	if s == "" {
		return ""
	}
	// If snake_case, convert to camelCase
	if strings.Contains(s, "_") {
		parts := strings.Split(s, "_")
		for i, part := range parts {
			if i == 0 {
				parts[i] = strings.ToLower(part)
			} else if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			}
		}
		return strings.Join(parts, "")
	}
	// If PascalCase, lower the first letter
	return strings.ToLower(s[:1]) + s[1:]
}
