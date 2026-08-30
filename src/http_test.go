package src

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnifiedHTTPClient(t *testing.T) {
	// Setup test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bytes":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hello world unified http"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "agent": r.Header.Get("User-Agent")})
		case "/echo-post":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"received": payload,
				"auth":     r.Header.Get("Authorization"),
			})
		case "/download":
			_, _ = w.Write([]byte("file-content-stream-data"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Test FetchURLBytes
	data, err := FetchURLBytes(ctx, server.URL+"/bytes")
	if err != nil {
		t.Fatalf("FetchURLBytes failed: %v", err)
	}
	if string(data) != "hello world unified http" {
		t.Fatalf("unexpected content: %s", string(data))
	}

	// 2. Test FetchJSON
	var jsonRes struct {
		Status string `json:"status"`
		Agent  string `json:"agent"`
	}
	err = FetchJSON(ctx, server.URL+"/json", &jsonRes, WithUserAgent("CustomAgent/1.0"))
	if err != nil {
		t.Fatalf("FetchJSON failed: %v", err)
	}
	if jsonRes.Status != "ok" || jsonRes.Agent != "CustomAgent/1.0" {
		t.Fatalf("unexpected JSON response: %+v", jsonRes)
	}

	// 3. Test PostJSON
	reqPayload := map[string]string{"msg": "ping"}
	var postRes struct {
		Received map[string]string `json:"received"`
		Auth     string            `json:"auth"`
	}
	err = PostJSON(ctx, server.URL+"/echo-post", reqPayload, &postRes, WithBearerToken("my-secret-token"))
	if err != nil {
		t.Fatalf("PostJSON failed: %v", err)
	}
	if postRes.Received["msg"] != "ping" || postRes.Auth != "Bearer my-secret-token" {
		t.Fatalf("unexpected PostJSON response: %+v", postRes)
	}

	// 4. Test DownloadFile
	tmpDir, err := os.MkdirTemp("", "http_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	destFile := filepath.Join(tmpDir, "download.txt")
	err = DownloadFile(ctx, server.URL+"/download", destFile)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	content, err := os.ReadFile(destFile)
	if err != nil || string(content) != "file-content-stream-data" {
		t.Fatalf("unexpected downloaded content: %s (err: %v)", string(content), err)
	}

	// 5. Test Get and Post raw responses
	getResp, err := Get(ctx, server.URL+"/bytes")
	if err != nil || getResp.StatusCode != http.StatusOK {
		t.Fatalf("Get failed: %v", err)
	}
	getResp.Body.Close()

	postResp, err := Post(ctx, server.URL+"/bytes", "text/plain", strings.NewReader("sample"))
	if err != nil || postResp.StatusCode != http.StatusOK {
		t.Fatalf("Post failed: %v", err)
	}
	postResp.Body.Close()
}

func TestHybridTransportQUICFallback(t *testing.T) {
	// Test TLS server to verify HTTPS transport fallback handling
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tls response from test server"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := server.Client()
	transport := NewHybridTransport()
	// Allow self-signed test cert
	transport.h2Transport.TLSClientConfig = client.Transport.(*http.Transport).TLSClientConfig
	transport.h3Transport.TLSClientConfig = client.Transport.(*http.Transport).TLSClientConfig

	customClient := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create TLS request: %v", err)
	}

	resp, err := customClient.Do(req)
	if err != nil {
		t.Fatalf("customClient.Do failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}
