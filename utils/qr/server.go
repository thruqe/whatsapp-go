package qr

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"whatsrook/logger"

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
			Logger.Debug("qr temp server closed", "err", err)
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
  <title>WhatsRook • WhatsApp Pairing</title>
  <style>
    :root {
      --bg: #0c1317;
      --card-bg: #111b21;
      --primary: #00a884;
      --text: #e9edef;
      --text-muted: #8696a0;
      --border: #222e35;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
    body {
      background: var(--bg);
      color: var(--text);
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 1.5rem;
    }
    .card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 1rem;
      padding: 2.5rem 2rem;
      max-width: 440px;
      width: 100%;
      text-align: center;
      box-shadow: 0 10px 30px rgba(0,0,0,0.5);
    }
    .brand {
      font-size: 1.5rem;
      font-weight: 700;
      color: var(--primary);
      margin-bottom: 0.5rem;
    }
    .title {
      font-size: 1.15rem;
      font-weight: 600;
      margin-bottom: 0.75rem;
    }
    .subtitle {
      font-size: 0.9rem;
      color: var(--text-muted);
      line-height: 1.4;
      margin-bottom: 1.75rem;
    }
    .qr-container {
      background: #ffffff;
      border-radius: 0.75rem;
      padding: 0.75rem;
      display: inline-block;
      margin-bottom: 1.5rem;
      min-width: 280px;
      min-height: 280px;
      box-shadow: 0 4px 15px rgba(0,0,0,0.3);
    }
    .qr-container img {
      width: 280px;
      height: 280px;
      display: block;
      border-radius: 0.25rem;
    }
    .steps {
      text-align: left;
      background: rgba(255,255,255,0.03);
      border-radius: 0.5rem;
      padding: 1rem;
      font-size: 0.85rem;
      color: var(--text-muted);
      line-height: 1.6;
    }
    .steps ol { padding-left: 1.25rem; }
    .success {
      display: none;
      padding: 2rem 1rem;
    }
    .success .icon {
      font-size: 3.5rem;
      color: var(--primary);
      margin-bottom: 1rem;
    }
  </style>
</head>
<body>
  <div class="card" id="card">
    <div id="pairing-view">
      <div class="brand">WhatsRook</div>
      <div class="title">Link with WhatsApp</div>
      <div class="subtitle">Scan this QR code with WhatsApp to connect your bot session.</div>
      <div class="qr-container">
        <img id="qr-img" src="/qr.png?t=0" alt="WhatsApp QR Code">
      </div>
      <div class="steps">
        <ol>
          <li>Open <b>WhatsApp</b> on your phone</li>
          <li>Tap <b>Settings</b> &gt; <b>Linked Devices</b></li>
          <li>Tap <b>Link a Device</b> and point camera at screen</li>
        </ol>
      </div>
    </div>
    <div class="success" id="success-view">
      <div class="icon">✓</div>
      <div class="title" style="color: var(--primary); font-size: 1.4rem;">Paired Successfully!</div>
      <div class="subtitle" style="margin-top: 0.75rem;">Your session is now connected. You can close this browser tab.</div>
    </div>
  </div>

  <script>
    const qrImg = document.getElementById('qr-img');
    const pairingView = document.getElementById('pairing-view');
    const successView = document.getElementById('success-view');

    function refreshQR() {
      qrImg.src = '/qr.png?t=' + Date.now();
    }

    if (window.EventSource) {
      const source = new EventSource('/events');
      source.onmessage = function(e) {
        if (e.data === 'paired') {
          pairingView.style.display = 'none';
          successView.style.display = 'block';
          source.close();
        } else if (e.data === 'update') {
          refreshQR();
        }
      };
    } else {
      setInterval(refreshQR, 4000);
    }
  </script>
</body>
</html>
`
