package main

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/skip2/go-qrcode"
)

// Server represents a temporary HTTP server that hosts the QR code for pairing.
type Server struct {
	listener    net.Listener
	server      *http.Server
	port        int
	mu          sync.RWMutex
	code        string
	paired      bool
	subscribers map[chan struct{}]struct{}
}

// StartServer starts a temporary HTTP server on a random available port.
func StartServer() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// Fallback to all interfaces if loopback fails
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to bind ephemeral port for qr server: %w", err)
		}
	}

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, fmt.Errorf("failed to determine TCP port")
	}

	s := &Server{
		listener:    listener,
		port:        tcpAddr.Port,
		subscribers: make(map[chan struct{}]struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/qr.png", s.handlePNG)
	mux.HandleFunc("/events", s.handleEvents)

	s.server = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			Debug("qr temp server closed", "err", err)
		}
	}()

	return s, nil
}

// Port returns the ephemeral port the server is listening on.
func (s *Server) Port() int {
	return s.port
}

// URL returns the full HTTP address for viewing the QR code in a browser.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// UpdateCode updates the active QR code string and notifies SSE subscribers.
func (s *Server) UpdateCode(code string) {
	s.mu.Lock()
	s.code = code
	s.notifySubscribers()
	s.mu.Unlock()
}

// SetPaired marks the session as successfully paired and notifies subscribers.
func (s *Server) SetPaired() {
	s.mu.Lock()
	s.paired = true
	s.notifySubscribers()
	s.mu.Unlock()
}

// Close stops the temporary HTTP server and immediately releases the port.
func (s *Server) Close() error {
	s.mu.Lock()
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
	s.mu.Unlock()

	if s.server != nil {
		s.server.SetKeepAlivesEnabled(false)
		return s.server.Close()
	}
	return nil
}

func (s *Server) notifySubscribers() {
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, indexHTML)
}

func (s *Server) handlePNG(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	code := s.code
	s.mu.RUnlock()

	if code == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "QR code not ready")
		return
	}

	pngBytes, err := qrcode.Encode(code, qrcode.Medium, 380)
	if err != nil {
		http.Error(w, "Failed to encode QR", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(pngBytes)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Flush headers immediately so client connection is established
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	notifyChan := make(chan struct{}, 1)

	s.mu.Lock()
	s.subscribers[notifyChan] = struct{}{}
	paired := s.paired
	hasCode := s.code != ""
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subscribers, notifyChan)
		s.mu.Unlock()
	}()

	// Send initial state if available
	if paired {
		_, _ = fmt.Fprintf(w, "data: paired\n\n")
		flusher.Flush()
		return
	} else if hasCode {
		_, _ = fmt.Fprintf(w, "data: update\n\n")
		flusher.Flush()
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-notifyChan:
			if !ok {
				return
			}
			s.mu.RLock()
			isPaired := s.paired
			s.mu.RUnlock()

			if isPaired {
				_, _ = fmt.Fprintf(w, "data: paired\n\n")
				flusher.Flush()
				return
			}
			_, _ = fmt.Fprintf(w, "data: update\n\n")
			flusher.Flush()
		}
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>QR Code</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      background: #fff;
    }
    img {
      display: block;
      max-width: 90vmin;
      max-height: 90vmin;
      image-rendering: pixelated;
    }
  </style>
</head>
<body>
  <img id="qr" src="/qr.png" alt="QR Code">
  <script>
    const img = document.getElementById('qr');
    if (window.EventSource) {
      const source = new EventSource('/events');
      source.onmessage = function(e) {
        if (e.data === 'update') {
          img.src = '/qr.png?t=' + Date.now();
        } else if (e.data === 'paired') {
          source.close();
        }
      };
    } else {
      setInterval(function() {
        img.src = '/qr.png?t=' + Date.now();
      }, 4000);
    }
  </script>
</body>
</html>
`
