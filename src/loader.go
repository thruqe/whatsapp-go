package src

import (
	"fmt"
	"sync"
	"time"

	Logger "whatsrook/src/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Braille spinner frames for smooth text animation
var loaderFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var (
	activeLoaders   sync.Map
	loaderIDCounter uint64
	loaderIDMu      sync.Mutex
)

func nextLoaderID() string {
	loaderIDMu.Lock()
	defer loaderIDMu.Unlock()
	loaderIDCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), loaderIDCounter)
}

// Loader represents an active background text animation editing a WhatsApp message.
type Loader struct {
	ctx         *PluginContext
	id          string
	initialText string
	msgID       types.MessageID
	stopChan    chan struct{}
	active      bool
	stopped     bool
	mu          sync.Mutex
}

// StartLoader returns a Loader that sends an animated loading message to the chat,
// continuously editing frame-by-frame until the operation completes, then deleting the message.
func (ctx *PluginContext) StartLoader(initialText ...string) *Loader {
	txt := "Please wait"
	if len(initialText) > 0 && initialText[0] != "" {
		txt = initialText[0]
	}
	loaderID := nextLoaderID()
	l := &Loader{
		ctx:         ctx,
		id:          loaderID,
		initialText: txt,
		stopChan:    make(chan struct{}),
	}

	activeLoaders.Store(loaderID, l)

	// Activate loader message synchronously so it appears instantly in chat
	l.activate()

	return l
}

func (l *Loader) activate() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.active = true
	l.mu.Unlock()

	frame := loaderFrames[0]
	displayText := fmt.Sprintf("%s %s", l.initialText, frame)

	Logger.Debug("StartLoader: sending loader message synchronously", "id", l.id, "text", l.initialText)
	resp, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, &waE2E.Message{
		Conversation: &displayText,
	})
	if err != nil {
		Logger.Error("StartLoader: failed to send loader message", "err", err)
		return
	}

	l.mu.Lock()
	l.msgID = resp.ID
	l.mu.Unlock()

	go l.run()
}

func (l *Loader) run() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 1
	for {
		select {
		case <-l.stopChan:
			return
		case <-l.ctx.Ctx.Done():
			l.Delete()
			return
		case <-ticker.C:
			l.mu.Lock()
			if l.stopped || l.msgID == "" {
				l.mu.Unlock()
				return
			}
			frame := loaderFrames[frameIdx%len(loaderFrames)]
			frameIdx++
			l.mu.Unlock()

			displayText := fmt.Sprintf("%s %s", l.initialText, frame)
			l.editFrame(displayText)
		}
	}
}

func (l *Loader) editFrame(text string) {
	l.mu.Lock()
	if l.stopped || l.msgID == "" {
		l.mu.Unlock()
		return
	}
	msgID := l.msgID
	l.mu.Unlock()

	if l.ctx == nil || l.ctx.Client == nil {
		return
	}

	formatted := l.ctx.formatTextResponse(text)
	msg := &waE2E.Message{
		Conversation: &formatted,
	}
	editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
	_, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
	if err != nil {
		Logger.Debug("Loader animation edit failed", "msgID", msgID, "err", err)
	}
}

// Cancel cancels the running operation context associated with this loader.
func (l *Loader) Cancel() {
	l.Stop()
	if l.ctx != nil {
		l.ctx.Cancel()
	}
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		formatted := l.ctx.formatTextResponse("Operation cancelled.")
		msg := &waE2E.Message{
			Conversation: &formatted,
		}
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
	}
}

// CancelLoader looks up active loader by ID and cancels its operation.
func CancelLoader(id string) bool {
	val, ok := activeLoaders.Load(id)
	if !ok {
		return false
	}
	if l, ok := val.(*Loader); ok {
		l.Cancel()
		return true
	}
	return false
}

// MessageID returns the underlying MessageID of the loader message.
func (l *Loader) MessageID() types.MessageID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.msgID
}

// Stop halts the animation ticker.
func (l *Loader) Stop() {
	l.mu.Lock()
	if !l.stopped {
		l.stopped = true
		close(l.stopChan)
		activeLoaders.Delete(l.id)
	}
	l.mu.Unlock()
}

// Done stops the animation ticker and updates the loader message with final text if active.
func (l *Loader) Done(finalText string) {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && finalText != "" && l.ctx != nil && l.ctx.Client != nil {
		formatted := l.ctx.formatTextResponse(finalText)
		msg := &waE2E.Message{
			Conversation: &formatted,
		}
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, msg)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
	}
}

// Delete stops the animation ticker and deletes the loader message from chat if active.
func (l *Loader) Delete() {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		Logger.Debug("Loader: deleting loader message", "msgID", msgID)
		revokeMsg := l.ctx.Client.BuildRevoke(l.ctx.Chat, types.EmptyJID, msgID)
		_, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, revokeMsg)
		if err != nil {
			Logger.Debug("Loader: failed to delete loader message", "msgID", msgID, "err", err)
		}
	}
}
