package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thruqe/whatsrook"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// BotConfig encapsulates runtime configuration parameters parsed from the CLI interface.
type BotConfig struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	ClientType      whatsrook.ClientType
	Business        bool
	Database        string
	Verbose         bool
	AsyncMessageAck bool
}

// Bot orchestrates the core WhatsApp client, event dispatcher, and session lifecycle.
type Bot struct {
	cfg          BotConfig
	client       *whatsrook.Client
	groupManager *GroupManager
	startupTime  time.Time
	loggedOut    atomic.Bool
	onLoggedOut  func()
	mu           sync.Mutex
}

// NewBot constructs and initializes a new Bot lifecycle manager.
func NewBot(cfg BotConfig) *Bot {
	b := &Bot{
		cfg:          cfg,
		groupManager: NewGroupManager(),
		startupTime:  time.Now(),
	}
	SetStartupTime(b.startupTime)
	return b
}

// Start boots the WhatsApp client and enters the session event loop.
func (b *Bot) Start(ctx context.Context) error {
	if b.cfg.Session == "" {
		return errors.New("session phone number is required")
	}

	client := whatsrook.NewClient(whatsrook.Config{
		Session:         b.cfg.Session,
		DataDir:         whatsrook.DefaultDataDir(),
		Database:        b.cfg.Database,
		ClientType:      b.cfg.ClientType,
		Business:        b.cfg.Business,
		Verbose:         b.cfg.Verbose,
		AsyncMessageAck: b.cfg.AsyncMessageAck,
	})

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	for {
		err := b.runSession(ctx)

		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		if errors.Is(err, whatsrook.ErrLoggedOut) || strings.Contains(err.Error(), "logged out") || b.loggedOut.Load() {
			Warn("logged out session detected; device record cleared from database")
			b.loggedOut.Store(false)
			return whatsrook.ErrLoggedOut
		}

		if errors.Is(err, whatsrook.ErrPairingTimedOut) {
			Error("session error", "err", "pairing timed out due to invalid remote response")
			Warn("session action", "warn", "clearing device record and regenerating pairing key")

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
		StopAutoMuteScheduler()
		StopAutoBioScheduler()
		_ = b.client.Close()
	}()

	if err := b.client.InitSession(sessionCtx); err != nil {
		return err
	}

	cli := b.client.WAClient()
	if cli == nil {
		return errors.New("failed to initialize lib client")
	}

	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
		InitTables(sessionCtx, s)
		if val, err := GetSetting(sessionCtx, s, BotNamePromptDismissedKey); err == nil && val == "true" {
			BotNamePromptDismissedCacheMu.Lock()
			BotNamePromptDismissedCache[s.JID] = true
			if cli.Store != nil && cli.Store.ID != nil {
				BotNamePromptDismissedCache[cli.Store.ID.ToNonAD().String()] = true
			}
			BotNamePromptDismissedCacheMu.Unlock()
		}
	}

	_ = b.groupManager.LoadFromDB(sessionCtx, cli)
	RegisterWACaller(cli)

	// Explicit session logout routine
	if b.cfg.Logout {
		Debug("initiating session logout", "session", b.cfg.Session)
		Info("Logging out of WhatsApp...")

		if cli.Store.ID == nil {
			Debug("session was never paired; skipping server-side revocation")
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
				Warn("Socket connection failed prior to logout; purging local device state only", "err", err)
			} else {
				logoutCtx, logoutCancel := context.WithTimeout(sessionCtx, 10*time.Second)
				select {
				case <-connected:
					Debug("connected to WhatsApp routing servers; dispatching logout frame")
				case <-logoutCtx.Done():
					Warn("connection timeout during logout sequence; forcing server revocation")
				}
				logoutCancel()

				if err := cli.Logout(sessionCtx); err != nil {
					Warn("server logout command returned error", "err", err)
				}
				cli.Disconnect()
			}
		}

		b.client.ClearSessionDB(sessionCtx, b.cfg.Session)
		Debug("session credentials and records purged successfully", "session", b.cfg.Session)
		Info("Logged out successfully")
		return nil
	}

	cli.AddEventHandler(func(evt any) {
		b.WAEventHandler(evt)
	})

	if cli.Store.ID == nil {
		if b.cfg.Pair {
			if err := b.runPairCode(sessionCtx); err != nil {
				return err
			}
		} else {
			go func() {
				if err := b.runQR(sessionCtx); err != nil {
					Error("runQR execution error", "err", err)
				}
			}()
		}
	} else {
		if err := cli.Connect(); err != nil {
			if b.loggedOut.Load() || strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "logged out") {
				Warn("connect rejected due to expired authentication; clearing local device record", "err", err)
				b.client.ClearSessionDB(sessionCtx, "")
				return whatsrook.ErrLoggedOut
			}
			return err
		}
	}

	if b.loggedOut.Load() {
		Warn("session revoked; clearing local device record")
		b.client.ClearSessionDB(sessionCtx, "")
		return whatsrook.ErrLoggedOut
	}

	if s, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
		StartAutoMuteScheduler(sessionCtx, cli)
		StartAutoBioScheduler(sessionCtx, cli)
	}

	go b.startPresenceHeartbeat(sessionCtx, cli)

	<-sessionCtx.Done()
	if b.loggedOut.Load() {
		Warn("session terminated during runtime; purging device record")
		b.client.ClearSessionDB(ctx, "")
		return whatsrook.ErrLoggedOut
	}
	return nil
}

