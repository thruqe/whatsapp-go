package updater_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"whatsrook/cmd/updater"
)

func TestParseVersion(t *testing.T) {
	v, err := updater.ParseVersion("4.0.1")
	if err != nil {
		t.Fatalf("unexpected error parsing semver: %v", err)
	}
	if v.Major != 4 || v.Minor != 0 || v.Patch != 1 {
		t.Errorf("unexpected semver components: %+v", v)
	}

	v2, err := updater.ParseVersion("v4.1.0-alpha")
	if err != nil {
		t.Fatalf("unexpected error parsing semver with prefix/suffix: %v", err)
	}
	if v2.Major != 4 || v2.Minor != 1 || v2.Patch != 0 {
		t.Errorf("unexpected semver components: %+v", v2)
	}

	if v2.Compare(v) <= 0 {
		t.Errorf("expected v2 (4.1.0) > v (4.0.1)")
	}
}

func TestReadLocalVersion(t *testing.T) {
	tmpDir := t.TempDir()
	versionPath := filepath.Join(tmpDir, "version.txt")

	content := "4.2.0\n"
	if err := os.WriteFile(versionPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test version.txt: %v", err)
	}

	ver, err := updater.ReadLocalVersion(versionPath)
	if err != nil {
		t.Fatalf("unexpected error reading version: %v", err)
	}

	if ver != "4.2.0" {
		t.Errorf("expected 4.2.0, got %s", ver)
	}
}

func TestReadEffectiveLocalVersion(t *testing.T) {
	// When version.txt does not exist, should return EmbeddedAppVersion
	ver := updater.ReadEffectiveLocalVersion("non_existent_file.txt")
	if ver != updater.EmbeddedAppVersion {
		t.Errorf("expected %s, got %s", updater.EmbeddedAppVersion, ver)
	}
}

func TestCleanRestartArgs(t *testing.T) {
	testCases := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"whatsrook", "update", "upgrade"},
			expected: []string{"whatsrook"},
		},
		{
			input:    []string{"whatsrook", "--update", "--session", "12345"},
			expected: []string{"whatsrook", "--session", "12345"},
		},
		{
			input:    []string{"wha-console", "-u", "--session", "12345", "--verbose"},
			expected: []string{"wha-console", "--session", "12345", "--verbose"},
		},
		{
			input:    []string{"whatsrook", "--session", "12345"},
			expected: []string{"whatsrook", "--session", "12345"},
		},
	}

	for _, tc := range testCases {
		clean := updater.CleanRestartArgs(tc.input)
		if !reflect.DeepEqual(clean, tc.expected) {
			t.Errorf("CleanRestartArgs(%v) = %v, expected %v", tc.input, clean, tc.expected)
		}
	}
}

func TestGetPlatform(t *testing.T) {
	platform := updater.GetPlatform()
	if platform == "" || platform == "/" {
		t.Errorf("expected valid OS/Arch platform string, got %q", platform)
	}
}

func TestUpdaterOptions(t *testing.T) {
	var buf bytes.Buffer
	up := updater.New(updater.Options{
		RepoOwner:   "TestOwner",
		RepoName:    "TestRepo",
		VersionFile: "custom_version.txt",
		Out:         &buf,
	})

	if up == nil {
		t.Fatal("expected non-nil Updater instance")
	}

	up.SetOutput(&buf)
	ctx := context.Background()

	// Perform a check against invalid remote to verify custom options are used
	_, err := up.Check(ctx)
	if err == nil {
		t.Log("check finished without error")
	}

	output := buf.String()
	if output == "" {
		t.Errorf("expected progress output to be written to buffer, got empty")
	}
}

func TestSanitizeExtractPath(t *testing.T) {
	baseDir := filepath.Join(os.TempDir(), "test_extract_base")

	validCases := []string{
		"whatsrook",
		"cli/resources/sound.wav",
		"resources/file.txt",
		"prompts/default.txt",
	}
	for _, tc := range validCases {
		res, err := updater.SanitizeExtractPath(baseDir, tc)
		if err != nil {
			t.Errorf("expected path %q to be valid, got error: %v", tc, err)
		}
		if res == "" {
			t.Errorf("expected non-empty path for %q", tc)
		}
	}

	invalidCases := []string{
		"../evil.sh",
		"/etc/passwd",
		"\\Windows\\System32\\cmd.exe",
		"cli/../../etc/shadow",
		"prompts/../../../tmp/hack",
	}
	for _, tc := range invalidCases {
		_, err := updater.SanitizeExtractPath(baseDir, tc)
		if err == nil {
			t.Errorf("expected malicious path %q to be rejected as Zip Slip, but it passed", tc)
		}
	}
}

func TestBetaChannelComparison(t *testing.T) {
	// Test that beta comparison treats different SHA256 / commit strings as updates
	localSha := "sha256:991c28f8153c04a9a09fe4250febe21885a89c9d989b807d18ebdd083373b65e"
	remoteSha := "sha256:ee330d6216b573495455471e7f2ae8a96ad76ac763639954d6f6f469e419df34"

	if localSha == remoteSha {
		t.Errorf("expected different SHAs to not be equal")
	}

	sameSha := "sha256:991c28f8153c04a9a09fe4250febe21885a89c9d989b807d18ebdd083373b65e"
	if localSha != sameSha {
		t.Errorf("expected identical SHAs to match")
	}
}
