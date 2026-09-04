package dispatch

import (
	"context"
	"sync"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// MessageInterceptor is a function that inspects an incoming message event and returns true if it handled it.
type MessageInterceptor func(c *Context, text string) bool

// MessagePostProcessor is a background hook that processes an incoming message without consuming it (e.g. auto-read, auto-react).
type MessagePostProcessor func(ctx context.Context, client *whatsmeow.Client, s *StoreWrapper, evt *events.Message)

type interceptorEntry struct {
	name string
	fn   MessageInterceptor
}

type postProcessorEntry struct {
	name string
	fn   MessagePostProcessor
}

var (
	preInterceptorsMu sync.RWMutex
	preInterceptors   []interceptorEntry

	postProcessorsMu sync.RWMutex
	postProcessors   []postProcessorEntry

	fallbackInterceptorsMu sync.RWMutex
	fallbackInterceptors   []interceptorEntry
)

// RegisterPreInterceptor registers a handler that runs before command prefix matching.
// If it returns true, message processing stops (command will not run).
func RegisterPreInterceptor(name string, fn MessageInterceptor) {
	preInterceptorsMu.Lock()
	defer preInterceptorsMu.Unlock()
	for i, it := range preInterceptors {
		if it.name == name {
			preInterceptors[i].fn = fn
			return
		}
	}
	preInterceptors = append(preInterceptors, interceptorEntry{name: name, fn: fn})
}

// RegisterPostProcessor registers an asynchronous message listener (e.g. AutoRead, AutoReact).
func RegisterPostProcessor(name string, fn MessagePostProcessor) {
	postProcessorsMu.Lock()
	defer postProcessorsMu.Unlock()
	for i, it := range postProcessors {
		if it.name == name {
			postProcessors[i].fn = fn
			return
		}
	}
	postProcessors = append(postProcessors, postProcessorEntry{name: name, fn: fn})
}

// RegisterFallbackInterceptor registers a handler that runs if no command prefix matched.
// If it returns true, message processing stops.
func RegisterFallbackInterceptor(name string, fn MessageInterceptor) {
	fallbackInterceptorsMu.Lock()
	defer fallbackInterceptorsMu.Unlock()
	for i, it := range fallbackInterceptors {
		if it.name == name {
			fallbackInterceptors[i].fn = fn
			return
		}
	}
	fallbackInterceptors = append(fallbackInterceptors, interceptorEntry{name: name, fn: fn})
}

// RunCommand executes a command line directly on behalf of a context.
func RunCommand(c *Context, cmdLine string) bool {
	if c == nil {
		return false
	}
	return runCommand(c.Ctx, c.Client, c.Evt, cmdLine)
}

// RunCommandPublicly exports runCommand for external package invocation.
func RunCommandPublicly(ctx context.Context, client *whatsmeow.Client, evt *events.Message, cmdLine string) bool {
	return runCommand(ctx, client, evt, cmdLine)
}

// HandleStickerCommandPublicly exports handleStickerCommand for external package invocation and testing.
func HandleStickerCommandPublicly(ctx context.Context, client *whatsmeow.Client, s *sqlstore.SQLStore, evt *events.Message, stk *waE2E.StickerMessage) bool {
	return handleStickerCommand(ctx, client, s, evt, stk)
}

// HandleQuotedStickerCommandPublicly exports handleQuotedStickerCommand for external package invocation and testing.
func HandleQuotedStickerCommandPublicly(ctx context.Context, client *whatsmeow.Client, s *sqlstore.SQLStore, evt *events.Message, replyText string) bool {
	return handleQuotedStickerCommand(ctx, client, s, evt, replyText)
}
