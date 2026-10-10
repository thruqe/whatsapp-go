package ai

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsrook"
	"whatsrook/util/external"
)

// ChatTurn represents a single conversational turn in a chat.
type ChatTurn struct {
	Sender    string    `json:"sender"`
	IsBot     bool      `json:"is_bot"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	maxTurnsPerChat = 10
	historyTTL      = 30 * time.Minute
)

type chatHistoryStore struct {
	mu      sync.RWMutex
	history map[string][]ChatTurn
}

var globalChatHistory = &chatHistoryStore{
	history: make(map[string][]ChatTurn),
}

// RecordUserMessage records an incoming user message for conversational context tracking.
func RecordUserMessage(chatID, senderName, text string) {
	text = strings.TrimSpace(text)
	if chatID == "" || text == "" {
		return
	}
	if senderName == "" {
		senderName = "User"
	}
	recordTurn(chatID, ChatTurn{
		Sender:    senderName,
		IsBot:     false,
		Text:      text,
		Timestamp: time.Now(),
	})
}

// RecordBotMessage records an outgoing bot response for conversational context tracking.
func RecordBotMessage(chatID, botName, text string) {
	text = strings.TrimSpace(text)
	if chatID == "" || text == "" {
		return
	}
	if botName == "" {
		botName = "WhatsRook"
	}
	recordTurn(chatID, ChatTurn{
		Sender:    botName,
		IsBot:     true,
		Text:      text,
		Timestamp: time.Now(),
	})
}

func recordTurn(chatID string, turn ChatTurn) {
	globalChatHistory.mu.Lock()
	defer globalChatHistory.mu.Unlock()

	turns := globalChatHistory.history[chatID]
	now := time.Now()

	// Prune expired turns
	var active []ChatTurn
	for _, t := range turns {
		if now.Sub(t.Timestamp) <= historyTTL {
			active = append(active, t)
		}
	}

	// Avoid duplicate consecutive identical message from same party
	if len(active) > 0 {
		last := active[len(active)-1]
		if last.IsBot == turn.IsBot && last.Text == turn.Text {
			return
		}
	}

	active = append(active, turn)
	if len(active) > maxTurnsPerChat {
		active = active[len(active)-maxTurnsPerChat:]
	}

	globalChatHistory.history[chatID] = active
}

// GetRecentTurns returns active recent turns for the specified chat.
func GetRecentTurns(chatID string) []ChatTurn {
	globalChatHistory.mu.RLock()
	defer globalChatHistory.mu.RUnlock()

	turns := globalChatHistory.history[chatID]
	if len(turns) == 0 {
		return nil
	}

	now := time.Now()
	var result []ChatTurn
	for _, t := range turns {
		if now.Sub(t.Timestamp) <= historyTTL {
			result = append(result, t)
		}
	}
	return result
}

// FormatRecentHistory formats the recent conversation turns as a clean multi-line string.
func FormatRecentHistory(chatID string, currentQuestion string) string {
	turns := GetRecentTurns(chatID)
	if len(turns) == 0 {
		return ""
	}

	tb := whatsrook.NewText()
	for _, t := range turns {
		if !t.IsBot && t.Text == currentQuestion {
			continue
		}
		content := t.Text
		if len(content) > 300 {
			content = content[:297] + "..."
		}
		tb.Linef("%s: %s", t.Sender, content)
	}
	return tb.Trimmed()
}

// ClearChatHistory purges the in-memory conversation memory for a chat.
func ClearChatHistory(chatID string) {
	globalChatHistory.mu.Lock()
	defer globalChatHistory.mu.Unlock()
	delete(globalChatHistory.history, chatID)
}

var urlRegex = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"]+`)

func findURL(text string) string {
	return urlRegex.FindString(text)
}

// InferContextualArgs attempts to resolve missing arguments for commands like "install", "uninstall", or "dl"
// by inspecting the user's prompt, quoted message, and recent conversation history.
func InferContextualArgs(cmdName, rawArgs, question, quotedText string, history []ChatTurn) string {
	trimmedArgs := strings.TrimSpace(rawArgs)
	if trimmedArgs != "" {
		return trimmedArgs
	}

	cmdLower := strings.ToLower(cmdName)
	switch cmdLower {
	case "install", "uninstall":
		qLower := strings.ToLower(question)
		if strings.Contains(qLower, " all") || strings.HasPrefix(qLower, "all") {
			return "all"
		}

		official := external.OfficialPlugins

		findPlugin := func(text string) string {
			tLower := strings.ToLower(text)
			for _, p := range official {
				pattern := `\b` + regexp.QuoteMeta(p) + `\b`
				if matched, _ := regexp.MatchString(pattern, tLower); matched {
					return p
				}
			}
			return ""
		}

		if quotedText != "" {
			if p := findPlugin(quotedText); p != "" {
				return p
			}
		}

		if p := findPlugin(question); p != "" {
			return p
		}

		// Scan recent conversation history in reverse order (newest to oldest)
		for _, turn := range slices.Backward(history) {
			if p := findPlugin(turn.Text); p != "" {
				return p
			}
		}

		return ""

	case "dl", "ytdl", "download":
		if u := findURL(question); u != "" {
			return u
		}
		if u := findURL(quotedText); u != "" {
			return u
		}
		for _, turn := range slices.Backward(history) {
			if u := findURL(turn.Text); u != "" {
				return u
			}
		}
		return ""
	}

	return trimmedArgs
}
