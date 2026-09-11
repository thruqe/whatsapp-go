package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// DefaultMaxResponseBytes caps how much of a response body FetchBytes/FetchJSON/PostJSON
	// will read, guarding against unbounded memory growth from a misbehaving or hostile server.
	DefaultMaxResponseBytes = 64 << 20 // 64 MiB
	// DefaultMaxRetries is the number of retry attempts made for idempotent requests that
	// fail with a retryable error or status code.
	DefaultMaxRetries = 2
	// defaultFilePerm is used for files written by DownloadFile.
	defaultFilePerm os.FileMode = 0o644
	// defaultDirPerm is used when DownloadFile needs to create the destination directory.
	defaultDirPerm os.FileMode = 0o755
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

// altSvcAdvertisesH3 reports whether an Alt-Svc header value advertises an
// HTTP/3 alternative (an "h3" or "h3-*" protocol ID), rather than merely
// advertising some other alternative service such as h2.
func altSvcAdvertisesH3(altSvc string) bool {
	for entry := range strings.SplitSeq(altSvc, ",") {
		entry = strings.TrimSpace(entry)
		proto, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		proto = strings.Trim(proto, `"`)
		if proto == "h3" || strings.HasPrefix(proto, "h3-") {
			return true
		}
	}
	return false
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
		// This host previously advertised H3 support but the attempt just
		// failed (e.g. UDP blocked on this network path); fall back for
		// this and future requests rather than retrying H3 every time.
		t.h3Supported.Store(host, false)
	}

	resp, err := t.tcpClient.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if altSvcAdvertisesH3(resp.Header.Get("Alt-Svc")) {
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

// retryConfig controls the retry helper used internally by the do* functions.
type retryConfig struct {
	maxRetries int
}

// isRetryableStatus reports whether an HTTP status code is worth retrying
// for an idempotent request (GET). 429 and 5xx are considered transient.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// retryBackoff returns the delay before the given retry attempt (0-indexed),
// using exponential backoff with full jitter to avoid thundering-herd retries.
func retryBackoff(attempt int) time.Duration {
	base := 200 * time.Millisecond
	maxDelay := 5 * time.Second
	delay := min(base<<uint(attempt), maxDelay)
	return time.Duration(rand.Int64N(int64(delay) + 1))
}

// doWithRetry executes fn, retrying on transient network errors or
// retryable status codes using exponential backoff with jitter. fn is
// expected to close/drain any response body it does not return before
// returning a retryable error, since a new attempt will replace it.
func doWithRetry(ctx context.Context, client *http.Client, req *http.Request, cfg retryConfig) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff(attempt - 1)):
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		if isRetryableStatus(resp.StatusCode) && attempt < cfg.maxRetries {
			// Drain and close so the connection can be reused, then retry.
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("httpx: retryable status %s", resp.Status)
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

// readLimited reads up to DefaultMaxResponseBytes from r, returning an error
// if the body is truncated for exceeding the cap.
func readLimited(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, DefaultMaxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > DefaultMaxResponseBytes {
		return nil, fmt.Errorf("httpx: response body exceeds %d byte limit", DefaultMaxResponseBytes)
	}
	return data, nil
}

func statusError(rawURL string, resp *http.Response, body []byte) error {
	if len(body) > 0 {
		return fmt.Errorf("httpx: request to %s: status %s: %s", rawURL, resp.Status, string(body))
	}
	return fmt.Errorf("httpx: request to %s: unexpected status %s", rawURL, resp.Status)
}

// FetchBytes issues a GET request to rawURL and returns the response body bytes.
// Transient network errors and 429/5xx responses are retried with backoff.
func FetchBytes(ctx context.Context, rawURL string, opts ...RequestOption) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("httpx: build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := doWithRetry(ctx, HTTPClient(), req, retryConfig{maxRetries: DefaultMaxRetries})
	if err != nil {
		return nil, fmt.Errorf("httpx: fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	data, err := readLimited(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("httpx: read %s body: %w", rawURL, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(rawURL, resp, data)
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
// POST is not retried by default, since it is not inherently idempotent; only the initial
// network dial failing before any bytes were sent is safe to retry, which doWithRetry's
// client.Do already handles at the transport/dial level via req's context.
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
		body, _ := readLimited(resp.Body)
		return statusError(rawURL, resp, body)
	}

	if target != nil {
		limited := io.LimitReader(resp.Body, DefaultMaxResponseBytes+1)
		if err := json.NewDecoder(limited).Decode(target); err != nil {
			return fmt.Errorf("httpx: decode response from %s: %w", rawURL, err)
		}
	}
	return nil
}

// DownloadFile downloads a remote file to a local destination path, streaming
// directly to disk rather than buffering the full body in memory.
func DownloadFile(ctx context.Context, rawURL, destPath string, opts ...RequestOption) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("httpx: build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	for _, opt := range opts {
		opt(req)
	}

	resp, err := doWithRetry(ctx, HTTPClient(), req, retryConfig{maxRetries: DefaultMaxRetries})
	if err != nil {
		return fmt.Errorf("httpx: download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := readLimited(resp.Body)
		return statusError(rawURL, resp, body)
	}

	if dir := filepath.Dir(destPath); dir != "." {
		if err := os.MkdirAll(dir, defaultDirPerm); err != nil {
			return fmt.Errorf("httpx: create destination directory %q: %w", dir, err)
		}
	}

	// Write to a temp file in the same directory and rename into place, so a
	// failed or interrupted download never leaves a partial file at destPath.
	tmp, err := os.CreateTemp(filepath.Dir(destPath), filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("httpx: create temp file for %q: %w", destPath, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if _, copyErr := io.Copy(tmp, resp.Body); copyErr != nil {
		tmp.Close()
		return fmt.Errorf("httpx: write %s to %q: %w", rawURL, destPath, copyErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("httpx: close temp file for %q: %w", destPath, closeErr)
	}
	if chmodErr := os.Chmod(tmpPath, defaultFilePerm); chmodErr != nil {
		return fmt.Errorf("httpx: set permissions on %q: %w", destPath, chmodErr)
	}
	if renameErr := os.Rename(tmpPath, destPath); renameErr != nil {
		return fmt.Errorf("httpx: rename temp file to %q: %w", destPath, renameErr)
	}

	return nil
}
