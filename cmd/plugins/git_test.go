package plugins

import (
	"context"
	"testing"
	"whatsrook/src"
)

func TestGitCommandRegistered(t *testing.T) {
	cmd, ok := Get("git")
	if !ok || cmd == nil {
		t.Fatalf("git command is not registered in plugins registry")
	}

	if cmd.Name != "git" {
		t.Errorf("expected command name 'git', got %q", cmd.Name)
	}
	if cmd.Category != "tools" {
		t.Errorf("expected category 'tools', got %q", cmd.Category)
	}

	// Verify aliases
	for _, alias := range []string{"github", "gh", "gitclone"} {
		aliasCmd, ok := Get(alias)
		if !ok || aliasCmd == nil {
			t.Errorf("git alias %q not found in registry", alias)
		}
	}
}

func TestGitHelpOutput(t *testing.T) {
	ctx := &src.PluginContext{
		Ctx:     context.Background(),
		Command: "git",
		Args:    []string{"help"},
	}

	// Ensure handleGit routes to help without crashing
	// (Reply might return nil or error when client is nil, but should not panic)
	_ = handleGit(ctx)
}
