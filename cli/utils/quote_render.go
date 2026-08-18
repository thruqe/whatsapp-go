package cliutils

import (
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fogleman/gg"
)

type QuoteColorScheme struct {
	Background string
	BubbleBg   string
	QuotedBg   string
	AccentBar  string
	NameColor  string
	PhoneColor string
	TextColor  string
	QuotedName string
	QuotedText string
	TimeColor  string
}

func DefaultQuoteScheme() QuoteColorScheme {
	return QuoteColorScheme{
		Background: "", // transparent canvas
		BubbleBg:   "#FFFFFF",
		QuotedBg:   "#F0F0F0",
		AccentBar:  "#25D366", // WhatsApp green
		NameColor:  "#128C7E", // WhatsApp teal
		PhoneColor: "#757575",
		TextColor:  "#111111",
		QuotedName: "#128C7E",
		QuotedText: "#555555",
		TimeColor:  "#9E9E9E",
	}
}

type QuoteMessage struct {
	Username   string
	UserPhone  string
	AvatarPath string
	Quoted     bool
	QuotedName string
	QuotedText string
	Content    string
	Timestamp  string
}

var (
	quoteRegularFontPath string
	quoteBoldFontPath    string
	quoteCJKFontPath     string
)

func resolveQuoteFonts() error {
	candidatesRegular := []string{
		"cli/resources/fonts/static/Roboto-Regular.ttf",
		"resources/fonts/static/Roboto-Regular.ttf",
		"cli/resources/fonts/Roboto-VariableFont_wdth,wght.ttf",
		"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
	}
	candidatesBold := []string{
		"cli/resources/fonts/static/Roboto-Bold.ttf",
		"resources/fonts/static/Roboto-Bold.ttf",
		"cli/resources/fonts/static/Roboto-SemiBold.ttf",
		"/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	}

	for _, p := range candidatesRegular {
		if _, err := os.Stat(p); err == nil {
			quoteRegularFontPath, _ = filepath.Abs(p)
			break
		}
	}
	for _, p := range candidatesBold {
		if _, err := os.Stat(p); err == nil {
			quoteBoldFontPath, _ = filepath.Abs(p)
			break
		}
	}

	if quoteRegularFontPath == "" {
		return fmt.Errorf("regular font not found")
	}
	if quoteBoldFontPath == "" {
		quoteBoldFontPath = quoteRegularFontPath
	}

	cjkCandidates := []string{
		"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/google-noto-sans-cjk-jp-fonts/NotoSansCJKjp-Regular.otf",
		"/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	}
	for _, c := range cjkCandidates {
		if _, err := os.Stat(c); err == nil {
			quoteCJKFontPath = c
			break
		}
	}

	return nil
}

func isLatinCoverable(r rune) bool {
	switch {
	case r < 0x0530:
		return true
	case unicode.IsPunct(r), unicode.IsSpace(r), unicode.IsSymbol(r):
		return true
	case r >= 0x2000 && r <= 0x2BFF:
		return true
	default:
		return false
	}
}

type textRun struct {
	text   string
	useCJK bool
}

func splitRuns(s string) []textRun {
	var runs []textRun
	var cur []rune
	curCJK := false
	first := true

	flush := func() {
		if len(cur) > 0 {
			runs = append(runs, textRun{text: string(cur), useCJK: curCJK})
			cur = nil
		}
	}

	for _, r := range s {
		needsCJK := !isLatinCoverable(r)
		if first {
			curCJK = needsCJK
			first = false
		} else if needsCJK != curCJK {
			flush()
			curCJK = needsCJK
		}
		cur = append(cur, r)
	}
	flush()
	return runs
}

func drawMixedString(dc *gg.Context, s string, x, y float64, size float64, bold bool) float64 {
	basePath := quoteRegularFontPath
	if bold {
		basePath = quoteBoldFontPath
	}
	cursorX := x
	for _, run := range splitRuns(s) {
		facePath := basePath
		if run.useCJK && quoteCJKFontPath != "" {
			facePath = quoteCJKFontPath
		}
		_ = dc.LoadFontFace(facePath, size)
		dc.DrawString(run.text, cursorX, y)
		w, _ := dc.MeasureString(run.text)
		cursorX += w
	}
	return cursorX - x
}

func measureMixedString(dc *gg.Context, s string, size float64, bold bool) float64 {
	basePath := quoteRegularFontPath
	if bold {
		basePath = quoteBoldFontPath
	}
	total := 0.0
	for _, run := range splitRuns(s) {
		facePath := basePath
		if run.useCJK && quoteCJKFontPath != "" {
			facePath = quoteCJKFontPath
		}
		_ = dc.LoadFontFace(facePath, size)
		w, _ := dc.MeasureString(run.text)
		total += w
	}
	return total
}

func formatPhone(raw string) string {
	if raw == "" {
		return ""
	}
	hasPlus := strings.HasPrefix(raw, "+")
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, raw)
	if digits == "" {
		return raw
	}

	oneDigit := map[byte]bool{'1': true, '7': true}
	twoDigitPrefixes := map[string]bool{
		"20": true, "27": true, "30": true, "31": true, "32": true, "33": true,
		"34": true, "36": true, "39": true, "40": true, "41": true, "43": true,
		"44": true, "45": true, "46": true, "47": true, "48": true, "49": true,
		"51": true, "52": true, "53": true, "54": true, "55": true, "56": true,
		"57": true, "58": true, "60": true, "61": true, "62": true, "63": true,
		"64": true, "65": true, "66": true, "81": true, "82": true, "84": true,
		"86": true, "90": true, "91": true, "92": true, "93": true, "94": true,
		"95": true, "98": true,
	}

	ccLen := 3
	if len(digits) >= 1 && oneDigit[digits[0]] {
		ccLen = 1
	} else if len(digits) >= 2 && twoDigitPrefixes[digits[:2]] {
		ccLen = 2
	}
	if ccLen >= len(digits) {
		ccLen = 1
	}

	cc := digits[:ccLen]
	rest := digits[ccLen:]

	var groups []string
	for len(rest) > 4 {
		groups = append(groups, rest[:3])
		rest = rest[3:]
	}
	if len(rest) > 0 {
		groups = append(groups, rest)
	}

	prefix := ""
	if hasPlus {
		prefix = "+"
	}
	return fmt.Sprintf("%s%s %s", prefix, cc, strings.Join(groups, "-"))
}

var pastelPalette = []string{
	"#AEE1F9",
	"#B8F2C9",
	"#FDE2B8",
	"#E8C7F9",
	"#FFD6E0",
	"#C7F0EB",
	"#FFF3B0",
}

func pickAvatarColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	idx := int(h.Sum32()) % len(pastelPalette)
	if idx < 0 {
		idx += len(pastelPalette)
	}
	return pastelPalette[idx]
}

