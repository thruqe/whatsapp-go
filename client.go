package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"whatsrook/util/botctx"
	"whatsrook/util/cache"
	"whatsrook/util/logger"
	"whatsrook/util/message"
	"whatsrook/util/qr"
	"whatsrook/util/send"
	"whatsrook/util/system"
	"whatsrook/util/webp"
)

type ClientType int

var (
	ErrLoggedOut = errors.New("logged out from WhatsApp")

	ErrPairingTimedOut = errors.New("pairing timed out")
	ErrPairTimeout     = ErrPairingTimedOut
)

const (
	ClientChrome ClientType = iota
	ClientAndroid
	ClientIos
)

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

type Config struct {
	DataDir         string
	Database        string
	Session         string
	ClientType      ClientType
	Business        bool
	Verbose         bool
	AsyncMessageAck bool
}

type Client struct {
	Config Config

	mu        sync.Mutex
	rawClient *whatsmeow.Client
	container *sqlstore.Container
}

func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	return &Client{
		Config: cfg,
	}
}

func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

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
	cli.AsyncMessageAck = c.Config.AsyncMessageAck
	cli.SetCallLogger(logger.ZerologStyle("wacaller"))

	isBusiness := c.Config.Business || (deviceStore != nil && (deviceStore.BusinessName != "" || strings.HasPrefix(strings.ToLower(deviceStore.Platform), "smb")))
	configureCompanionPlatform(c.Config.ClientType, isBusiness)

	c.rawClient = cli
	return nil
}

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

func OpenStoreContainer(ctx context.Context, dataDir, database string, sessionPhone ...string) (*sqlstore.Container, error) {
	waLogger := logger.NewWaLogger("database")

	trimmedDB := strings.TrimSpace(database)
	dbConn := ResolvePostgresURL(trimmedDB, sessionPhone...)

	// If resolved to a PostgreSQL URI, connect to Postgres
	if strings.HasPrefix(dbConn, "postgres://") || strings.HasPrefix(dbConn, "postgresql://") {
		logger.Debug("attempting connection to PostgreSQL database...", "url", sanitizeDBURL(dbConn))
		container, err := sqlstore.New(ctx, "postgres", dbConn, waLogger)
		if err == nil && container != nil {
			configureConnectionPool(container, dbConn)
			logger.Debug("successfully connected to PostgreSQL database")
			logger.Info("Database connected successfully")
			return container, nil
		}

		// SSL fallback retry logic
		if !strings.HasSuffix(dbConn, "?sslmode=disable") && !strings.Contains(dbConn, "sslmode=disable") {
			disableURL := ensureSSLDisabled(dbConn)
			logger.Warn("PostgreSQL SSL connection failed, attempting reconnection with sslmode=disable...", "err", err, "url", sanitizeDBURL(disableURL))
			container, errDisable := sqlstore.New(ctx, "postgres", disableURL, waLogger)
			if errDisable == nil && container != nil {
				configureConnectionPool(container, disableURL)
				logger.Debug("successfully connected to PostgreSQL database with sslmode=disable")
				logger.Info("Database connected successfully")
				return container, nil
			}
			return nil, fmt.Errorf("failed to connect to PostgreSQL (ssl retry also failed: %v): %w", errDisable, err)
		}

		return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
	}

	// SQLite fallback logic
	targetPath := trimmedDB
	if targetPath == "" || targetPath == "default" {
		if dataDir == "" {
			dataDir = DefaultDataDir()
		}
		targetPath = filepath.Join(dataDir, "whatsrook.db")
	}

	sqliteDSN := targetPath
	if !strings.HasPrefix(sqliteDSN, "file:") {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create sqlite storage directory: %w", err)
		}
		// Extreme performance tuning for SQLite:
		// - WAL: Write-Ahead Logging for concurrency
		// - synchronous=NORMAL: Safe for WAL, removes fsync bottleneck
		// - busy_timeout=30000: Wait up to 30s for lock to clear instead of erroring out
		// - cache_size=-64000: 64MB in-memory page cache
		// - temp_store=MEMORY: Store temporary tables/indices in RAM
		// - mmap_size=268435456: 256MB memory-mapped I/O
		sqliteDSN = fmt.Sprintf(
			"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(30000)&_pragma=cache_size(-64000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)",
			filepath.ToSlash(targetPath),
		)
	}

	logger.Debug("attempting connection to SQLite database...", "dsn", sqliteDSN)
	container, err := sqlstore.New(ctx, "sqlite", sqliteDSN, waLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %w", err)
	}

	// CRITICAL FOR SQLITE: Limit pool to 1 open connection so Go's SQL pool
	// queues concurrent goroutine queries in RAM instead of hammering the filesystem lock
	if db := container.Database(); db != nil && db.RawDB != nil {
		db.RawDB.SetMaxOpenConns(1)
		db.RawDB.SetMaxIdleConns(1)
		db.RawDB.SetConnMaxLifetime(0) // keep connection open
	}

	logger.Debug("successfully initialized SQLite database store")
	logger.Info("Database connected successfully")
	return container, nil
}

