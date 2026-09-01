package external

import (
	"fmt"
	"sync"
	"time"

	utils "whatsrook"
	Logger "whatsrook/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var loadingPhrases = []string{
	"Initializing plugin...",
	"Executing command...",
	"Fetching data...",
	"Processing response...",
	"Formatting output...",
}

// Loader manages a stylized, animated loading indicator for external plugin execution.
type Loader struct {
	ctx      *utils.PluginContext
	name     string
	prefix   string
	msgID    types.MessageID
	stopChan chan struct{}
	stopped  bool
	sent     bool
	mu       sync.Mutex
}

// startLoader launches an external plugin loader with an initial activation delay.
func startLoader(ctx *utils.PluginContext, name string, delay time.Duration) *Loader {
	p := ctx.GetPrefix()
	if p == "" {
		p = "."
	}
	l := &Loader{
		ctx:      ctx,
		name:     name,
		prefix:   p,
		stopChan: make(chan struct{}),
	}
	go l.run(delay)
	return l
}

func (l *Loader) run(delay time.Duration) {
	// Wait for the delay threshold before sending the loader message
	select {
	case <-l.stopChan:
		return
	case <-l.ctx.Ctx.Done():
		return
	case <-time.After(delay):
	}

	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.sent = true
	l.mu.Unlock()

	// Send the initial stylized loader message
	initialText := fmt.Sprintf("🔌 *[%s%s]* %s %s", l.prefix, l.name, spinnerFrames[0], loadingPhrases[0])
	resp, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, &waE2E.Message{
		Conversation: &initialText,
	})
	if err != nil {
		Logger.Debug("external loader: failed to send initial loader message", "plugin", l.name, "err", err)
		return
	}

	l.mu.Lock()
	l.msgID = resp.ID
	l.mu.Unlock()

	ticker := time.NewTicker(650 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 1
	phraseIdx := 1

	for {
		select {
		case <-l.stopChan:
			return
		case <-l.ctx.Ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			if l.stopped || l.msgID == "" {
				l.mu.Unlock()
				return
			}
			msgID := l.msgID
			frame := spinnerFrames[frameIdx%len(spinnerFrames)]
			phrase := loadingPhrases[phraseIdx%len(loadingPhrases)]
			frameIdx++
			if frameIdx%2 == 0 {
				phraseIdx++
			}
			l.mu.Unlock()

			newText := fmt.Sprintf("🔌 *[%s%s]* %s %s", l.prefix, l.name, frame, phrase)
			editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, &waE2E.Message{
				Conversation: &newText,
			})
			_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
		}
	}
}

// Stop halts the animation ticker.
func (l *Loader) Stop() {
	l.mu.Lock()
	if !l.stopped {
		l.stopped = true
		close(l.stopChan)
	}
	l.mu.Unlock()
}

// MessageID returns the sent message ID if the loader was posted.
func (l *Loader) MessageID() types.MessageID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.msgID
}

// HasSent returns true if the loader message was actually posted to chat.
func (l *Loader) HasSent() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sent && l.msgID != ""
}

// Done stops the loader and edits the loader message in-place with the final response.
// If the loader was not sent, it sends a new reply message.
func (l *Loader) Done(finalText string) error {
	l.Stop()

	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, &waE2E.Message{
			Conversation: &finalText,
		})
		_, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
		return err
	}

	return l.ctx.Reply(finalText)
}

// DoneWithReply transitions the loader into the initial reply in-place and returns the message ID.
// If the loader was not sent, it calls ReplyWithID.
func (l *Loader) DoneWithReply(replyText string) (types.MessageID, error) {
	l.Stop()

	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		editMsg := l.ctx.Client.BuildEdit(l.ctx.Chat, msgID, &waE2E.Message{
			Conversation: &replyText,
		})
		_, err := l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, editMsg)
		if err == nil {
			return msgID, nil
		}
	}

	return l.ctx.ReplyWithID(replyText)
}

// Delete stops the loader and revokes the loader message from chat if posted.
func (l *Loader) Delete() {
	l.Stop()

	l.mu.Lock()
	msgID := l.msgID
	l.msgID = ""
	l.mu.Unlock()

	if msgID != "" && l.ctx != nil && l.ctx.Client != nil {
		revokeMsg := l.ctx.Client.BuildRevoke(l.ctx.Chat, types.EmptyJID, msgID)
		_, _ = l.ctx.Client.SendMessage(l.ctx.GetSendContext(), l.ctx.Chat, revokeMsg)
	}
}
