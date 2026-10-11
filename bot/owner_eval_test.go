package main

import (
	"context"
	"strings"
	"testing"
)

func TestOwnerEvalPreprocessor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:     "unquoted IP address in function call",
			input:    "scanPort(127.0.0.1, 80)",
			contains: []string{`scanPort("127.0.0.1", 80)`},
		},
		{
			name:     "unquoted template URL",
			input:    "const url = http://${targetIp}:${port};",
			contains: []string{"const url = `http://${targetIp}:${port}`;"},
		},
		{
			name:     "unquoted template string label",
			input:    "return { reason: HTTP ${response.status} };",
			contains: []string{"reason: `HTTP ${response.status}`"},
		},
		{
			name:     "code fence stripping",
			input:    "```js\nconst x = 1;\nreturn x;\n```",
			contains: []string{"const x = 1;", "return x;"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := preprocessEvalCode(tc.input)
			for _, exp := range tc.contains {
				if !strings.Contains(got, exp) {
					t.Errorf("preprocessEvalCode(%q) = %q, expected to contain %q", tc.input, got, exp)
				}
			}
		})
	}
}

func TestOwnerEvalExecutionDetailed(t *testing.T) {
	ctx := &Context{
		Ctx: context.Background(),
	}

	// 1. Basic math and return
	res, err := executeJavaScriptEvalDetailed(ctx, "return 1 + 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Value != "3" {
		t.Errorf("expected value '3', got %q", res.Value)
	}

	// 2. CommonJS require modules
	res, err = executeJavaScriptEvalDetailed(ctx, `
		const path = require("path");
		const net = require("net");
		const Buffer = require("buffer");
		return path.join("a", "b") + "|" + typeof net.scanPort + "|" + Buffer.isBuffer(Buffer.from("hi"));
	`)
	if err != nil {
		t.Fatalf("unexpected error with require: %v", err)
	}
	if !strings.Contains(res.Value, "a/b|function|true") {
		t.Errorf("expected 'a/b|function|true', got %q", res.Value)
	}

	// 3. User snippet 1 (bare function with bare return)
	snippet1 := `
function scanPort(targetIp, port) {
    const url = ` + "`" + `http://${targetIp}:${port}` + "`" + `;
    try {
        const response = fetch(url, { timeout: 10 });
        if (response && response.status) {
            return { port: port, status: "open", reason: ` + "`" + `HTTP ${response.status}` + "`" + ` };
        }
        return { port: port, status: "unknown" };
    } catch (error) {
        const errorMsg = error.toString();
        if (errorMsg.includes("connection refused")) {
            return { port: port, status: "closed" };
        }
        if (errorMsg.includes("timeout") || errorMsg.includes("deadline exceeded")) {
            return { port: port, status: "filtered" };
        }
        return { port: port, status: "open/malformed", details: errorMsg };
    }
};

return [80].map(a => scanPort(127.0.0.1, a))
`
	res, err = executeJavaScriptEvalDetailed(ctx, snippet1)
	if err != nil {
		t.Fatalf("snippet 1 failed: %v", err)
	}
	if !strings.Contains(res.Value, `"port": 80`) || !strings.Contains(res.Value, `"status"`) {
		t.Errorf("snippet 1 unexpected result: %s", res.Value)
	}

	// 4. User snippet 2 (IIFE with unquoted URL & unquoted template string)
	snippet2 := `
(function () {
  function scanPort(targetIp, port) {
    const url = http://${targetIp}:${port};
    try {
        const response = fetch(url, { timeout: 10 });
        if (response && response.status) {
            return { port: port, status: "open", reason: HTTP ${response.status} };
        }
        return { port: port, status: "unknown" };
    } catch (error) {
        const errorMsg = error.toString();
        if (errorMsg.includes("connection refused")) {
            return { port: port, status: "closed" };
        }
        if (errorMsg.includes("timeout") || errorMsg.includes("deadline exceeded")) {
            return { port: port, status: "filtered" };
        }
        return { port: port, status: "open/malformed", details: errorMsg };
    }
};

return [80].map(a => scanPort(127.0.0.1, a))
})()
`
	res, err = executeJavaScriptEvalDetailed(ctx, snippet2)
	if err != nil {
		t.Fatalf("snippet 2 failed: %v", err)
	}
	if !strings.Contains(res.Value, `"port": 80`) || !strings.Contains(res.Value, `"status"`) {
		t.Errorf("snippet 2 unexpected result: %s", res.Value)
	}
}
