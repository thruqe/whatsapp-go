package src

import (
	"fmt"
)

// Bold returns plain text without WhatsApp bold formatting symbols (*).
func Bold(text string) string {
	return text
}

// Boldf formats text according to format specifier without bold formatting symbols.
func Boldf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// Italic returns plain text without WhatsApp italic formatting symbols (_).
func Italic(text string) string {
	return text
}

// Italicf formats text according to format specifier without italic formatting symbols.
func Italicf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// Code returns plain text without WhatsApp inline code formatting symbols (`).
func Code(text string) string {
	return text
}

// Codef formats text according to format specifier without inline code formatting symbols.
func Codef(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// CodeBlock returns plain text without WhatsApp code block formatting symbols (```).
func CodeBlock(code string, lang ...string) string {
	return code
}

// Strike returns plain text without WhatsApp strikethrough formatting symbols (~).
func Strike(text string) string {
	return text
}

// Strikef formats text according to format specifier without strikethrough formatting symbols.
func Strikef(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// Quote returns plain text without WhatsApp quote formatting symbols (>).
func Quote(text string) string {
	return text
}

// Quotef formats text according to format specifier without quote formatting symbols.
func Quotef(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
