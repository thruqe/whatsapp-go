package tools

import (
	"strings"
	"sync"

	utils "whatsrook"
)

// FormatTextResponseRaw formats a text response with monospace/active font format, removing asterisks and emojis.
func FormatTextResponseRaw(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = utils.RemoveEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return ConvertFontStyle(text)
}

// ToSmallCaps converts a string to Unicode small-caps formatting.
func ToSmallCaps(s string) string {
	return convertSegment(s, "small-caps")
}

var (
	currentStyle = "normal"
	mu           sync.RWMutex
)

// SetFontStyle sets the active font style for text conversion.
func SetFontStyle(style string) {
	mu.Lock()
	defer mu.Unlock()
	currentStyle = strings.ToLower(style)
}

// GetFontStyle returns the currently active font style name.
func GetFontStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentStyle
}

// ConvertFontStyle transforms the input string to the currently active font style,
// while preserving URLs (http:// and https://) in standard normal font.
func ConvertFontStyle(s string) string {
	style := GetFontStyle()
	if style == "" || style == "normal" {
		return s
	}

	var sb strings.Builder
	pos := 0
	for pos < len(s) {
		idx := strings.Index(s[pos:], "http://")
		idx2 := strings.Index(s[pos:], "https://")
		urlIdx := -1
		if idx != -1 && idx2 != -1 {
			if idx < idx2 {
				urlIdx = pos + idx
			} else {
				urlIdx = pos + idx2
			}
		} else if idx != -1 {
			urlIdx = pos + idx
		} else if idx2 != -1 {
			urlIdx = pos + idx2
		}

		if urlIdx == -1 {
			sb.WriteString(convertSegment(s[pos:], style))
			break
		}

		if urlIdx > pos {
			sb.WriteString(convertSegment(s[pos:urlIdx], style))
		}

		urlEnd := urlIdx
		for urlEnd < len(s) && !isURLSeparator(s[urlEnd]) {
			urlEnd++
		}

		actualURLEnd := urlEnd
		for actualURLEnd > urlIdx && isTrailingURLPunctuation(s[actualURLEnd-1]) {
			actualURLEnd--
		}

		sb.WriteString(s[urlIdx:actualURLEnd])
		if actualURLEnd < urlEnd {
			sb.WriteString(convertSegment(s[actualURLEnd:urlEnd], style))
		}

		pos = urlEnd
	}

	return sb.String()
}

