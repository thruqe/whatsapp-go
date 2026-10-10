package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"grol.io/grol/object"
	"grol.io/grol/repl"

	"whatsrook/cmd/dispatch"
)

func TestGrolCommandRegistration(t *testing.T) {
	cmd, ok := dispatch.Get("grol")
	if !ok || cmd == nil {
		t.Fatalf("expected command 'grol' to be registered")
	}
	if cmd.Category != "tools" {
		t.Errorf("expected category 'tools', got %q", cmd.Category)
	}
	if !cmd.IsPublic {
		t.Errorf("expected command to be public")
	}

	aliasCmd, okAlias := dispatch.Get("gr")
	if !okAlias || aliasCmd != cmd {
		t.Errorf("expected alias 'gr' to map to grol command")
	}
}

func TestNormalizeSmartQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: `“hello world”`, want: `"hello world"`},
		{input: `‘test’`, want: `'test'`},
		{input: `«french»`, want: `"french"`},
		{input: `"already ascii"`, want: `"already ascii"`},
	}

	for _, tt := range tests {
		got := normalizeSmartQuotes(tt.input)
		if got != tt.want {
			t.Errorf("normalizeSmartQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "```grol\n1 + 2\n```", want: "1 + 2"},
		{input: "```go\nfunc add(a, b) { a + b }\n```", want: "func add(a, b) { a + b }"},
		{input: "```\nx = 10\n```", want: "x = 10"},
		{input: "`2 * 3`", want: "2 * 3"},
		{input: "4 + 5", want: "4 + 5"},
	}

	for _, tt := range tests {
		got := stripMarkdownFences(tt.input)
		if got != tt.want {
			t.Errorf("stripMarkdownFences(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseGrolFlags(t *testing.T) {
	tests := []struct {
		input       string
		wantFormat  bool
		wantParse   bool
		wantCompact bool
		wantClean   bool
		wantReset   bool
		wantHelp    bool
		wantCode    string
	}{
		{
			input:    "1 + 2",
			wantCode: "1 + 2",
		},
		{
			input:      "-f func add(a,b){return a+b}",
			wantFormat: true,
			wantCode:   "func add(a,b){return a+b}",
		},
		{
			input:     "-p 2 * 3",
			wantParse: true,
			wantCode:  "2 * 3",
		},
		{
			input:       "-c -clean [1, 2, 3]",
			wantCompact: true,
			wantClean:   true,
			wantCode:    "[1, 2, 3]",
		},
		{
			input:     "reset",
			wantReset: true,
		},
		{
			input:    "help",
			wantHelp: true,
		},
		{
			input:    "-5 + 10",
			wantCode: "-5 + 10",
		},
	}

	for _, tt := range tests {
		flags := parseGrolFlags(tt.input)
		if flags.formatOnly != tt.wantFormat {
			t.Errorf("parseGrolFlags(%q).formatOnly = %v, want %v", tt.input, flags.formatOnly, tt.wantFormat)
		}
		if flags.showParse != tt.wantParse {
			t.Errorf("parseGrolFlags(%q).showParse = %v, want %v", tt.input, flags.showParse, tt.wantParse)
		}
		if flags.compact != tt.wantCompact {
			t.Errorf("parseGrolFlags(%q).compact = %v, want %v", tt.input, flags.compact, tt.wantCompact)
		}
		if flags.cleanState != tt.wantClean {
			t.Errorf("parseGrolFlags(%q).cleanState = %v, want %v", tt.input, flags.cleanState, tt.wantClean)
		}
		if flags.resetState != tt.wantReset {
			t.Errorf("parseGrolFlags(%q).resetState = %v, want %v", tt.input, flags.resetState, tt.wantReset)
		}
		if flags.showHelp != tt.wantHelp {
			t.Errorf("parseGrolFlags(%q).showHelp = %v, want %v", tt.input, flags.showHelp, tt.wantHelp)
		}
		if flags.code != tt.wantCode {
			t.Errorf("parseGrolFlags(%q).code = %q, want %q", tt.input, flags.code, tt.wantCode)
		}
	}
}

func TestRunGrolEvaluation(t *testing.T) {
	ctx := context.Background()
	sessionKey := "test-session-eval"

	// 1. Math evaluation
	res, err := runGrol(ctx, sessionKey, "10 + 32", grolParsedFlags{})
	if err != nil {
		t.Fatalf("runGrol failed: %v", err)
	}
	if !strings.Contains(res, "42") {
		t.Errorf("expected output to contain 42, got %q", res)
	}

	// 2. State retention across same session
	_, err = runGrol(ctx, sessionKey, "my_val = 100", grolParsedFlags{})
	if err != nil {
		t.Fatalf("runGrol failed: %v", err)
	}

	res2, err := runGrol(ctx, sessionKey, "my_val * 2", grolParsedFlags{})
	if err != nil {
		t.Fatalf("runGrol failed: %v", err)
	}
	if !strings.Contains(res2, "200") {
		t.Errorf("expected session variable retention to produce 200, got %q", res2)
	}

	// 3. Reset session
	deleted := sessionsRegistry.deleteSession(sessionKey)
	if !deleted {
		t.Errorf("expected session to be deleted")
	}

	// 4. After reset, variable should not be found
	res3, _ := runGrol(ctx, sessionKey, "my_val", grolParsedFlags{})
	if !strings.Contains(res3, "identifier not found") {
		t.Errorf("expected identifier not found error after reset, got %q", res3)
	}
}

func TestRunGrolFormatOnly(t *testing.T) {
	ctx := context.Background()
	res, err := runGrol(ctx, "test-format", "func add(a,b){return a+b}", grolParsedFlags{formatOnly: true})
	if err != nil {
		t.Fatalf("runGrol format failed: %v", err)
	}
	if !strings.Contains(res, "*Grol Formatted:*") {
		t.Errorf("expected format header, got %q", res)
	}
	if !strings.Contains(res, "func add(a, b)") {
		t.Errorf("expected formatted code, got %q", res)
	}
}

func TestRunGrolShowParse(t *testing.T) {
	ctx := context.Background()
	res, err := runGrol(ctx, "test-parse", "2 + 3 * 4", grolParsedFlags{showParse: true})
	if err != nil {
		t.Fatalf("runGrol parse failed: %v", err)
	}
	if !strings.Contains(res, "*Grol Parse & Eval Result:*") {
		t.Errorf("expected parse header, got %q", res)
	}
	if !strings.Contains(res, "== Parse ==>") || !strings.Contains(res, "== Eval  ==>") {
		t.Errorf("expected parse and eval lines, got %q", res)
	}
}

func TestRunGrolBuiltinsAndMath(t *testing.T) {
	ctx := context.Background()
	sessionKey := "test-session-builtins"

	res, err := runGrol(ctx, sessionKey, "sqrt(49)", grolParsedFlags{cleanState: true})
	if err != nil {
		t.Fatalf("runGrol math failed: %v", err)
	}
	if !strings.Contains(res, "7") {
		t.Errorf("expected sqrt(49) = 7, got %q", res)
	}
}

func TestRunGrolTimeoutProtection(t *testing.T) {
	ctx := context.Background()
	sessionKey := "test-session-timeout"

	// Sleep longer than timeout should be cancelled
	start := time.Now()
	res, _ := runGrol(ctx, sessionKey, "sleep(10)", grolParsedFlags{cleanState: true})
	elapsed := time.Since(start)

	if elapsed > 7*time.Second {
		t.Errorf("expected evaluation to timeout within ~5 seconds, took %v", elapsed)
	}
	if !strings.Contains(res, "context deadline exceeded") {
		t.Errorf("expected timeout error in result, got %q", res)
	}
}

func TestGrolButterfly(t *testing.T) {
	initGrol()
	sessionKey := "test-butterfly"
	sess := sessionsRegistry.getSession(sessionKey)
	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Load discord.gr into sess.state
	opts := repl.EvalStringOptions()
	opts.MaxDepth = 3000
	buf := &strings.Builder{}
	sess.state.Out = buf
	sess.state.LogOut = buf
	sess.state.NoLog = true
	_, _, errs, _ := repl.EvalOne(context.Background(), sess.state, discordLibraryCode, buf, opts)
	if len(errs) > 0 {
		t.Fatalf("errors loading discord.gr: %v", errs)
	}

	imageSent := false
	sess.state.Extensions["SendImage"] = object.Extension{
		Name:     "SendImage",
		MinArgs:  1,
		MaxArgs:  2,
		ArgTypes: []object.Type{object.STRING, object.STRING},
		Callback: func(cdata any, _ string, args []object.Object) object.Object {
			imageSent = true
			return object.String{Value: "mock_id"}
		},
	}

	start := time.Now()
	// Run Butterfly()
	evalOpts := repl.EvalStringOptions()
	evalOpts.MaxDuration = 15 * time.Second
	buf.Reset()
	_, _, errs, _ = repl.EvalOne(context.Background(), sess.state, "Butterfly()", buf, evalOpts)
	elapsed := time.Since(start)
	t.Logf("Butterfly took %v, out: %s, errs: %v", elapsed, buf.String(), errs)
	if len(errs) > 0 {
		t.Fatalf("errors running Butterfly: %v", errs)
	}
	if !imageSent {
		t.Errorf("expected SendImage to be called")
	}
}

func TestRunGrolButterflyIntegration(t *testing.T) {
	ctx := context.Background()
	res, err := runGrol(ctx, "session-butterfly-direct", "Butterfly()", grolParsedFlags{})
	if err != nil {
		t.Fatalf("runGrol Butterfly failed: %v", err)
	}
	if !strings.Contains(res, "Time elapsed:") {
		t.Errorf("expected Time elapsed in Butterfly output, got %q", res)
	}
}
