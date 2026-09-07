package main

import (
	"testing"
)

func TestParseCLIArgsLogout(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantLogout bool
		wantPhone  string
	}{
		{
			name:       "short flag without phone",
			args:       []string{"-l"},
			wantLogout: true,
			wantPhone:  "",
		},
		{
			name:       "long flag without phone",
			args:       []string{"--logout"},
			wantLogout: true,
			wantPhone:  "",
		},
		{
			name:       "subcommand without phone",
			args:       []string{"logout"},
			wantLogout: true,
			wantPhone:  "",
		},
		{
			name:       "phone followed by long flag",
			args:       []string{"2348060598064", "--logout"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "phone followed by short flag",
			args:       []string{"2348060598064", "-l"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "long flag followed by phone",
			args:       []string{"--logout", "2348060598064"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "subcommand followed by phone",
			args:       []string{"logout", "2348060598064"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "interleaved with other flags",
			args:       []string{"-v", "2348060598064", "--logout"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "normal start without logout",
			args:       []string{"2348060598064"},
			wantLogout: false,
			wantPhone:  "2348060598064",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := parseCLIArgsFrom(tc.args)
			if res.Logout != tc.wantLogout {
				t.Errorf("Logout = %v; want %v", res.Logout, tc.wantLogout)
			}
			if res.Session != tc.wantPhone {
				t.Errorf("Session = %q; want %q", res.Session, tc.wantPhone)
			}
		})
	}
}
