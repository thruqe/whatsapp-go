package cliutils

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitTarget(t *testing.T) {
	tests := []struct {
		input        string
		expectOwner  string
		expectRepo   string
		expectHost   string
		expectBranch string
		expectErr    bool
	}{
		{
			input:        "Thruqe/whatsrook",
			expectOwner:  "Thruqe",
			expectRepo:   "whatsrook",
			expectHost:   "github.com",
			expectBranch: "",
			expectErr:    false,
		},
		{
			input:        "https://github.com/gin-gonic/gin",
			expectOwner:  "gin-gonic",
			expectRepo:   "gin",
			expectHost:   "github.com",
			expectBranch: "",
			expectErr:    false,
		},
		{
			input:        "https://github.com/golang/go/tree/dev.boringcrypto",
			expectOwner:  "golang",
			expectRepo:   "go",
			expectHost:   "github.com",
			expectBranch: "dev.boringcrypto",
			expectErr:    false,
		},
		{
			input:        "https://gitlab.com/inkscape/inkscape.git",
			expectOwner:  "inkscape",
			expectRepo:   "inkscape",
			expectHost:   "gitlab.com",
			expectBranch: "",
			expectErr:    false,
		},
		{
			input:        "git@github.com:torvalds/linux.git",
			expectOwner:  "torvalds",
			expectRepo:   "linux",
			expectHost:   "github.com",
			expectBranch: "",
			expectErr:    false,
		},
		{
			input:     "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		target, err := ParseGitTarget(tt.input)
		if tt.expectErr {
			if err == nil {
				t.Errorf("ParseGitTarget(%q) expected error, got nil", tt.input)
			}
			continue
		}

		if err != nil {
			t.Fatalf("ParseGitTarget(%q) unexpected error: %v", tt.input, err)
		}

		if target.Owner != tt.expectOwner {
			t.Errorf("ParseGitTarget(%q) owner = %q, want %q", tt.input, target.Owner, tt.expectOwner)
		}
		if target.Repo != tt.expectRepo {
			t.Errorf("ParseGitTarget(%q) repo = %q, want %q", tt.input, target.Repo, tt.expectRepo)
		}
		if target.Host != tt.expectHost {
			t.Errorf("ParseGitTarget(%q) host = %q, want %q", tt.input, target.Host, tt.expectHost)
		}
		if tt.expectBranch != "" && target.Branch != tt.expectBranch {
			t.Errorf("ParseGitTarget(%q) branch = %q, want %q", tt.input, target.Branch, tt.expectBranch)
		}
	}
}

func TestCreateZipAndTarGzFromDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	file1 := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(file1, []byte("# Test Project"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	file2 := filepath.Join(subDir, "main.go")
	if err := os.WriteFile(file2, []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 1. Test Zip
	var zipBuf bytes.Buffer
	if err := CreateZipFromDir(tempDir, &zipBuf); err != nil {
		t.Fatalf("CreateZipFromDir failed: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBuf.Bytes()), int64(zipBuf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader failed: %v", err)
	}

	foundReadme := false
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "README.md" {
			foundReadme = true
			break
		}
	}
	if !foundReadme {
		t.Errorf("README.md not found in zip archive")
	}

	// 2. Test Tar.Gz
	var tarBuf bytes.Buffer
	if err := CreateTarGzFromDir(tempDir, &tarBuf); err != nil {
		t.Fatalf("CreateTarGzFromDir failed: %v", err)
	}

	gr, err := gzip.NewReader(&tarBuf)
	if err != nil {
		t.Fatalf("gzip.NewReader failed: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	foundTarReadme := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next failed: %v", err)
		}
		if filepath.Base(hdr.Name) == "README.md" {
			foundTarReadme = true
		}
	}
	if !foundTarReadme {
		t.Errorf("README.md not found in tar.gz archive")
	}
}

func TestDecodeBase64Content(t *testing.T) {
	encoded := "SGVsbG8gV29ybGQh\n"
	decoded, err := DecodeBase64Content(encoded)
	if err != nil {
		t.Fatalf("DecodeBase64Content failed: %v", err)
	}
	if string(decoded) != "Hello World!" {
		t.Errorf("got %q, want %q", string(decoded), "Hello World!")
	}
}
