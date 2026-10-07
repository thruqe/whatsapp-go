package test

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"whatsrook/util/logger"
)

//go:embed hello.html
var helloHTML []byte

// tunnelURLPattern matches the public hostname printed by a cloudflared quick tunnel.
var tunnelURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

var (
	pageMu  sync.Mutex
	pageURL string
)

// publicPageURL lazily starts a loopback HTTP server for hello.html and exposes
// it through a cloudflared quick tunnel. Phones cannot reach a local file on the
// bot host, and WhatsApp requires valid https links to render in-app webviews.
func publicPageURL(ctx context.Context) (string, error) {
	pageMu.Lock()
	defer pageMu.Unlock()

	if pageURL != "" {
		return pageURL, nil
	}

	u, err := startPage(ctx)
	if err != nil {
		return "", err
	}
	pageURL = u
	return pageURL, nil
}

func startPage(ctx context.Context) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("bind hello page server: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("webview test page hit", "ua", r.UserAgent(), "remote", r.RemoteAddr)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(helloHTML)
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Debug("hello page server closed", "err", err)
		}
	}()

	local := "http://" + listener.Addr().String()
	cmd := exec.CommandContext(context.Background(), "cloudflared", "tunnel", "--no-autoupdate", "--url", local)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = srv.Close()
		return "", fmt.Errorf("cloudflared stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = srv.Close()
		return "", fmt.Errorf("cloudflared stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = srv.Close()
		return "", fmt.Errorf("start cloudflared: %w", err)
	}

	found := make(chan string, 1)

	// Scan stdout and stderr concurrently so neither pipe blocks on a full buffer
	// and we don't stall waiting for EOF on an idle pipe.
	scanStream := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if u := tunnelURLPattern.FindString(line); u != "" && u != "https://api.trycloudflare.com" {
				select {
				case found <- u:
				default:
				}
			}
		}
	}

	go scanStream(stdout)
	go scanStream(stderr)

	select {
	case u := <-found:
		logger.Info("cloudflared tunnel established", "url", u)
		return u, nil
	case <-time.After(30 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = srv.Close()
		return "", errors.New("timed out waiting for cloudflared tunnel URL")
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = srv.Close()
		return "", ctx.Err()
	}
}
