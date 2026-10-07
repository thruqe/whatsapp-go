package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"whatsrook/pkg/addons/sdk"
)

type fontStyle struct {
	key  string
	name string
}

var fonts = []fontStyle{
	{key: "bold", name: "Bold (Serif)"},
	{key: "italic", name: "Italic"},
	{key: "bold-italic", name: "Bold Italic"},
	{key: "double-struck", name: "Double Struck"},
	{key: "script", name: "Script"},
	{key: "bold-script", name: "Bold Script"},
	{key: "fraktur", name: "Fraktur"},
	{key: "bold-fraktur", name: "Bold Fraktur"},
	{key: "sans", name: "Sans-Serif"},
	{key: "sans-bold", name: "Sans-Serif Bold"},
	{key: "sans-italic", name: "Sans-Serif Italic"},
	{key: "sans-bold-italic", name: "Sans-Serif Bold Italic"},
	{key: "monospace", name: "Monospace / Typewriter"},
	{key: "small-caps", name: "Small Caps"},
	{key: "inverted", name: "Inverted / Upside Down"},
	{key: "circled", name: "Circled Letters"},
	{key: "squared", name: "Squared"},
	{key: "parenthesized", name: "Parenthesized"},
	{key: "fullwidth", name: "Fullwidth / Wide"},
}

var smallCapsMap = map[rune]rune{
	'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ꜰ', 'g': 'ɢ',
	'h': 'ʜ', 'i': 'ɪ', 'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ', 'm': 'ᴍ', 'n': 'ɴ',
	'o': 'ᴏ', 'p': 'ᴘ', 'q': 'ꞯ', 'r': 'ʀ', 's': 'ꜱ', 't': 'ᴛ', 'u': 'ᴜ',
	'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x', 'y': 'ʏ', 'z': 'ᴢ',
}

func convertChar(c rune, style string) string {
	switch style {
	case "bold":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D5BA))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D5A0))
		} else if c >= '0' && c <= '9' {
			return string(rune(c - '0' + 0x1D7EC))
		}
	case "italic":
		if c == 'h' {
			return "ℎ"
		} else if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D434 + 26))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D434))
		}
	case "bold-italic":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D468 + 26))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D468))
		}
	case "double-struck":
		switch c {
		case 'C':
			return "ℂ"
		case 'H':
			return "ℍ"
		case 'N':
			return "ℕ"
		case 'P':
			return "ℙ"
		case 'Q':
			return "ℚ"
		case 'R':
			return "ℝ"
		case 'Z':
			return "ℤ"
		}
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D552))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D538))
		} else if c >= '0' && c <= '9' {
			return string(rune(c - '0' + 0x1D7D8))
		}
	case "script":
		switch c {
		case 'B':
			return "ℬ"
		case 'E':
			return "ℰ"
		case 'F':
			return "ℱ"
		case 'H':
			return "ℋ"
		case 'I':
			return "ℐ"
		case 'L':
			return "ℒ"
		case 'M':
			return "ℳ"
		case 'R':
			return "ℛ"
		case 'e':
			return "ℯ"
		case 'g':
			return "ℊ"
		case 'o':
			return "ℴ"
		}
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D4B6))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D49C))
		}
	case "bold-script":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D4EA))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D4D0))
		}
	case "fraktur":
		switch c {
		case 'C':
			return "ℭ"
		case 'H':
			return "ℌ"
		case 'I':
			return "ℑ"
		case 'R':
			return "ℜ"
		case 'Z':
			return "ℨ"
		}
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D51E))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D504))
		}
	case "bold-fraktur":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D586))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D56C))
		}
	case "sans":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D5EE))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D5D4))
		} else if c >= '0' && c <= '9' {
			return string(rune(c - '0' + 0x1D7E2))
		}
	case "monospace":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1D68A))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1D670))
		} else if c >= '0' && c <= '9' {
			return string(rune(c - '0' + 0x1D7F6))
		}
	case "small-caps":
		lower := unicode.ToLower(c)
		if sc, ok := smallCapsMap[lower]; ok {
			return string(sc)
		}
	case "circled":
		if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x24D0))
		} else if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x24B6))
		} else if c >= '1' && c <= '9' {
			return string(rune(c - '1' + 0x2460))
		} else if c == '0' {
			return "⓪"
		}
	case "squared":
		if c >= 'A' && c <= 'Z' {
			return string(rune(c - 'A' + 0x1F130))
		} else if c >= 'a' && c <= 'z' {
			return string(rune(c - 'a' + 0x1F130))
		}
	case "fullwidth":
		if c >= '!' && c <= '~' {
			return string(rune(c - '!' + 0xFF01))
		}
	}
	return string(c)
}

func convertText(text, style string) string {
	var b strings.Builder
	for _, r := range text {
		b.WriteString(convertChar(r, style))
	}
	return b.String()
}

func main() {
	req := sdk.Load()
	quoted := req.QuotedText()
	query := req.Query()

	if query == "" && quoted == "" {
		var help strings.Builder
		fmt.Fprintf(&help, "*AVAILABLE FONT STYLES (1-%d)*\n\n", len(fonts))
		for i, f := range fonts {
			preview := convertText("WhatsRook Bot", f.key)
			fmt.Fprintf(&help, "%d. %s → %s\n", i+1, f.name, preview)
		}
		p := req.EffectivePrefix()
		fmt.Fprintf(&help, "\n*Usage:*\n- %sfont <number> <text>\n- %sfont <number> (as reply to a message)\n- Example: `%sfont 14 Hello World`", p, p, p)
		sdk.Respond(help.String())
		return
	}

	fontIdx := 13 // default small-caps (14th)
	textToConvert := query

	if len(req.Args) > 0 {
		if num, err := strconv.Atoi(req.Args[0]); err == nil && num >= 1 && num <= len(fonts) {
			fontIdx = num - 1
			offset := len(req.Args[0])
			textToConvert = strings.TrimSpace(req.RawArgs[offset:])
		}
	}

	if textToConvert == "" {
		textToConvert = quoted
	}

	if textToConvert == "" {
		sdk.Respond(fmt.Sprintf("Please provide text to convert.\nExample: `%sfont %d Hello World`", req.EffectivePrefix(), fontIdx+1))
		return
	}

	style := fonts[fontIdx].key
	sdk.Respond(convertText(textToConvert, style))
}
