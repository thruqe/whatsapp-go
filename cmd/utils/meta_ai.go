package cliutils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var MetaAiBotJID = types.NewJID("867051314767696", types.BotServer)

type MetaAiResult struct {
	Text           string
	GeneratedMedia []byte
	MediaMimeType  string
	MediaCaption   string

	GeneratedImg []byte
	ImgMimeType  string
	ImgCaption   string
}

type metaAiRequest struct {
	ctx      context.Context
	client   *whatsmeow.Client
	chat     types.JID
	request  string
	onUpdate func(text string) error
	resCh    chan metaAiResponse
}

type metaAiResponse struct {
	res MetaAiResult
	err error
}

type metaAiUnifiedData struct {
	ResponseID string `json:"response_id"`
	Sections   []struct {
		ViewModel struct {
			Primitive struct {
				Typename    string `json:"__typename"`
				ImagineType string `json:"imagine_type"`
				Media       struct {
					URL      string `json:"url"`
					MimeType string `json:"mime_type"`
				} `json:"media"`
				Status struct {
					Status     string `json:"status"`
					UpdateText string `json:"update_text"`
				} `json:"status"`
				Text string `json:"text"`
			} `json:"primitive"`
		} `json:"view_model"`
	} `json:"sections"`
}

var (
	metaAiQueues   = make(map[string]chan metaAiRequest)
	metaAiQueuesMu sync.Mutex
)

func getOrCreateMetaAiQueue(chatKey string) chan metaAiRequest {
	metaAiQueuesMu.Lock()
	defer metaAiQueuesMu.Unlock()
	ch, exists := metaAiQueues[chatKey]
	if !exists {
		ch = make(chan metaAiRequest, 100)
		metaAiQueues[chatKey] = ch
		go func() {
			processMetaAiQueue(ch)
		}()
	}
	return ch
}

func processMetaAiQueue(ch chan metaAiRequest) {
	for req := range ch {
		res, err := ExecuteMetaAiQuery(req.ctx, req.client, req.chat, req.request, req.onUpdate)
		req.resCh <- metaAiResponse{res: res, err: err}
	}
}

func IsDummyPlaceholderText(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || trimmed == "_" || trimmed == "__" || trimmed == "___" ||
		trimmed == "_We_" || trimmed == "_Thinking_" || trimmed == "..." ||
		strings.HasPrefix(trimmed, "_We need to respond") ||
		strings.HasPrefix(trimmed, "_Thinking") {
		return true
	}
	return false
}

func ExtractMetaAiText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if conv := msg.GetConversation(); conv != "" {
		if !IsDummyPlaceholderText(conv) {
			return conv
		}
		return ""
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		if !IsDummyPlaceholderText(ext.GetText()) {
			return ext.GetText()
		}
		return ""
	}
	if rich := msg.GetRichResponseMessage(); rich != nil {
		var text strings.Builder
		for _, sub := range rich.GetSubmessages() {
			s := sub.GetMessageText()
			if !IsDummyPlaceholderText(s) {
				text.WriteString(s)
			}
		}
		res := text.String()
		if !IsDummyPlaceholderText(res) && res != "" {
			return res
		}

		if unified := rich.GetUnifiedResponse(); unified != nil && len(unified.GetData()) > 0 {
			res = extractTextFromUnifiedJSON(unified.GetData())
			if !IsDummyPlaceholderText(res) && res != "" {
				return res
			}
		}
		return ""
	}
	return ""
}

func extractTextFromUnifiedJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var jsonBytes []byte
	if json.Valid(raw) {
		jsonBytes = raw
	} else {
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(raw)))
		if n, err := base64.StdEncoding.Decode(decoded, raw); err == nil {
			jsonBytes = decoded[:n]
		}
	}
	if len(jsonBytes) == 0 {
		return ""
	}

	var data struct {
		Sections []struct {
			ViewModel struct {
				Primitive struct {
					Typename string `json:"__typename"`
					Text     string `json:"text"`
					Rows     []struct {
						IsHeader bool     `json:"is_header"`
						Cells    []string `json:"cells"`
					} `json:"rows"`
					Language   string `json:"language"`
					CodeBlocks []struct {
						Content string `json:"content"`
						Type    string `json:"type"`
					} `json:"code_blocks"`
				} `json:"primitive"`
			} `json:"view_model"`
		} `json:"sections"`
	}

	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, sec := range data.Sections {
		p := sec.ViewModel.Primitive
		switch p.Typename {
		case "GenAIMarkdownTextUXPrimitive":
			if p.Text != "" && !IsDummyPlaceholderText(p.Text) {
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
				sb.WriteString(p.Text)
			}
		case "GenATableUXPrimitive":
			if len(p.Rows) > 0 {
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
				for rIdx, row := range p.Rows {
					sb.WriteString("| " + strings.Join(row.Cells, " | ") + " |\n")
					if rIdx == 0 || row.IsHeader {
						var seps []string
						for range row.Cells {
							seps = append(seps, "---")
						}
						sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")
					}
				}
			}
		case "GenAICodeUXPrimitive":
			if len(p.CodeBlocks) > 0 {
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
				lang := p.Language
				if lang == "" {
					lang = "text"
				}
				sb.WriteString("```" + lang + "\n")
				for _, b := range p.CodeBlocks {
					sb.WriteString(b.Content)
				}
				if !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
				sb.WriteString("```\n")
			}
		}
	}
	return sb.String()
}

func searchMediaInJSON(val any) (mediaURL, mimeType, text string) {
	switch v := val.(type) {
	case map[string]any:
		if urlVal, ok := v["url"].(string); ok && (strings.HasPrefix(urlVal, "http://") || strings.HasPrefix(urlVal, "https://")) {
			mediaURL = strings.ReplaceAll(urlVal, `\/`, `/`)
			if mVal, ok := v["mime_type"].(string); ok {
				mimeType = mVal
			}
		}
		if textVal, ok := v["text"].(string); ok && text == "" {
			text = textVal
		}
		for _, sub := range v {
			if subURL, subMime, subText := searchMediaInJSON(sub); subURL != "" {
				if mediaURL == "" {
					mediaURL = subURL
					mimeType = subMime
				}
				if text == "" {
					text = subText
				}
				return mediaURL, mimeType, text
			}
		}
	case []any:
		for _, item := range v {
			if subURL, subMime, subText := searchMediaInJSON(item); subURL != "" {
				if mediaURL == "" {
					mediaURL = subURL
					mimeType = subMime
				}
				if text == "" {
					text = subText
				}
				return mediaURL, mimeType, text
			}
		}
	}
	return mediaURL, mimeType, text
}

func detectMediaMime(data []byte, declaredMime, url string) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 8 && (string(data[4:8]) == "ftyp" || strings.Contains(string(data[:min(len(data), 32)]), "ftyp")) {
		return "video/mp4"
	}
	if declaredMime != "" && declaredMime != "application/octet-stream" {
		return declaredMime
	}
	if strings.Contains(url, ".mp4") || strings.Contains(declaredMime, "video") {
		return "video/mp4"
	}
	return "image/jpeg"
}

func parseUnifiedMediaState(msg *waE2E.Message) (mediaURL, mimeType, text, imagineType, status string) {
	if msg == nil {
		return "", "", "", "", ""
	}

	var rawB64 []byte
	if rich := msg.GetRichResponseMessage(); rich != nil && rich.GetUnifiedResponse() != nil {
		rawB64 = rich.GetUnifiedResponse().GetData()
	} else if pm := msg.GetProtocolMessage(); pm != nil && pm.GetEditedMessage() != nil {
		if rich := pm.GetEditedMessage().GetRichResponseMessage(); rich != nil && rich.GetUnifiedResponse() != nil {
			rawB64 = rich.GetUnifiedResponse().GetData()
		}
	}

	if len(rawB64) > 0 {
		var jsonBytes []byte
		if json.Valid(rawB64) {
			jsonBytes = rawB64
		} else {
			decoded := make([]byte, base64.StdEncoding.DecodedLen(len(rawB64)))
			if n, err := base64.StdEncoding.Decode(decoded, rawB64); err == nil {
				jsonBytes = decoded[:n]
			}
		}

		if len(jsonBytes) > 0 {
			var uData metaAiUnifiedData
			if err := json.Unmarshal(jsonBytes, &uData); err == nil {
				for _, sec := range uData.Sections {
					p := sec.ViewModel.Primitive
					if p.Media.URL != "" {
						mediaURL = strings.ReplaceAll(p.Media.URL, `\/`, `/`)
						mimeType = p.Media.MimeType
					}
					if p.Text != "" {
						text = p.Text
					}
					if p.ImagineType != "" {
						imagineType = p.ImagineType
					}
					if p.Status.Status != "" {
						status = p.Status.Status
					}
				}
			}
			if mediaURL == "" {
				var genericData any
				if err := json.Unmarshal(jsonBytes, &genericData); err == nil {
					mediaURL, mimeType, text = searchMediaInJSON(genericData)
				}
			}
		}
	}
	return mediaURL, mimeType, text, imagineType, status
}

