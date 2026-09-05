package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtoValidation(t *testing.T) {
	// Test valid proto
	tmpDir := t.TempDir()
	validProto := filepath.Join(tmpDir, "valid.proto")
	validContent := "syntax = \"proto3\";\npackage test;\n"
	for i := 0; i < 55; i++ {
		validContent += "message Msg" + string(rune('A'+i%26)) + " {\n  optional string field = 1;\n}\n"
	}
	if err := os.WriteFile(validProto, []byte(validContent), 0644); err != nil {
		t.Fatalf("writing valid proto: %v", err)
	}

	if !isValidProtoSchema(validProto) {
		t.Errorf("expected valid proto to pass validation")
	}
	if err := validateProtoSchema(validProto); err != nil {
		t.Errorf("validateProtoSchema error: %v", err)
	}

	// Test invalid/empty proto
	invalidProto := filepath.Join(tmpDir, "invalid.proto")
	invalidContent := "syntax = \"proto3\";\npackage test;\nenum ACK { SENT = 1; }\n"
	if err := os.WriteFile(invalidProto, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("writing invalid proto: %v", err)
	}

	if isValidProtoSchema(invalidProto) {
		t.Errorf("expected invalid proto to fail validation")
	}
	if err := validateProtoSchema(invalidProto); err == nil {
		t.Errorf("expected error validating invalid proto, got nil")
	}
}

func TestFetchRemoteWaProto(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	filePath, cleanup, err := fetchRemoteWaProto()
	if err != nil {
		t.Fatalf("fetchRemoteWaProto failed: %v", err)
	}
	defer cleanup()

	count, err := countProtoMessages(filePath)
	if err != nil {
		t.Fatalf("countProtoMessages failed: %v", err)
	}
	if count < 50 {
		t.Errorf("expected >= 50 messages from remote, got %d", count)
	}
	t.Logf("Successfully fetched and validated remote WAProto.proto with %d messages", count)
}
