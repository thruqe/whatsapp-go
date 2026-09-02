// version package provides semantic release version parsing and runtime build metadata.
//
// it embeds version.txt directly into the compiled library and binary, exposing structured
// major/minor/patch numeric components alongside raw build string descriptors.
package whatsrook

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed version.txt
var rawVersion string

// Version represents the parsed semantic version segments of the active WhatsRook release.
type Version struct {
	// Major segment (typically release day or major epoch).
	Major int
	// Minor segment (typically release month or feature sequence).
	Minor int
	// Patch segment (typically release year suffix or bugfix revision).
	Patch int
	// Raw is the original unparsed version string.
	Raw string
}

// GetVersion parses the embedded version.txt string into a structured Version instance.
// returns a descriptive error if the embedded version string deviates from the standard "X.Y.Z" format.
func GetVersion() (Version, error) {
	clean := strings.TrimSpace(rawVersion)
	parts := strings.Split(clean, ".")
	if len(parts) != 3 {
		return Version{Raw: clean}, fmt.Errorf("invalid version format: %q (expected X.Y.Z)", clean)
	}

	nums := make([]int, 3)
	for i, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil {
			return Version{Raw: clean}, fmt.Errorf("invalid numeric segment %q: %w", part, err)
		}
		nums[i] = val
	}

	return Version{
		Major: nums[0],
		Minor: nums[1],
		Patch: nums[2],
		Raw:   clean,
	}, nil
}
