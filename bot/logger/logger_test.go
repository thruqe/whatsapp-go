package logger

import (
	"sync"
	"testing"
)

func TestWarnSuppressedWhenNotVerbose(t *testing.T) {
	// Ensure clean state in non-verbose mode
	Close()
	SetVerbose(false)

	var mu sync.Mutex
	var entries []LogEntry

	unsub := AddHook(func(entry LogEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, entry)
	})
	defer unsub()

	// In non-verbose mode:
	Debug("this debug log should be suppressed")
	Warn("this warn log should be suppressed")
	Warnf("this warnf log should be suppressed: %d", 123)
	Warnw("this warnw log should be suppressed", "key", "val")
	Info("this info log should be displayed")
	Error("this error log should be displayed")

	mu.Lock()
	defer mu.Unlock()

	for _, e := range entries {
		if e.Level == "WARN" {
			t.Errorf("expected no WARN logs when verbose is false, got: %s", e.Message)
		}
		if e.Level == "DEBUG" {
			t.Errorf("expected no DEBUG logs when verbose is false, got: %s", e.Message)
		}
	}

	foundInfo := false
	foundError := false
	for _, e := range entries {
		if e.Level == "INFO" && e.Message == "this info log should be displayed" {
			foundInfo = true
		}
		if e.Level == "ERROR" && e.Message == "this error log should be displayed" {
			foundError = true
		}
	}

	if !foundInfo {
		t.Errorf("expected INFO log to be displayed when verbose is false")
	}
	if !foundError {
		t.Errorf("expected ERROR log to be displayed when verbose is false")
	}
}

func TestWarnShownWhenVerbose(t *testing.T) {
	// Enable verbose mode
	Close()
	SetVerbose(true)

	var mu sync.Mutex
	var entries []LogEntry

	unsub := AddHook(func(entry LogEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, entry)
	})
	defer unsub()

	Debug("this debug log should be displayed in verbose")
	Warn("this warn log should be displayed in verbose")
	Info("this info log should be displayed in verbose")
	Error("this error log should be displayed in verbose")

	mu.Lock()
	defer mu.Unlock()

	foundDebug := false
	foundWarn := false
	foundInfo := false
	foundError := false

	for _, e := range entries {
		if e.Level == "DEBUG" && e.Message == "this debug log should be displayed in verbose" {
			foundDebug = true
		}
		if e.Level == "WARN" && e.Message == "this warn log should be displayed in verbose" {
			foundWarn = true
		}
		if e.Level == "INFO" && e.Message == "this info log should be displayed in verbose" {
			foundInfo = true
		}
		if e.Level == "ERROR" && e.Message == "this error log should be displayed in verbose" {
			foundError = true
		}
	}

	if !foundDebug {
		t.Errorf("expected DEBUG log to be displayed when verbose is true")
	}
	if !foundWarn {
		t.Errorf("expected WARN log to be displayed when verbose is true")
	}
	if !foundInfo {
		t.Errorf("expected INFO log to be displayed when verbose is true")
	}
	if !foundError {
		t.Errorf("expected ERROR log to be displayed when verbose is true")
	}

	// Reset back to default non-verbose
	SetVerbose(false)
}

func TestWaLoggerAndSlogWarnSuppression(t *testing.T) {
	Close()
	SetVerbose(false)

	var mu sync.Mutex
	var entries []LogEntry

	unsub := AddHook(func(entry LogEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, entry)
	})
	defer unsub()

	waLog := NewWaLogger("testmod")
	waLog.Warnf("waLog warning that should be hidden")
	waLog.Infof("waLog info that should be shown")

	mu.Lock()
	defer mu.Unlock()

	for _, e := range entries {
		if e.Level == "WARN" {
			t.Errorf("expected no WARN logs from waLog when verbose is false, got: %s", e.Message)
		}
	}

	foundWaInfo := false
	for _, e := range entries {
		if e.Level == "INFO" && e.Message == "waLog info that should be shown" {
			foundWaInfo = true
		}
	}
	if !foundWaInfo {
		t.Errorf("expected waLog INFO log to be displayed")
	}
}
