package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"whatsrook/logger"

	"whatsrook"
	commands "whatsrook/cli/plugins"
	clistore "whatsrook/cli/store"
	"whatsrook/cli/updater"
	cliutils "whatsrook/cli/utils"
	"whatsrook/utils"
	"whatsrook/utils/qr"

	"go.mau.fi/whatsmeow"
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
	AsyncMessageAck bool
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
	onLoggedOut  func()
	mu           sync.Mutex
}

// Initiates a new Bot instance
func NewBot(cfg BotConfig) *Bot {
	if cfg.DataDir == "" {
		cfg.DataDir = whatsrook.DefaultDataDir()
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

	// Initialize core WhatsApp client
	client := whatsrook.NewClient(whatsrook.Config{
		Session:         b.cfg.Session,
		DataDir:         b.cfg.DataDir,
		Database:        b.cfg.Database,
		ClientType:      b.cfg.ClientType,
		Verbose:         b.cfg.Verbose,
		SkipOldMessages: b.cfg.SkipOldMessages,
		AsyncMessageAck: b.cfg.AsyncMessageAck,
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
			Logger.Warn("port in use, attempting to bind alternative port", "attempted_port", p, "err", err)
		}
	}

	if listener == nil {
		return errors.New("failed to find an available port to bind HTTP server")
	}

	if actualPort != startPort {
		Logger.Warn("port in use — switched to alternative port", "original_port", startPort, "new_port", actualPort)
	}

	b.listener = listener
	server := &http.Server{Handler: mux}
	b.httpServer = server

	go func() {
		Logger.Info("listening", "port", actualPort, "session", b.cfg.Session)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Error("http server error", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		if listener != nil {
			_ = listener.Close()
		}
	}()

	for {
		err := b.runSession(ctx)

		// Clean shutdown or context cancelled
		// Exit normally.
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		// Session logged out (401 or unpaired)
		// Clear device record from shared DB and exit so caller can enter idle mode.
		if errors.Is(err, whatsrook.ErrLoggedOut) || strings.Contains(err.Error(), "logged out") || b.loggedOut.Load() {
			Logger.Warn("Logged out session detected — device record cleared from shared database.")
			b.loggedOut.Store(false)
			return whatsrook.ErrLoggedOut
		}

		// Pairing stalled (malformed WA notification). Wipe the device record and retry.
		if errors.Is(err, whatsrook.ErrPairTimeout) {
			Logger.Error("session error", "err", "Pairing timed out — WhatsApp sent a bad response.")
			Logger.Warn("session action", "warn", "The device record will be cleared and a new code generated.")

			b.mu.Lock()
			cli := b.client
			b.mu.Unlock()
			if cli != nil {
				cli.ClearSessionDB(ctx, "")
			}

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

func (b *Bot) runSession(ctx context.Context) error {
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	defer sessionCancel()

	b.mu.Lock()
	b.onLoggedOut = func() {
		sessionCancel()
	}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.onLoggedOut = nil
		b.mu.Unlock()
		commands.StopAutoMuteScheduler()
		commands.StopAutoBioScheduler()
		_ = b.client.Close()
	}()

	if err := b.client.InitSession(sessionCtx); err != nil {
		return err
	}

	cli := b.client.WAClient()
	if cli == nil {
		return errors.New("failed to initialize wa-core client")
	}

	// Initialize and migrate CLI custom database tables at startup
	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
		clistore.InitTables(sessionCtx, s)
		if val, err := clistore.GetSetting(sessionCtx, s, cliutils.BotNamePromptDismissedKey); err == nil && val == "true" {
			cliutils.BotNamePromptDismissedCacheMu.Lock()
			cliutils.BotNamePromptDismissedCache[s.JID] = true
			if cli.Store != nil && cli.Store.ID != nil {
				cliutils.BotNamePromptDismissedCache[cli.Store.ID.ToNonAD().String()] = true
			}
			cliutils.BotNamePromptDismissedCacheMu.Unlock()
		}
	}

	_ = b.groupManager.LoadFromDB(sessionCtx, cli)

	// Register wacaller raw call adapter hook
	commands.RegisterWACaller(cli)

	// ── Logout
	if b.cfg.Logout {
		Logger.Info("logging out session", "session", b.cfg.Session)

		if cli.Store.ID == nil {
			Logger.Info("session was never paired, skipping server logout")
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
				Logger.Warn("connect failed before logout, clearing device record only", "err", err)
			} else {
				logoutCtx, logoutCancel := context.WithTimeout(sessionCtx, 10*time.Second)
				select {
				case <-connected:
					Logger.Info("connected — sending logout to WhatsApp servers")
				case <-logoutCtx.Done():
					Logger.Warn("timed out waiting for connection, sending logout anyway")
				}
				logoutCancel()

				if err := cli.Logout(sessionCtx); err != nil {
					Logger.Warn("server logout returned error", "err", err)
				}
				cli.Disconnect()
			}
		}

		b.client.ClearSessionDB(sessionCtx, "")
		Logger.Info("session device record cleared successfully", "session", b.cfg.Session)
		return nil
	}

	// ── Normal / pair run
	cli.AddEventHandler(func(evt any) {
		b.WAEventHandler(evt)
	})

	go tmpCron(sessionCtx)

	if cli.Store.ID == nil {
		if b.cfg.Pair {
			if err := b.runPairCode(sessionCtx); err != nil {
				return err
			}
		} else {
			go func() {
				if err := b.runQR(sessionCtx); err != nil {
					Logger.Error("runQR failed", "err", err)
				}
			}()
		}
	} else {
		if err := cli.Connect(); err != nil {
			if b.loggedOut.Load() || strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "logged out") {
				Logger.Warn("Connect failed due to logged-out status — clearing device record from shared database...", "err", err)
				b.client.ClearSessionDB(sessionCtx, "")
				return whatsrook.ErrLoggedOut
			}
			return err
		}
	}

	if b.loggedOut.Load() {
		Logger.Warn("Session logged out — clearing device record from shared database...")
		b.client.ClearSessionDB(sessionCtx, "")
		return whatsrook.ErrLoggedOut
	}

	// Start background schedulers tied to session context
	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
		commands.StartAutoMuteScheduler(sessionCtx, cli)
		commands.StartAutoBioScheduler(sessionCtx, cli)
	}

	for {
		select {
		case <-sessionCtx.Done():
			if b.loggedOut.Load() {
				Logger.Warn("Session logged out during runtime — clearing device record from shared database...")
				b.client.ClearSessionDB(ctx, "")
				return whatsrook.ErrLoggedOut
			}
			return nil
		case ctrl := <-b.hub.Control:
			ack := b.Controller(sessionCtx, ctrl)
			b.hub.Broadcast(ack)
		}
		if b.loggedOut.Load() {
			Logger.Warn("Session logged out during runtime — clearing device record from shared database...")
			b.client.ClearSessionDB(ctx, "")
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

			if bn, err := clistore.GetSetting(ctx, s, cliutils.BotNameSettingKey); err == nil && bn != "" {
				botName = &bn
			}
			if p, err := clistore.GetSetting(ctx, s, cliutils.PrefixSettingKey); err == nil && p != "" {
				prefix = &p
			}
			if m, err := clistore.GetSetting(ctx, s, "mode"); err == nil && m != "" {
				mode = &m
			}
			if ac, err := clistore.GetSetting(ctx, s, "anticall_status"); err == nil && ac == "on" {
				anticallEnabled = true
			}
			if ls, err := clistore.GetSetting(ctx, s, "likestatus_status"); err == nil && ls == "on" {
				likestatusEnabled = true
			}
			if sudoRaw, err := clistore.GetSetting(ctx, s, "sudoers"); err == nil && sudoRaw != "" {
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
	Logger.Debug("pair code issued", "code", code)
	Logger.Info(fmt.Sprintf("PAIR CODE: %s", code))
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

	qrServer, err := qr.StartServer()
	if err != nil {
		Logger.Warn("failed to start temporary qr server", "err", err)
	} else {
		defer func() {
			_ = qrServer.Close()
			Logger.Debug("temporary qr server closed and port released", "port", qrServer.Port())
		}()
		Logger.Info("temporary QR server started", "url", qrServer.URL())
		if b.cfg.QRCode {
			fmt.Printf("\n==> Open this URL in your browser to scan the QR code: %s\n\n", qrServer.URL())
		}
	}

	for evt := range qrChan {
		if evt.Event == "code" {
			if qrServer != nil {
				qrServer.UpdateCode(evt.Code)
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: PairQRPayload{Code: evt.Code},
			})
		} else if evt.Event == "success" {
			if qrServer != nil {
				qrServer.SetPaired()
				time.Sleep(1 * time.Second)
			}
			Logger.Info("QR code pairing successful, shutting down temporary QR server")
			return nil
		} else {
			Logger.Debug("qr channel event", "event", evt.Event)
		}
	}
	return nil
}

func (b *Bot) WAEventHandler(evt any) {
	var cli *whatsmeow.Client
	if b.client != nil {
		cli = b.client.WAClient()
	}

	broadcast := func(msg EventMessage) {
		if b.hub != nil {
			b.hub.Broadcast(msg)
		}
	}

	switch v := evt.(type) {
	case *events.QR:
		_ = v // handled via qrChan in runQR

	case *events.PairSuccess:
		Logger.Info("paired successfully")
		broadcast(simpleEvent(EventPairSuccess))

	case *events.PairError:
		Logger.Warn("pairing failed", "err", v.Error)
		broadcast(EventMessage{
			Kind:    EventPairError,
			Payload: PairErrorPayload{Reason: v.Error.Error()},
		})
	case *events.LoggedOut:
		Logger.Warn("logged out", "reason", v.Reason)
		b.loggedOut.Store(true)
		broadcast(simpleEvent(EventLoggedOut))
		b.mu.Lock()
		onLoggedOut := b.onLoggedOut
		b.mu.Unlock()
		if onLoggedOut != nil {
			onLoggedOut()
		}

	case *events.Disconnected:
		Logger.Info("disconnected")
		broadcast(simpleEvent(EventDisconnected))

	case *events.Connected:
		Logger.Info("connected", "session", b.cfg.Session)
		broadcast(simpleEvent(EventConnected))
		if cli != nil {
			go func() {
				if err := b.groupManager.SyncAll(context.Background(), cli); err != nil {
					Logger.Warn("groupManager.SyncAll returned error", "err", err)
				}
			}()
		}

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
		if commands.HandlePendingCaptchaReply(context.Background(), cli, v) {
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
		Logger.Debug("events: received Presence event", "from", v.From.String(), "unavailable", v.Unavailable, "lastSeen", v.LastSeen)
		commands.TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		Logger.Debug("events: received ChatPresence event", "sender", v.Sender.String(), "state", v.State, "media", v.Media)
		commands.TrackPresence(v.Sender, true)

	case *events.Receipt:
		if !v.Sender.IsEmpty() {
			commands.TrackPresence(v.Sender, true)
		}

	case *events.CallOffer:
		Logger.Info("call offer received", "from", v.CallCreator.String())
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
		Logger.Info("group info update received", "jid", v.JID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)
		b.handleGroupGreetings(context.Background(), v)
		b.handleGroupEventsNotification(context.Background(), v)
		b.handleGroupCaptcha(context.Background(), v)

	case *events.JoinedGroup:
		Logger.Info("joined group event received", "jid", v.JID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.Picture:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterJoin:
		Logger.Info("newsletter joined", "jid", v.ID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLeave:
		Logger.Info("newsletter left", "jid", v.ID.String())
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterMuteChange:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLiveUpdate:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.HistorySync, *events.UndecryptableMessage, *events.UndecryptedMessage, *events.StreamError, *events.Blocklist, *events.NotifyAccountReachoutTimelock, *events.UserAbout, *events.IdentityChange, *events.PrivacySettings, *events.KeepAliveTimeout, *events.KeepAliveRestored, *events.MediaRetry, *events.QRScannedWithoutMultidevice, *events.ManualLoginReconnect, *events.PushName, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted, *events.CallOfferNotice, *events.CallAccept, *events.CallPreAccept, *events.CallRelayLatency, *events.CallTransport, *events.CallTerminate, *events.CallReject, *events.UnknownCallEvent:
		// Ignore low-level history sync, call signaling, and receipt events to avoid log clutter

	default:
		Logger.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}
