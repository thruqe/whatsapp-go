package tools

import (
	"fmt"
	"strings"
	"sync"
	"time"
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
	tzLoadOnce         sync.Once
	tzAliasIndex       map[string]string // lowercased alias -> canonical IANA name
	SupportedTimezones []string
	TzEntries          = tzEntries
)

func init() {
	loadTimezoneAliases()
}

func loadTimezoneAliases() {
	tzLoadOnce.Do(func() {
		tzAliasIndex = make(map[string]string)
		for _, e := range tzEntries {
			SupportedTimezones = append(SupportedTimezones, e.Text)
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

// TimezoneResult provides structured information about a timezone match.
type TimezoneResult struct {
	ID          string
	Location    string
	CurrentTime string
	UTCOffset   string
}

// SearchTimezone searches for a timezone by name, city, abbreviation, or IANA name.
func SearchTimezone(query string) (*TimezoneResult, error) {
	loadTimezoneAliases()
	query = strings.TrimSpace(query)
	iana, ok := ResolveTimezoneAlias(query)
	if !ok {
		qLower := strings.ToLower(query)
		for _, e := range tzEntries {
			if strings.Contains(strings.ToLower(e.Text), qLower) || strings.Contains(strings.ToLower(e.Value), qLower) {
				if len(e.UTC) > 0 {
					iana = e.UTC[0]
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("no timezone found for %q", query)
	}

	loc, err := time.LoadLocation(iana)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(loc)
	_, offsetSec := now.Zone()
	offsetHours := float64(offsetSec) / 3600.0
	offsetSign := "+"
	if offsetHours < 0 {
		offsetSign = "-"
		offsetHours = -offsetHours
	}

	return &TimezoneResult{
		ID:          iana,
		Location:    strings.ReplaceAll(iana, "_", " "),
		CurrentTime: now.Format("03:04 PM (15:04)"),
		UTCOffset:   fmt.Sprintf("%s%.0f", offsetSign, offsetHours),
	}, nil
}