func firstLetter(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

func drawAvatar(dc *gg.Context, msg QuoteMessage, cx, cy, radius float64) {
	if msg.AvatarPath != "" {
		if img, err := gg.LoadImage(msg.AvatarPath); err == nil {
			dc.Push()
			dc.DrawCircle(cx, cy, radius)
			dc.Clip()
			dc.DrawImageAnchored(resizeToFit(img, int(radius*2), int(radius*2)), int(cx), int(cy), 0.5, 0.5)
			dc.ResetClip()
			dc.Pop()
			return
		}
	}

	bg := pickAvatarColor(msg.Username)
	dc.SetColor(hexToRGBA(bg))
	dc.DrawCircle(cx, cy, radius)
	dc.Fill()

	letter := firstLetter(msg.Username)
	dc.SetColor(color.RGBA{40, 40, 40, 255})
	_ = dc.LoadFontFace(quoteBoldFontPath, radius)
	tw, th := dc.MeasureString(letter)
	dc.DrawString(letter, cx-tw/2, cy+th/2-2)
}

func resizeToFit(img image.Image, w, h int) image.Image {
	dc := gg.NewContext(w, h)
	sx := float64(w) / float64(img.Bounds().Dx())
	sy := float64(h) / float64(img.Bounds().Dy())
	dc.Scale(sx, sy)
	dc.DrawImage(img, 0, 0)
	return dc.Image()
}

func drawBubbleTail(dc *gg.Context, bubbleX, bubbleY float64, fillColor color.Color) {
	dc.SetColor(fillColor)
	dc.MoveTo(bubbleX, bubbleY+4)
	dc.LineTo(bubbleX-10, bubbleY+18)
	dc.LineTo(bubbleX, bubbleY+28)
	dc.ClosePath()
	dc.Fill()
}

func hexToRGBA(hex string) color.Color {
	if hex == "" {
		return color.Transparent
	}
	c, err := parseHexColor(hex)
	if err != nil {
		return color.Black
	}
	return c
}

func parseHexColor(s string) (color.RGBA, error) {
	var r, g, b uint8
	_, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}, err
}

