package src

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// TextBuilder provides a high-level, fluent, memory-efficient abstraction
// for assembling clean plain-text WhatsApp messages without raw strings.Builder,
// fmt.Sprintf boilerplate, or unwanted formatting symbols (*, _, `, ~, •).
type TextBuilder struct {
	sb       strings.Builder
	ctx      *PluginContext
	mentions []types.JID
}

// NewText initializes a new standalone TextBuilder, optionally with initial text.
func NewText(initial ...string) *TextBuilder {
	b := &TextBuilder{}
	if len(initial) > 0 {
		b.Text(initial...)
	}
	return b
}

// NewTextf initializes a new standalone TextBuilder with formatted text.
func NewTextf(format string, args ...any) *TextBuilder {
	b := &TextBuilder{}
	b.Textf(format, args...)
	return b
}

// Sprintf formats according to a format specifier using TextBuilder.
func Sprintf(format string, args ...any) string {
	return NewTextf(format, args...).String()
}

// NewTextWithContext initializes a TextBuilder bound to a PluginContext for direct reply/send dispatch.
func NewTextWithContext(ctx *PluginContext, initial ...string) *TextBuilder {
	b := &TextBuilder{ctx: ctx}
	if len(initial) > 0 {
		b.Text(initial...)
	}
	return b
}

// --- Text & Line Appenders ---

// Line appends the given text strings joined by space, followed by a newline (\n).
// When called with no arguments, it appends a single newline.
func (b *TextBuilder) Line(text ...string) *TextBuilder {
	if len(text) == 0 {
		b.sb.WriteByte('\n')
		return b
	}
	b.sb.WriteString(strings.Join(text, " "))
	b.sb.WriteByte('\n')
	return b
}

// Linef formats text according to format specifier and appends it with a trailing newline (\n).
func (b *TextBuilder) Linef(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// Text appends text strings joined by space inline without a trailing newline.
func (b *TextBuilder) Text(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// Textf formats text according to format specifier and appends it inline without a trailing newline.
func (b *TextBuilder) Textf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	return b
}

// Paragraph appends text followed by two newlines (\n\n).
func (b *TextBuilder) Paragraph(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	b.sb.WriteString("\n\n")
	return b
}

// Paragraphf formats text and appends it followed by two newlines (\n\n).
func (b *TextBuilder) Paragraphf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteString("\n\n")
	return b
}

// Blank appends one or more empty newlines (\n). Defaults to 1 if no count is specified.
func (b *TextBuilder) Blank(count ...int) *TextBuilder {
	n := 1
	if len(count) > 0 && count[0] > 0 {
		n = count[0]
	}
	for i := 0; i < n; i++ {
		b.sb.WriteByte('\n')
	}
	return b
}

// NewLine is an alias for Blank(1).
func (b *TextBuilder) NewLine() *TextBuilder {
	return b.Blank(1)
}

// Space appends a single space character.
func (b *TextBuilder) Space() *TextBuilder {
	b.sb.WriteByte(' ')
	return b
}

// Tab appends a tab indentation character (\t).
func (b *TextBuilder) Tab() *TextBuilder {
	b.sb.WriteByte('\t')
	return b
}

