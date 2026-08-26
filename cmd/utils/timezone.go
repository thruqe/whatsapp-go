package cliutils

import (
	"strings"
	"sync"
)

type WindowsTZEntry struct {
	Value  string   `json:"value"`
	Abbr   string   `json:"abbr"`
	Offset float64  `json:"offset"`
	IsDST  bool     `json:"isdst"`
	Text   string   `json:"text"`
	UTC    []string `json:"utc"`
}

var (
	tzLoadOnce   sync.Once
	tzAliasIndex map[string]string // lowercased alias -> canonical IANA name
)

func loadTimezoneAliases() {
	tzLoadOnce.Do(func() {
		tzAliasIndex = make(map[string]string)
		for _, e := range tzEntries {
			if len(e.UTC) == 0 {
				continue
			}
			canonical := e.UTC[0] // first UTC entry as the canonical IANA zone

			// index every UTC/IANA name to itself
			for _, iana := range e.UTC {
				tzAliasIndex[strings.ToLower(iana)] = iana
			}
			// index the Windows "value" (e.g. "GMT Standard Time")
			tzAliasIndex[strings.ToLower(e.Value)] = canonical
			// index the abbreviation (e.g. "GST") — only if not ambiguous/already set,
			// abbreviations collide a lot so last-write-wins is acceptable here
			if e.Abbr != "" {
				if _, exists := tzAliasIndex[strings.ToLower(e.Abbr)]; !exists {
					tzAliasIndex[strings.ToLower(e.Abbr)] = canonical
				}
			}
		}
	})
}

// ResolveTimezoneAlias tries to resolve user input (IANA name, Windows name,
// or abbreviation) to a canonical IANA timezone string.
// Returns ("", false) if nothing matches.
func ResolveTimezoneAlias(input string) (string, bool) {
	loadTimezoneAliases()
	key := strings.ToLower(strings.TrimSpace(input))
	if canonical, ok := tzAliasIndex[key]; ok {
		return canonical, true
	}
	return "", false
}
