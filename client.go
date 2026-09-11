// package whatsrook implements the core session manager, database coordinator, and lifecycle bridge
// for the whatsmeow whatsapp protocol engine.
//
// architectural mechanics:
// this is the central orchestrator coordinating persistent storage (PostgreSQL),
// device registration identity resolution, companion hardware profile emulation (chrome, android, ios),
// and event-driven message dispatching. it encapsulates raw connection primitives inside a thread-safe
// client abstraction, providing structured fallback strategies, integrated caching layers, and high-level messaging helpers.
package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	_ "github.com/lib/pq"

	"whatsrook/cache"
	"whatsrook/util"
	"whatsrook/util/logger"
	"whatsrook/util/qr"
)

// clienttype specifies the companion operating system and hardware profile to emulate during registration.
type ClientType int

var (
	// errloggedout is the sentinel error indicating that the active session has been explicitly
	// terminated by the remote whatsapp server or unlinked by the primary device.
	ErrLoggedOut = errors.New("logged out from WhatsApp")

	// ErrPairingTimedOut indicates that the phone pairing handshake failed or timed out.
	ErrPairingTimedOut = errors.New("pairing timed out")
	// ErrPairTimeout is an alias for ErrPairingTimedOut.
	ErrPairTimeout = ErrPairingTimedOut
)

const (
	// clientchrome emulates a desktop web client running on linux.
	ClientChrome ClientType = iota
	// clientandroid emulates an android mobile companion device.
	ClientAndroid
	// clientios emulates an ios mobile companion device.
	ClientIos
)

// parseclienttype parses an arbitrary platform string into its corresponding clienttype enum.
// this performs case-insensitive normalization; returns false if the platform identifier is unknown.
func ParseClientType(s string) (ClientType, bool) {
	c, ok := map[string]ClientType{
		"chrome":      ClientChrome,
		"android":     ClientAndroid,
		"ios":         ClientIos,
		"smb_android": ClientAndroid,
		"smba":        ClientAndroid,
		"smb_ios":     ClientIos,
		"smbi":        ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// Config configures client session parameters.
type Config struct {
	DataDir    string
	Database   string
	Session    string // Phone number
	ClientType ClientType
	Business   bool // Emulate WhatsApp Business client
	Verbose    bool // Verbose logging toggle
}

// Client wraps whatsmeow.Client with session lifecycle and event dispatching.
type Client struct {
	Config Config

	mu        sync.Mutex
	rawClient *whatsmeow.Client
	container *sqlstore.Container
}

// NewClient creates an uninitialized Client instance.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	return &Client{
		Config: cfg,
	}
}

// WAClient returns the underlying whatsmeow.Client instance, or nil if uninitialized.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// InitSession initializes persistent storage drivers, retrieves or provisions the companion device record,
// and constructs the active whatsmeow client instance.
//
// it automatically connects to the PostgreSQL database, resolves the companion device record
// based on the configured phone session, and binds zap-based structured loggers.
func (c *Client) InitSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	container, err := OpenStoreContainer(ctx, c.Config.DataDir, c.Config.Database, c.Config.Session)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	c.container = container

	deviceStore, err := c.resolveDeviceStore(ctx, container)
	if err != nil {
		return fmt.Errorf("failed to get device store: %w", err)
	}
	deviceStore.ExternalCache = cache.Default()

	waLogger := logger.NewWaLogger("client")
	cli := whatsmeow.NewClient(deviceStore, waLogger)
	cli.SetCallLogger(logger.ZerologStyle("wacaller"))

	// Configure companion platform registration headers and os version payloads
	isBusiness := c.Config.Business || (deviceStore != nil && (deviceStore.BusinessName != "" || strings.HasPrefix(strings.ToLower(deviceStore.Platform), "smb")))
	configureCompanionPlatform(c.Config.ClientType, isBusiness)

	c.rawClient = cli
	return nil
}