func configureConnectionPool(container *sqlstore.Container, dbConn string) {
	if container != nil {
		if db := container.Database(); db != nil && db.RawDB != nil {
			maxOpen := 50
			maxIdle := 25

			if strings.Contains(dbConn, "pooler.supabase.com") || strings.Contains(dbConn, "supabase.co") {
				maxOpen = 8
				maxIdle = 4
			}

			if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
				if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
					maxOpen = parsed
				}
			}

			if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
				if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
					maxIdle = parsed
				}
			} else if maxIdle > maxOpen {
				maxIdle = maxOpen
			}

			db.RawDB.SetMaxOpenConns(maxOpen)
			db.RawDB.SetMaxIdleConns(maxIdle)
			db.RawDB.SetConnMaxLifetime(10 * time.Minute)
			db.RawDB.SetConnMaxIdleTime(2 * time.Minute)
			logger.Debug("Configured database connection pool", "max_open", maxOpen, "max_idle", maxIdle)
		}
	}
}

func ParseDatabaseConfig(dbConf string, sessionPhone ...string) (string, string, error) {
	dbConn := ResolvePostgresURL(dbConf, sessionPhone...)
	if dbConn != "" {
		return "postgres", dbConn, nil
	}
	return "sqlite", filepath.Join(DefaultDataDir(), "whatsrook.db"), nil
}

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

func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
		if cli.Store != nil {
			_ = cli.Store.FlushSessions(context.Background())
		}
	}
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsConnected()
}

func (c *Client) IsLoggedIn() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsLoggedIn()
}

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

func (c *Client) SetPresence(state types.Presence) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), state)
}

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

func (c *Client) SetBrowserActive(active bool) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), types.PresenceAvailable)
}

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

	logger.Debug("initiating companion device phone pairing...", "phone", cleanPhone)
	logger.Info("Requesting pairing code from WhatsApp...")

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", fmt.Errorf("failed to connect for pairing: %w", err)
		}
	}

	code, err := cli.PairPhone(ctx, cleanPhone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("phone pairing request failed: %w", err)
	}

	logger.Debug("pairing code generated successfully", "phone", cleanPhone, "code", code)
	logger.Info("Pairing code generated successfully")
	return code, nil
}

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

func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

func (c *Client) QRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

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

type StoredSession struct {
	JID      string
	User     string
	PushName string
	Platform string
	Business bool
}

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

// SessionPurgerFunc defines a callback to purge custom bot domain data from the database.
type SessionPurgerFunc func(ctx context.Context, db *dbutil.Database, session string) error

