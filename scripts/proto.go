package main

import (
	"bytes"
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

	// 1. Setup PATH with Go bin directory (GOPATH/bin, GOBIN)
	setupGoBinPath()

	// 2. Ensure protoc-gen-go plugin is installed and accessible
	if err := ensureProtocGenGo(); err != nil {
		return err
	}

	// 3. Check for protoc compiler
	protocPath, err := exec.LookPath("protoc")
	if err != nil {
		printProtocInstallInstructions()
		return fmt.Errorf("protoc compiler not found in PATH")
	}

	// 4. Normalize option go_package in proto files (ensure go.mau.fi/whatsmeow/proto/...)
	if err := normalizeProtoPackageOptions(protoDir); err != nil {
		return fmt.Errorf("failed to normalize proto go_package options: %w", err)
	}

	// 5. Find all .proto files (with optional filter)
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

	// 6. Execute protoc for each proto file
	successCount := 0
	var failedFiles []string

	for _, rel := range protoFiles {
		protocArgs := []string{
			"--proto_path=" + protoDir,
			"--go_out=" + protoDir,
			"--go_opt=paths=source_relative",
			rel,
		}

		cmd := exec.Command("protoc", protocArgs...)
		cmd.Dir = protoDir
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error compiling %s: %v\n%s\n", rel, err, string(output))
			failedFiles = append(failedFiles, rel)
		} else {
			fmt.Printf("✓ Compiled %s\n", rel)
			successCount++
		}
	}

	fmt.Printf("\nProtobuf update complete: %d/%d files compiled successfully.\n", successCount, len(protoFiles))
	if len(failedFiles) > 0 {
		return fmt.Errorf("%d of %d protobuf file(s) failed to compile: %s", len(failedFiles), len(protoFiles), strings.Join(failedFiles, ", "))
	}

	return nil
}

func setupGoBinPath() {
	goBinDir := getGoBinDir()
	if goBinDir == "" {
		return
	}

	currPath := os.Getenv("PATH")
	paths := filepath.SplitList(currPath)
	for _, p := range paths {
		if p == goBinDir {
			return
		}
	}

	_ = os.Setenv("PATH", goBinDir+string(os.PathListSeparator)+currPath)
}

func getGoBinDir() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}

	if out, err := exec.Command("go", "env", "GOBIN").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}

	if gopath := os.Getenv("GOPATH"); gopath != "" {
		return filepath.Join(gopath, "bin")
	}

	if out, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return filepath.Join(s, "bin")
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "bin")
	}

	return ""
}

func ensureProtocGenGo() error {
	if _, err := exec.LookPath("protoc-gen-go"); err == nil {
		return nil
	}

	goBinDir := getGoBinDir()
	if goBinDir != "" {
		binPath := filepath.Join(goBinDir, "protoc-gen-go")
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}
		if _, err := os.Stat(binPath); err == nil {
			return nil
		}
	}

	fmt.Println("Installing protoc-gen-go plugin...")
	cmd := exec.Command("go", "install", "google.golang.org/protobuf/cmd/protoc-gen-go@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install protoc-gen-go plugin: %w", err)
	}

	setupGoBinPath()
	if _, err := exec.LookPath("protoc-gen-go"); err != nil {
		return fmt.Errorf("protoc-gen-go was installed but could not be located in PATH")
	}

	return nil
}

func normalizeProtoPackageOptions(protoDir string) error {
	const legacyPrefix = "github.com/polymorfa/hypermeow/proto/"
	const targetPrefix = "go.mau.fi/whatsmeow/proto/"

	return filepath.WalkDir(protoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".proto") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if bytes.Contains(data, []byte(legacyPrefix)) {
			updated := bytes.ReplaceAll(data, []byte(legacyPrefix), []byte(targetPrefix))
			if err := os.WriteFile(path, updated, 0644); err != nil {
				return err
			}
		}

		return nil
	})
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