// configureCompanionPlatform configures device properties and base client payload based on client type and business status.
func configureCompanionPlatform(clientType ClientType, isBusiness bool) {
	switch clientType {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("16")
		if isBusiness {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_SMB_ANDROID.Enum()
		} else {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_ANDROID.Enum()
		}
		store.BaseClientPayload.UserAgent.OsVersion = new("16.0.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("16.0.0")
		store.BaseClientPayload.WebInfo = nil
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("18.0")
		if isBusiness {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_SMB_IOS.Enum()
		} else {
			store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_IOS.Enum()
		}
		store.BaseClientPayload.UserAgent.OsVersion = new("18.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("18.0")
		store.BaseClientPayload.WebInfo = nil
	default:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		osName := "Linux"
		switch runtime.GOOS {
		case "windows":
			osName = "Windows"
		case "darwin":
			osName = "macOS"
		}
		store.DeviceProps.Os = &osName
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
		store.BaseClientPayload.WebInfo = &waWa6.ClientPayload_WebInfo{
			WebSubPlatform: waWa6.ClientPayload_WebInfo_WEB_BROWSER.Enum(),
		}
	}
}

// sanitizeDBURL redacts sensitive database credentials prior to log emission.
func sanitizeDBURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "postgres://") || strings.HasPrefix(rawURL, "postgresql://") {
		parts := strings.SplitN(rawURL, "@", 2)
		if len(parts) == 2 {
			schemeUser := parts[0]
			subParts := strings.SplitN(schemeUser, ":", 3)
			if len(subParts) == 3 {
				return fmt.Sprintf("%s:%s:****@%s", subParts[0], subParts[1], parts[1])
			}
			return fmt.Sprintf("%s:****@%s", parts[0], parts[1])
		}
	}
	return rawURL
}

