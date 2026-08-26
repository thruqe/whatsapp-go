package main

import (
	"os"
	"testing"
)

func TestParseCLIArgsVerbose(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		envVerbose  string
		wantVerbose bool
	}{
		{
			name:        "default without flag",
			args:        []string{},
			wantVerbose: false,
		},
		{
			name:        "with -v flag",
			args:        []string{"-v"},
			wantVerbose: true,
		},
		{
			name:        "with -verbose flag",
			args:        []string{"-verbose"},
			wantVerbose: true,
		},
		{
			name:        "with --verbose flag",
			args:        []string{"--verbose"},
			wantVerbose: true,
		},
		{
			name:        "with VERBOSE=true env",
			args:        []string{},
			envVerbose:  "true",
			wantVerbose: true,
		},
		{
			name:        "with VERBOSE=1 env",
			args:        []string{},
			envVerbose:  "1",
			wantVerbose: true,
		},
		{
			name:        "with -v and other flags",
			args:        []string{"-s", "1234567890", "-a", "pair", "-v"},
			wantVerbose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVerbose != "" {
				_ = os.Setenv("VERBOSE", tt.envVerbose)
				defer func() { _ = os.Unsetenv("VERBOSE") }()
			} else {
				_ = os.Unsetenv("VERBOSE")
			}

			parsed := parseCLIArgsFrom(tt.args)
			if parsed.Verbose != tt.wantVerbose {
				t.Errorf("parseCLIArgsFrom(%v) Verbose = %v, want %v", tt.args, parsed.Verbose, tt.wantVerbose)
			}
		})
	}
}
