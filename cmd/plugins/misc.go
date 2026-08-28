package plugins

import (
	"encoding/hex"

	"github.com/skip2/go-qrcode"
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
		Name:        "qrcode",
		Alias:       "qr",
		Description: "Generate a QR code image for a text or URL",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleQRCode,
	})
	Register(&Command{
		Name:        "stkinfo",
		Alias:       "stickerinfo",
		Description: "View technical metadata for a replied sticker",
		Category:    "misc",
		IsPublic:    true,
		Handler:     handleStickerInfo,
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
