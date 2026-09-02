// dispatch package provides the core command registry, execution pipeline,
// and dispatch coordinator for the WhatsRook CLI bot application.
//
// it maintains an in-memory synchronized registry of command handlers, parses incoming
// message triggers against configured prefixes, evaluates authorization policies (owner, sudo, admin),
// and executes commands with context cancellation and error handling.
package dispatch

import (
	"strings"
	"sync"
	utils "whatsrook"
	"whatsrook/builder"
)

// Context aliases the core whatsrook.PluginContext execution context.
type Context = utils.PluginContext

// TextBuilder aliases the fluent text composition builder.
type TextBuilder = builder.TextBuilder

// PollBuilder aliases the interactive poll creation builder.
type PollBuilder = builder.PollBuilder

// PollRequest aliases the structured poll configuration payload.
type PollRequest = builder.PollRequest

// NewText constructs a fluent text message builder.
func NewText(initial ...string) *TextBuilder {
	return utils.NewText(initial...)
}

// NewTextf constructs a fluent text message builder with initial formatted content.
func NewTextf(format string, args ...any) *TextBuilder {
	b := utils.NewText()
	b.Textf(format, args...)
	return b
}

var (
	// Sprintf is an alias for standard string formatting.
	Sprintf = utils.Sprintf
	// Bold formats plain text as bold.
	Bold = utils.Bold
	// Boldf formats formatted text as bold.
	Boldf = utils.Boldf
	// Italic formats plain text as italic.
	Italic = utils.Italic
	// Italicf formats formatted text as italic.
	Italicf = utils.Italicf
	// Code formats plain text as inline code.
	Code = utils.Code
	// Codef formats formatted text as inline code.
	Codef = utils.Codef
	// CodeBlock formats plain text as a code block.
	CodeBlock = utils.CodeBlock
	// Strike formats plain text with strikethrough.
	Strike = utils.Strike
	// Strikef formats formatted text with strikethrough.
	Strikef = utils.Strikef
	// Quote formats plain text as a blockquote.
	Quote = utils.Quote
	// Quotef formats formatted text as a blockquote.
	Quotef = utils.Quotef
)

// Handler represents the function signature executed when a command is triggered.
type Handler func(ctx *Context) error

// Command defines the metadata, authorization rules, and execution handler for a bot command.
type Command struct {
	// Name is the primary trigger word for the command (e.g. "ping").
	Name string
	// Alias is a comma-separated list of secondary trigger words (e.g. "p,pong").
	Alias string
	// Description provides a concise explanation of what the command does.
	Description string
	// Category groups the command under a menu domain (e.g. "Group", "Owner", "Tools").
	Category string
	// HideFromMenu determines if the command is omitted from the public help menu.
	HideFromMenu bool
	// GroupOnly restricts execution exclusively to group chats.
	GroupOnly bool
	// IsPublic allows regular group members and DM users to execute without sudo privileges.
	IsPublic bool
	// NoLoader suppresses the automatic animated loading indicator during execution.
	NoLoader bool
	// Handler is the function executed when the command matches.
	Handler Handler
}

// CommandInfo provides a lightweight JSON-serializable snapshot of command metadata.
type CommandInfo struct {
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description"`
	Category    string `json:"category"`
	GroupOnly   bool   `json:"group_only"`
	IsPublic    bool   `json:"is_public"`
}

var (
	// registryMu synchronizes access to the command registry maps.
	registryMu sync.RWMutex
	// registry maps normalized command trigger words to their definitions.
	registry = map[string]*Command{}
	// order preserves the registration sequence of primary command names.
	order = []string{}
)

// Register adds one or more commands to the global dispatch registry.
func Register(commands ...*Command) {
	registryMu.Lock()
	defer registryMu.Unlock()

	for _, c := range commands {
		if c == nil || c.Name == "" {
			continue
		}
		key := strings.ToLower(c.Name)
		if _, exists := registry[key]; !exists {
			order = append(order, c.Name)
		}
		registry[key] = c
		if c.Alias != "" {
			for _, a := range strings.Split(c.Alias, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					registry[strings.ToLower(a)] = c
				}
			}
		}
	}
}

// Get retrieves a command by its primary name or alias.
func Get(name string) (*Command, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	c, ok := registry[strings.ToLower(name)]
	return c, ok
}

// All returns all registered commands in registration order.
func All() []*Command {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]*Command, 0, len(order))
	seen := make(map[*Command]bool)
	for _, name := range order {
		if c, ok := registry[strings.ToLower(name)]; ok && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Count returns the total number of unique registered commands.
func Count() int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return len(order)
}
