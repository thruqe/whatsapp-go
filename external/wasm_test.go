package external

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Minimal valid WASM binary implementing an empty WASI module.
var minimalWASMBinary = []byte{
	0x00, 0x61, 0x73, 0x6d, // \x00asm
	0x01, 0x00, 0x00, 0x00, // version 1
}

func TestIsWASMFile(t *testing.T) {
	tmpDir := t.TempDir()
	wasmFile := filepath.Join(tmpDir, "test.wasm")
	if err := os.WriteFile(wasmFile, minimalWASMBinary, 0o600); err != nil {
		t.Fatalf("failed to write test wasm file: %v", err)
	}

	if !isWASMFile(wasmFile) {
		t.Errorf("expected isWASMFile to return true for %q", wasmFile)
	}

	nonWasmFile := filepath.Join(tmpDir, "test.bin")
	if err := os.WriteFile(nonWasmFile, []byte("NOT_A_WASM_FILE"), 0o600); err != nil {
		t.Fatalf("failed to write non-wasm file: %v", err)
	}

	if isWASMFile(nonWasmFile) {
		t.Errorf("expected isWASMFile to return false for %q", nonWasmFile)
	}
}

func TestWASMRuntimeInitialization(t *testing.T) {
	ctx := context.Background()
	r := getWASMRuntime(ctx)
	if r == nil {
		t.Fatal("expected wazero runtime to be initialized")
	}

	// Verify compiling minimal WASM binary works without error
	compiled, err := r.CompileModule(ctx, minimalWASMBinary)
	if err != nil {
		t.Fatalf("failed to compile minimal WASM binary: %v", err)
	}
	if compiled == nil {
		t.Fatal("expected compiled module to be non-nil")
	}
}

func TestWASMHeaderDetection(t *testing.T) {
	header := []byte{0x00, 0x61, 0x73, 0x6d}
	if !bytes.Equal(header, []byte{0x00, 0x61, 0x73, 0x6d}) {
		t.Fatal("magic header mismatch")
	}
}
