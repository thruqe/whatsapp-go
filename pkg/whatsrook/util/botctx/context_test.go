package botctx

import (
	"testing"
)

func TestPluginContext_GetPrefix(t *testing.T) {
	// Nil context
	var nilCtx *PluginContext
	if p := nilCtx.GetPrefix(); p != "." {
		t.Errorf("nilCtx.GetPrefix() = %q, want \".\"", p)
	}
	if pfxs := nilCtx.GetPrefixes(); len(pfxs) != 1 || pfxs[0] != "." {
		t.Errorf("nilCtx.GetPrefixes() = %v, want [\".\"]", pfxs)
	}

	// Empty context without client
	ctx := &PluginContext{}
	if p := ctx.GetPrefix(); p != "." {
		t.Errorf("ctx.GetPrefix() = %q, want \".\"", p)
	}
}