const (
	quoteMaxCanvasW = 1000
	quoteMinCanvasW = 480

	quoteMaxLines = 14

	quotePaddingX      = 28
	quotePaddingY      = 24.0
	quoteShadowOffset  = 5.0
	quoteAvatarRadius  = 38.0
	quoteAvatarGap     = 18.0
	quoteBubblePadding = 26
	quoteBubbleRadius  = 18.0

	quoteFontSize      = 26
	quoteNameFontSize  = 27
	quoteSmallFontSize = 21
	quoteLineHeight    = 34.0
	quoteNameRowH      = 36.0
)

// RenderQuote creates a transparent-background PNG of a chat-style quote bubble.
func RenderQuote(ctx context.Context, msg QuoteMessage, scheme QuoteColorScheme) (image.Image, error) {
	if err := resolveQuoteFonts(); err != nil {
		return nil, fmt.Errorf("font resolution error: %w", err)
	}

	msg.Username = NormalizeFancyText(msg.Username)
	msg.QuotedName = NormalizeFancyText(msg.QuotedName)
	msg.QuotedText = NormalizeFancyText(msg.QuotedText)
	msg.Content = NormalizeFancyText(msg.Content)

	formattedPhone := formatPhone(msg.UserPhone)
	displayName := "~ " + msg.Username

	bubbleXStart := float64(quotePaddingX) + quoteAvatarRadius*2 + quoteAvatarGap
	maxCanvasInnerW := float64(quoteMaxCanvasW) - bubbleXStart - float64(quotePaddingX) - float64(quoteBubblePadding)*2

	probe := gg.NewContext(quoteMaxCanvasW, 10)

	// ── 1. Measure quoted block ───────────────────────────────────────────────
	quoteHeight := 0.0
	var quoteLines []string
	if msg.Quoted && msg.QuotedText != "" {
		_ = probe.LoadFontFace(quoteRegularFontPath, quoteSmallFontSize)
		quoteLines = probe.WordWrap(msg.QuotedText, maxCanvasInnerW-36)
		if len(quoteLines) > 4 {
			quoteLines = quoteLines[:4]
			quoteLines[3] = quoteLines[3] + " …"
		}
		quoteHeight = 32 + float64(len(quoteLines))*24 + 14
	}

	// ── 2. Wrap content at max width, then truncate ───────────────────────────
	_ = probe.LoadFontFace(quoteRegularFontPath, quoteFontSize)
	contentLines := probe.WordWrap(msg.Content, maxCanvasInnerW)

	truncated := len(contentLines) > quoteMaxLines
	if truncated {
		contentLines = contentLines[:quoteMaxLines]
		suffix := " … read more"
		last := contentLines[len(contentLines)-1]
		for {
			testW := measureMixedString(probe, last+suffix, quoteFontSize, false)
			if testW <= maxCanvasInnerW || len(last) == 0 {
				break
			}
			i := strings.LastIndex(strings.TrimRight(last, " "), " ")
			if i < 0 {
				break
			}
			last = last[:i]
		}
		contentLines[len(contentLines)-1] = last + suffix
	}

	// ── 3. Determine responsive canvas width ─────────────────────────────────
	maxContentW := 0.0
	for _, line := range contentLines {
		w := measureMixedString(probe, line, quoteFontSize, false)
		if w > maxContentW {
			maxContentW = w
		}
	}

	nameW := measureMixedString(probe, displayName, quoteNameFontSize, true)
	phoneW := 0.0
	if formattedPhone != "" {
		phoneW = measureMixedString(probe, formattedPhone, quoteNameFontSize-4, false) + 28
	}
	nameRowW := nameW + phoneW

	maxQuotedW := 0.0
	if msg.Quoted && msg.QuotedText != "" {
		qNameW := measureMixedString(probe, msg.QuotedName, quoteSmallFontSize, true) + 36
		if qNameW > maxQuotedW {
			maxQuotedW = qNameW
		}
		for _, ql := range quoteLines {
			w := measureMixedString(probe, ql, quoteSmallFontSize, false) + 36
			if w > maxQuotedW {
				maxQuotedW = w
			}
		}
	}

	idealInnerW := math.Max(maxContentW, math.Max(nameRowW, maxQuotedW))
	idealInnerW = math.Max(idealInnerW, 260.0)
	idealInnerW = math.Min(idealInnerW, maxCanvasInnerW)

	bubbleW := idealInnerW + float64(quoteBubblePadding)*2
	canvasW := int(math.Ceil(math.Max(
		bubbleXStart+bubbleW+float64(quotePaddingX),
		float64(quoteMinCanvasW),
	)))

	// Re-wrap both contentLines and quoteLines at idealInnerW
	if idealInnerW < maxCanvasInnerW {
		_ = probe.LoadFontFace(quoteRegularFontPath, quoteFontSize)
		contentLines = probe.WordWrap(msg.Content, idealInnerW)
		if len(contentLines) > quoteMaxLines {
			contentLines = contentLines[:quoteMaxLines]
			suffix := " … read more"
			last := contentLines[len(contentLines)-1]
			for {
				testW := measureMixedString(probe, last+suffix, quoteFontSize, false)
				if testW <= idealInnerW || len(last) == 0 {
					break
				}
				i := strings.LastIndex(strings.TrimRight(last, " "), " ")
				if i < 0 {
					break
				}
				last = last[:i]
			}
			contentLines[len(contentLines)-1] = last + suffix
		}

		if msg.Quoted && msg.QuotedText != "" {
			_ = probe.LoadFontFace(quoteRegularFontPath, quoteSmallFontSize)
			quoteLines = probe.WordWrap(msg.QuotedText, idealInnerW-36)
			if len(quoteLines) > 4 {
				quoteLines = quoteLines[:4]
				quoteLines[3] = quoteLines[3] + " …"
			}
			quoteHeight = 32 + float64(len(quoteLines))*24 + 14
		}
	}

	// ── 4. Layout heights ────────────────────────────────────────────────────
	contentHeight := float64(len(contentLines)) * quoteLineHeight
	bubbleHeight := float64(quoteBubblePadding)*2 + quoteNameRowH + quoteHeight + contentHeight + 30

	totalHeight := int(math.Ceil(math.Max(
		quotePaddingY+bubbleHeight+quotePaddingY+quoteShadowOffset,
		quoteAvatarRadius*2+48,
	)))

	// ── 5. Draw ──────────────────────────────────────────────────────────────
	dc := gg.NewContext(canvasW, totalHeight)
	if scheme.Background != "" {
		dc.SetColor(hexToRGBA(scheme.Background))
		dc.Clear()
	}

	bubbleY := quotePaddingY
	avatarCX := float64(quotePaddingX) + quoteAvatarRadius
	avatarCY := bubbleY + bubbleHeight/2

	drawAvatar(dc, msg, avatarCX, avatarCY, quoteAvatarRadius)

	// Drop shadow
	dc.SetColor(color.RGBA{0, 0, 0, 38})
	dc.DrawRoundedRectangle(bubbleXStart+quoteShadowOffset, bubbleY+quoteShadowOffset, bubbleW, bubbleHeight, quoteBubbleRadius)
	dc.Fill()

	// Bubble background (white / light mode)
	dc.SetColor(hexToRGBA(scheme.BubbleBg))
	dc.DrawRoundedRectangle(bubbleXStart, bubbleY, bubbleW, bubbleHeight, quoteBubbleRadius)
	dc.Fill()

	// Subtle border for definition
	dc.SetColor(color.RGBA{215, 215, 215, 255})
	dc.SetLineWidth(1.5)
	dc.DrawRoundedRectangle(bubbleXStart, bubbleY, bubbleW, bubbleHeight, quoteBubbleRadius)
	dc.Stroke()

	// Bubble tail
	drawBubbleTail(dc, bubbleXStart, bubbleY, hexToRGBA(scheme.BubbleBg))

	textX := bubbleXStart + float64(quoteBubblePadding)
	textY := bubbleY + float64(quoteBubblePadding)
	rightEdge := bubbleXStart + bubbleW - float64(quoteBubblePadding)

	// Name
	dc.SetColor(hexToRGBA(scheme.NameColor))
	drawMixedString(dc, displayName, textX, textY+20, quoteNameFontSize, true)

	// Phone
	if formattedPhone != "" {
		dc.SetColor(hexToRGBA(scheme.PhoneColor))
		pw := measureMixedString(dc, formattedPhone, quoteNameFontSize-4, false)
		drawMixedString(dc, formattedPhone, rightEdge-pw, textY+20, quoteNameFontSize-4, false)
	}

	textY += quoteNameRowH

	// Quoted block
	if msg.Quoted && msg.QuotedText != "" {
		qBlockW := bubbleW - float64(quoteBubblePadding)*2 + 12
		dc.SetColor(hexToRGBA(scheme.QuotedBg))
		dc.DrawRoundedRectangle(textX-6, textY-4, qBlockW, quoteHeight+4, 8)
		dc.Fill()

		dc.SetColor(hexToRGBA(scheme.AccentBar))
		dc.DrawRectangle(textX, textY, 4, quoteHeight-6)
		dc.Fill()

		quotedName := msg.QuotedName
		if quotedName == "" {
			quotedName = "Quoted User"
		}
		dc.SetColor(hexToRGBA(scheme.QuotedName))
		drawMixedString(dc, quotedName, textX+18, textY+20, quoteSmallFontSize, true)

		dc.SetColor(hexToRGBA(scheme.QuotedText))
		qy := textY + 44
		for _, line := range quoteLines {
			drawMixedString(dc, line, textX+18, qy, quoteSmallFontSize, false)
			qy += 24
		}
		textY += quoteHeight + 10
	}

	// Message content
	dc.SetColor(hexToRGBA(scheme.TextColor))
	cy := textY + float64(quoteFontSize)
	for _, line := range contentLines {
		drawMixedString(dc, line, textX, cy, quoteFontSize, false)
		cy += quoteLineHeight
	}

	// Timestamp
	if msg.Timestamp != "" {
		dc.SetColor(hexToRGBA(scheme.TimeColor))
		_ = dc.LoadFontFace(quoteRegularFontPath, 16)
		tw, _ := dc.MeasureString(msg.Timestamp)
		dc.DrawString(msg.Timestamp, bubbleXStart+bubbleW-float64(quoteBubblePadding)-tw, bubbleY+bubbleHeight-12)
	}

	return dc.Image(), nil
}