func ExecuteMetaAiQuery(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (MetaAiResult, error) {
	chatKey := chat.String()

	Logger.Debug("executeMetaAiQuery: sending request", "chat", chatKey, "request", request)

	ackCh := make(chan error, 1)
	sendResp, err := client.SendMessage(ctx, MetaAiBotJID, &waE2E.Message{
		Conversation: new(request),
	}, whatsmeow.SendRequestExtra{
		OnAck: func(_ whatsmeow.SendResponse, ackErr error) {
			ackCh <- ackErr
		},
	})
	if err != nil {
		Logger.Error("executeMetaAiQuery: failed to send request", "chat", chatKey, "err", err)
		return MetaAiResult{}, fmt.Errorf("failed to send request to meta ai: %w", err)
	}
	if client.AsyncMessageAck {
		select {
		case ackErr := <-ackCh:
			if ackErr != nil {
				Logger.Error("executeMetaAiQuery: server rejected request", "chat", chatKey, "err", ackErr)
				return MetaAiResult{}, fmt.Errorf("failed to send request to meta ai: %w", ackErr)
			}
		case <-ctx.Done():
			return MetaAiResult{}, ctx.Err()
		}
	}
	sentMsgID := sendResp.ID

	var (
		mu              sync.Mutex
		metaMsgID       string
		seen            bool
		final           string
		genMediaData    []byte
		genMediaMime    string
		genMediaCap     string
		isMediaExpected bool
		done            = make(chan struct{})
		closeOnce       sync.Once
	)

	handlerID := client.AddEventHandler(func(evt any) {
		msgEvt, ok := evt.(*events.Message)
		if !ok || msgEvt.Info.Sender.String() != MetaAiBotJID.String() {
			return
		}

		pm := msgEvt.Message.GetProtocolMessage()

		mu.Lock()
		targetID := msgEvt.Info.MsgMetaInfo.TargetID
		isOurQuery := false
		if targetID != "" && targetID == sentMsgID {
			isOurQuery = true
		} else if pm != nil {
			pmKeyID := pm.GetKey().GetID()
			if pmKeyID == sentMsgID || (metaMsgID != "" && pmKeyID == metaMsgID) {
				isOurQuery = true
			}
		} else if !seen {
			isOurQuery = true
		} else if metaMsgID != "" && msgEvt.Info.ID == metaMsgID {
			isOurQuery = true
		}

		if !isOurQuery {
			mu.Unlock()
			return
		}

		if !seen && pm == nil {
			metaMsgID = msgEvt.Info.ID
			seen = true
			Logger.Debug("executeMetaAiQuery: captured meta ai reply message id", "chat", chatKey, "meta_msg_id", metaMsgID)
		}

		// 1. Direct image message
		if imgMsg := msgEvt.Message.GetImageMessage(); imgMsg != nil {
			Logger.Debug("executeMetaAiQuery: captured direct imageMessage from Meta AI", "chat", chatKey)
			imgBytes, err := client.Download(ctx, imgMsg)
			if err == nil && len(imgBytes) > 0 {
				genMediaData = imgBytes
				genMediaMime = detectMediaMime(imgBytes, imgMsg.GetMimetype(), "")
				genMediaCap = imgMsg.GetCaption()
				mu.Unlock()
				Logger.Debug("executeMetaAiQuery: successfully downloaded direct imageMessage", "len", len(imgBytes))
				closeOnce.Do(func() { close(done) })
				return
			}
		}

		// 2. Direct video message
		if vidMsg := msgEvt.Message.GetVideoMessage(); vidMsg != nil {
			Logger.Debug("executeMetaAiQuery: captured direct videoMessage from Meta AI", "chat", chatKey)
			vidBytes, err := client.Download(ctx, vidMsg)
			if err == nil && len(vidBytes) > 0 {
				genMediaData = vidBytes
				genMediaMime = detectMediaMime(vidBytes, vidMsg.GetMimetype(), "")
				genMediaCap = vidMsg.GetCaption()
				mu.Unlock()
				Logger.Debug("executeMetaAiQuery: successfully downloaded direct videoMessage", "len", len(vidBytes))
				closeOnce.Do(func() { close(done) })
				return
			}
		}

		// 3. Direct document media
		if docMsg := msgEvt.Message.GetDocumentMessage(); docMsg != nil {
			docMime := docMsg.GetMimetype()
			if strings.HasPrefix(docMime, "image/") || strings.HasPrefix(docMime, "video/") {
				docBytes, err := client.Download(ctx, docMsg)
				if err == nil && len(docBytes) > 0 {
					genMediaData = docBytes
					genMediaMime = detectMediaMime(docBytes, docMime, "")
					genMediaCap = docMsg.GetCaption()
					mu.Unlock()
					Logger.Debug("executeMetaAiQuery: successfully downloaded direct document media", "len", len(docBytes))
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}

		// 4. Protocol message edits for direct attachments
		if pm != nil && pm.GetEditedMessage() != nil {
			edited := pm.GetEditedMessage()
			if imgMsg := edited.GetImageMessage(); imgMsg != nil {
				imgBytes, err := client.Download(ctx, imgMsg)
				if err == nil && len(imgBytes) > 0 {
					genMediaData = imgBytes
					genMediaMime = detectMediaMime(imgBytes, imgMsg.GetMimetype(), "")
					genMediaCap = imgMsg.GetCaption()
					mu.Unlock()
					closeOnce.Do(func() { close(done) })
					return
				}
			}
			if vidMsg := edited.GetVideoMessage(); vidMsg != nil {
				vidBytes, err := client.Download(ctx, vidMsg)
				if err == nil && len(vidBytes) > 0 {
					genMediaData = vidBytes
					genMediaMime = detectMediaMime(vidBytes, vidMsg.GetMimetype(), "")
					genMediaCap = vidMsg.GetCaption()
					mu.Unlock()
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}

		// 5. UnifiedResponse media & state inspection
		mediaURL, mimeType, imgCap, imagineType, mediaStatus := parseUnifiedMediaState(msgEvt.Message)
		if mediaURL == "" && pm != nil {
			mediaURL, mimeType, imgCap, imagineType, mediaStatus = parseUnifiedMediaState(pm.GetEditedMessage())
		}

		if imagineType == "ANIMATE" || imagineType == "IMAGINE" || mediaStatus == "GENERATING" {
			isMediaExpected = true
		}

		if mediaURL != "" {
			req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "WhatsApp/2.24.1.76 A")
				resp, err := http.DefaultClient.Do(req)
				if err == nil && resp.StatusCode == 200 {
					mediaBytes, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err == nil && len(mediaBytes) > 0 {
						detectedMime := detectMediaMime(mediaBytes, mimeType, mediaURL)
						genMediaData = mediaBytes
						genMediaMime = detectedMime
						if genMediaCap == "" {
							genMediaCap = imgCap
						}
						Logger.Debug("executeMetaAiQuery: downloaded generated media", "len", len(mediaBytes), "mime", detectedMime)
						mu.Unlock()
						closeOnce.Do(func() { close(done) })
						return
					}
				}
			}
		}

		var text string
		if pm == nil {
			text = ExtractMetaAiText(msgEvt.Message)
		} else {
			text = ExtractMetaAiText(pm.GetEditedMessage())
		}

		editType := string(msgEvt.Info.MsgBotInfo.EditType)
		if text != "" {
			Logger.Debug("executeMetaAiQuery: update", "chat", chatKey, "edit_type", editType, "text", text)
			if _, _, isRunCmd := ParseRunCommand(text); isRunCmd {
				final = text
				if editType == "last" || editType == "inner" {
					Logger.Debug("executeMetaAiQuery: RUN_COMMAND captured", "chat", chatKey, "cmd_text", text, "edit_type", editType)
					if editType == "last" {
						mu.Unlock()
						closeOnce.Do(func() { close(done) })
						return
					}
				}
			} else if onUpdate != nil {
				if err := onUpdate(text); err != nil {
					Logger.Error("executeMetaAiQuery: onUpdate callback failed", "chat", chatKey, "err", err)
				}
			}
		}

		if text != "" && final == "" {
			final = text
		}

		lower := strings.ToLower(final + " " + text)
		if strings.Contains(lower, "image") || strings.Contains(lower, "video") ||
			strings.Contains(lower, "picture") || strings.Contains(lower, "photo") ||
			strings.Contains(lower, "clip") || strings.Contains(lower, "animation") ||
			strings.Contains(lower, "creating") || strings.Contains(lower, "generating") ||
			strings.Contains(lower, "here is your") || strings.Contains(lower, "here you go") {
			isMediaExpected = true
		}

		if len(genMediaData) > 0 {
			mu.Unlock()
			closeOnce.Do(func() { close(done) })
			return
		}

		if mediaStatus == "FAILED" || mediaStatus == "ERROR" {
			Logger.Warn("executeMetaAiQuery: media generation failed on server", "chat", chatKey, "status", mediaStatus)
			mu.Unlock()
			closeOnce.Do(func() { close(done) })
			return
		}

		if (editType == "last" || editType == "full") && !isMediaExpected {
			mu.Unlock()
			closeOnce.Do(func() { close(done) })
			return
		}

		mu.Unlock()
	})
	defer client.RemoveEventHandler(handlerID)

	select {
	case <-ctx.Done():
		Logger.Warn("executeMetaAiQuery: context cancelled/timed out before completion", "chat", chatKey, "err", ctx.Err())
		return MetaAiResult{}, ctx.Err()
	case <-time.After(50 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		Logger.Debug("executeMetaAiQuery: max timeout reached, returning gathered result", "chat", chatKey, "final_text_len", len(final), "media_len", len(genMediaData))
		return MetaAiResult{
			Text:           final,
			GeneratedMedia: genMediaData,
			MediaMimeType:  genMediaMime,
			MediaCaption:   genMediaCap,
			GeneratedImg:   genMediaData,
			ImgMimeType:    genMediaMime,
			ImgCaption:     genMediaCap,
		}, nil
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		Logger.Debug("executeMetaAiQuery: completed", "chat", chatKey, "final_text_len", len(final), "media_len", len(genMediaData), "media_mime", genMediaMime)
		return MetaAiResult{
			Text:           final,
			GeneratedMedia: genMediaData,
			MediaMimeType:  genMediaMime,
			MediaCaption:   genMediaCap,
			GeneratedImg:   genMediaData,
			ImgMimeType:    genMediaMime,
			ImgCaption:     genMediaCap,
		}, nil
	}
}

func QueryMetaAi(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (MetaAiResult, error) {
	chatKey := chat.String()
	q := getOrCreateMetaAiQueue(chatKey)

	req := metaAiRequest{
		ctx:      ctx,
		client:   client,
		chat:     chat,
		request:  request,
		onUpdate: onUpdate,
		resCh:    make(chan metaAiResponse, 1),
	}

	select {
	case q <- req:
	case <-ctx.Done():
		return MetaAiResult{}, ctx.Err()
	}

	select {
	case res := <-req.resCh:
		return res.res, res.err
	case <-ctx.Done():
		return MetaAiResult{}, ctx.Err()
	}
}

type CSAITrait struct {
	Name        string
	Instruction string
}

var DefaultCSAITraits = []CSAITrait{
	{Name: "Professional", Instruction: "Be formal, objective, concise, and highly professional in all responses."},
	{Name: "Friendly & Warm", Instruction: "Be extremely friendly, encouraging, warm, and approachable in tone."},
	{Name: "Sarcastic & Witty", Instruction: "Use playful sarcasm, humor, clever retorts, and witty banter in all interactions."},
	{Name: "Scientific & Precise", Instruction: "Respond with deep technical accuracy, scientific precision, and analytical depth."},
	{Name: "Poetic & Creative", Instruction: "Use eloquent, expressive, poetic, and creative language when answering."},
	{Name: "Motivational Coach", Instruction: "Act as an energetic, inspiring, and relentless motivational coach."},
	{Name: "Pirate", Instruction: "Speak like a pirate using nautical slang, 'Ahoy', 'Matey', and maritime flair."},
	{Name: "Gen-Z & Trendy", Instruction: "Use modern Gen-Z slang, casual expressions, and trendy internet vibe."},
	{Name: "Philosophical Thinker", Instruction: "Reflect deeply on questions, offering thoughtful, philosophical perspectives."},
	{Name: "Strict Sudo Assistant", Instruction: "Treat Sudoers with utmost authority and honor, addressing them respectfully as Master/Boss while serving all requests strictly."},
}
