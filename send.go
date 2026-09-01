package whatsrook

import (
	"whatsrook/builder"
)

// DefaultPollTimeout is the default duration a poll remains active before auto-deleting.
const DefaultPollTimeout = builder.DefaultPollTimeout

type (
	// WARook is the per-request builder engine bound to a PluginContext.
	WARook = builder.WARook

	// Response is the reply surface handed to reactive handlers.
	Response = builder.Response

	// PollRequest carries the data of a decrypted poll vote.
	PollRequest = builder.PollRequest

	// ListRequest carries the data of a list row selection.
	ListRequest = builder.ListRequest

	// PollRouteConfig configures a reactive route for a poll message.
	PollRouteConfig = builder.PollRouteConfig

	// TextBuilder provides a fluent abstraction for assembling clean plain-text WhatsApp messages.
	TextBuilder = builder.TextBuilder

	// PollBuilder builds and sends a WhatsApp poll with reactive callback capabilities.
	PollBuilder = builder.PollBuilder

	// ListBuilder builds and sends a WhatsApp list message with interactive row selection handlers.
	ListBuilder = builder.ListBuilder

	// MessageBuilder is a fluent builder for sending messages of any supported media type.
	MessageBuilder = builder.MessageBuilder

	// ListRow is a single row entry within a list section.
	ListRow = builder.ListRow

	// ListSection is a named group of ListRows.
	ListSection = builder.ListSection
)

var (
	// From creates a WARook bound to the given sender context.
	From = builder.From

	// NewText initializes a new standalone TextBuilder, optionally with initial text.
	NewText = builder.NewText

	// NewTextf initializes a new standalone TextBuilder with formatted text.
	NewTextf = builder.NewTextf

	// NewTextWithSender initializes a TextBuilder bound to a sender context.
	NewTextWithSender = builder.NewTextWithSender

	// NewTextWithContext initializes a TextBuilder bound to a plugin context.
	NewTextWithContext = builder.NewTextWithContext

	// Sprintf formats according to a format specifier using TextBuilder.
	Sprintf = builder.Sprintf

	// NewPoll initializes a new PollBuilder for the given question.
	NewPoll = builder.NewPoll

	// RegisterPollRoute registers a reactive route with full lifecycle, timeout, and auto-delete management.
	RegisterPollRoute = builder.RegisterPollRoute

	// RegisterPollHandler registers a reactive handler for votes on a specific poll message.
	RegisterPollHandler = builder.RegisterPollHandler

	// RegisterListHandler registers a reactive handler for a list row ID.
	RegisterListHandler = builder.RegisterListHandler

	// DeregisterPollHandler removes the registered handler for a poll message and cancels any pending timeout.
	DeregisterPollHandler = builder.DeregisterPollHandler

	// DispatchPollVoteEvent decrypts the poll vote in evt, matches selected options, and fires handlers.
	DispatchPollVoteEvent = builder.DispatchPollVoteEvent

	// DispatchListSelection looks up and fires a registered handler for a list row selection.
	DispatchListSelection = builder.DispatchListSelection
)
