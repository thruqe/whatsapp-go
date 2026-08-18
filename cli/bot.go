package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"whatsrook"
	commands "whatsrook/cli/plugins"
	clistore "whatsrook/cli/store"
	"whatsrook/cli/updater"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// BotConfig holds configuration parameters for the CLI
type BotConfig struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	Verbose         bool
	ClientType      whatsrook.ClientType
	SkipOldMessages bool
	DataDir         string
	WSPort          int
	Database        string
}

// Bot manages the CLI bot lifecycle: WhatsApp client init, event handling,
// WebSocket communication, and session state (pairing, logout, persistence).
type Bot struct {
	cfg          BotConfig
	client       *whatsrook.Client
	groupManager *GroupManager
	hub          *Hub
	httpServer   *http.Server
	listener     net.Listener
	startupTime  time.Time
	loggedOut    atomic.Bool
	mu           sync.Mutex
}

// Initiates a new Bot instance
func NewBot(cfg BotConfig) *Bot {
	if cfg.DataDir == "" {
		cfg.DataDir = "auth"
	}
	if cfg.WSPort <= 0 {
		cfg.WSPort = 3000
	}
	return &Bot{
		cfg:          cfg,
		groupManager: NewGroupManager(),
		startupTime:  time.Now(),
	}
}

// GroupManager returns the Bot's GroupManager instance.
func (b *Bot) GroupManager() *GroupManager {
	return b.groupManager
}

// Launches the Client and it's Activities
func (b *Bot) Start(ctx context.Context) error {
	// Validate session phone number
	// This can be improved later
	// TODO: Add more robust validation
	if b.cfg.Session == "" {
		return errors.New("session phone number is required")
	}

	sessionDir := filepath.Join(b.cfg.DataDir, b.cfg.Session)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session dir %q: %w", sessionDir, err)
	}

	// Initialize core WhatsApp client
	client := whatsrook.NewClient(whatsrook.Config{
		Session:         b.cfg.Session,
		DataDir:         b.cfg.DataDir,
		Database:        b.cfg.Database,
		ClientType:      b.cfg.ClientType,
		Verbose:         b.cfg.Verbose,
		SkipOldMessages: b.cfg.SkipOldMessages,
	})

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	// Initiate WebSocket, HTTP & bind port
	hub := newHub()
	b.mu.Lock()
	b.hub = hub
	b.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS(false))

	startPort := b.cfg.WSPort
	var listener net.Listener
	var actualPort int
	for p := startPort; p < startPort+100; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener = l
			actualPort = p
			break
		}
		if p == startPort {
			slog.Warn("port in use, attempting to bind alternative port", "attempted_port", p, "err", err)
		}
	}

	if listener == nil {
		return errors.New("failed to find an available port to bind HTTP server")
	}

	if actualPort != startPort {
		slog.Warn("port in use — switched to alternative port", "original_port", startPort, "new_port", actualPort)
	}

	b.listener = listener
	server := &http.Server{Handler: mux}
	b.httpServer = server

	go func() {
		slog.Info("listening", "port", actualPort, "session", b.cfg.Session)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	for {
		err := b.runSession(ctx, sessionDir)

		// Clean shutdown or context cancelled
		// Exit normally.
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		// Session logged out (401 or unpaired)
		// Clear database/session files and restart connection cycle to re-pair.
		if errors.Is(err, whatsrook.ErrLoggedOut) || strings.Contains(err.Error(), "logged out") || b.loggedOut.Load() {
			slog.Warn("Logged out session detected — database and session cleared. Restarting connection cycle...")
			b.loggedOut.Store(false)
			whatsrook.WipeSession(sessionDir)

			b.mu.Lock()
			b.client = whatsrook.NewClient(whatsrook.Config{
				Session:         b.cfg.Session,
				DataDir:         b.cfg.DataDir,
				Database:        b.cfg.Database,
				ClientType:      b.cfg.ClientType,
				Verbose:         b.cfg.Verbose,
				SkipOldMessages: b.cfg.SkipOldMessages,
			})
			b.mu.Unlock()

			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		// Pairing stalled (malformed WA notification). Wipe the session and retry.
		if errors.Is(err, whatsrook.ErrPairTimeout) {
			slog.Error("session error", "err", "Pairing timed out — WhatsApp sent a bad response.")
			slog.Warn("session action", "warn", "The session directory will be cleared and a new code generated.")

			whatsrook.WipeSession(sessionDir)

			for i := 10; i > 0; i-- {
				fmt.Printf("\r  Retrying in %2ds…", i)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					fmt.Println()
					return nil
				}
			}
			fmt.Println("\r  Retrying now…         ")
			continue
		}

		// Any other error is fatal.
		return fmt.Errorf("session error: %w", err)
	}
}