// ensureSSLDisabled injects or overrides sslmode=disable on postgresql connection strings during fallback attempts.
func ensureSSLDisabled(rawURL string) string {
	if strings.HasSuffix(rawURL, "?sslmode=disable") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		if strings.Contains(rawURL, "?") {
			return rawURL + "&sslmode=disable"
		}
		return rawURL + "?sslmode=disable"
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

// ResolvePostgresURL resolves the PostgreSQL database connection string following precedence:
// 1. Explicit database configuration argument (if postgres:// or postgresql://)
// 2. DATABASE_URL_<phone>
// 3. DATABASE_URL
// 4. POSTGRES_URL
// 5. DB_URL
// 6. Default fallback
func ResolvePostgresURL(dbConf string, sessionPhone ...string) string {
	dbConf = strings.TrimSpace(dbConf)
	if dbConf != "" && dbConf != "default" && dbConf != "postgres" && dbConf != "postgresql" {
		if strings.HasPrefix(dbConf, "postgres://") || strings.HasPrefix(dbConf, "postgresql://") {
			return dbConf
		}
	}

	if len(sessionPhone) > 0 {
		phone := strings.TrimPrefix(sessionPhone[0], "+")
		phone = strings.ReplaceAll(phone, " ", "")
		phone = strings.ReplaceAll(phone, "-", "")
		if phone != "" {
			if envURL := os.Getenv("DATABASE_URL_" + phone); envURL != "" {
				return strings.TrimSpace(envURL)
			}
		}
	}

	if env := os.Getenv("DATABASE_URL"); env != "" {
		return strings.TrimSpace(env)
	}
	if env := os.Getenv("POSTGRES_URL"); env != "" {
		return strings.TrimSpace(env)
	}
	if env := os.Getenv("DB_URL"); env != "" {
		return strings.TrimSpace(env)
	}

	return ""
}

// OpenStoreContainer opens and prepares a sqlstore.Container storage backend connected to PostgreSQL.
//
// it automatically handles connection retries, SSL mode fallbacks, and binds structured logging diagnostics.
func OpenStoreContainer(ctx context.Context, dataDir, database string, sessionPhone ...string) (*sqlstore.Container, error) {
	waLogger := logger.NewWaLogger("database")

	dbConfLower := strings.ToLower(strings.TrimSpace(database))
	if strings.Contains(dbConfLower, "sqlite") || strings.HasSuffix(dbConfLower, ".db") {
		return nil, fmt.Errorf("sqlite is not supported: PostgreSQL is the only supported database engine (e.g. postgres://user:password@host:5432/dbname)")
	}

	dbConn := ResolvePostgresURL(database, sessionPhone...)
	if !strings.HasPrefix(dbConn, "postgres://") && !strings.HasPrefix(dbConn, "postgresql://") {
		return nil, fmt.Errorf("invalid database connection string: PostgreSQL URL required (e.g. postgres://user:password@host:5432/dbname)")
	}

	logger.Info("attempting connection to PostgreSQL database...", "url", sanitizeDBURL(dbConn))
	container, err := sqlstore.New(ctx, "postgres", dbConn, waLogger)
	if err == nil && container != nil {
		configureConnectionPool(container)
		logger.Info("successfully connected to PostgreSQL database")
		return container, nil
	}

	// SSL fallback retry logic (if SSL fails, attempt with sslmode=disable)
	if !strings.HasSuffix(dbConn, "?sslmode=disable") && !strings.Contains(dbConn, "sslmode=disable") {
		disableURL := ensureSSLDisabled(dbConn)
		logger.Warn("PostgreSQL SSL connection failed, attempting reconnection with sslmode=disable...", "err", err, "url", sanitizeDBURL(disableURL))
		container, errDisable := sqlstore.New(ctx, "postgres", disableURL, waLogger)
		if errDisable == nil && container != nil {
			configureConnectionPool(container)
			logger.Info("successfully connected to PostgreSQL database with sslmode=disable")
			return container, nil
		}
		return nil, fmt.Errorf("failed to connect to PostgreSQL (ssl retry also failed: %v): %w", errDisable, err)
	}

	return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
}

// configureConnectionPool tunes PostgreSQL connection pool limits to prevent connection churn and network latency.
func configureConnectionPool(container *sqlstore.Container) {
	if container != nil {
		if db := container.Database(); db != nil && db.RawDB != nil {
			db.RawDB.SetMaxOpenConns(50)
			db.RawDB.SetMaxIdleConns(25)
			db.RawDB.SetConnMaxLifetime(15 * time.Minute)
			db.RawDB.SetConnMaxIdleTime(5 * time.Minute)
		}
	}
}

// ParseDatabaseConfig parses a database configuration string or URL into driver name and DSN.
func ParseDatabaseConfig(dbConf string, sessionPhone ...string) (string, string, error) {
	dbConn := ResolvePostgresURL(dbConf, sessionPhone...)
	return "postgres", dbConn, nil
}

// resolveDeviceStore retrieves an existing registered device or creates a fresh companion device identity.
func (c *Client) resolveDeviceStore(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	cleanPhone := strings.TrimPrefix(c.Config.Session, "+")
	cleanPhone = strings.ReplaceAll(cleanPhone, " ", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")

	if cleanPhone != "" {
		jid := types.NewJID(cleanPhone, types.DefaultUserServer)
		dev, err := container.GetDevice(ctx, jid)
		if err == nil && dev != nil {
			return dev, nil
		}

		devices, err := container.GetAllDevices(ctx)
		if err == nil {
			for _, d := range devices {
				if d.ID != nil && (d.ID.User == cleanPhone || strings.Contains(d.ID.String(), cleanPhone)) {
					return d, nil
				}
			}
		}

		return container.NewDevice(), nil
	}

	device, err := container.GetFirstDevice(ctx)
	if err == nil && device != nil {
		return device, nil
	}

	return container.NewDevice(), nil
}

// Connect starts the background network connection loop.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

// Disconnect gracefully terminates the active network connection.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
	}
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsConnected()
}

// IsLoggedIn returns whether the client has an active authenticated session.
func (c *Client) IsLoggedIn() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsLoggedIn()
}

