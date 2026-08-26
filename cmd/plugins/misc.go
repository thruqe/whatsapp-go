package plugins

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"strings"
	cliutils "whatsrook/cmd/utils"
	"whatsrook/logger"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "save",
		Alias:       "savestatus",
		Description: "Forward a replied message to your DM (or save status)",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleSave,
	})
	Register(&Command{
		Name:        "weather",
		Alias:       "forecast",
		Description: "Check real-time weather forecast for a city or town. Usage: weather [city]",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleWeather,
	})
	Register(&Command{
		Name:        "urban",
		Alias:       "ud",
		Description: "Look up a word or phrase on Urban Dictionary",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleUrban,
	})
	Register(&Command{
		Name:        "qrcode",
		Alias:       "qr",
		Description: "Generate a QR code image for a text or URL",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleQRCode,
	})
	Register(&Command{
		Name:        "shorturl",
		Alias:       "shorten",
		Description: "Shorten a long URL using TinyURL",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleShortURL,
	})
	Register(&Command{
		Name:        "stkinfo",
		Alias:       "stickerinfo",
		Description: "View technical metadata for a replied sticker",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleStickerInfo,
	})
	Register(&Command{
		Name:        "calc",
		Alias:       "math",
		Description: "Evaluate a mathematical expression. Usage: calc [expression]",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleCalc,
	})
}

func handleSave(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ctx.Reply("The basic functionality of this command is to save status updates. Please reply to a status update or any message to forward it to your DM.")
	}

	if ctx.Client.Store.ID == nil {
		return ctx.Reply("Owner ID unavailable.")
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Sender, quoted)
	if err != nil {
		return ctx.Replyf("Failed to forward message: %v", err)
	}

	return ctx.Reply("Message forwarded to your DM.")
}

func handleWeather(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: weather [city/town]")
	}

	query := strings.Join(ctx.Args, " ")
	escapedQuery := url.QueryEscape(query)
	apiURL := Sprintf("https://wttr.in/%s?format=4", escapedQuery)

	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Replyf("Error creating request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Replyf("Network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Replyf("Weather service returned status: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Replyf("Error reading response: %v", err)
	}

	forecast := strings.TrimSpace(string(bodyBytes))
	if forecast == "" || strings.Contains(forecast, "Unknown location") {
		return ctx.Replyf("Could not find weather info for %q.", query)
	}

	return ctx.Reply(forecast)
}

func handleUrban(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: urban [term]")
	}

	query := strings.Join(ctx.Args, " ")
	apiURL := Sprintf("https://api.urbandictionary.com/v0/define?term=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Replyf("Error creating request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Replyf("Network error: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		List []struct {
			Word       string `json:"word"`
			Definition string `json:"definition"`
			Example    string `json:"example"`
			Author     string `json:"author"`
			Permalink  string `json:"permalink"`
		} `json:"list"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.List) == 0 {
		return ctx.Replyf("Could not find Urban Dictionary definition for %q.", query)
	}

	def := result.List[0]
	cleanDef := strings.ReplaceAll(strings.ReplaceAll(def.Definition, "[", ""), "]", "")
	cleanExample := strings.ReplaceAll(strings.ReplaceAll(def.Example, "[", ""), "]", "")

	tb := ctx.Text().
		Header("Urban Dictionary: " + def.Word).
		Section("Definition:").
		Line(cleanDef)

	if cleanExample != "" {
		tb.Blank().Section("Example:").Line(cleanExample)
	}
	if def.Author != "" {
		tb.Blank().Line("Author: " + def.Author)
	}

	out := tb.String()
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: new(out),
		},
	})

	if err != nil {
		Logger.Debug("urban dictionary error", "error", err.Error())
	}
	return nil
}

func handleQRCode(ctx *Context) error {
	query := ctx.RawArgs
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = extractTextFromProto(quoted)
		}
	}
	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %sqr [text or url] (or reply to a message)", p)
	}

	pngBytes, err := qrcode.Encode(query, qrcode.Medium, 500)
	if err != nil {
		return ctx.Replyf("Error generating QR code: %v", err)
	}

	return ctx.ReplyWithImage(pngBytes, "image/png", "QR Code Generated")
}

func handleShortURL(ctx *Context) error {
	query := ctx.RawArgs
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = extractTextFromProto(quoted)
		}
	}
	query = strings.TrimSpace(query)
	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %sshorturl [url]", p)
	}

	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		query = "https://" + query
	}

	apiURL := Sprintf("https://tinyurl.com/api-create.php?url=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", apiURL, nil)
	if err != nil {
		return ctx.Replyf("Error creating request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ctx.Replyf("Network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ctx.Reply("Failed to shorten URL. Please check if the URL is valid.")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply("Failed to read URL shortener response.")
	}

	short := strings.TrimSpace(string(bodyBytes))
	return ctx.Reply("Shortened URL: " + short)
}

func handleStickerInfo(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("Please reply to a sticker message to view its metadata.")
	}

	stk := quoted.StickerMessage
	mime := stk.GetMimetype()
	if mime == "" {
		mime = "image/webp"
	}

	shaHex := hex.EncodeToString(stk.GetFileSHA256())
	if shaHex == "" {
		shaHex = "unknown"
	}

	length := stk.GetFileLength()
	sizeStr := Sprintf("%d bytes", length)
	if length > 1024*1024 {
		sizeStr = Sprintf("%.2f MB", float64(length)/(1024*1024))
	} else if length > 1024 {
		sizeStr = Sprintf("%.2f KB", float64(length)/1024)
	}

	isAnimated := "No"
	if stk.GetIsAnimated() {
		isAnimated = "Yes"
	}

	return ctx.Text().
		Header("Sticker Metadata").
		Field("MIME Type", mime).
		Field("File Size", sizeStr).
		Field("Animated", isAnimated).
		Field("SHA256", shaHex).
		Reply()
}

func handleCalc(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Replyf("Usage: %scalc [expression]", p)
	}

	exprStr := strings.Join(ctx.Args, "")
	val, err := cliutils.EvalMathExpr(exprStr)
	if err != nil {
		return ctx.Replyf("Math error: %v", err)
	}

	return ctx.Replyf("Result: %g", val)
}
