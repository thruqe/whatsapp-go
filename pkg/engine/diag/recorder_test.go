package diag

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRecorderEmit verifies Emit writes one JSON object per line to <dir>/<stream>.jsonl,
// injects ts_ms, and keeps streams in separate files.
func TestRecorderEmit(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	rec.Emit("rtp", map[string]any{"seq": 1, "ssrc": "0x01"})
	rec.Emit("rtp", map[string]any{"seq": 2})
	rec.Emit("keying", map[string]any{"call_key_hex": "deadbeef"})
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countLines(t, filepath.Join(dir, "rtp.jsonl")); got != 2 {
		t.Errorf("rtp.jsonl lines = %d, want 2", got)
	}
	if got := countLines(t, filepath.Join(dir, "keying.jsonl")); got != 1 {
		t.Errorf("keying.jsonl lines = %d, want 1", got)
	}

	// First rtp record must be valid JSON carrying its field plus the injected ts_ms.
	rec0 := firstRecord(t, filepath.Join(dir, "rtp.jsonl"))
	if rec0["seq"].(float64) != 1 {
		t.Errorf("seq = %v, want 1", rec0["seq"])
	}
	if _, ok := rec0["ts_ms"]; !ok {
		t.Error("ts_ms missing from record")
	}
}

// TestRecorderNilSafe confirms a nil *Recorder no-ops rather than panicking, so
// callers can emit unconditionally when diagnostics are off.
func TestRecorderNilSafe(t *testing.T) {
	var r *Recorder
	r.Emit("anything", map[string]any{"x": 1})
	if err := r.Close(); err != nil {
		t.Errorf("nil Close: %v", err)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n
}

func firstRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() {
		t.Fatalf("%s is empty", path)
	}
	var m map[string]any
	if err := json.Unmarshal(s.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}