func (b *Bot) runSession(ctx context.Context, sessionDir string) error {
	if err := b.client.InitSession(ctx); err != nil {
		return err
	}
	defer func() {
		_ = b.client.Close()
	}()

	cli := b.client.WAClient()
	if cli == nil {
		return errors.New("failed to initialize wa-core client")
	}

	// Initialize and migrate CLI custom database tables at startup
	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok {
		clistore.InitTables(ctx, s)
	}

	_ = b.groupManager.LoadFromDB(ctx, cli)

	// Register wacaller raw call adapter hook
	commands.RegisterWACaller(cli)

	// ── Logout
	if b.cfg.Logout {
		slog.Info("logging out session", "session", b.cfg.Session)

		if cli.Store.ID == nil {
			slog.Info("session was never paired, skipping server logout")
		} else {
			connected := make(chan struct{}, 1)
			cli.AddEventHandler(func(evt any) {
				if _, ok := evt.(*events.Connected); ok {
					select {
					case connected <- struct{}{}:
					default:
					}
				}
			})

			if err := cli.Connect(); err != nil {
				slog.Warn("connect failed before logout, wiping local files only", "err", err)
			} else {
				logoutCtx, logoutCancel := context.WithTimeout(ctx, 10*time.Second)
				select {
				case <-connected:
					slog.Info("connected — sending logout to WhatsApp servers")
				case <-logoutCtx.Done():
					slog.Warn("timed out waiting for connection, sending logout anyway")
				}
				logoutCancel()

				if err := cli.Logout(ctx); err != nil {
					slog.Warn("server logout returned error", "err", err)
				}
				cli.Disconnect()
			}
		}

		_ = b.client.Close()
		whatsrook.WipeSession(sessionDir)
		slog.Info("session directory cleared successfully", "session", b.cfg.Session)
		return nil
	}

	// ── Normal / pair run
	cli.AddEventHandler(func(evt any) {
		b.WAEventHandler(evt)
	})

	go tmpCron()

	if cli.Store.ID == nil {
		if b.cfg.Pair {
			if err := b.runPairCode(ctx); err != nil {
				return err
			}
		} else {
			go func() {
				if err := b.runQR(ctx); err != nil {
					slog.Error("runQR failed", "err", err)
				}
			}()
		}
	} else {
		if err := cli.Connect(); err != nil {
			if b.loggedOut.Load() || strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "logged out") {
				slog.Warn("Connect failed due to logged-out status — clearing session database and local files...", "err", err)
				b.client.ClearSessionDB(ctx, sessionDir)
				return whatsrook.ErrLoggedOut
			}
			return err
		}
	}

	if b.loggedOut.Load() {
		slog.Warn("Session logged out — clearing session database and local files...")
		b.client.ClearSessionDB(ctx, sessionDir)
		return whatsrook.ErrLoggedOut
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ctrl := <-b.hub.Control:
			ack := b.Controller(ctx, ctrl)
			b.hub.Broadcast(ack)
		}
		if b.loggedOut.Load() {
			slog.Warn("Session logged out during runtime — clearing session database and local files...")
			b.client.ClearSessionDB(ctx, sessionDir)
			return whatsrook.ErrLoggedOut
		}
	}
}