var GlobalSessionPurger SessionPurgerFunc

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

		if GlobalSessionPurger != nil && client.container != nil && phone != "" {
			_ = GlobalSessionPurger(ctx, client.container.GetDB(), phone)
		}

		_ = cli.Store.Delete(ctx)
		_ = fallbackDeleteDevice(ctx, dataDir, database, phone)
		return nil
	}

	return fallbackDeleteDevice(ctx, dataDir, database, phone)
}

func fallbackDeleteDevice(ctx context.Context, dataDir, database, phone string) error {
	container, err := OpenStoreContainer(ctx, dataDir, database)
	if err != nil {
		return fmt.Errorf("failed to open database container: %w", err)
	}
	defer func() {
		_ = container.Close()
	}()

	if GlobalSessionPurger != nil && phone != "" {
		_ = GlobalSessionPurger(ctx, container.GetDB(), phone)
	}

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

func (c *Client) ClearSessionDB(ctx context.Context, phone string) {
	c.mu.Lock()
	raw := c.rawClient
	container := c.container
	c.mu.Unlock()

	if phone == "" && raw != nil && raw.Store != nil && raw.Store.ID != nil {
		phone = raw.Store.ID.User
	}
	if phone == "" {
		phone = c.Config.Session
	}

	if GlobalSessionPurger != nil && phone != "" {
		if container != nil && container.GetDB() != nil {
			_ = GlobalSessionPurger(ctx, container.GetDB(), phone)
		} else if raw != nil && raw.Store != nil {
			if s, ok := raw.Store.Identities.(*sqlstore.SQLStore); ok && s != nil {
				_ = GlobalSessionPurger(ctx, s.GetDB(), phone)
			}
		}
	}

	if raw != nil && raw.Store != nil {
		_ = raw.Store.Delete(ctx)
	}
	if phone != "" {
		_ = fallbackDeleteDevice(ctx, c.Config.DataDir, c.Config.Database, phone)
	}
}

var (
	NewMemoryStore       = cache.NewMemoryStore
	InitCache            = cache.Init
	NewWaLogger          = logger.NewWaLogger
	GetSystemStats       = system.GetStats
	FormatBytes          = system.FormatBytes
	AddStickerMetadata   = webp.AddStickerMetadata
	WriteStickerMetadata = webp.WriteStickerMetadata
	EncodePNG            = qr.EncodePNG
	StartQRServer        = qr.StartServer
)

// PluginContext captures the invocation execution environment for external/native plugin actions.
type PluginContext = botctx.PluginContext

// Loader represents a no-op loader retained for interface compatibility.
type Loader = botctx.Loader

// SettingGetter retrieves a setting value for a client and key from the database store.
type SettingGetter = botctx.SettingGetter

// SettingSetter updates a setting value for a client and key in the database store.
type SettingSetter = botctx.SettingSetter

// SettingDeleter removes a setting value for a client and key from the database store.
type SettingDeleter = botctx.SettingDeleter

// RecentMessageCache keeps an in-memory cache of recent incoming message events per JID.
type RecentMessageCache = message.RecentMessageCache

// AlbumMediaItem represents a media payload item to be bundled in a WhatsApp album.
type AlbumMediaItem = send.AlbumMediaItem

// Sender encapsulates message sending capabilities.
type Sender = send.Sender

var (
	GlobalSettingGetter  SettingGetter
	GlobalSettingSetter  SettingSetter
	GlobalSettingDeleter SettingDeleter
)

func init() {
	botctx.GlobalSettingGetter = func(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
		if GlobalSettingGetter != nil {
			return GlobalSettingGetter(ctx, client, key)
		}
		return "", nil
	}
	botctx.GlobalSettingSetter = func(ctx context.Context, client *whatsmeow.Client, key, value string) error {
		if GlobalSettingSetter != nil {
			return GlobalSettingSetter(ctx, client, key, value)
		}
		return nil
	}
	botctx.GlobalSettingDeleter = func(ctx context.Context, client *whatsmeow.Client, key string) error {
		if GlobalSettingDeleter != nil {
			return GlobalSettingDeleter(ctx, client, key)
		}
		return nil
	}
}

var (
	// Text formatting re-exports
	Bold         = botctx.Bold
	Boldf        = botctx.Boldf
	Italic       = botctx.Italic
	Italicf      = botctx.Italicf
	Code         = botctx.Code
	Codef        = botctx.Codef
	CodeBlock    = botctx.CodeBlock
	Strike       = botctx.Strike
	Strikef      = botctx.Strikef
	Quote        = botctx.Quote
	Quotef       = botctx.Quotef
	NewText      = botctx.NewText
	Sprintf      = botctx.Sprintf
	CancelLoader = botctx.CancelLoader

	// Message extraction and processing re-exports
	GetMediaType                 = message.GetMediaType
	ExtractMessageText           = message.ExtractMessageText
	ExtractTextFromProto         = message.ExtractTextFromProto
	ExtractMediaFromEvent        = message.ExtractMediaFromEvent
	ExtractMedia                 = message.ExtractMedia
	UnwrapMessageProto           = message.UnwrapMessageProto
	GetContextInfoFromProto      = message.GetContextInfoFromProto
	AttachContextInfo            = message.AttachContextInfo
	StripContextInfo             = message.StripContextInfo
	IsViewOnceMessage            = message.IsViewOnceMessage
	ExtractViewOnceMessage       = message.ExtractViewOnceMessage
	UnwrapAndSendViewOnceMessage = message.UnwrapAndSendViewOnceMessage
	RemoveEmojis                 = message.RemoveEmojis
	FormatTextResponseRaw        = message.FormatTextResponseRaw
	EncodeProtoMessage           = message.EncodeProtoMessage
	DecodeProtoMessage           = message.DecodeProtoMessage
	RecordRecentMessage          = message.RecordRecentMessage
	GetRecentMessageForJID       = message.GetRecentMessageForJID
	GlobalRecentMessages         = message.GlobalRecentMessages

	// Context, permission, and resolution re-exports
	IsSudoRaw              = botctx.IsSudoRaw
	IsAdminRaw             = botctx.IsAdminRaw
	IsBotAdminRaw          = botctx.IsBotAdminRaw
	IsSameUserRaw          = botctx.IsSameUserRaw
	ParticipantMatchesUser = botctx.ParticipantMatchesUser
	ResolveMentionJIDs     = botctx.ResolveMentionJIDs
	ResolveMentionRaw      = botctx.ResolveMentionRaw
	ResolveContactName     = botctx.ResolveContactName
	GetClientSetting       = botctx.GetClientSetting
	PutClientSetting       = botctx.PutClientSetting
	DeleteClientSetting    = botctx.DeleteClientSetting

	// Message sending re-exports
	NewSender                = send.New
	ResolveMentionJIDStrings = send.ResolveMentionJIDStrings
	SendText                 = send.Text
	SendTextWithID           = send.TextWithID
	SendTextWithMentions     = send.TextWithMentions
	SendReply                = send.Reply
	SendReplyWithID          = send.ReplyWithID
	SendReplyWithMentions    = send.ReplyWithMentions
	SendImage                = send.Image
	SendImageWithMentions    = send.ImageWithMentions
	SendVideo                = send.Video
	SendVideoGif             = send.VideoGif
	SendVideoWithMentions    = send.VideoWithMentions
	SendAudio                = send.Audio
	SendDocument             = send.Document
	SendSticker              = send.Sticker
	SendAlbum                = send.Album
	SendReact                = send.React
	SendReactMessage         = send.ReactMessage
	SendEdit                 = send.Edit
	SendDelete               = send.Delete

	// Interactive component dispatch re-exports
	DispatchListSelection = botctx.DispatchListSelection
	DispatchPollVoteEvent = botctx.DispatchPollVoteEvent
)
