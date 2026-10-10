package system

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetStats(t *testing.T) {
	st := GetStats()
	if st.OS == "" {
		t.Fatal("expected non-empty OS")
	}
	if st.Arch == "" {
		t.Fatal("expected non-empty Arch")
	}
	if st.NumCPU <= 0 {
		t.Fatalf("expected NumCPU > 0, got %d", st.NumCPU)
	}
	str := st.String()
	if str == "" || !strings.Contains(str, st.OS) {
		t.Fatalf("expected valid Stats string, got %s", str)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tc := range cases {
		got := FormatBytes(tc.in)
		if got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	d := 2*time.Hour + 30*time.Minute + 15*time.Second
	got := FormatDuration(d)
	if got != "2h 30m 15s" {
		t.Fatalf("expected '2h 30m 15s', got %q", got)
	}
}

func TestRecordCrash(t *testing.T) {
	// nil panic is a no-op
	if path := RecordCrash(nil); path != "" {
		t.Fatalf("expected empty string for nil panic, got %q", path)
	}

	tmpDir := t.TempDir()
	t.Setenv("WHATSDATA_DIR", tmpDir)

	path := RecordCrash("test panic", "extra context info")
	if path == "" {
		t.Fatal("expected non-empty crash path")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read crash log: %v", err)
	}
	if !strings.Contains(string(content), "test panic") {
		t.Fatalf("crash log missing panic value: %s", string(content))
	}
	if !strings.Contains(string(content), "extra context info") {
		t.Fatalf("crash log missing extra context: %s", string(content))
	}
}