func (b *Bot) runPairCode(ctx context.Context) error {
	code, err := b.client.PairPhone(ctx, b.cfg.Session)
	if err != nil {
		return err
	}
	Debug("pair code issued", "code", code)
	Info(fmt.Sprintf("PAIR CODE: %s", code))
	return nil
}

func (b *Bot) runQR(ctx context.Context) error {
	qrChan, err := b.client.PairQR(ctx)
	if err != nil {
		return err
	}

	qrServer, err := StartServer()
	if err != nil {
		Warn("failed to start temporary qr server", "err", err)
	} else {
		defer func() {
			_ = qrServer.Close()
			Debug("temporary qr server released", "port", qrServer.Port())
		}()
		Debug("temporary QR server started", "url", qrServer.URL())
		Info("Scan the QR code to pair your device")
		if b.cfg.QRCode {
			fmt.Printf("\n==> Scan QR Code interface via browser: %s\n\n", qrServer.URL())
		}
	}

	cli := b.client.WAClient()
	if cli != nil && !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			Warn("failed to connect socket for QR streaming", "err", err)
		}
	}

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			if qrServer != nil {
				qrServer.UpdateCode(evt.Code)
			}
			if termQR := RenderTerminal(evt.Code); termQR != "" {
				fmt.Printf("\n%s\n", termQR)
			}
		case "success":
			if qrServer != nil {
				qrServer.SetPaired()
				time.Sleep(1 * time.Second)
			}
			Debug("QR code pairing successful, shutting down temporary QR server")
			return nil
		default:
			Debug("qr event dispatched", "event", evt.Event)
		}
	}

	return nil
}

