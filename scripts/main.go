package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// scripts/ is expected to be a direct child of the repo root
	rootDir := filepath.Dir(wd)
	protoDir := filepath.Join(rootDir, "proto")
	protoSrc := filepath.Join(protoDir, "ws.proto")
	outDir := filepath.Join(protoDir, "wsproto")

	if _, err := os.Stat(filepath.Join(protoDir, "buf.yaml")); err != nil {
		return fmt.Errorf("buf.yaml not found in %s: %w", protoDir, err)
	}
	if _, err := os.Stat(filepath.Join(protoDir, "buf.gen.yaml")); err != nil {
		return fmt.Errorf("buf.gen.yaml not found in %s: %w", protoDir, err)
	}
	if _, err := os.Stat(protoSrc); err != nil {
		return fmt.Errorf("proto source not found: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	fmt.Printf("Generating Go code from %s...\n", protoSrc)

	cmd := exec.Command("go", "run", "github.com/bufbuild/buf/cmd/buf@v1.32.0", "generate")
	cmd.Dir = protoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buf generate failed: %w", err)
	}

	fmt.Printf("Successfully generated Protobuf code in %s\n", outDir)
	return nil
}
