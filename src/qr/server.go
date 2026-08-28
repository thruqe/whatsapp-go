package qr

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	Logger "whatsrook/src/logger"

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

// OpenBrowser attempts to open the default system web browser to the QR server's URL.
func (s *Server) OpenBrowser() error {
	return OpenBrowser(s.URL())
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
  <title>Connect your WhatsApp • WhatsRook</title>
  <style>
    :root { --background: #eef6f4; --surface: rgba(255,255,255,.58); --text: #173b36; --muted: #5f7772; --border: rgba(255,255,255,.8); --whatsapp: #128c7e; --whatsapp-dark: #075e54; }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: radial-gradient(circle at 12% 12%, rgba(37,211,102,.18), transparent 28rem), radial-gradient(circle at 90% 88%, rgba(18,140,126,.14), transparent 24rem), var(--background);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      min-height: 100vh;
    }
    .shell { animation: rise .7s ease-out both; margin: auto; max-width: 620px; padding: 3rem 1.25rem; }
    header { margin-bottom: 1.5rem; text-align: center; }
    .brand { color: var(--whatsapp-dark); font-size: 1.25rem; font-weight: 600; }
    .subtitle { color: var(--muted); font-size: .9rem; margin-top: .4rem; }
    .qr-card { animation: float 6s ease-in-out 1s infinite; background: linear-gradient(135deg, rgba(255,255,255,.78), rgba(255,255,255,.38)); backdrop-filter: blur(22px); border: 1px solid var(--border); border-radius: 18px; box-shadow: 0 24px 60px rgba(7,94,84,.13), inset 0 1px 0 rgba(255,255,255,.9); overflow: hidden; padding: 1rem; position: relative; }
    .qr-card::before { animation: sheen 7s ease-in-out infinite; background: linear-gradient(110deg, transparent 25%, rgba(255,255,255,.55) 48%, transparent 65%); content: ""; inset: 0; pointer-events: none; position: absolute; transform: translateX(-120%); }
    .card-top { border-bottom: 1px solid rgba(7,94,84,.12); color: var(--text); font-size: .9rem; font-weight: 600; padding: .25rem .25rem .9rem; position: relative; text-align: center; z-index: 1; }
    .status { color: var(--whatsapp-dark); font-size: .75rem; font-weight: 400; margin-left: .35rem; }
    .status::before { animation: pulse 1.8s ease-in-out infinite; background: var(--whatsapp); border-radius: 50%; content: ""; display: inline-block; height: .45rem; margin-right: .3rem; width: .45rem; }
    .qr-container { align-items: center; background: rgba(255,255,255,.7); border-radius: 12px; display: flex; justify-content: center; margin-top: 1rem; padding: 1.25rem; position: relative; z-index: 1; }
    .qr-container img { display: block; image-rendering: pixelated; max-width: 100%; width: 360px; }
    .card-foot { color: var(--muted); font-size: .75rem; padding: 0 .25rem .25rem; text-align: center; }
    .instructions { margin-top: 1.25rem; }
    .instructions p { font-size: .85rem; font-weight: 600; margin-bottom: .65rem; }
    .steps { color: var(--muted); counter-reset: step; display: grid; gap: .45rem; list-style: none; }
    .steps li { align-items: center; display: flex; font-size: .83rem; gap: .6rem; }
    .steps li::before { align-items: center; background: #dff5f0; border-radius: 50%; color: var(--whatsapp-dark); content: counter(step); counter-increment: step; display: flex; font-size: .7rem; font-weight: 600; height: 1.35rem; justify-content: center; width: 1.35rem; }
    .success { display: none; padding: 4rem 1rem; text-align: center; }
    .success .icon { animation: pop .5s ease-out both; color: var(--whatsapp); font-size: 3rem; }
    .success .title { font-size: 1.25rem; font-weight: 600; margin-top: .5rem; }
    .success .subtitle { color: var(--muted); font-size: .9rem; margin-top: .5rem; }
    @keyframes rise { from { opacity: 0; transform: translateY(14px); } to { opacity: 1; transform: translateY(0); } }
    @keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-4px); } }
    @keyframes sheen { 0%, 35% { transform: translateX(-120%); } 65%, 100% { transform: translateX(120%); } }
    @keyframes pulse { 0%, 100% { box-shadow: 0 0 0 0 rgba(18,140,126,.35); } 50% { box-shadow: 0 0 0 5px rgba(18,140,126,0); } }
    @keyframes pop { from { opacity: 0; transform: scale(.75); } to { opacity: 1; transform: scale(1); } }
    @media (prefers-reduced-motion: reduce) { *, *::before, *::after { animation-duration: .01ms !important; animation-iteration-count: 1 !important; scroll-behavior: auto !important; } }
  </style>
</head>
<body>
  <div class="shell">
    <header>
      <div class="brand">WhatsRook</div>
      <div class="subtitle">Scan to connect your WhatsApp account</div>
    </header>
    <main id="card">
    <div id="pairing-view">
      <div class="qr-card">
      <div class="card-top">Scan with WhatsApp <span class="status">Waiting</span></div>
      <div class="qr-container">
        <img id="qr-img" src="/qr.png?t=0" alt="WhatsApp QR Code">
      </div>
      <div class="card-foot">The code refreshes automatically.</div>
      </div>
      <div class="instructions">
        <p>How to connect</p>
        <ol class="steps">
          <li>Open WhatsApp on your phone</li>
          <li>Go to Settings &gt; Linked Devices</li>
          <li>Choose Link a Device and scan</li>
        </ol>
      </div>
    </div>
    <div class="success" id="success-view">
      <div class="icon">✓</div>
      <div class="title">Paired successfully</div>
      <div class="subtitle">Your WhatsApp session is connected. You can close this browser tab.</div>
    </div>
    </main>
  </div>

  <script>
    const qrImg = document.getElementById('qr-img');
    const pairingView = document.getElementById('pairing-view');
    const qrCard = document.querySelector('.qr-card');
    const successView = document.getElementById('success-view');

    function refreshQR() {
      qrImg.src = '/qr.png?t=' + Date.now();
    }

    if (window.EventSource) {
      const source = new EventSource('/events');
      source.onmessage = function(e) {
        if (e.data === 'paired') {
          pairingView.style.display = 'none';
          qrCard.style.display = 'none';
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