func (b *Bot) GetStatsPayload(ctx context.Context) StatsPayload {
	var connected bool
	var loggedIn bool
	var jidStr *string
	var pushName *string
	var botName *string
	defaultPrefix := cliutils.DefaultPrefix
	prefix := &defaultPrefix
	var mode *string
	var dbContactsCount uint32
	var dbDriver string = "sqlite"
	if b.client != nil && b.client.Config.Database != "" {
		dbDriver = b.client.Config.Database
	}
	var anticallEnabled bool
	var likestatusEnabled bool
	var sudoersCount uint32

	cli := b.client.WAClient()
	if cli != nil {
		connected = cli.IsConnected()
		loggedIn = cli.IsLoggedIn()

		if cli.Store != nil && cli.Store.ID != nil {
			str := cli.Store.ID.String()
			jidStr = &str
			if cli.Store.PushName != "" {
				pn := cli.Store.PushName
				pushName = &pn
			}
		}

		if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok {
			if contacts, err := s.GetAllContacts(ctx); err == nil {
				dbContactsCount = uint32(len(contacts))
			}

			if bn, err := s.GetSetting(ctx, cliutils.BotNameSettingKey); err == nil && bn != "" {
				botName = &bn
			}
			if p, err := s.GetSetting(ctx, cliutils.PrefixSettingKey); err == nil && p != "" {
				prefix = &p
			}
			if m, err := s.GetSetting(ctx, "mode"); err == nil && m != "" {
				mode = &m
			}
			if ac, err := s.GetSetting(ctx, "anticall_status"); err == nil && ac == "on" {
				anticallEnabled = true
			}
			if ls, err := s.GetSetting(ctx, "likestatus_status"); err == nil && ls == "on" {
				likestatusEnabled = true
			}
			if sudoRaw, err := s.GetSetting(ctx, "sudoers"); err == nil && sudoRaw != "" {
				parts := strings.Fields(strings.ReplaceAll(sudoRaw, ",", " "))
				sudoersCount = uint32(len(parts))
			}
		}
	}

	uptimeSec := int64(time.Since(b.startupTime).Seconds())
	uptimeFmt := utils.FormatUptime(float64(uptimeSec))

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memUsed := ms.Alloc
	memUsedFmt := utils.FormatBytes(memUsed)

	wsClients := uint32(0)
	if b.hub != nil {
		wsClients = uint32(b.hub.ConnectedClientsCount())
	}

	activePlugins := uint32(len(commands.Visible()))

	return StatsPayload{
		Connected:           connected,
		LoggedIn:            loggedIn,
		JID:                 jidStr,
		PushName:            pushName,
		BotName:             botName,
		Prefix:              prefix,
		Mode:                mode,
		UptimeSeconds:       uptimeSec,
		UptimeFormatted:     uptimeFmt,
		MemoryUsedBytes:     memUsed,
		MemoryUsedFormatted: memUsedFmt,
		MemorySysBytes:      ms.Sys,
		ActivePluginsCount:  activePlugins,
		ConnectedWSClients:  wsClients,
		PlatformOS:          runtime.GOOS,
		GoVersion:           runtime.Version(),
		AppVersion:          updater.GetAppVersion(),
		SessionPhone:        b.cfg.Session,
		NetworkPaused:       false,
		DBContactsCount:     dbContactsCount,
		DBDriver:            dbDriver,
		AnticallEnabled:     anticallEnabled,
		LikestatusEnabled:   likestatusEnabled,
		SudoersCount:        sudoersCount,
	}
}

func (b *Bot) runPairCode(ctx context.Context) error {
	code, err := b.client.PairPhone(ctx, b.cfg.Session)
	if err != nil {
		return err
	}
	slog.Debug("pair code issued", "code", code)
	slog.Info(fmt.Sprintf("PAIR CODE: %s", code))
	b.hub.Broadcast(EventMessage{
		Kind:    EventPairCode,
		Payload: PairCodePayload{Code: code},
	})
	return nil
}

func (b *Bot) runQR(ctx context.Context) error {
	qrChan, err := b.client.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	for evt := range qrChan {
		if evt.Event == "code" {
			if b.cfg.QRCode {
				fmt.Println("QR code:", evt.Code)
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: PairQRPayload{Code: evt.Code},
			})
		} else {
			slog.Debug("qr channel event", "event", evt.Event)
		}
	}
	return nil
}