// WaitForConnection blocks until the client is connected or timeout expires.
func (c *Client) WaitForConnection(ctx context.Context, timeout time.Duration) bool {
	if c.IsConnected() {
		return true
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
			if c.IsConnected() {
				return true
			}
		}
	}
	return c.IsConnected()
}

// SetPresence sends an explicit presence state to the WhatsApp servers.
func (c *Client) SetPresence(state types.Presence) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), state)
}

// SetOnline sets the bot's presence to online (available).
func (c *Client) SetOnline() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	if err := cli.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
		return err
	}
	return cli.SendPresence(context.Background(), types.PresenceAvailable)
}

// SetBrowserActive instructs the server whether this companion browser session is in the foreground.
func (c *Client) SetBrowserActive(active bool) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), types.PresenceAvailable)
}

// StartPresenceKeepalive starts a background goroutine that periodically sends PresenceAvailable.
func (c *Client) StartPresenceKeepalive(ctx context.Context, interval time.Duration) func() {
	if interval <= 0 {
		interval = 25 * time.Second
	}

	stop := make(chan struct{})
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.IsConnected() {
					_ = c.SetPresence(types.PresenceAvailable)
				}
			}
		}
	}()

	return func() {
		close(stop)
	}
}

// HandleSessionReset attempts to gracefully tear down and re-establish a session.
func (c *Client) HandleSessionReset(ctx context.Context) error {
	c.mu.Lock()
	cli := c.rawClient
	container := c.container
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
	}

	if container != nil {
		_ = container.Close()
	}

	c.mu.Lock()
	c.rawClient = nil
	c.container = nil
	c.mu.Unlock()

	return c.InitSession(ctx)
}

// Close gracefully closes the client connection and database storage container.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rawClient != nil {
		c.rawClient.Disconnect()
		c.rawClient = nil
	}

	if c.container != nil {
		err := c.container.Close()
		c.container = nil
		return err
	}

	return nil
}

// PairPhone initiates the modern 8-character pairing code linking procedure.
//
// it requests an alphanumeric pairing code for the provided international phone number.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return "", fmt.Errorf("client not initialized: call InitSession first")
	}

	cleanPhone := strings.TrimPrefix(phone, "+")
	cleanPhone = strings.ReplaceAll(cleanPhone, " ", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")

	if cleanPhone == "" {
		return "", fmt.Errorf("invalid phone number: cannot be empty")
	}

	logger.Info("initiating companion device phone pairing...", "phone", cleanPhone)

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", fmt.Errorf("failed to connect for pairing: %w", err)
		}
	}

	code, err := cli.PairPhone(ctx, cleanPhone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("phone pairing request failed: %w", err)
	}

	logger.Info("pairing code generated successfully", "phone", cleanPhone, "code", code)
	return code, nil
}

// PairQR requests a live QR authentication channel for pairing via visual camera scan.
func (c *Client) PairQR(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return nil, fmt.Errorf("client not initialized: call InitSession first")
	}

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect for QR pairing: %w", err)
		}
	}

	return qrChan, nil
}

// GetQRChannel is an alias for PairQR.
func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// QRChannel is an alias for GetQRChannel.
func (c *Client) QRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// defaultdatadir returns the default filesystem storage path across target operating systems.
func DefaultDataDir() string {
	if dir := os.Getenv("WHATSROOK_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".whatsrook"
	}
	return filepath.Join(home, ".whatsrook")
}

// storedsession describes an existing whatsapp device session found in the store.
type StoredSession struct {
	JID      string
	User     string // phone number (without plus)
	PushName string
	Platform string
	Business bool
}

// ListStoredSessions queries the database store for all saved companion device sessions in PostgreSQL.
//
// it inspects registered device records without opening active network websockets, returning
// structured metadata such as JID, push name, emulated platform, and business account indicators.
func ListStoredSessions(ctx context.Context, dataDir, database string) ([]StoredSession, error) {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = container.Close()
	}()

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	var sessions []StoredSession
	for _, dev := range devices {
		if dev.ID == nil || dev.ID.User == "" {
			continue
		}
		platform := "Chrome"
		if dev.Platform != "" {
			platform = dev.Platform
		}
		sessions = append(sessions, StoredSession{
			JID:      dev.ID.String(),
			User:     dev.ID.User,
			PushName: dev.PushName,
			Platform: platform,
			Business: dev.BusinessName != "",
		})
	}
	return sessions, nil
}