func (b *Bot) WAEventHandler(evt any) {
	defer func() {
		if r := recover(); r != nil {
			crashPath := RecordCrash(r, fmt.Sprintf("WAEventHandler: %T", evt))
			Error("Panic recovered in WhatsApp event handler", "panic", r, "crash_log", crashPath)
		}
	}()

	var cli *whatsmeow.Client
	if b.client != nil {
		cli = b.client.WAClient()
	}

	switch v := evt.(type) {
	case *events.QR:
		_ = v // QR frames handled directly via runQR channel loop

	case *events.PairSuccess:
		Debug("pairing completed successfully", "event", v)
		Info("Device paired successfully!")
		// After QR pairing, WhatsApp drops the pairing socket via stream:error 516.
		// PairSuccess fires while the socket is still alive, so we must wait for the
		// disconnect before calling Connect() — whatsmeow does not emit events.Disconnected
		// for expected pair-drops, so we poll IsConnected() with a short deadline instead.
		go func() {
			cli := b.client.WAClient()
			if cli == nil {
				return
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if !cli.IsConnected() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if err := cli.Connect(); err != nil {
				Error("failed to reconnect after QR pairing", "err", err)
			}
		}()

	case *events.PairError:
		Warn("pairing procedure failed", "err", v.Error, "event", v)

	case *events.LoggedOut:
		Warn("device logged out by remote session", "reason", v.Reason, "event", v)
		b.loggedOut.Store(true)
		b.mu.Lock()
		onLoggedOut := b.onLoggedOut
		b.mu.Unlock()
		if onLoggedOut != nil {
			onLoggedOut()
		}

	case *events.Disconnected:
		Debug("Socket connection disconnected", "event", v)
		Info("Disconnected from WhatsApp")

	case *events.Connected:
		Debug("Socket connection established", "session", b.cfg.Session, "event", v)
		Info("Connected to WhatsApp")
		if cli != nil {
			if len(cli.Store.PushName) == 0 {
				cli.Store.PushName = "WhatsRook"
			}
			cli.SetForceActiveDeliveryReceipts(true)
			if err := cli.SetPassive(context.Background(), false); err != nil {
				Warn("Failed to set browser active", "err", err)
			}
			if err := cli.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
				Warn("Failed to send presence available", "err", err)
			} else {
				Debug("Client presence set to online and browser active")
				Info("Bot is now online")
			}

			go func() {
				if err := b.groupManager.SyncAll(context.Background(), cli); err != nil {
					Warn("groupManager.SyncAll returned error", "err", err)
				}
			}()
		}

	case *events.Message:
		Debug("incoming message received", "event", v)
		go func(v *events.Message) {
			msgStart := time.Now()
			msgID := v.Info.ID
			defer func() {
				Debug("events.Message handler finished", "msgID", msgID, "elapsed", time.Since(msgStart))
			}()

			if v.Info.Chat.Server == "broadcast" || v.Info.Chat.String() == "status@broadcast" {
				HandleStatusBroadcast(context.Background(), cli, v, b.startupTime)
			}

			if !v.Info.Timestamp.IsZero() && v.Info.Timestamp.Before(b.startupTime) {
				Debug("events.Message: message sent before bot startup, ignoring commands",
					"msgID", v.Info.ID,
					"chat", v.Info.Chat.String(),
					"sender", v.Info.Sender.String(),
					"timestamp", v.Info.Timestamp,
					"startupTime", b.startupTime,
				)
				Dispatch(context.Background(), cli, v)
				return
			}

			t0 := time.Now()
			if HandlePendingAudioReply(context.Background(), cli, v) {
				Debug("HandlePendingAudioReply intercepted", "msgID", msgID, "elapsed", time.Since(t0))
				return
			}
			tAudio := time.Since(t0)

			t0 = time.Now()
			if HandlePendingMenuMediaReply(context.Background(), cli, v) {
				Debug("HandlePendingMenuMediaReply intercepted", "msgID", msgID, "elapsed", time.Since(t0))
				return
			}
			tMenu := time.Since(t0)

			t0 = time.Now()
			if HandlePendingBotCustomizationReply(context.Background(), cli, v) {
				Debug("HandlePendingBotCustomizationReply intercepted", "msgID", msgID, "elapsed", time.Since(t0))
				return
			}
			tCustom := time.Since(t0)

			t0 = time.Now()
			if HandlePendingCaptchaReply(context.Background(), cli, v) {
				Debug("HandlePendingCaptchaReply intercepted", "msgID", msgID, "elapsed", time.Since(t0))
				return
			}
			tCaptcha := time.Since(t0)

			Debug("Pre-dispatch reply checks completed",
				"msgID", msgID,
				"audio", tAudio,
				"menu", tMenu,
				"customization", tCustom,
				"captcha", tCaptcha,
			)

			t0 = time.Now()
			if Dispatch(context.Background(), cli, v) {
				Debug("Dispatch completed (handled)", "msgID", msgID, "elapsed", time.Since(t0))
				return
			}
			Debug("Dispatch completed (unhandled)", "msgID", msgID, "elapsed", time.Since(t0))
		}(v)

	case *events.Presence:
		Debug("presence update received", "event", v)
		TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		Debug("chat presence update received", "event", v)
		TrackPresence(v.Sender, true)

	case *events.Receipt:
		if !v.Sender.IsEmpty() {
			TrackPresence(v.Sender, true)
		}

	case *events.CallOffer:
		Debug("incoming call offer received", "event", v)
		HandleAntiCallEvent(context.Background(), cli, v, b.startupTime)

	case *events.GroupInfo:
		Debug("group metadata update received", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)
		HandleGreetings(context.Background(), cli, v, b.startupTime)
		HandleEventsNotification(context.Background(), cli, v, b.startupTime)
		HandleCaptcha(context.Background(), cli, v, b.startupTime)

	case *events.JoinedGroup:
		Debug("joined group event received", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.Picture:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterJoin:
		Debug("newsletter subscribed", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLeave:
		Debug("newsletter unlinked", "event", v)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterMuteChange:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	case *events.NewsletterLiveUpdate:
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)

	// Stream, Session & Connection Diagnostics
	case *events.StreamError:
		Error("stream error received", "event", v)
	case *events.KeepAliveTimeout:
		Warn("keepalive ping timed out", "event", v)
	case *events.KeepAliveRestored:
		Debug("keepalive connection restored", "event", v)
	case *events.ManualLoginReconnect:
		Debug("manual login reconnect triggered", "event", v)
	case *events.QRScannedWithoutMultidevice:
		Warn("qr scanned on legacy non-multidevice client", "event", v)

	// Cryptography & Decryption Failures
	case *events.UndecryptableMessage:
		Warn("undecryptable message received", "event", v)
	case *events.UndecryptedMessage:
		Warn("undecrypted message received", "event", v)
	case *events.MediaRetry:
		Debug("media download retry signal", "event", v)

	// History & App State Synchronizations
	case *events.HistorySync:
		Debug("history synchronization chunk received", "event", v)
	case *events.OfflineSyncPreview:
		Debug("Offline message sync preview", "event", v)
	case *events.OfflineSyncCompleted:
		Debug("Offline message sync completed", "event", v)
	case *events.AppState:
		Debug("app state sync mutation received", "event", v)
	case *events.AppStateSyncComplete:
		Debug("app state sync complete", "event", v)

	// User, Contacts & Privacy Metadata
	case *events.PushName:
		Debug("push name update received", "event", v)
		if cli != nil && cli.IsConnected() && cli.IsLoggedIn() {
			_ = cli.SendPresence(context.Background(), types.PresenceAvailable)
		}
	case *events.UserAbout:
		Debug("user about/status text updated", "event", v)
	case *events.Contact:
		Debug("contact record updated", "event", v)
	case *events.IdentityChange:
		Warn("e2ee identity key changed", "event", v)
	case *events.PrivacySettings:
		Debug("account privacy settings updated", "event", v)
	case *events.DisappearingMode:
		Debug("disappearing mode updated",
			"chat", v.Chat.String(),
			"timer", v.Timer.String(),
			"is_ephemeral", v.IsEphemeral,
			"trigger", v.Trigger,
			"initiator", v.Initiator,
		)
		b.groupManager.UpdateFromEvent(context.Background(), cli, v)
	case *events.Blocklist:
		Debug("blocklist synchronized", "event", v)
	case *events.NotifyAccountReachoutTimelock:
		Warn("account reachout timelock notification", "event", v)

	// Call Signaling Transitions
	case *events.CallOfferNotice:
		Debug("call offer notice received", "event", v)
	case *events.CallAccept:
		Debug("call accepted", "event", v)
	case *events.CallPreAccept:
		Debug("call pre-accept signal", "event", v)
	case *events.CallRelayLatency:
		Debug("call relay latency update", "event", v)
	case *events.CallTransport:
		Debug("call transport parameters negotiated", "event", v)
	case *events.CallTerminate:
		Debug("call terminated", "event", v)
	case *events.CallReject:
		Debug("call rejected", "event", v)
	case *events.UnknownCallEvent:
		Debug("unknown call event frame", "event", v)

	default:
		Debug("unhandled event received", "type", fmt.Sprintf("%T", evt), "event", evt)
	}
}

// startPresenceHeartbeat periodically refreshes presence available and active browser status.
func (b *Bot) startPresenceHeartbeat(ctx context.Context, cli *whatsmeow.Client) {
	ticker := time.NewTicker(4 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cli != nil && cli.IsConnected() && cli.IsLoggedIn() {
				if len(cli.Store.PushName) == 0 {
					cli.Store.PushName = "WhatsRook"
				}
				cli.SetForceActiveDeliveryReceipts(true)
				_ = cli.SetPassive(ctx, false)
				if err := cli.SendPresence(ctx, types.PresenceAvailable); err != nil {
					Debug("presence heartbeat failed", "err", err)
				} else {
					Debug("presence heartbeat refreshed (online / browser active)")
				}
			}
		}
	}
}
