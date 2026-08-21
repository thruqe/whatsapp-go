package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func runProto(args []string) error {
	rootDir, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("error finding project root: %w", err)
	}

	protoDir := filepath.Join(rootDir, "wa-core", "proto")
	if _, err := os.Stat(protoDir); os.IsNotExist(err) {
		return fmt.Errorf("protobuf directory not found at: %s", protoDir)
	}

	// 1. Ensure protoc-gen-go is installed
	ensureProtocGenGo()

	// 2. Check for protoc compiler
	protocPath, err := exec.LookPath("protoc")
	if err != nil {
		printProtocInstallInstructions()
		return fmt.Errorf("protoc compiler not found")
	}

	// 3. Find all .proto files
	var protoFiles []string
	targetFilter := ""
	if len(args) > 0 {
		targetFilter = strings.ToLower(args[0])
	}

	err = filepath.WalkDir(protoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".proto") {
			relPath, relErr := filepath.Rel(protoDir, path)
			if relErr == nil {
				if targetFilter == "" || strings.Contains(strings.ToLower(relPath), targetFilter) {
					protoFiles = append(protoFiles, relPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan proto files: %w", err)
	}

	if len(protoFiles) == 0 {
		fmt.Println("No .proto files found matching criteria.")
		return nil
	}

	fmt.Printf("Found %d protobuf file(s) to compile using %s...\n", len(protoFiles), protocPath)

	// Ensure GOPATH/bin is in PATH for protoc plugins
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath != "" {
		gobin := filepath.Join(gopath, "bin")
		currPath := os.Getenv("PATH")
		if !strings.Contains(currPath, gobin) {
			_ = os.Setenv("PATH", gobin+string(os.PathListSeparator)+currPath)
		}
	}

	// 4. Execute protoc for each proto file
	successCount := 0
	for _, rel := range protoFiles {
		args := []string{
			"--proto_path=" + protoDir,
			"--go_out=" + protoDir,
			"--go_opt=paths=source_relative",
			rel,
		}

		cmd := exec.Command("protoc", args...)
		cmd.Dir = protoDir
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error compiling %s: %v\n%s\n", rel, err, string(output))
		} else {
			fmt.Printf("✓ Compiled %s\n", rel)
			successCount++
		}
	}

	fmt.Printf("\nProtobuf update complete: %d/%d files compiled successfully.\n", successCount, len(protoFiles))
	return nil
}

func ensureProtocGenGo() {
	if _, err := exec.LookPath("protoc-gen-go"); err == nil {
		return
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath != "" {
		binPath := filepath.Join(gopath, "bin", "protoc-gen-go")
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}
		if _, err := os.Stat(binPath); err == nil {
			return
		}
	}

	fmt.Println("Installing protoc-gen-go plugin...")
	cmd := exec.Command("go", "install", "google.golang.org/protobuf/cmd/protoc-gen-go@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to automatically install protoc-gen-go: %v\n", err)
	}
}

func printProtocInstallInstructions() {
	fmt.Fprintln(os.Stderr, "❌ Error: 'protoc' (Protocol Buffer Compiler) is not installed or not found in PATH.")
	fmt.Fprintln(os.Stderr, "\nPlease install protoc using your package manager:")
	switch runtime.GOOS {
	case "darwin":
		fmt.Fprintln(os.Stderr, "  brew install protobuf")
	case "linux":
		fmt.Fprintln(os.Stderr, "  Debian/Ubuntu: sudo apt-get install protobuf-compiler")
		fmt.Fprintln(os.Stderr, "  Fedora/RHEL:   sudo dnf install protobuf-compiler")
		fmt.Fprintln(os.Stderr, "  Arch Linux:    sudo pacman -S protobuf")
		fmt.Fprintln(os.Stderr, "  Homebrew:      brew install protobuf")
	case "windows":
		fmt.Fprintln(os.Stderr, "  winget install ProtocolBuffers.Protoc")
		fmt.Fprintln(os.Stderr, "  scoop install protobuf")
		fmt.Fprintln(os.Stderr, "  choco install protoc")
	}
	fmt.Fprintln(os.Stderr, "\nOr download official binary releases from:")
	fmt.Fprintln(os.Stderr, "  https://github.com/protocolbuffers/protobuf/releases")
}