func (b *Bot) WAEventHandler(evt any) {
	cli := b.client.WAClient()

	switch v := evt.(type) {
	case *events.QR:
		_ = v // handled via qrChan in runQR

	case *events.PairSuccess:
		slog.Info("paired successfully")
		b.hub.Broadcast(simpleEvent(EventPairSuccess))

	case *events.PairError:
		slog.Warn("pairing failed", "err", v.Error)
		b.hub.Broadcast(EventMessage{
			Kind:    EventPairError,
			Payload: PairErrorPayload{Reason: v.Error.Error()},
		})
	case *events.LoggedOut:
		slog.Warn("logged out", "reason", v.Reason)
		b.loggedOut.Store(true)
		b.hub.Broadcast(simpleEvent(EventLoggedOut))

	case *events.Disconnected:
		slog.Info("disconnected")
		b.hub.Broadcast(simpleEvent(EventDisconnected))

	case *events.Connected:
		slog.Info("connected", "session", b.cfg.Session)
		b.hub.Broadcast(simpleEvent(EventConnected))
		go func() {
			if err := b.groupManager.SyncAll(context.Background(), cli); err != nil {
				slog.Warn("groupManager.SyncAll returned error", "err", err)
			}
		}()

	case *events.Message:
		// Skip messages sent before the bot started running
		if b.cfg.SkipOldMessages && v.Info.Timestamp.Before(b.startupTime) {
			return
		}

		if v.Info.Chat.Server == "broadcast" || v.Info.Chat.String() == "status@broadcast" {
			go b.handleLikeStatus(context.Background(), v)
		}

		if commands.HandlePendingAudioReply(context.Background(), cli, v) {
			return
		}

		if commands.HandlePendingMenuMediaReply(context.Background(), cli, v) {
			return
		}
		if commands.HandlePendingBotCustomizationReply(context.Background(), cli, v) {
			return
		}

		if commands.Dispatch(context.Background(), cli, v) {
			return
		}

		payload := buildIncomingMessagePayload(v)
		b.hub.Broadcast(EventMessage{
			Kind:    EventIncomingMessage,
			Payload: payload,
		})

	case *events.Presence:
		slog.Debug("events: received Presence event", "from", v.From.String(), "unavailable", v.Unavailable, "lastSeen", v.LastSeen)
		commands.TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		slog.Debug("events: received ChatPresence event", "sender", v.Sender.String(), "state", v.State, "media", v.Media)
		commands.TrackPresence(v.Sender, true)

	case *events.Receipt:
		if !v.Sender.IsEmpty() {
			commands.TrackPresence(v.Sender, true)
		}

	case *events.CallOffer:
		slog.Info("call offer received", "from", v.CallCreator.String())
		b.handleAntiCall(context.Background(), v)
		b.hub.Broadcast(EventMessage{
			Kind: EventIncomingCall,
			Payload: IncomingCallPayload{
				CallID:    v.CallID,
				From:      v.CallCreator.String(),
				Timestamp: v.Timestamp,
			},
		})

	case *events.GroupInfo:
		slog.Info("group info update received", "jid", v.JID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)
		b.handleGroupGreetings(context.Background(), v)
		b.handleGroupEventsNotification(context.Background(), v)

	case *events.JoinedGroup:
		slog.Info("joined group event received", "jid", v.JID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.Picture:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterJoin:
		slog.Info("newsletter joined", "jid", v.ID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLeave:
		slog.Info("newsletter left", "jid", v.ID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterMuteChange:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLiveUpdate:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.HistorySync, *events.UndecryptableMessage, *events.UndecryptedMessage, *events.StreamError, *events.Blocklist, *events.NotifyAccountReachoutTimelock, *events.UserAbout, *events.IdentityChange, *events.PrivacySettings, *events.KeepAliveTimeout, *events.KeepAliveRestored, *events.MediaRetry, *events.QRScannedWithoutMultidevice, *events.ManualLoginReconnect, *events.PushName, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted, *events.CallOfferNotice, *events.CallAccept, *events.CallPreAccept, *events.CallRelayLatency, *events.CallTransport, *events.CallTerminate, *events.CallReject, *events.UnknownCallEvent:
		// Ignore low-level history sync, call signaling, and receipt events to avoid log clutter

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}
