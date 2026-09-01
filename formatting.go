package whatsrook

import (
	"whatsrook/builder"
)

var (
	// Bold returns plain text without WhatsApp bold formatting symbols (*).
	Bold = builder.Bold

	// Boldf formats text according to format specifier without bold formatting symbols.
	Boldf = builder.Boldf

	// Italic returns plain text without WhatsApp italic formatting symbols (_).
	Italic = builder.Italic

	// Italicf formats text according to format specifier without italic formatting symbols.
	Italicf = builder.Italicf

	// Code returns plain text without WhatsApp inline code formatting symbols (`).
	Code = builder.Code

	// Codef formats text according to format specifier without inline code formatting symbols.
	Codef = builder.Codef

	// CodeBlock returns plain text without WhatsApp code block formatting symbols (```).
	CodeBlock = builder.CodeBlock

	// Strike returns plain text without WhatsApp strikethrough formatting symbols (~).
	Strike = builder.Strike

	// Strikef formats text according to format specifier without strikethrough formatting symbols.
	Strikef = builder.Strikef

	// Quote returns plain text without WhatsApp quote formatting symbols (>).
	Quote = builder.Quote

	// Quotef formats text according to format specifier without quote formatting symbols.
	Quotef = builder.Quotef
)
