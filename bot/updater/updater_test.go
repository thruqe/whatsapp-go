package updater

import (
	"testing"
)

func TestParseVersion_Monthly(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantYear    int
		wantMonth   int
		wantPatch   string
		wantExtra   string
		wantNumeric int
		wantErr     bool
	}{
		{
			name:        "monthly release format with short sha",
			raw:         "26.09.7a3b4c1",
			wantYear:    26,
			wantMonth:   9,
			wantPatch:   "7a3b4c1",
			wantNumeric: 0,
			wantErr:     false,
		},
		{
			name:        "monthly release format with v prefix",
			raw:         "v26.09.7a3b4c1",
			wantYear:    26,
			wantMonth:   9,
			wantPatch:   "7a3b4c1",
			wantNumeric: 0,
			wantErr:     false,
		},
		{
			name:        "monthly release format with extra build suffix",
			raw:         "26.09.7a3b4c1_prod",
			wantYear:    26,
			wantMonth:   9,
			wantPatch:   "7a3b4c1",
			wantExtra:   "prod",
			wantNumeric: 0,
			wantErr:     false,
		},
		{
			name:        "legacy semver format",
			raw:         "4.9.26",
			wantYear:    4,
			wantMonth:   9,
			wantPatch:   "26",
			wantNumeric: 26,
			wantErr:     false,
		},
		{
			name:    "invalid format with single segment",
			raw:     "26",
			wantErr: true,
		},
		{
			name:    "invalid format with non-numeric year",
			raw:     "year.09.7a3b4c1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseVersion(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVersion(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if v.Year != tt.wantYear || v.Major != tt.wantYear {
				t.Errorf("Year/Major = %d, want %d", v.Year, tt.wantYear)
			}
			if v.Month != tt.wantMonth || v.Minor != tt.wantMonth {
				t.Errorf("Month/Minor = %d, want %d", v.Month, tt.wantMonth)
			}
			if v.Patch != tt.wantPatch {
				t.Errorf("Patch = %q, want %q", v.Patch, tt.wantPatch)
			}
			if v.ExtraBuildInfo != tt.wantExtra {
				t.Errorf("ExtraBuildInfo = %q, want %q", v.ExtraBuildInfo, tt.wantExtra)
			}
			if v.NumericPatch != tt.wantNumeric {
				t.Errorf("NumericPatch = %d, want %d", v.NumericPatch, tt.wantNumeric)
			}
		})
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		wantDiff int // -1 if v1 < v2, 1 if v1 > v2, 0 if same/equal
	}{
		{
			name:     "newer year",
			v1:       "27.01.abc1234",
			v2:       "26.12.abc1234",
			wantDiff: 1,
		},
		{
			name:     "older year",
			v1:       "25.12.abc1234",
			v2:       "26.01.abc1234",
			wantDiff: -1,
		},
		{
			name:     "newer month same year",
			v1:       "26.10.abc1234",
			v2:       "26.09.abc1234",
			wantDiff: 1,
		},
		{
			name:     "older month same year",
			v1:       "26.08.abc1234",
			v2:       "26.09.abc1234",
			wantDiff: -1,
		},
		{
			name:     "same year and month with different sha",
			v1:       "26.09.abc1234",
			v2:       "26.09.def5678",
			wantDiff: 0,
		},
		{
			name:     "numeric patch comparison newer",
			v1:       "26.09.2",
			v2:       "26.09.1",
			wantDiff: 1,
		},
		{
			name:     "numeric patch comparison older",
			v1:       "26.09.1",
			v2:       "26.09.2",
			wantDiff: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed1, err1 := ParseVersion(tt.v1)
			if err1 != nil {
				t.Fatalf("ParseVersion(%q) failed: %v", tt.v1, err1)
			}
			parsed2, err2 := ParseVersion(tt.v2)
			if err2 != nil {
				t.Fatalf("ParseVersion(%q) failed: %v", tt.v2, err2)
			}
			got := parsed1.Compare(parsed2)
			if got != tt.wantDiff {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.v1, tt.v2, got, tt.wantDiff)
			}
		})
	}
}

func TestIsBetaVersion(t *testing.T) {
	tests := []struct {
		v        string
		wantBeta bool
	}{
		{"26.09.7a3b4c1", false},
		{"v26.09.7a3b4c1", false},
		{"26.09.7a3b4c1_prod", false},
		{"4.9.26", false},
		{"alpha", true},
		{"beta", true},
		{"beta-2026-09-10", true},
		{"7a3b4c1e2f3a4b", true}, // pure commit hash
	}

	for _, tt := range tests {
		got := IsBetaVersion(tt.v)
		if got != tt.wantBeta {
			t.Errorf("IsBetaVersion(%q) = %v, want %v", tt.v, got, tt.wantBeta)
		}
	}
}
