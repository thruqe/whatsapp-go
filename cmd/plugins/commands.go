package plugins

import (
	"strings"
	"sync"
	"whatsrook/utils"
)

type Context = utils.PluginContext
type TextBuilder = utils.TextBuilder

func NewText(initial ...string) *TextBuilder {
	return utils.NewText(initial...)
}

func NewTextf(format string, args ...any) *TextBuilder {
	return utils.NewTextf(format, args...)
}

var (
	Sprintf   = utils.Sprintf
	Bold      = utils.Bold
	Boldf     = utils.Boldf
	Italic    = utils.Italic
	Italicf   = utils.Italicf
	Code      = utils.Code
	Codef     = utils.Codef
	CodeBlock = utils.CodeBlock
	Strike    = utils.Strike
	Strikef   = utils.Strikef
	Quote     = utils.Quote
	Quotef    = utils.Quotef
)

type Handler func(ctx *Context) error

type Command struct {
	Name         string
	Alias        string
	Description  string
	Category     string
	HideFromMenu bool
	GroupOnly    bool
	IsPublic     bool
	Handler      Handler
}

type CommandInfo struct {
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description"`
	Category    string `json:"category"`
	GroupOnly   bool   `json:"group_only"`
	IsPublic    bool   `json:"is_public"`
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*Command{}
	order      = []string{}
)

func Register(c *Command) {
	if c == nil || c.Name == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()

	key := strings.ToLower(c.Name)
	if _, exists := registry[key]; !exists {
		order = append(order, c.Name)
	}
	registry[key] = c
	if c.Alias != "" {
		registry[strings.ToLower(c.Alias)] = c
	}
}

func Get(name string) (*Command, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	c, ok := registry[strings.ToLower(name)]
	return c, ok
}

func All() []*Command {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]*Command, 0, len(order))
	for _, name := range order {
		c := registry[strings.ToLower(name)]
		if c != nil {
			out = append(out, c)
		}
	}
	return out
}

func Visible() []*Command {
	registryMu.RLock()
	defer registryMu.RUnlock()

	seen := map[*Command]bool{}
	var out []*Command
	for _, name := range order {
		c := registry[strings.ToLower(name)]
		if c == nil || c.HideFromMenu || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func ListCommands() []CommandInfo {
	visible := Visible()
	out := make([]CommandInfo, 0, len(visible))
	for _, c := range visible {
		out = append(out, CommandInfo{
			Name:        c.Name,
			Alias:       c.Alias,
			Description: c.Description,
			Category:    c.Category,
			GroupOnly:   c.GroupOnly,
			IsPublic:    c.IsPublic,
		})
	}
	return out
}
