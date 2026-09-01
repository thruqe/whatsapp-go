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

	Logger "whatsrook/logger"

	"whatsrook"
	commands "whatsrook/cmd/plugins"
	clistore "whatsrook/cmd/store"
	"whatsrook/cmd/updater"
	cliutils "whatsrook/cmd/utils"
	"whatsrook/qr"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// BotConfig encapsulates runtime configuration parameters parsed from the CLI interface.
type BotConfig struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	ClientType      whatsrook.ClientType
	Database        string
	Verbose         bool
	WSPort          int
	AsyncMessageAck bool
}

// Bot orchestrates the core WhatsApp client, event dispatcher, and API/WebSocket lifecycle.
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

// NewBot constructs and initializes a new Bot lifecycle manager.
func NewBot(cfg BotConfig) *Bot {
	return &Bot{
		cfg:          cfg,
		groupManager: NewGroupManager(),
		startupTime:  time.Now(),
	}
}

// GroupManager returns the Bot's associated group state manager.
func (b *Bot) GroupManager() *GroupManager {
	return b.groupManager
}

// Start boots the WhatsApp client, initializes the WebSocket API server, and enters the event loop.
func (b *Bot) Start(ctx context.Context) error {
	if b.cfg.Session == "" {
		return errors.New("session phone number is required")
	}

	client := whatsrook.NewClient(whatsrook.Config{
		Session:         b.cfg.Session,
		DataDir:         whatsrook.DefaultDataDir(),
		Database:        b.cfg.Database,
		ClientType:      b.cfg.ClientType,
		Verbose:         b.cfg.Verbose,
		AsyncMessageAck: b.cfg.AsyncMessageAck,
	})

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	hub := newHub()
	b.mu.Lock()
	b.hub = hub
	b.mu.Unlock()

	unsubLog := Logger.AddHook(func(entry Logger.LogEntry) {
		hub.Broadcast(EventMessage{
			Kind:    EventLog,
			Payload: entry,
		})
	})
	defer unsubLog()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS(false))

	// Bind to port :0 to allow the OS to allocate a dynamic ephemeral port
	bindAddr := ":0"
	if b.cfg.WSPort > 0 {
		bindAddr = fmt.Sprintf(":%d", b.cfg.WSPort)
	}

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to bind API listener on %s: %w", bindAddr, err)
	}

	boundPort := listener.Addr().(*net.TCPAddr).Port
	b.listener = listener

	server := &http.Server{Handler: mux}
	b.httpServer = server

	go func() {
		Logger.Info("API and WebSocket server online", "port", boundPort, "session", b.cfg.Session, "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Error("http server runtime error", "err", err)
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

		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		if errors.Is(err, whatsrook.ErrLoggedOut) || strings.Contains(err.Error(), "logged out") || b.loggedOut.Load() {
			Logger.Warn("logged out session detected; device record cleared from database")
			b.loggedOut.Store(false)
			return whatsrook.ErrLoggedOut
		}

		if errors.Is(err, whatsrook.ErrPairTimeout) {
			Logger.Error("session error", "err", "pairing timed out due to invalid remote response")
			Logger.Warn("session action", "warn", "clearing device record and regenerating pairing key")

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

		return fmt.Errorf("session encountered unrecoverable error: %w", err)
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
	commands.RegisterWACaller(cli)

	// Explicit session logout routine
	if b.cfg.Logout {
		Logger.Info("initiating session logout", "session", b.cfg.Session)

		if cli.Store.ID == nil {
			Logger.Info("session was never paired; skipping server-side revocation")
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
				Logger.Warn("Socket connection failed prior to logout; purging local device state only", "err", err)
			} else {
				logoutCtx, logoutCancel := context.WithTimeout(sessionCtx, 10*time.Second)
				select {
				case <-connected:
					Logger.Info("connected to WhatsApp routing servers; dispatching logout frame")
				case <-logoutCtx.Done():
					Logger.Warn("connection timeout during logout sequence; forcing server revocation")
				}
				logoutCancel()

				if err := cli.Logout(sessionCtx); err != nil {
					Logger.Warn("server logout command returned error", "err", err)
				}
				cli.Disconnect()
			}
		}

		b.client.ClearSessionDB(sessionCtx, "")
		Logger.Info("session credentials and records purged successfully", "session", b.cfg.Session)
		return nil
	}

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
					Logger.Error("runQR execution error", "err", err)
				}
			}()
		}
	} else {
		if err := cli.Connect(); err != nil {
			if b.loggedOut.Load() || strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "logged out") {
				Logger.Warn("connect rejected due to expired authentication; clearing local device record", "err", err)
				b.client.ClearSessionDB(sessionCtx, "")
				return whatsrook.ErrLoggedOut
			}
			return err
		}
	}

	if b.loggedOut.Load() {
		Logger.Warn("session revoked; clearing local device record")
		b.client.ClearSessionDB(sessionCtx, "")
		return whatsrook.ErrLoggedOut
	}

	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
		commands.StartAutoMuteScheduler(sessionCtx, cli)
		commands.StartAutoBioScheduler(sessionCtx, cli)
	}

	for {
		select {
		case <-sessionCtx.Done():
			if b.loggedOut.Load() {
				Logger.Warn("session terminated during runtime; purging device record")
				b.client.ClearSessionDB(ctx, "")
				return whatsrook.ErrLoggedOut
			}
			return nil
		case ctrl := <-b.hub.Control:
			ack := b.Controller(sessionCtx, ctrl)
			b.hub.Broadcast(ack)
		}
		if b.loggedOut.Load() {
			Logger.Warn("session terminated during runtime; purging device record")
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
	uptimeFmt := whatsrook.FormatUptime(float64(uptimeSec))

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memUsed := ms.Alloc
	memUsedFmt := whatsrook.FormatBytes(memUsed)

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
			Logger.Debug("temporary qr server released", "port", qrServer.Port())
		}()
		Logger.Info("temporary QR server started", "url", qrServer.URL())
		if b.cfg.QRCode {
			fmt.Printf("\n==> Scan QR Code interface via browser: %s\n\n", qrServer.URL())
		}
	}

	var openedBrowser sync.Once
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			if qrServer != nil {
				qrServer.UpdateCode(evt.Code)
				openedBrowser.Do(func() {
					if err := qrServer.OpenBrowser(); err != nil {
						Logger.Debug("unable to auto-open browser for qr pairing", "url", qrServer.URL(), "err", err)
					} else {
						Logger.Info("opened browser for qr pairing", "url", qrServer.URL())
					}
				})
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: PairQRPayload{Code: evt.Code},
			})
		case "success":
			if qrServer != nil {
				qrServer.SetPaired()
				time.Sleep(1 * time.Second)
			}
			Logger.Info("QR code pairing successful, shutting down temporary QR server")
			return nil
		default:
			Logger.Debug("qr event dispatched", "event", evt.Event)
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
		_ = v // QR frames handled directly via runQR channel loop

	case *events.PairSuccess:
		Logger.Info("pairing completed successfully", "event", v)
		broadcast(simpleEvent(EventPairSuccess))

	case *events.PairError:
		Logger.Warn("pairing procedure failed", "err", v.Error, "event", v)
		broadcast(EventMessage{
			Kind:    EventPairError,
			Payload: PairErrorPayload{Reason: v.Error.Error()},
		})

	case *events.LoggedOut:
		Logger.Warn("device logged out by remote session", "reason", v.Reason, "event", v)
		b.loggedOut.Store(true)
		broadcast(simpleEvent(EventLoggedOut))
		b.mu.Lock()
		onLoggedOut := b.onLoggedOut
		b.mu.Unlock()
		if onLoggedOut != nil {
			onLoggedOut()
		}

	case *events.Disconnected:
		Logger.Info("Socket connection disconnected", "event", v)
		broadcast(simpleEvent(EventDisconnected))

	case *events.Connected:
		Logger.Info("Socket connection established", "session", b.cfg.Session, "event", v)
		broadcast(simpleEvent(EventConnected))
		if cli != nil {
			go func() {
				if err := b.groupManager.SyncAll(context.Background(), cli); err != nil {
					Logger.Warn("groupManager.SyncAll returned error", "err", err)
				}
			}()
		}

	case *events.Message:
		Logger.Debug("incoming message received", "event", v)
		go func(v *events.Message) {
			if v.Info.Chat.Server == "broadcast" || v.Info.Chat.String() == "status@broadcast" {
				b.handleLikeStatus(context.Background(), v)
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
		}(v)

	case *events.Presence:
		Logger.Debug("presence update received", "event", v)
		commands.TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		Logger.Debug("chat presence update received", "event", v)
		commands.TrackPresence(v.Sender, true)

	case *events.Receipt:
		if !v.Sender.IsEmpty() {
			commands.TrackPresence(v.Sender, true)
		}

	case *events.CallOffer:
		Logger.Debug("incoming call offer received", "event", v)
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
		Logger.Info("group metadata update received", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)
		b.handleGroupGreetings(context.Background(), v)
		b.handleGroupEventsNotification(context.Background(), v)
		b.handleGroupCaptcha(context.Background(), v)

	case *events.JoinedGroup:
		Logger.Info("joined group event received", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.Picture:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterJoin:
		Logger.Info("newsletter subscribed", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLeave:
		Logger.Info("newsletter unlinked", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterMuteChange:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLiveUpdate:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	// Stream, Session & Connection Diagnostics
	case *events.StreamError:
		Logger.Error("stream error received", "event", v)
	case *events.KeepAliveTimeout:
		Logger.Warn("keepalive ping timed out", "event", v)
	case *events.KeepAliveRestored:
		Logger.Info("keepalive connection restored", "event", v)
	case *events.ManualLoginReconnect:
		Logger.Info("manual login reconnect triggered", "event", v)
	case *events.QRScannedWithoutMultidevice:
		Logger.Warn("qr scanned on legacy non-multidevice client", "event", v)

	// Cryptography & Decryption Failures
	case *events.UndecryptableMessage:
		Logger.Warn("undecryptable message received", "event", v)
	case *events.UndecryptedMessage:
		Logger.Warn("undecrypted message received", "event", v)
	case *events.MediaRetry:
		Logger.Debug("media download retry signal", "event", v)

	// History & App State Synchronizations
	case *events.HistorySync:
		Logger.Info("history synchronization chunk received", "event", v)
	case *events.OfflineSyncPreview:
		Logger.Info("Offline message sync preview", "event", v)
	case *events.OfflineSyncCompleted:
		Logger.Info("Offline message sync completed", "event", v)
	case *events.AppState:
		Logger.Debug("app state sync mutation received", "event", v)
	case *events.AppStateSyncComplete:
		Logger.Info("app state sync complete", "event", v)

	// User, Contacts & Privacy Metadata
	case *events.PushName:
		Logger.Info("push name update received", "event", v)
	case *events.UserAbout:
		Logger.Info("user about/status text updated", "event", v)
	case *events.Contact:
		Logger.Debug("contact record updated", "event", v)
	case *events.IdentityChange:
		Logger.Warn("e2ee identity key changed", "event", v)
	case *events.PrivacySettings:
		Logger.Info("account privacy settings updated", "event", v)
	case *events.Blocklist:
		Logger.Info("blocklist synchronized", "event", v)
	case *events.NotifyAccountReachoutTimelock:
		Logger.Warn("account reachout timelock notification", "event", v)

	// Call Signaling Transitions
	case *events.CallOfferNotice:
		Logger.Info("call offer notice received", "event", v)
	case *events.CallAccept:
		Logger.Info("call accepted", "event", v)
	case *events.CallPreAccept:
		Logger.Debug("call pre-accept signal", "event", v)
	case *events.CallRelayLatency:
		Logger.Debug("call relay latency update", "event", v)
	case *events.CallTransport:
		Logger.Debug("call transport parameters negotiated", "event", v)
	case *events.CallTerminate:
		Logger.Info("call terminated", "event", v)
	case *events.CallReject:
		Logger.Info("call rejected", "event", v)
	case *events.UnknownCallEvent:
		Logger.Debug("unknown call event frame", "event", v)

	default:
		Logger.Debug("unhandled event received", "type", fmt.Sprintf("%T", evt), "event", evt)
	}
}

func buildIncomingMessagePayload(v *events.Message) IncomingMessagePayload {
	text := whatsrook.ExtractMessageText(v)
	mediaType := whatsrook.GetMediaType(v.Message)

	var quotedID string
	var quotedText string

	if ext := v.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		ci := ext.GetContextInfo()
		quotedID = ci.GetStanzaID()
		if ci.QuotedMessage != nil {
			quotedText = whatsrook.ExtractTextFromProto(ci.QuotedMessage)
		}
	}

	return IncomingMessagePayload{
		From:       v.Info.Chat.String(),
		Chat:       v.Info.Chat.String(),
		Sender:     v.Info.Sender.String(),
		Text:       text,
		MessageID:  v.Info.ID,
		PushName:   v.Info.PushName,
		Timestamp:  v.Info.Timestamp,
		IsGroup:    v.Info.IsGroup,
		IsFromMe:   v.Info.IsFromMe,
		MediaType:  mediaType,
		QuotedID:   quotedID,
		QuotedText: quotedText,
	}
}
