package src

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
	h2Transport *http.Transport

	// quicFailedHosts caches hosts where HTTP/3 failed or was unavailable (to avoid retrying QUIC repeatedly on non-QUIC servers)
	quicFailedHosts sync.Map
}

// NewHybridTransport initializes a unified HTTP/3 + HTTP/2 + HTTP/1.1 transport.
func NewHybridTransport() *HybridTransport {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	h2Transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}

	h3Transport := &http3.Transport{
		TLSClientConfig: tlsConfig,
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout: 5 * time.Second,
			MaxIdleTimeout:       30 * time.Second,
			KeepAlivePeriod:      15 * time.Second,
		},
	}

	return &HybridTransport{
		h3Transport: h3Transport,
		h2Transport: h2Transport,
	}
}

func (t *HybridTransport) isQUICDisabledForHost(host string) bool {
	if exp, ok := t.quicFailedHosts.Load(host); ok {
		if expireTime, okTime := exp.(time.Time); okTime && time.Now().Before(expireTime) {
			return true
		}
		t.quicFailedHosts.Delete(host)
	}
	return false
}

func (t *HybridTransport) markQUICFailedForHost(host string) {
	t.quicFailedHosts.Store(host, time.Now().Add(5*time.Minute))
}

// RoundTrip executes a single HTTP transaction, attempting HTTP/3 for HTTPS and falling back to HTTP/2/1.1.
func (t *HybridTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || req.URL.Scheme != "https" || t.isQUICDisabledForHost(req.URL.Host) {
		return t.h2Transport.RoundTrip(req)
	}

	// Buffer request body if rewind is required on fallback
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Use a fast probe timeout (1.5s) for HTTP/3 so non-QUIC servers fallback immediately without consuming the client timeout
	h3Ctx := req.Context()
	probeCtx, cancelProbe := context.WithTimeout(h3Ctx, 1500*time.Millisecond)
	defer cancelProbe()

	h3Req := req.Clone(probeCtx)
	if len(bodyBytes) > 0 {
		h3Req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	resp, err := t.h3Transport.RoundTrip(h3Req)
	if err == nil {
		return resp, nil
	}

	// Mark host as failing HTTP/3 temporarily so future requests go straight to HTTP/2
	t.markQUICFailedForHost(req.URL.Host)

	// Fallback to HTTP/2 / HTTP/1.1 with original request context
	h2Req := req.Clone(req.Context())
	if len(bodyBytes) > 0 {
		h2Req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return t.h2Transport.RoundTrip(h2Req)
}

// CloseIdleConnections closes all idle connections across HTTP/3 and HTTP/2 transports.
func (t *HybridTransport) CloseIdleConnections() {
	if t.h3Transport != nil {
		t.h3Transport.CloseIdleConnections()
	}
	if t.h2Transport != nil {
		t.h2Transport.CloseIdleConnections()
	}
}

var (
	defaultTransport  = NewHybridTransport()
	defaultHTTPClient = &http.Client{
		Transport: defaultTransport,
		Timeout:   DefaultHTTPTimeout,
	}
)

// DefaultHTTPClient returns the shared HTTP/3-capable HTTP client.
func DefaultHTTPClient() *http.Client {
	return defaultHTTPClient
}

// NewHTTPClient creates a new HTTP/3-capable client with a custom timeout.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: defaultTransport,
		Timeout:   timeout,
	}
}

// FetchURLBytes fetches the content of a target URL over HTTP GET using the unified HTTP/3 client.
func FetchURLBytes(ctx context.Context, targetURL string, opts ...RequestOption) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP status %d fetching URL", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// FetchJSON fetches JSON from targetURL and unmarshals it into target.
func FetchJSON(ctx context.Context, targetURL string, target any, opts ...RequestOption) error {
	data, err := FetchURLBytes(ctx, targetURL, opts...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// PostJSON marshals body to JSON and performs an HTTP POST, decoding response JSON into target if provided.
func PostJSON(ctx context.Context, targetURL string, body any, target any, opts ...RequestOption) error {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP POST failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

// DownloadFile streams a remote file to destinationPath.
func DownloadFile(ctx context.Context, targetURL, destPath string, opts ...RequestOption) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d downloading file", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// Get performs an HTTP GET request with the unified client and returns the raw *http.Response.
func Get(ctx context.Context, targetURL string, opts ...RequestOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}
	return defaultHTTPClient.Do(req)
}

// Post performs an HTTP POST request with the unified client and returns the raw *http.Response.
func Post(ctx context.Context, targetURL, contentType string, body io.Reader, opts ...RequestOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}
	return defaultHTTPClient.Do(req)
}