// Indent appends indentation spaces followed by text.
func (b *TextBuilder) Indent(spaces int, text ...string) *TextBuilder {
	for range spaces {
		b.sb.WriteByte(' ')
	}
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// --- Text Styling / Inline (No symbols) ---

// Bold appends text cleanly without bold symbols.
func (b *TextBuilder) Bold(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// Boldf formats text and appends it cleanly without bold symbols.
func (b *TextBuilder) Boldf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	return b
}

// Italic appends text cleanly without italic symbols.
func (b *TextBuilder) Italic(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// Italicf formats text and appends it cleanly without italic symbols.
func (b *TextBuilder) Italicf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	return b
}

// Code appends text cleanly without code backticks.
func (b *TextBuilder) Code(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// Codef formats text and appends it cleanly without code backticks.
func (b *TextBuilder) Codef(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	return b
}

// CodeBlock appends code cleanly without code block markers.
func (b *TextBuilder) CodeBlock(code string, lang ...string) *TextBuilder {
	b.sb.WriteString(code)
	b.sb.WriteByte('\n')
	return b
}

// Strike appends text cleanly without strikethrough symbols.
func (b *TextBuilder) Strike(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	return b
}

// Strikef formats text and appends it cleanly without strikethrough symbols.
func (b *TextBuilder) Strikef(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	return b
}

// Quote appends text cleanly.
func (b *TextBuilder) Quote(text ...string) *TextBuilder {
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
		b.sb.WriteByte('\n')
	}
	return b
}

// Quotef formats text and appends it cleanly.
func (b *TextBuilder) Quotef(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// --- Structured Layout Helpers (Pure Clean Plaintext) ---

// Header appends a clean header followed by double newline (TITLE\n\n).
func (b *TextBuilder) Header(title string) *TextBuilder {
	if title != "" {
		b.sb.WriteString(title)
		b.sb.WriteString("\n\n")
	}
	return b
}

// Headerf formats and appends a clean header followed by double newline.
func (b *TextBuilder) Headerf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteString("\n\n")
	return b
}

// Section appends a clean section title followed by a single newline (Section\n).
func (b *TextBuilder) Section(title string) *TextBuilder {
	if title != "" {
		b.sb.WriteString(title)
		b.sb.WriteByte('\n')
	}
	return b
}

// Sectionf formats and appends a clean section title followed by a single newline.
func (b *TextBuilder) Sectionf(format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// Bullet appends a clean list item (- text\n).
func (b *TextBuilder) Bullet(text ...string) *TextBuilder {
	b.sb.WriteString("- ")
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	b.sb.WriteByte('\n')
	return b
}

// Bulletf formats and appends a clean list item (- formatted\n).
func (b *TextBuilder) Bulletf(format string, args ...any) *TextBuilder {
	b.sb.WriteString("- ")
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// Numbered appends a numbered list item (1. text\n).
func (b *TextBuilder) Numbered(index int, text ...string) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf("%d. ", index))
	if len(text) > 0 {
		b.sb.WriteString(strings.Join(text, " "))
	}
	b.sb.WriteByte('\n')
	return b
}

// Numberedf formats and appends a numbered list item (1. formatted\n).
func (b *TextBuilder) Numberedf(index int, format string, args ...any) *TextBuilder {
	b.sb.WriteString(fmt.Sprintf("%d. ", index))
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// Field appends a clean key-value line (Label: value\n) and optionally registers associated mention JIDs.
func (b *TextBuilder) Field(label, value string, mentions ...types.JID) *TextBuilder {
	b.sb.WriteString(label)
	b.sb.WriteString(": ")
	b.sb.WriteString(value)
	b.sb.WriteByte('\n')
	b.Mentions(mentions...)
	return b
}

// Fieldf formats and appends a clean key-value line (Label: formatted\n).
func (b *TextBuilder) Fieldf(label, format string, args ...any) *TextBuilder {
	b.sb.WriteString(label)
	b.sb.WriteString(": ")
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// KV appends a plain key-value line (Label: value\n).
func (b *TextBuilder) KV(label, value string) *TextBuilder {
	b.sb.WriteString(label)
	b.sb.WriteString(": ")
	b.sb.WriteString(value)
	b.sb.WriteByte('\n')
	return b
}

// KVf formats and appends a plain key-value line (Label: formatted\n).
func (b *TextBuilder) KVf(label, format string, args ...any) *TextBuilder {
	b.sb.WriteString(label)
	b.sb.WriteString(": ")
	b.sb.WriteString(fmt.Sprintf(format, args...))
	b.sb.WriteByte('\n')
	return b
}

// --- WhatsApp Mentions Integration ---

// Mention appends @<user> and automatically tracks the JID for mention delivery.
func (b *TextBuilder) Mention(jid types.JID) *TextBuilder {
	if !jid.IsEmpty() {
		user := jid.ToNonAD().User
		if user == "" {
			user = jid.User
		}
		b.sb.WriteByte('@')
		b.sb.WriteString(user)
		b.mentions = append(b.mentions, jid)
	}
	return b
}

// MentionUser appends @<name> and registers the underlying JID.
func (b *TextBuilder) MentionUser(name string, jid types.JID) *TextBuilder {
	b.sb.WriteByte('@')
	b.sb.WriteString(name)
	if !jid.IsEmpty() {
		b.mentions = append(b.mentions, jid)
	}
	return b
}

// Mentions attaches one or more JIDs to the builder for mention delivery.
func (b *TextBuilder) Mentions(jids ...types.JID) *TextBuilder {
	for _, j := range jids {
		if !j.IsEmpty() {
			b.mentions = append(b.mentions, j)
		}
	}
	return b
}

// GetMentions returns all tracked mention JIDs.
func (b *TextBuilder) GetMentions() []types.JID {
	return b.mentions
}

// --- Conditional Helpers (Fluid logic without broken method chains) ---

// If executes fn if condition is true.
func (b *TextBuilder) If(condition bool, fn func(b *TextBuilder)) *TextBuilder {
	if condition && fn != nil {
		fn(b)
	}
	return b
}

// IfElse executes thenFn if condition is true, or elseFn if false.
func (b *TextBuilder) IfElse(condition bool, thenFn, elseFn func(b *TextBuilder)) *TextBuilder {
	if condition {
		if thenFn != nil {
			thenFn(b)
		}
	} else {
		if elseFn != nil {
			elseFn(b)
		}
	}
	return b
}

// LineIf appends text + \n only if condition is true and text is non-empty.
func (b *TextBuilder) LineIf(condition bool, text ...string) *TextBuilder {
	if condition && len(text) > 0 {
		b.Line(text...)
	}
	return b
}

// LinefIf formats and appends text + \n only if condition is true.
func (b *TextBuilder) LinefIf(condition bool, format string, args ...any) *TextBuilder {
	if condition {
		b.Linef(format, args...)
	}
	return b
}

// BulletIf appends a list item only if condition is true and text is non-empty.
func (b *TextBuilder) BulletIf(condition bool, text ...string) *TextBuilder {
	if condition && len(text) > 0 {
		b.Bullet(text...)
	}
	return b
}

// BulletfIf formats and appends a list item only if condition is true.
func (b *TextBuilder) BulletfIf(condition bool, format string, args ...any) *TextBuilder {
	if condition {
		b.Bulletf(format, args...)
	}
	return b
}

// FieldIf appends a key-value field only if condition is true and value is non-empty.
func (b *TextBuilder) FieldIf(condition bool, label, value string, mentions ...types.JID) *TextBuilder {
	if condition && value != "" {
		b.Field(label, value, mentions...)
	}
	return b
}

// FieldfIf formats and appends a key-value field only if condition is true.
func (b *TextBuilder) FieldfIf(condition bool, label, format string, args ...any) *TextBuilder {
	if condition {
		b.Fieldf(label, format, args...)
	}
	return b
}

// KVIf appends a plain key-value line only if condition is true and value is non-empty.
func (b *TextBuilder) KVIf(condition bool, label, value string) *TextBuilder {
	if condition && value != "" {
		b.KV(label, value)
	}
	return b
}

// --- Collections & Slices ---

// Bullets appends each item in items as a list item.
func (b *TextBuilder) Bullets(items ...string) *TextBuilder {
	for _, item := range items {
		if item != "" {
			b.Bullet(item)
		}
	}
	return b
}

// NumberedList appends each item in items as a numbered list item starting from 1.
func (b *TextBuilder) NumberedList(items ...string) *TextBuilder {
	for i, item := range items {
		if item != "" {
			b.Numbered(i+1, item)
		}
	}
	return b
}

// Join appends items joined by separator sep.
func (b *TextBuilder) Join(sep string, items ...string) *TextBuilder {
	b.sb.WriteString(strings.Join(items, sep))
	return b
}

// --- String Inspection & Output ---

// String returns the constructed text.
func (b *TextBuilder) String() string {
	return b.sb.String()
}

// Trimmed returns the constructed text with leading and trailing whitespace stripped.
func (b *TextBuilder) Trimmed() string {
	return strings.TrimSpace(b.sb.String())
}

// Bytes returns the constructed text as a byte slice.
func (b *TextBuilder) Bytes() []byte {
	return []byte(b.sb.String())
}

// Len returns the current length of the constructed text in bytes.
func (b *TextBuilder) Len() int {
	return b.sb.Len()
}

// IsEmpty returns true if no text has been written.
func (b *TextBuilder) IsEmpty() bool {
	return b.sb.Len() == 0
}

// Write appends bytes to the builder (io.Writer).
func (b *TextBuilder) Write(p []byte) (int, error) {
	return b.sb.Write(p)
}

// WriteString appends a string to the builder (io.StringWriter).
func (b *TextBuilder) WriteString(s string) (int, error) {
	return b.sb.WriteString(s)
}

// WriteByte appends a byte to the builder.
func (b *TextBuilder) WriteByte(c byte) error {
	return b.sb.WriteByte(c)
}

// Reset clears the builder state.
func (b *TextBuilder) Reset() *TextBuilder {
	b.sb.Reset()
	b.mentions = nil
	return b
}

// --- Context / WARook Dispatch Methods ---

// Reply sends the constructed text as a quoted reply to the triggering message.
// Automatically includes any tracked mentions.
func (b *TextBuilder) Reply() error {
	if b.ctx == nil {
		return fmt.Errorf("TextBuilder: Reply called without PluginContext")
	}
	text := b.Trimmed()
	if len(b.mentions) > 0 {
		return b.ctx.ReplyWithMentions(text, b.mentions)
	}
	return b.ctx.Reply(text)
}

// Send sends the constructed text as a standard message (not quoted).
// Automatically includes any tracked mentions.
func (b *TextBuilder) Send() error {
	if b.ctx == nil {
		return fmt.Errorf("TextBuilder: Send called without PluginContext")
	}
	text := b.Trimmed()
	if len(b.mentions) > 0 {
		return b.ctx.SendTextWithMentions(text, b.mentions)
	}
	return b.ctx.SendText(text)
}

// ReplyWithImage uploads and replies with an image, using the constructed text as the caption.
func (b *TextBuilder) ReplyWithImage(data []byte, mimetype string) error {
	if b.ctx == nil {
		return fmt.Errorf("TextBuilder: ReplyWithImage called without PluginContext")
	}
	caption := b.Trimmed()
	if len(b.mentions) > 0 {
		return b.ctx.ReplyWithImageWithMentions(data, mimetype, caption, b.mentions)
	}
	return b.ctx.ReplyWithImage(data, mimetype, caption)
}

// SendWithImage uploads and sends an image to the chat, using the constructed text as the caption.
func (b *TextBuilder) SendWithImage(data []byte, mimetype string) error {
	if b.ctx == nil {
		return fmt.Errorf("TextBuilder: SendWithImage called without PluginContext")
	}
	caption := b.Trimmed()
	if len(b.mentions) > 0 {
		return b.ctx.SendImageWithMentions(data, mimetype, caption, b.mentions)
	}
	return b.ctx.SendImage(data, mimetype, caption)
}
