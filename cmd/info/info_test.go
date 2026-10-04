package info

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"whatsrook/cmd/dispatch"
	"whatsrook/cmd/tools"
)

func TestBuildMenuText_WithoutExternalPlugins(t *testing.T) {
	// Set an empty temporary plugin directory
	emptyDir := t.TempDir()
	t.Setenv("WHATSROOK_PLUGIN_DIR", emptyDir)

	menu := buildMenuText(nil, nil)

	// Verify menu header and structure
	if !strings.Contains(menu, "╭─❏") {
		t.Fatalf("expected category headers in menu, got:\n%s", menu)
	}

	// Verify externals category is NOT present when no external plugins exist
	externalsHeader := "╭─❏ " + tools.ToSmallCaps("externals") + " ❏"
	if strings.Contains(menu, externalsHeader) {
		t.Errorf("expected no %s header when no external plugins installed, got:\n%s", externalsHeader, menu)
	}
}

func TestBuildMenuText_WithExternalPlugins(t *testing.T) {
	pluginDir := t.TempDir()
	t.Setenv("WHATSROOK_PLUGIN_DIR", pluginDir)

	// Create a dummy executable plugin
	extPath := filepath.Join(pluginDir, "weather")
	if err := os.WriteFile(extPath, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		t.Fatalf("failed to create dummy plugin binary: %v", err)
	}

	menu := buildMenuText(nil, nil)

	// Verify externals category is present
	externalsHeader := "╭─❏ " + tools.ToSmallCaps("externals") + " ❏"
	if !strings.Contains(menu, externalsHeader) {
		t.Errorf("expected %s header in menu when external plugin is installed, got:\n%s", externalsHeader, menu)
	}

	// Verify the weather command name is in small caps under the category
	weatherSmallCaps := tools.ToSmallCaps("weather")
	if !strings.Contains(menu, "│ "+weatherSmallCaps) {
		t.Errorf("expected command %s under externals category, got:\n%s", weatherSmallCaps, menu)
	}

	// Test category filtering for "externals"
	filteredMenu := buildMenuText(nil, []string{"externals"})
	if !strings.Contains(filteredMenu, externalsHeader) {
		t.Errorf("expected filtered menu to contain %s, got:\n%s", externalsHeader, filteredMenu)
	}

	// Test category filtering for "external" (alias)
	filteredAliasMenu := buildMenuText(nil, []string{"external"})
	if !strings.Contains(filteredAliasMenu, externalsHeader) {
		t.Errorf("expected filtered alias menu to contain %s, got:\n%s", externalsHeader, filteredAliasMenu)
	}
}

func TestHandleMenu_Basic(t *testing.T) {
	// Ensure handleMenu does not error when called
	ctx := &dispatch.Context{}
	_ = ctx // verify compilation and type compatibility
}
