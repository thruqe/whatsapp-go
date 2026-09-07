package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSanitizeWaE2EProto(t *testing.T) {
	tmpDir := t.TempDir()
	protoPath := filepath.Join(tmpDir, "WAWebProtobufsE2E.proto")

	content := `syntax = "proto2";
package WAWebProtobufsE2E;

enum HistorySyncType {
	INITIAL_BOOTSTRAP = 0;
	FULL = 2;
}

enum JUNK_WEB_ENUM {
	UNKNOWN = 0;
	FULL = 1;
}

message BCallMessage {
	enum MediaType {
		UNKNOWN = 0;
		AUDIO = 1;
	}
	optional MediaType mediaType = 1;
}

enum KeepType {
	UNKNOWN_KEEP_TYPE = 0;
	KEEP_FOR_ALL = 1;
}
`
	if err := os.WriteFile(protoPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing test proto: %v", err)
	}

	referenced := map[string]bool{}

	if err := sanitizeWaE2EProto(protoPath, referenced); err != nil {
		t.Fatalf("sanitizeWaE2EProto failed: %v", err)
	}

	res, err := os.ReadFile(protoPath)
	if err != nil {
		t.Fatalf("reading sanitized proto: %v", err)
	}

	resStr := string(res)
	if strings.Contains(resStr, "JUNK_WEB_ENUM") {
		t.Errorf("expected JUNK_WEB_ENUM to be pruned")
	}
	if !strings.Contains(resStr, "HistorySyncType") {
		t.Errorf("expected canonical HistorySyncType to be preserved")
	}
	if !strings.Contains(resStr, "KeepType") {
		t.Errorf("expected canonical KeepType to be preserved")
	}
	if !strings.Contains(resStr, "enum MediaType") {
		t.Errorf("expected nested enum MediaType inside BCallMessage to be preserved")
	}
}