// DeleteStoredSession initiates a graceful server-side logout request to unpair the companion device,
// followed by complete purging of local session keys and device records from persistent storage.
func DeleteStoredSession(ctx context.Context, dataDir, database, phone string) error {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	client := NewClient(Config{
		DataDir:    dataDir,
		Database:   database,
		Session:    phone,
		ClientType: ClientChrome,
	})
	defer func() {
		_ = client.Close()
	}()

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	if err := client.InitSession(initCtx); err != nil {
		return fallbackDeleteDevice(ctx, dataDir, database, phone)
	}

	cli := client.WAClient()
	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		connected := make(chan struct{}, 1)
		cli.AddEventHandler(func(evt any) {
			if _, ok := evt.(*events.Connected); ok {
				select {
				case connected <- struct{}{}:
				default:
				}
			}
		})

		_ = cli.Connect()

		select {
		case <-connected:
			logoutCtx, logoutCancel := context.WithTimeout(ctx, 8*time.Second)
			_ = cli.Logout(logoutCtx)
			logoutCancel()
		case <-time.After(8 * time.Second):
		}

		_ = cli.Store.Delete(ctx)
		_ = fallbackDeleteDevice(ctx, dataDir, database, phone)
		return nil
	}

	return fallbackDeleteDevice(ctx, dataDir, database, phone)
}

// fallbackDeleteDevice performs direct database-level removal of device records when network logout is impossible or failed.
func fallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return fmt.Errorf("failed to open database container: %w", err)
	}
	defer func() {
		_ = container.Close()
	}()

	cleanPhone := strings.TrimPrefix(phone, "+")
	jid := types.NewJID(cleanPhone, types.DefaultUserServer)

	dev, err := container.GetDevice(ctx, jid)
	if err != nil {
		return fmt.Errorf("failed to lookup device in store: %w", err)
	}
	if dev != nil {
		if err := dev.Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete device record: %w", err)
		}
		return nil
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to scan devices: %w", err)
	}
	for _, d := range devices {
		if d.ID != nil && (d.ID.User == cleanPhone || strings.Contains(d.ID.String(), cleanPhone)) {
			_ = d.Delete(ctx)
			return nil
		}
	}

	return nil
}

// ClearSessionDB deletes the device record associated with this client or phone number.
func (c *Client) ClearSessionDB(ctx context.Context, phone string) {
	c.mu.Lock()
	raw := c.rawClient
	c.mu.Unlock()
	if raw != nil && raw.Store != nil {
		_ = raw.Store.Delete(ctx)
	}
	if phone != "" {
		_ = fallbackDeleteDevice(ctx, c.Config.DataDir, c.Config.Database, phone)
	}
}

var (
	// NewMemoryStore initializes a memory cache store.
	NewMemoryStore = cache.NewMemoryStore
	// InitCache initializes global caching.
	InitCache = cache.Init
	// NewWaLogger constructs a Zap protocol logger adapter.
	NewWaLogger = logger.NewWaLogger
	// GetSystemStats retrieves host hardware metrics.
	GetSystemStats = util.GetStats
	// FormatBytes formats byte counts.
	FormatBytes = util.FormatBytes
	// AddStickerMetadata injects EXIF metadata into WebP stickers.
	AddStickerMetadata = util.AddStickerMetadata
	// WriteStickerMetadata writes EXIF metadata to a WebP file.
	WriteStickerMetadata = util.WriteStickerMetadata
	// EncodePNG generates a QR code PNG image.
	EncodePNG = qr.EncodePNG
	// StartQRServer starts the local QR web pairing server.
	StartQRServer = qr.StartServer
)
