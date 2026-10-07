package sdk

import (
	"fmt"
	"strings"
)

// Bold formats text in bold for WhatsApp (*text*).
func Bold(text string) string {
	return fmt.Sprintf("*%s*", text)
}

// Italic formats text in italics for WhatsApp (_text_).
func Italic(text string) string {
	return fmt.Sprintf("_%s_", text)
}

// Strikethrough formats text with strikethrough for WhatsApp (~text~).
func Strikethrough(text string) string {
	return fmt.Sprintf("~%s~", text)
}

// Monospace formats text in inline code monospace for WhatsApp (`text`).
func Monospace(text string) string {
	return fmt.Sprintf("`%s`", text)
}

// CodeBlock formats text inside a multi-line code block.
func CodeBlock(text string) string {
	return fmt.Sprintf("```\n%s\n```", text)
}

// Quote prefixes each line of input with a WhatsApp blockquote marker (> ).
func Quote(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}

// BulletList formats items as a bulleted list (• item).
func BulletList(items []string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = "• " + item
	}
	return strings.Join(lines, "\n")
}

// NumberedList formats items as a 1-indexed numbered list (1. item).
func NumberedList(items []string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = fmt.Sprintf("%d. %s", i+1, item)
	}
	return strings.Join(lines, "\n")
}

// MessageBuilder provides a fluent interface for composing formatted WhatsApp messages.
type MessageBuilder struct {
	buf strings.Builder
}

// NewMessageBuilder initializes a new empty MessageBuilder.
func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

// Text appends plain text without a trailing newline.
func (mb *MessageBuilder) Text(text string) *MessageBuilder {
	mb.buf.WriteString(text)
	return mb
}

// Line appends text followed by a newline.
func (mb *MessageBuilder) Line(text string) *MessageBuilder {
	mb.buf.WriteString(text)
	mb.buf.WriteByte('\n')
	return mb
}

// Newline appends a blank newline.
func (mb *MessageBuilder) Newline() *MessageBuilder {
	mb.buf.WriteByte('\n')
	return mb
}

// Bold appends bold text (*text*).
func (mb *MessageBuilder) Bold(text string) *MessageBuilder {
	mb.buf.WriteByte('*')
	mb.buf.WriteString(text)
	mb.buf.WriteByte('*')
	return mb
}

// Italic appends italic text (_text_).
func (mb *MessageBuilder) Italic(text string) *MessageBuilder {
	mb.buf.WriteByte('_')
	mb.buf.WriteString(text)
	mb.buf.WriteByte('_')
	return mb
}

// Strike appends strikethrough text (~text~).
func (mb *MessageBuilder) Strike(text string) *MessageBuilder {
	mb.buf.WriteByte('~')
	mb.buf.WriteString(text)
	mb.buf.WriteByte('~')
	return mb
}

// Mono appends monospace text (`text`).
func (mb *MessageBuilder) Mono(text string) *MessageBuilder {
	mb.buf.WriteByte('`')
	mb.buf.WriteString(text)
	mb.buf.WriteByte('`')
	return mb
}

// CodeBlock appends a multi-line code block.
func (mb *MessageBuilder) CodeBlock(code string) *MessageBuilder {
	mb.buf.WriteString("```\n")
	mb.buf.WriteString(code)
	mb.buf.WriteString("\n```\n")
	return mb
}

// Quote appends lines quoted with > .
func (mb *MessageBuilder) Quote(text string) *MessageBuilder {
	lines := strings.SplitSeq(text, "\n")
	for l := range lines {
		mb.buf.WriteString("> ")
		mb.buf.WriteString(l)
		mb.buf.WriteByte('\n')
	}
	return mb
}

// Bullet appends a bulleted item (• text\n).
func (mb *MessageBuilder) Bullet(text string) *MessageBuilder {
	mb.buf.WriteString("• ")
	mb.buf.WriteString(text)
	mb.buf.WriteByte('\n')
	return mb
}

// Numbered appends a numbered item (index. text\n).
func (mb *MessageBuilder) Numbered(index int, text string) *MessageBuilder {
	fmt.Fprintf(&mb.buf, "%d. %s\n", index, text)
	return mb
}

// Header appends a bold title followed by two newlines.
func (mb *MessageBuilder) Header(title string) *MessageBuilder {
	return mb.Bold(title).Newline().Newline()
}

// Build returns the completed message string.
func (mb *MessageBuilder) Build() string {
	return mb.buf.String()
}

// String implements fmt.Stringer.
func (mb *MessageBuilder) String() string {
	return mb.buf.String()
}
