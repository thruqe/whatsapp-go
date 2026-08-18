package cliutils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var MetaAiBotJID = types.NewJID("867051314767696", types.BotServer)

type MetaAiResult struct {
	Text         string
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
				Typename string `json:"__typename"`
				Media    struct {
					URL      string `json:"url"`
					MimeType string `json:"mime_type"`
				} `json:"media"`
				Status struct {
					Status string `json:"status"`
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

func ExtractMetaAiText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if conv := msg.GetConversation(); conv != "" {
		return conv
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if rich := msg.GetRichResponseMessage(); rich != nil {
		var text strings.Builder
		for _, sub := range rich.GetSubmessages() {
			text.WriteString(sub.GetMessageText())
		}
		return text.String()
	}
	return ""
}

func ExtractMetaAiGeneratedImage(msg *waE2E.Message) (mediaURL string, mimeType string, text string) {
	if msg == nil {
		return "", "", ""
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
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(rawB64)))
		n, err := base64.StdEncoding.Decode(decoded, rawB64)
		if err == nil {
			var uData metaAiUnifiedData
			if err := json.Unmarshal(decoded[:n], &uData); err == nil {
				for _, sec := range uData.Sections {
					p := sec.ViewModel.Primitive
					if p.Media.URL != "" {
						mediaURL = strings.ReplaceAll(p.Media.URL, `\/`, `/`)
						mimeType = p.Media.MimeType
					}
					if p.Text != "" {
						text = p.Text
					}
				}
			}
		}
	}
	return mediaURL, mimeType, text
}

func ExecuteMetaAiQuery(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (MetaAiResult, error) {
	chatKey := chat.String()

	slog.Debug("executeMetaAiQuery: sending request", "chat", chatKey, "request", request)

	if _, err := client.SendMessage(ctx, MetaAiBotJID, &waE2E.Message{
		Conversation: new(request),
	}); err != nil {
		slog.Error("executeMetaAiQuery: failed to send request", "chat", chatKey, "err", err)
		return MetaAiResult{}, fmt.Errorf("failed to send request to meta ai: %w", err)
	}

	var (
		mu         sync.Mutex
		metaMsgID  string
		seen       bool
		finished   bool
		final      string
		genImgData []byte
		genImgMime string
		genImgCap  string
		done       = make(chan struct{})
		closeOnce  sync.Once
	)

	handlerID := client.AddEventHandler(func(evt any) {
		msgEvt, ok := evt.(*events.Message)
		if !ok || msgEvt.Info.Sender.String() != MetaAiBotJID.String() {
			return
		}

		pm := msgEvt.Message.GetProtocolMessage()

		mu.Lock()
		if finished {
			if imgMsg := msgEvt.Message.GetImageMessage(); imgMsg != nil {
				slog.Debug("executeMetaAiQuery: captured follow-up imageMessage after finished", "chat", chatKey)
				imgBytes, err := client.Download(ctx, imgMsg)
				if err == nil && len(imgBytes) > 0 {
					genImgData = imgBytes
					genImgMime = imgMsg.GetMimetype()
					if genImgMime == "" {
						genImgMime = "image/jpeg"
					}
					genImgCap = imgMsg.GetCaption()
					slog.Debug("executeMetaAiQuery: successfully downloaded follow-up imageMessage", "len", len(imgBytes))
					closeOnce.Do(func() { close(done) })
				}
			}
			mu.Unlock()
			return
		}

		if imgMsg := msgEvt.Message.GetImageMessage(); imgMsg != nil {
			slog.Debug("executeMetaAiQuery: captured direct imageMessage from Meta AI", "chat", chatKey)
			imgBytes, err := client.Download(ctx, imgMsg)
			if err == nil && len(imgBytes) > 0 {
				genImgData = imgBytes
				genImgMime = imgMsg.GetMimetype()
				if genImgMime == "" {
					genImgMime = "image/jpeg"
				}
				genImgCap = imgMsg.GetCaption()
				slog.Debug("executeMetaAiQuery: successfully downloaded direct imageMessage", "len", len(imgBytes))
				mu.Unlock()
				closeOnce.Do(func() { close(done) })
				return
			}
		}

		if !seen {
			if pm != nil {
				mu.Unlock()
				return
			}
			metaMsgID = msgEvt.Info.ID
			seen = true
			mu.Unlock()
			slog.Debug("executeMetaAiQuery: captured meta ai reply message id", "chat", chatKey, "meta_msg_id", metaMsgID)
		} else if pm == nil || pm.GetKey().GetID() != metaMsgID {
			mu.Unlock()
			return
		} else {
			mu.Unlock()
		}

		mediaURL, mimeType, imgCap := ExtractMetaAiGeneratedImage(msgEvt.Message)
		if mediaURL == "" && pm != nil {
			mediaURL, mimeType, imgCap = ExtractMetaAiGeneratedImage(pm.GetEditedMessage())
		}
		if mediaURL != "" {
			req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "WhatsApp/2.24.1.76 A")
				resp, err := http.DefaultClient.Do(req)
				if err == nil && resp.StatusCode == 200 {
					imgBytes, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err == nil && len(imgBytes) > 0 {
						mu.Lock()
						genImgData = imgBytes
						genImgMime = mimeType
						genImgCap = imgCap
						mu.Unlock()
						slog.Debug("executeMetaAiQuery: downloaded generated image", "len", len(imgBytes), "mime", mimeType)
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
		if text == "" {
			slog.Debug("executeMetaAiQuery: empty text extracted, skipping update", "chat", chatKey, "info_id", msgEvt.Info.ID)
			return
		}

		editType := string(msgEvt.Info.MsgBotInfo.EditType)
		slog.Debug("executeMetaAiQuery: update", "chat", chatKey, "edit_type", editType, "text", text)

		if _, _, isRunCmd := ParseRunCommand(text); isRunCmd {
			mu.Lock()
			final = text
			mu.Unlock()
			if editType == "last" || editType == "inner" {
				slog.Debug("executeMetaAiQuery: RUN_COMMAND captured", "chat", chatKey, "cmd_text", text, "edit_type", editType)
				if editType == "last" {
					mu.Lock()
					finished = true
					mu.Unlock()
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		} else if onUpdate != nil {
			if err := onUpdate(text); err != nil {
				slog.Error("executeMetaAiQuery: onUpdate callback failed", "chat", chatKey, "err", err)
			}
		}

		if editType == "last" {
			mu.Lock()
			if final == "" {
				final = text
			}
			finished = true
			hasImgData := len(genImgData) > 0
			mu.Unlock()

			lower := strings.ToLower(text)
			if !hasImgData && (strings.Contains(lower, "image") || strings.Contains(lower, "creating") || strings.Contains(lower, "ready")) {
				slog.Debug("executeMetaAiQuery: text indicates image generation, waiting briefly for follow-up imageMessage", "chat", chatKey)
				go func() {
					time.Sleep(4 * time.Second)
					closeOnce.Do(func() { close(done) })
				}()
			} else {
				closeOnce.Do(func() { close(done) })
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	select {
	case <-ctx.Done():
		slog.Warn("executeMetaAiQuery: context cancelled/timed out before completion", "chat", chatKey, "err", ctx.Err())
		return MetaAiResult{}, ctx.Err()
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		slog.Debug("executeMetaAiQuery: completed", "chat", chatKey, "final_text_len", len(final))
		return MetaAiResult{
			Text:         final,
			GeneratedImg: genImgData,
			ImgMimeType:  genImgMime,
			ImgCaption:   genImgCap,
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
