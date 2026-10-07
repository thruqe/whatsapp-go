package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
	"whatsrook/pkg/addons/sdk"
)

var knownLangs = map[string]bool{
	"af": true, "ar": true, "bg": true, "bn": true, "bs": true, "ca": true, "cs": true, "da": true,
	"de": true, "el": true, "en": true, "es": true, "et": true, "fi": true, "fr": true, "gu": true,
	"hi": true, "hr": true, "hu": true, "id": true, "is": true, "it": true, "ja": true, "jv": true,
	"kn": true, "ko": true, "la": true, "lv": true, "ml": true, "mr": true, "ms": true, "my": true,
	"ne": true, "nl": true, "no": true, "pl": true, "pt": true, "ro": true, "ru": true, "si": true,
	"sk": true, "sq": true, "sr": true, "su": true, "sv": true, "sw": true, "ta": true, "te": true,
	"th": true, "tl": true, "tr": true, "uk": true, "ur": true, "vi": true, "zh": true, "zh-cn": true,
	"zh-tw": true,
}

func main() {
	req := sdk.Load()
	textToSpeak := req.Query()

	if textToSpeak == "" {
		if qText := req.QuotedText(); qText != "" {
			textToSpeak = strings.TrimSpace(qText)
		}
	}

	if textToSpeak == "" {
		p := req.EffectivePrefix()
		sdk.Respond(fmt.Sprintf(
			"Usage: %stts <text> or %stts <lang_code> <text>\n\nExamples:\n• %stts Hello world!\n• %stts es Hola, ¿cómo estás?\n• %stts fr Bonjour tout le monde",
			p, p, p, p, p,
		))
		return
	}

	lang := "en"
	if len(req.Args) > 1 {
		firstWord := strings.ToLower(req.Args[0])
		if knownLangs[firstWord] {
			lang = firstWord
			offset := len(req.Args[0])
			textToSpeak = strings.TrimSpace(req.RawArgs[offset:])
		}
	}

	if textToSpeak == "" {
		sdk.Respond("Please provide text to convert to speech.")
		return
	}

	// Limit length
	if utf8.RuneCountInString(textToSpeak) > 500 {
		runes := []rune(textToSpeak)
		textToSpeak = string(runes[:500])
	}

	ttsURL := fmt.Sprintf(
		"https://translate.google.com/translate_tts?ie=UTF-8&total=1&idx=0&textlen=%d&client=tw-ob&q=%s&tl=%s",
		utf8.RuneCountInString(textToSpeak),
		url.QueryEscape(textToSpeak),
		lang,
	)

	client := sdk.CreateHTTPClient(15)
	httpReq, err := http.NewRequest(http.MethodGet, ttsURL, nil)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to construct TTS request: %v", err))
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")
	httpReq.Header.Set("Referer", "http://translate.google.com/")

	resp, err := client.Do(httpReq)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Network error generating TTS: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sdk.RespondErr(fmt.Sprintf("Google TTS service returned status: %s", resp.Status))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil || len(audioBytes) == 0 {
		sdk.RespondErr("Failed to read TTS audio stream.")
	}

	dataURL := sdk.ToDataURL("audio/mp3", sdk.EncodeBase64(audioBytes))
	_ = sdk.SendAudio(dataURL, true)
}
