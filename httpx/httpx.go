// httpx package provides a high-performance, unified HTTP client layer for WhatsRook.
//
// it transparently supports HTTP/3 (QUIC) with automatic fallback to HTTP/2 and HTTP/1.1,
// connection pooling, automated retry logic, and configurable timeouts for outgoing requests.
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	// DefaultUserAgent is the standard User-Agent header used for outbound HTTP requests.
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 WhatsRook/1.0"
	// DefaultHTTPTimeout is the default duration before an outbound request is aborted.
	DefaultHTTPTimeout = 30 * time.Second
)

// RequestOption configures an outgoing HTTP request.
type RequestOption func(*http.Request)

// WithHeader sets a single request header.
func WithHeader(key, value string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(key, value)
	}
}

// WithHeaders sets multiple request headers.
func WithHeaders(headers map[string]string) RequestOption {
	return func(req *http.Request) {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
}

// WithBearerToken sets the Authorization header with a Bearer token.
func WithBearerToken(token string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set("User-Agent", ua)
	}
}

// HybridTransport transparently supports HTTP/3 (QUIC) with fallback to HTTP/2 and HTTP/1.1.
type HybridTransport struct {
	h3Transport *http3.Transport
	tcpClient   *http.Client
	h3Supported sync.Map // host -> bool
}

// NewHybridTransport constructs a HybridTransport instance with sensible defaults.
func NewHybridTransport() *HybridTransport {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	tcpTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	h3Transport := &http3.Transport{
		TLSClientConfig: tlsConfig,
		QUICConfig: &quic.Config{
			MaxIdleTimeout:  30 * time.Second,
			KeepAlivePeriod: 10 * time.Second,
		},
	}

	return &HybridTransport{
		h3Transport: h3Transport,
		tcpClient: &http.Client{
			Transport: tcpTransport,
			Timeout:   DefaultHTTPTimeout,
		},
	}
}

// NewClient constructs an http.Client equipped with the HybridTransport and the requested timeout.
func NewClient(timeout ...time.Duration) *http.Client {
	t := DefaultHTTPTimeout
	if len(timeout) > 0 {
		t = timeout[0]
	}
	return &http.Client{
		Transport: NewHybridTransport(),
		Timeout:   t,
	}
}

// RoundTrip executes a single HTTP transaction.
func (t *HybridTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.tcpClient.Transport.RoundTrip(req)
	}

	host := req.URL.Host
	if supported, ok := t.h3Supported.Load(host); ok && supported.(bool) {
		resp, err := t.h3Transport.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		t.h3Supported.Store(host, false)
	}

	resp, err := t.tcpClient.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if altSvc := resp.Header.Get("Alt-Svc"); altSvc != "" {
		t.h3Supported.Store(host, true)
	}

	return resp, nil
}

// CloseIdleConnections cleans up pooled idle connections.
func (t *HybridTransport) CloseIdleConnections() {
	if transport, ok := t.tcpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	_ = t.h3Transport.Close()
}

var (
	defaultClient     *http.Client
	defaultClientOnce sync.Once
)

// HTTPClient returns the shared global HTTP client configured with HybridTransport (HTTP/3 + HTTP/2).
func HTTPClient() *http.Client {
	defaultClientOnce.Do(func() {
		defaultClient = &http.Client{
			Transport: NewHybridTransport(),
			Timeout:   DefaultHTTPTimeout,
		}
	})
	return defaultClient
}

// FetchBytes issues a GET request to rawURL and returns the response body bytes.
func FetchBytes(ctx context.Context, rawURL string, opts ...RequestOption) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("httpx: build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpx: fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("httpx: fetch %s: unexpected status %s", rawURL, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("httpx: read %s body: %w", rawURL, err)
	}
	return data, nil
}

// FetchJSON issues a GET request and decodes the JSON response into target.
func FetchJSON(ctx context.Context, rawURL string, target any, opts ...RequestOption) error {
	data, err := FetchBytes(ctx, rawURL, opts...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("httpx: unmarshal json from %s: %w", rawURL, err)
	}
	return nil
}

// PostJSON issues a POST request with payload encoded as JSON, and decodes the response into target.
func PostJSON(ctx context.Context, rawURL string, payload any, target any, opts ...RequestOption) error {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("httpx: marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("httpx: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("httpx: post %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("httpx: post %s: status %s: %s", rawURL, resp.Status, string(respBody))
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("httpx: decode response from %s: %w", rawURL, err)
		}
	}
	return nil
}

// DownloadFile downloads a remote file to a local destination path.
func DownloadFile(ctx context.Context, rawURL, destPath string, opts ...RequestOption) error {
	data, err := FetchBytes(ctx, rawURL, opts...)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0644)
}
