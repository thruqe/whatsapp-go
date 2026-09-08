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
			name:       "plain logout without phone",
			args:       []string{"logout"},
			wantLogout: true,
			wantPhone:  "",
		},
		{
			name:       "phone followed by logout",
			args:       []string{"2348060598064", "logout"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "logout followed by phone",
			args:       []string{"logout", "2348060598064"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "interleaved with verbose plain word",
			args:       []string{"verbose", "2348060598064", "logout"},
			wantLogout: true,
			wantPhone:  "2348060598064",
		},
		{
			name:       "normal start without logout",
			args:       []string{"2348060598064"},
			wantLogout: false,
			wantPhone:  "2348060598064",
		},
		{
			name:       "short flag -l is removed and ignored",
			args:       []string{"-l"},
			wantLogout: false,
			wantPhone:  "",
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

func TestParseCLIArgsPlainWords(t *testing.T) {
	// Version
	verRes := parseCLIArgsFrom([]string{"version"})
	if !verRes.Version {
		t.Errorf("expected Version = true for plain 'version', got false")
	}

	// Verbose and Phone
	verbRes := parseCLIArgsFrom([]string{"2348060598064", "verbose"})
	if !verbRes.Verbose || verbRes.Session != "2348060598064" {
		t.Errorf("expected Verbose=true and Session=2348060598064, got %+v", verbRes)
	}

	// Auth pair
	authPair := parseCLIArgsFrom([]string{"2348060598064", "auth", "pair"})
	if authPair.Auth != "pair" {
		t.Errorf("expected Auth=pair, got %q", authPair.Auth)
	}

	// Standalone pair
	pairRes := parseCLIArgsFrom([]string{"2348060598064", "pair"})
	if pairRes.Auth != "pair" {
		t.Errorf("expected Auth=pair for standalone 'pair', got %q", pairRes.Auth)
	}

	// Standalone qr
	qrRes := parseCLIArgsFrom([]string{"2348060598064", "qr"})
	if qrRes.Auth != "qr" {
		t.Errorf("expected Auth=qr for standalone 'qr', got %q", qrRes.Auth)
	}

	// Client platform
	clientRes := parseCLIArgsFrom([]string{"2348060598064", "client", "android"})
	if clientRes.Client != "android" {
		t.Errorf("expected Client=android, got %q", clientRes.Client)
	}

	// Database URL
	dbRes := parseCLIArgsFrom([]string{"2348060598064", "db", "postgres://localhost:5432/mydb"})
	if dbRes.Database != "postgres://localhost:5432/mydb" {
		t.Errorf("expected Database=postgres://..., got %q", dbRes.Database)
	}

	// Update variants
	upCheck := parseCLIArgsFrom([]string{"update", "check"})
	if !upCheck.Update || upCheck.UpdateOp != "check" {
		t.Errorf("expected Update=true, Op=check, got %+v", upCheck)
	}

	upBeta := parseCLIArgsFrom([]string{"update", "beta"})
	if !upBeta.Update || upBeta.UpdateOp != "beta" {
		t.Errorf("expected Update=true, Op=beta, got %+v", upBeta)
	}

	upStable := parseCLIArgsFrom([]string{"update", "stable"})
	if !upStable.Update || upStable.UpdateOp != "stable" {
		t.Errorf("expected Update=true, Op=stable, got %+v", upStable)
	}

	upDirect := parseCLIArgsFrom([]string{"update"})
	if !upDirect.Update || upDirect.UpdateOp != "" {
		t.Errorf("expected Update=true, Op='', got %+v", upDirect)
	}

	// Short flags -v, -a, -c are removed and not recognized
	shortRes := parseCLIArgsFrom([]string{"-v", "-a", "pair", "-c", "android", "2348060598064"})
	if shortRes.Verbose {
		t.Errorf("short flag -v should be ignored, got Verbose=true")
	}
	if shortRes.Client != "default" {
		t.Errorf("short flag -c should be ignored, got Client=%q", shortRes.Client)
	}
}
