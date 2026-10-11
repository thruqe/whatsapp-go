package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient(5 * time.Second)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %v", client.Timeout)
	}

	hybrid, ok := client.Transport.(*HybridTransport)
	if !ok || hybrid == nil {
		t.Fatal("expected client transport to be *HybridTransport")
	}
	if hybrid.H3 == nil {
		t.Fatal("expected non-nil H3 transport")
	}
	if hybrid.TCP == nil {
		t.Fatal("expected non-nil TCP transport")
	}
}

func TestHTTPClientSingleton(t *testing.T) {
	c1 := HTTPClient()
	c2 := HTTPClient()
	if c1 == nil || c2 == nil {
		t.Fatal("expected non-nil clients")
	}
	if c1 != c2 {
		t.Fatal("expected HTTPClient to return singleton instance")
	}
}

func TestHybridTransportTCPRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", `h3=":443"; ma=2592000`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	tr := client.Transport.(*HybridTransport)
	tr.CloseIdleConnections()
}

func TestFetchBytesAndJSON(t *testing.T) {
	type data struct {
		Message string `json:"message"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bytes":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello world"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(data{Message: "success"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()

	b, err := FetchBytes(ctx, server.URL+"/bytes")
	if err != nil {
		t.Fatalf("FetchBytes error: %v", err)
	}
	if string(b) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(b))
	}

	var d data
	err = FetchJSON(ctx, server.URL+"/json", &d)
	if err != nil {
		t.Fatalf("FetchJSON error: %v", err)
	}
	if d.Message != "success" {
		t.Fatalf("expected 'success', got %q", d.Message)
	}
}

func TestPostJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	type response struct {
		Echo string `json:"echo"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response{Echo: fmt.Sprintf("hi %s", p.Name)})
	}))
	defer server.Close()

	ctx := context.Background()
	var res response
	err := PostJSON(ctx, server.URL, payload{Name: "alice"}, &res)
	if err != nil {
		t.Fatalf("PostJSON error: %v", err)
	}
	if res.Echo != "hi alice" {
		t.Fatalf("expected 'hi alice', got %q", res.Echo)
	}
}

func TestDownloadFile(t *testing.T) {
	content := "file content for download"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "sub", "test.txt")

	ctx := context.Background()
	err := DownloadFile(ctx, server.URL, dest)
	if err != nil {
		t.Fatalf("DownloadFile error: %v", err)
	}

	read, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(read) != content {
		t.Fatalf("expected content %q, got %q", content, string(read))
	}
}