func isURLSeparator(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isTrailingURLPunctuation(b byte) bool {
	return b == '.' || b == ',' || b == ')' || b == ']' || b == '>' || b == '"' || b == '\''
}

func convertSegment(s string, style string) string {
	var sb strings.Builder
	for _, r := range s {
		switch style {
		case "monospace":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D68A)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D670)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7F6)
			} else {
				sb.WriteRune(r)
			}
		case "bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "italic":
			if r >= 'a' && r <= 'z' {
				if r == 'h' {
					sb.WriteRune(0x0210E)
				} else {
					sb.WriteRune(r - 'a' + 0x1D434 + 26)
				}
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D434)
			} else {
				sb.WriteRune(r)
			}
		case "bold-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D482)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D468)
			} else {
				sb.WriteRune(r)
			}
		case "double-struck":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D552)
			} else if r >= 'A' && r <= 'Z' {
				switch r {
				case 'C':
					sb.WriteRune(0x2102)
				case 'H':
					sb.WriteRune(0x210D)
				case 'N':
					sb.WriteRune(0x2115)
				case 'P':
					sb.WriteRune(0x2119)
				case 'Q':
					sb.WriteRune(0x211A)
				case 'R':
					sb.WriteRune(0x211D)
				case 'Z':
					sb.WriteRune(0x2124)
				default:
					sb.WriteRune(r - 'A' + 0x1D538)
				}
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7D8)
			} else {
				sb.WriteRune(r)
			}
		case "script":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4EA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D4D0)
			} else {
				sb.WriteRune(r)
			}
		case "bold-script":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4B6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D49C)
			} else {
				sb.WriteRune(r)
			}
		case "fraktur":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D520)
			} else if r >= 'A' && r <= 'Z' {
				switch r {
				case 'C':
					sb.WriteRune(0x212C)
				case 'H':
					sb.WriteRune(0x210C)
				case 'I':
					sb.WriteRune(0x2111)
				case 'R':
					sb.WriteRune(0x211C)
				case 'Z':
					sb.WriteRune(0x2128)
				default:
					sb.WriteRune(r - 'A' + 0x1D504)
				}
			} else {
				sb.WriteRune(r)
			}
		case "bold-fraktur":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D586)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D56C)
			} else {
				sb.WriteRune(r)
			}
		case "sans":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D586 - 52)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0 - 52)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7E2)
			} else {
				sb.WriteRune(r)
			}
		case "sans-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "sans-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D608)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5EE)
			} else {
				sb.WriteRune(r)
			}
		case "sans-bold-italic":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D63C)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D622)
			} else {
				sb.WriteRune(r)
			}
		case "circled":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x24D0)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x24B6)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2460)
			} else if r == '0' {
				sb.WriteRune(0x24EA)
			} else {
				sb.WriteRune(r)
			}
		case "circled-negative":
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				sb.WriteRune(r - 'A' + 0x1F150)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2776)
			} else if r == '0' {
				sb.WriteRune(0x24FF)
			} else {
				sb.WriteRune(r)
			}
		case "squared":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F130)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F130)
			} else {
				sb.WriteRune(r)
			}
		case "squared-negative":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F170)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F170)
			} else {
				sb.WriteRune(r)
			}
		case "fullwidth":
			if r >= '!' && r <= '~' {
				sb.WriteRune(r - '!' + 0xFF01)
			} else if r == ' ' {
				sb.WriteRune(0x3000)
			} else {
				sb.WriteRune(r)
			}
		case "small-caps":
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			switch r {
			case 'a':
				sb.WriteRune('ᴀ')
			case 'b':
				sb.WriteRune('ʙ')
			case 'c':
				sb.WriteRune('ᴄ')
			case 'd':
				sb.WriteRune('ᴅ')
			case 'e':
				sb.WriteRune('ᴇ')
			case 'f':
				sb.WriteRune('ғ')
			case 'g':
				sb.WriteRune('ɢ')
			case 'h':
				sb.WriteRune('ʜ')
			case 'i':
				sb.WriteRune('ɪ')
			case 'j':
				sb.WriteRune('ᴊ')
			case 'k':
				sb.WriteRune('ᴋ')
			case 'l':
				sb.WriteRune('ʟ')
			case 'm':
				sb.WriteRune('ᴍ')
			case 'n':
				sb.WriteRune('ɴ')
			case 'o':
				sb.WriteRune('ᴏ')
			case 'p':
				sb.WriteRune('ᴘ')
			case 'q':
				sb.WriteRune('ǫ')
			case 'r':
				sb.WriteRune('ʀ')
			case 's':
				sb.WriteRune('s')
			case 't':
				sb.WriteRune('ᴛ')
			case 'u':
				sb.WriteRune('ᴜ')
			case 'v':
				sb.WriteRune('ᴠ')
			case 'w':
				sb.WriteRune('ᴡ')
			case 'x':
				sb.WriteRune('x')
			case 'y':
				sb.WriteRune('ʏ')
			case 'z':
				sb.WriteRune('ᴢ')
			default:
				sb.WriteRune(r)
			}
		case "subscript":
			switch r {
			case '0':
				sb.WriteRune(0x2080)
			case '1':
				sb.WriteRune(0x2081)
			case '2':
				sb.WriteRune(0x2082)
			case '3':
				sb.WriteRune(0x2083)
			case '4':
				sb.WriteRune(0x2084)
			case '5':
				sb.WriteRune(0x2085)
			case '6':
				sb.WriteRune(0x2086)
			case '7':
				sb.WriteRune(0x2087)
			case '8':
				sb.WriteRune(0x2088)
			case '9':
				sb.WriteRune(0x2089)
			case 'a':
				sb.WriteRune(0x2090)
			case 'e':
				sb.WriteRune(0x2095)
			case 'h':
				sb.WriteRune(0x2096)
			case 'i':
				sb.WriteRune(0x1D62)
			case 'k':
				sb.WriteRune(0x2097)
			case 'l':
				sb.WriteRune(0x2098)
			case 'm':
				sb.WriteRune(0x2099)
			case 'n':
				sb.WriteRune(0x209A)
			case 'o':
				sb.WriteRune(0x2092)
			case 'p':
				sb.WriteRune(0x209B)
			case 'r':
				sb.WriteRune(0x1D63)
			case 's':
				sb.WriteRune(0x209C)
			case 't':
				sb.WriteRune(0x209D)
			case 'u':
				sb.WriteRune(0x1D64)
			case 'v':
				sb.WriteRune(0x1D65)
			case 'x':
				sb.WriteRune(0x2088)
			default:
				sb.WriteRune(r)
			}
		case "superscript":
			switch r {
			case '0':
				sb.WriteRune(0x2070)
			case '1':
				sb.WriteRune(0x00B9)
			case '2':
				sb.WriteRune(0x00B2)
			case '3':
				sb.WriteRune(0x00B3)
			case '4':
				sb.WriteRune(0x2074)
			case '5':
				sb.WriteRune(0x2075)
			case '6':
				sb.WriteRune(0x2076)
			case '7':
				sb.WriteRune(0x2077)
			case '8':
				sb.WriteRune(0x2078)
			case '9':
				sb.WriteRune(0x2079)
			case 'a':
				sb.WriteRune(0x1D43)
			case 'b':
				sb.WriteRune(0x1D47)
			case 'c':
				sb.WriteRune(0x1D48)
			case 'd':
				sb.WriteRune(0x1D49)
			case 'e':
				sb.WriteRune(0x1D4B)
			case 'f':
				sb.WriteRune(0x1D4C)
			case 'g':
				sb.WriteRune(0x1D4D)
			case 'h':
				sb.WriteRune(0x02B0)
			case 'i':
				sb.WriteRune(0x2071)
			case 'j':
				sb.WriteRune(0x02B2)
			case 'k':
				sb.WriteRune(0x1D4C)
			case 'l':
				sb.WriteRune(0x02E1)
			case 'm':
				sb.WriteRune(0x1D50)
			case 'n':
				sb.WriteRune(0x207F)
			case 'o':
				sb.WriteRune(0x1D52)
			case 'p':
				sb.WriteRune(0x1D56)
			case 'r':
				sb.WriteRune(0x02B3)
			case 's':
				sb.WriteRune(0x02E2)
			case 't':
				sb.WriteRune(0x1D57)
			case 'u':
				sb.WriteRune(0x1D58)
			case 'v':
				sb.WriteRune(0x1D5B)
			case 'w':
				sb.WriteRune(0x02B7)
			case 'x':
				sb.WriteRune(0x02E3)
			case 'y':
				sb.WriteRune(0x02B8)
			case 'z':
				sb.WriteRune(0x1D5C)
			default:
				sb.WriteRune(r)
			}
		case "parenthesized":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x249C)
			} else if r >= '1' && r <= '9' {
				sb.WriteRune(r - '1' + 0x2474)
			} else {
				sb.WriteRune(r)
			}
		case "bold-sans":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "regional-indicator":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1F1E6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1F1E6)
			} else {
				sb.WriteRune(r)
			}
		case "bold-script-alt":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4B6)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D49C)
			} else {
				sb.WriteRune(r)
			}
		case "sans-serif-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "monospace-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D68A)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D670)
			} else {
				sb.WriteRune(r)
			}
		case "double-struck-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D552)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D538)
			} else {
				sb.WriteRune(r)
			}
		case "circled-bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x24D0)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x24B6)
			} else {
				sb.WriteRune(r)
			}
		case "squared-bold":
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				if r >= 'a' && r <= 'z' {
					r -= 'a' - 'A'
				}
				sb.WriteRune(r - 'A' + 0x1F130)
			} else {
				sb.WriteRune(r)
			}
		case "small-caps-alt":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 'A')
			} else {
				sb.WriteRune(r)
			}
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

type FontEntry struct {
	Number int
	Name   string
	Key    string
}

var IndexedFonts = []FontEntry{
	{1, "Monospace", "monospace"},
	{2, "Bold", "bold"},
	{3, "Italic", "italic"},
	{4, "Bold Italic", "bold-italic"},
	{5, "Double Struck", "double-struck"},
	{6, "Script", "script"},
	{7, "Bold Script", "bold-script"},
	{8, "Fraktur", "fraktur"},
	{9, "Bold Fraktur", "bold-fraktur"},
	{10, "Sans", "sans"},
	{11, "Sans Bold", "sans-bold"},
	{12, "Sans Italic", "sans-italic"},
	{13, "Sans Bold Italic", "sans-bold-italic"},
	{14, "Small Caps", "small-caps"},
	{15, "Circled", "circled"},
	{16, "Squared", "squared"},
	{17, "Fullwidth", "fullwidth"},
	{18, "Subscript", "subscript"},
	{19, "Superscript", "superscript"},
	{20, "Parenthesized", "parenthesized"},
	{21, "Bold Sans", "bold-sans"},
	{22, "Circled Negative", "circled-negative"},
}
