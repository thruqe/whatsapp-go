package whatsrook

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed version.txt
var rawVersion string

type Version struct {
	Major int // e.g., 18 (Day or Major)
	Minor int // e.g., 8  (Month or Minor)
	Patch int // e.g., 26 (Year/Revision)
	Raw   string
}

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
