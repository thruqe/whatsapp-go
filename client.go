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
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"whatsrook/builder"
	"whatsrook/cache"
	"whatsrook/logger"
	"whatsrook/qr"
	"whatsrook/system"
	"whatsrook/webp"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	_ "github.com/lib/pq"
)

// clienttype specifies the companion operating system and hardware profile to emulate during registration.
type ClientType int

// RawVersion holds the raw version string, typically populated at build time via
// -ldflags "-X whatsrook.RawVersion=...".
var RawVersion = "dev"

// Version represents the parsed version segments of the active WhatsRook release
// following the YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO format.
type Version struct {
	// Year segment (e.g. 26 for 2026).
	Year int
	// Major is an alias for Year for backward compatibility.
	Major int

	// Month segment (1-12).
	Month int
	// Minor is an alias for Month for backward compatibility.
	Minor int

	// Patch segment (cryptographic patch hash / short commit SHA, e.g. "7a3b4c1").
	Patch string

	// NumericPatch is populated if the patch segment can be parsed as an integer.
	NumericPatch int

	// ExtraBuildInfo contains any additional build metadata or suffix (e.g. "prod").
	ExtraBuildInfo string

	// Raw is the original unparsed version string.
	Raw string
}

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

// GetVersion returns the structured Version instance for the active WhatsRook release.
// It resolves the version from RawVersion (or runtime VCS build info if RawVersion is unset),
// following the YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO format (e.g., 26.09.7a3b4c1 or 26.09.7a3b4c1_prod).
func GetVersion() (Version, error) {
	vStr := strings.TrimSpace(RawVersion)
	if vStr == "" || vStr == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				vStr = strings.TrimSpace(info.Main.Version)
			} else {
				for _, s := range info.Settings {
					if s.Key == "vcs.revision" && s.Value != "" {
						vStr = strings.TrimSpace(s.Value)
						break
					}
				}
			}
		}
	}
	clean := strings.TrimPrefix(strings.TrimSpace(vStr), "v")
	if clean == "" || clean == "dev" {
		now := time.Now()
		yr := now.Year() % 100
		mo := int(now.Month())
		return Version{
			Year:  yr,
			Major: yr,
			Month: mo,
			Minor: mo,
			Patch: "dev",
			Raw:   "dev",
		}, nil
	}

	parts := strings.Split(clean, ".")
	if len(parts) < 2 {
		return Version{Raw: clean}, fmt.Errorf("invalid version format: %q (expected YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO)", clean)
	}

	year, errYear := strconv.Atoi(parts[0])
	month, errMonth := strconv.Atoi(parts[1])
	if errYear != nil || errMonth != nil {
		return Version{Raw: clean}, fmt.Errorf("non-numeric year/month segment in %q", clean)
	}

	var patchStr, extra string
	var numericPatch int
	if len(parts) >= 3 {
		patchRaw := strings.Join(parts[2:], ".")
		if idx := strings.IndexAny(patchRaw, "_+"); idx != -1 {
			patchStr = patchRaw[:idx]
			extra = patchRaw[idx+1:]
		} else {
			patchStr = patchRaw
		}
		if num, err := strconv.Atoi(patchStr); err == nil {
			numericPatch = num
		}
	}

	return Version{
		Year:           year,
		Major:          year,
		Month:          month,
		Minor:          month,
		Patch:          patchStr,
		NumericPatch:   numericPatch,
		ExtraBuildInfo: extra,
		Raw:            clean,
	}, nil
}

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

// config defines the operational parameters, storage directories, and runtime flags for a client instance.
type Config struct {
	// session holds the primary identifier (e.g., phone number or session token) for the device.
	Session string

	// datadir specifies the base filesystem path for logs and application storage.
	DataDir string

	// database defines the PostgreSQL connection URI (e.g., postgres://user:password@host:5432/dbname).
	Database string

	// clienttype defines the companion device platform signature emulated during pairing.
	ClientType ClientType

	// Business indicates whether this session emulates or connects to a WhatsApp Business account (SMB).
	Business bool

	// verbose toggles debug-level tracing across whatsmeow protocol logs and internal drivers.
	Verbose bool
}

// client is the primary abstraction encapsulating the whatsmeow core client, database container,
// and concurrency control primitives.
type Client struct {
	Config Config

	rawClient *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.Mutex
}

// newclient constructs an uninitialized client instance and populates baseline configuration defaults.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	return &Client{
		Config: cfg,
	}
}

// waclient returns the underlying raw whatsmeow client instance.
// caller must ensure that initsession has been executed before consuming this handle.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// InitSession initializes persistent storage drivers, retrieves or provisions the companion device record,
// and constructs the active whatsmeow client instance.
//
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

	return "postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
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
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	phone := strings.TrimPrefix(c.Config.Session, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	// 1. Try to match by configured session phone number across stored companion devices
	if phone != "" {
		for _, dev := range devices {
			if dev != nil && dev.ID != nil {
				if strings.HasPrefix(dev.ID.User, phone) || dev.ID.User == phone || dev.ID.ToNonAD().User == phone {
					return dev, nil
				}
			}
		}

		// Also try exact JID lookup
		jid := types.NewJID(phone, types.DefaultUserServer)
		deviceStore, err := container.GetDevice(ctx, jid)
		if err == nil && deviceStore != nil && deviceStore.ID != nil {
			return deviceStore, nil
		}
	}

	// 2. If no session identifier is specified and a stored device exists, reuse it
	if phone == "" && len(devices) > 0 {
		return devices[0], nil
	}

	// 3. Otherwise create a new device identity
	return container.NewDevice(), nil
}

// connect establishes the active websocket connection to the remote whatsapp server cluster.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

// disconnect cleanly terminates the underlying websocket session.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil {
		cli.Disconnect()
	}
}

// isconnected returns true if the raw websocket transport layer is currently active.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsConnected()
}

// isloggedin returns true if the client holds a valid cryptographic session state recognized by whatsapp.
func (c *Client) IsLoggedIn() bool {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return false
	}
	return cli.IsLoggedIn()
}

// waitforconnection blocks execution until the websocket connection state transition completes or ctx expires.
func (c *Client) WaitForConnection(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
			if c.IsConnected() && c.IsLoggedIn() {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return false
}

// SetPresence updates the user's presence status (e.g. types.PresenceAvailable for online, types.PresenceUnavailable for offline).
func (c *Client) SetPresence(state types.Presence) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}
	if cli.Store != nil && len(cli.Store.PushName) == 0 {
		cli.Store.PushName = "WhatsRook"
	}
	return cli.SendPresence(context.Background(), state)
}

// SetOnline marks the client as active and online (available) on WhatsApp, refreshing companion active status.
func (c *Client) SetOnline() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}
	if cli.Store != nil && len(cli.Store.PushName) == 0 {
		cli.Store.PushName = "WhatsRook"
	}
	cli.SetForceActiveDeliveryReceipts(true)
	_ = cli.SetPassive(context.Background(), false)
	return cli.SendPresence(context.Background(), types.PresenceAvailable)
}

// SetBrowserActive toggles the WhatsApp browser passive/active state (active = true marks browser in foreground).
func (c *Client) SetBrowserActive(active bool) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}
	return cli.SetPassive(context.Background(), !active)
}

// StartPresenceKeepalive runs a periodic ticker that maintains the client's online presence and active browser state.
func (c *Client) StartPresenceKeepalive(ctx context.Context, interval time.Duration) func() {
	if interval <= 0 {
		interval = 4 * time.Minute
	}
	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-ticker.C:
				if c.IsConnected() && c.IsLoggedIn() {
					_ = c.SetOnline()
				}
			}
		}
	}()
	return func() {
		close(stopChan)
	}
}

// handlesessionreset cleanly wipes local session identity keys and resets store records.
func (c *Client) HandleSessionReset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rawClient != nil {
		c.rawClient.Disconnect()
		if c.rawClient.Store != nil && c.rawClient.Store.ID != nil {
			_ = c.rawClient.Store.Delete(ctx)
		}
		c.rawClient = nil
	}

	if c.container != nil {
		_ = c.container.Close()
		c.container = nil
	}

	return nil
}

// close cleanly tears down client connections, background workers, and persistent store handles.
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

// PairPhone initiates companion device linking using an 8-character numeric verification code.
//
// it ensures the underlying WebSocket connection is open and authenticated with noise keys before
// dispatching the linking code registration request to WhatsApp routing servers.
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return "", fmt.Errorf("client not initialized")
	}

	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	clientType := whatsmeow.PairClientChrome
	osName := "Linux"
	switch runtime.GOOS {
	case "windows":
		osName = "Windows"
	case "darwin":
		osName = "macOS"
	}
	displayName := "Chrome (" + osName + ")"
	switch c.Config.ClientType {
	case ClientAndroid:
		clientType = whatsmeow.PairClientAndroid
		// WhatsApp server strictly requires companion_platform_display formatted as "Browser (OS)".
		// "Chrome (Android)" is accepted by the WA server and shows as Android in the linked devices list.
		displayName = "Chrome (Android)"
	case ClientIos:
		clientType = whatsmeow.PairClientSafari
		// "Safari (iOS)" is accepted by the WA server for iOS companion pairing.
		displayName = "Safari (iOS)"
	}

	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", fmt.Errorf("failed to connect websocket for phone pairing: %w", err)
		}
	}

	code, err := cli.PairPhone(ctx, phone, true, clientType, displayName)
	if err != nil {
		return "", fmt.Errorf("failed to pair phone: %w", err)
	}

	return code, nil
}

// pairqr initiates companion device pairing by returning a stream of qr code data strings.
func (c *Client) PairQR(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	return qrChan, nil
}

// GetQRChannel streams QR code events for pairing.
//
//go:fix inline
func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// QRChannel is an alias for GetQRChannel.
func (c *Client) QRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.PairQR(ctx)
}

// GetMediaType returns the human-readable media classification string.
func GetMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return "text"
	}
	if msg.ImageMessage != nil {
		return "image"
	}
	if msg.VideoMessage != nil {
		if msg.VideoMessage.GetGifPlayback() {
			return "gif"
		}
		return "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "voice"
		}
		return "audio"
	}
	if msg.DocumentMessage != nil {
		return "document"
	}
	if msg.StickerMessage != nil {
		return "sticker"
	}
	return "text"
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

// AttachContextInfo attaches ContextInfo metadata to inner proto payloads.
func AttachContextInfo(msg *waE2E.Message, ci *waE2E.ContextInfo) {
	if msg == nil || ci == nil {
		return
	}
	if msg.ExtendedTextMessage != nil {
		msg.ExtendedTextMessage.ContextInfo = ci
	} else if msg.ImageMessage != nil {
		msg.ImageMessage.ContextInfo = ci
	} else if msg.VideoMessage != nil {
		msg.VideoMessage.ContextInfo = ci
	} else if msg.AudioMessage != nil {
		msg.AudioMessage.ContextInfo = ci
	} else if msg.DocumentMessage != nil {
		msg.DocumentMessage.ContextInfo = ci
	} else if msg.StickerMessage != nil {
		msg.StickerMessage.ContextInfo = ci
	} else if msg.Conversation != nil {
		text := *msg.Conversation
		msg.Conversation = nil
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        &text,
			ContextInfo: ci,
		}
	}
}

// ─── Protocol Helper Utilities ─────────────────────────────────────────────

// UnwrapMessageProto unwraps nested message envelopes (ViewOnce, Ephemeral, Edited, Forwarded, BotInvoke, etc.).
func UnwrapMessageProto(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}
	for {
		if ephem := msg.GetEphemeralMessage(); ephem != nil && ephem.GetMessage() != nil {
			msg = ephem.GetMessage()
			continue
		}
		if vo := msg.GetViewOnceMessage(); vo != nil && vo.GetMessage() != nil {
			msg = vo.GetMessage()
			continue
		}
		if vo2 := msg.GetViewOnceMessageV2(); vo2 != nil && vo2.GetMessage() != nil {
			msg = vo2.GetMessage()
			continue
		}
		if vo2ext := msg.GetViewOnceMessageV2Extension(); vo2ext != nil && vo2ext.GetMessage() != nil {
			msg = vo2ext.GetMessage()
			continue
		}
		if docCap := msg.GetDocumentWithCaptionMessage(); docCap != nil && docCap.GetMessage() != nil {
			msg = docCap.GetMessage()
			continue
		}
		if edited := msg.GetEditedMessage(); edited != nil && edited.GetMessage() != nil {
			msg = edited.GetMessage()
			continue
		}
		if pm := msg.GetProtocolMessage(); pm != nil && pm.GetEditedMessage() != nil {
			msg = pm.GetEditedMessage()
			continue
		}
		if bfm := msg.GetBotForwardedMessage(); bfm != nil && bfm.GetMessage() != nil {
			msg = bfm.GetMessage()
			continue
		}
		if bim := msg.GetBotInvokeMessage(); bim != nil && bim.GetMessage() != nil {
			msg = bim.GetMessage()
			continue
		}
		if gmm := msg.GetGroupMentionedMessage(); gmm != nil && gmm.GetMessage() != nil {
			msg = gmm.GetMessage()
			continue
		}
		if acm := msg.GetAssociatedChildMessage(); acm != nil && acm.GetMessage() != nil {
			msg = acm.GetMessage()
			continue
		}
		if smm := msg.GetStatusMentionMessage(); smm != nil && smm.GetMessage() != nil {
			msg = smm.GetMessage()
			continue
		}
		if lsm := msg.GetLimitSharingMessage(); lsm != nil && lsm.GetMessage() != nil {
			msg = lsm.GetMessage()
			continue
		}
		if devSent := msg.GetDeviceSentMessage(); devSent != nil && devSent.GetMessage() != nil {
			msg = devSent.GetMessage()
			continue
		}
		if lot := msg.GetLottieStickerMessage(); lot != nil && lot.GetMessage() != nil {
			msg = lot.GetMessage()
			continue
		}
		if spm := msg.GetSpoilerMessage(); spm != nil && spm.GetMessage() != nil {
			msg = spm.GetMessage()
			continue
		}
		if eci := msg.GetEventCoverImage(); eci != nil && eci.GetMessage() != nil {
			msg = eci.GetMessage()
			continue
		}
		if gsm := msg.GetGroupStatusMessage(); gsm != nil && gsm.GetMessage() != nil {
			msg = gsm.GetMessage()
			continue
		}
		if gsm2 := msg.GetGroupStatusMessageV2(); gsm2 != nil && gsm2.GetMessage() != nil {
			msg = gsm2.GetMessage()
			continue
		}
		if gsmm := msg.GetGroupStatusMentionMessage(); gsmm != nil && gsmm.GetMessage() != nil {
			msg = gsmm.GetMessage()
			continue
		}
		if qrm := msg.GetQuestionReplyMessage(); qrm != nil && qrm.GetMessage() != nil {
			msg = qrm.GetMessage()
			continue
		}
		if pcm4 := msg.GetPollCreationMessageV4(); pcm4 != nil && pcm4.GetMessage() != nil {
			msg = pcm4.GetMessage()
			continue
		}
		break
	}
	return msg
}

// GetContextInfoFromProto retrieves ContextInfo from any supported protobuf message variant.
func GetContextInfoFromProto(msg *waE2E.Message) *waE2E.ContextInfo {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		return ext.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetContextInfo() != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetContextInfo() != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetContextInfo() != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetContextInfo() != nil {
		return doc.GetContextInfo()
	}
	if stk := msg.GetStickerMessage(); stk != nil && stk.GetContextInfo() != nil {
		return stk.GetContextInfo()
	}
	if btn := msg.GetButtonsMessage(); btn != nil && btn.GetContextInfo() != nil {
		return btn.GetContextInfo()
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil && btnResp.GetContextInfo() != nil {
		return btnResp.GetContextInfo()
	}
	if list := msg.GetListResponseMessage(); list != nil && list.GetContextInfo() != nil {
		return list.GetContextInfo()
	}
	if listMsg := msg.GetListMessage(); listMsg != nil && listMsg.GetContextInfo() != nil {
		return listMsg.GetContextInfo()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetContextInfo() != nil {
		return poll.GetContextInfo()
	}
	if poll2 := msg.GetPollCreationMessageV2(); poll2 != nil && poll2.GetContextInfo() != nil {
		return poll2.GetContextInfo()
	}
	if poll3 := msg.GetPollCreationMessageV3(); poll3 != nil && poll3.GetContextInfo() != nil {
		return poll3.GetContextInfo()
	}
	if poll5 := msg.GetPollCreationMessageV5(); poll5 != nil && poll5.GetContextInfo() != nil {
		return poll5.GetContextInfo()
	}
	if poll6 := msg.GetPollCreationMessageV6(); poll6 != nil && poll6.GetContextInfo() != nil {
		return poll6.GetContextInfo()
	}
	if im := msg.GetInteractiveMessage(); im != nil && im.GetContextInfo() != nil {
		return im.GetContextInfo()
	}
	if irm := msg.GetInteractiveResponseMessage(); irm != nil && irm.GetContextInfo() != nil {
		return irm.GetContextInfo()
	}
	if tbr := msg.GetTemplateButtonReplyMessage(); tbr != nil && tbr.GetContextInfo() != nil {
		return tbr.GetContextInfo()
	}
	if tm := msg.GetTemplateMessage(); tm != nil && tm.GetContextInfo() != nil {
		return tm.GetContextInfo()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil && ptv.GetContextInfo() != nil {
		return ptv.GetContextInfo()
	}
	if loc := msg.GetLocationMessage(); loc != nil && loc.GetContextInfo() != nil {
		return loc.GetContextInfo()
	}
	if liveLoc := msg.GetLiveLocationMessage(); liveLoc != nil && liveLoc.GetContextInfo() != nil {
		return liveLoc.GetContextInfo()
	}
	if cont := msg.GetContactMessage(); cont != nil && cont.GetContextInfo() != nil {
		return cont.GetContextInfo()
	}
	if conts := msg.GetContactsArrayMessage(); conts != nil && conts.GetContextInfo() != nil {
		return conts.GetContextInfo()
	}
	if gi := msg.GetGroupInviteMessage(); gi != nil && gi.GetContextInfo() != nil {
		return gi.GetContextInfo()
	}
	if evtMsg := msg.GetEventMessage(); evtMsg != nil && evtMsg.GetContextInfo() != nil {
		return evtMsg.GetContextInfo()
	}
	return nil
}

// ExtractMessageText extracts the human-readable text string from any incoming message event.
func ExtractMessageText(evt *events.Message) string {
	if evt == nil {
		return ""
	}
	if evt.Message != nil {
		if text := ExtractTextFromProto(evt.Message); text != "" {
			return text
		}
	}
	if evt.RawMessage != nil {
		if text := ExtractTextFromProto(evt.RawMessage); text != "" {
			return text
		}
	}
	return ""
}

// ExtractTextFromProto extracts human-readable text from a raw protobuf message.
func ExtractTextFromProto(msg *waE2E.Message) string {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil && ptv.GetCaption() != "" {
		return ptv.GetCaption()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV2(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV3(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV5(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if poll := msg.GetPollCreationMessageV6(); poll != nil && poll.GetName() != "" {
		return poll.GetName()
	}
	if im := msg.GetInteractiveMessage(); im != nil {
		if body := im.GetBody(); body != nil && body.GetText() != "" {
			return body.GetText()
		}
		if hdr := im.GetHeader(); hdr != nil && hdr.GetTitle() != "" {
			return hdr.GetTitle()
		}
	}
	if irm := msg.GetInteractiveResponseMessage(); irm != nil {
		if body := irm.GetBody(); body != nil && body.GetText() != "" {
			return body.GetText()
		}
		if nfr := irm.GetNativeFlowResponseMessage(); nfr != nil {
			if nfr.GetParamsJSON() != "" {
				return nfr.GetParamsJSON()
			}
			if nfr.GetName() != "" {
				return nfr.GetName()
			}
		}
	}
	if btn := msg.GetButtonsResponseMessage(); btn != nil {
		if btn.GetSelectedDisplayText() != "" {
			return btn.GetSelectedDisplayText()
		}
		if btn.GetSelectedButtonID() != "" {
			return btn.GetSelectedButtonID()
		}
	}
	if tbr := msg.GetTemplateButtonReplyMessage(); tbr != nil {
		if tbr.GetSelectedDisplayText() != "" {
			return tbr.GetSelectedDisplayText()
		}
		if tbr.GetSelectedID() != "" {
			return tbr.GetSelectedID()
		}
	}
	if list := msg.GetListResponseMessage(); list != nil {
		if list.GetTitle() != "" {
			return list.GetTitle()
		}
		if single := list.GetSingleSelectReply(); single != nil && single.GetSelectedRowID() != "" {
			return single.GetSelectedRowID()
		}
		if list.GetDescription() != "" {
			return list.GetDescription()
		}
	}
	if btn := msg.GetButtonsMessage(); btn != nil && btn.GetContentText() != "" {
		return btn.GetContentText()
	}
	if tm := msg.GetTemplateMessage(); tm != nil {
		if h := tm.GetHydratedTemplate(); h != nil && h.GetHydratedContentText() != "" {
			return h.GetHydratedContentText()
		}
		if h4 := tm.GetHydratedFourRowTemplate(); h4 != nil && h4.GetHydratedContentText() != "" {
			return h4.GetHydratedContentText()
		}
	}
	if gi := msg.GetGroupInviteMessage(); gi != nil {
		if cap := gi.GetCaption(); cap != "" {
			return cap
		}
		if name := gi.GetGroupName(); name != "" {
			return name
		}
	}
	if evtMsg := msg.GetEventMessage(); evtMsg != nil {
		if name := evtMsg.GetName(); name != "" {
			return name
		}
		if desc := evtMsg.GetDescription(); desc != "" {
			return desc
		}
	}
	return ""
}

// ParticipantMatchesUser checks if a group participant matches the target JID,
// comparing primary JID, LID, and PhoneNumber, as well as resolving through the client's store.
func ParticipantMatchesUser(ctx context.Context, client *whatsmeow.Client, p types.GroupParticipant, target types.JID) bool {
	if target.IsEmpty() {
		return false
	}
	cleanTarget := target.ToNonAD()

	// 1. Direct match with p.JID
	if !p.JID.IsEmpty() {
		cleanPJID := p.JID.ToNonAD()
		if cleanPJID == cleanTarget || (cleanPJID.User == cleanTarget.User && (cleanPJID.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 2. Direct match with p.LID
	if !p.LID.IsEmpty() {
		cleanPLID := p.LID.ToNonAD()
		if cleanPLID == cleanTarget || (cleanPLID.User == cleanTarget.User && (cleanPLID.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 3. Direct match with p.PhoneNumber
	if !p.PhoneNumber.IsEmpty() {
		cleanPPN := p.PhoneNumber.ToNonAD()
		if cleanPPN == cleanTarget || (cleanPPN.User == cleanTarget.User && (cleanPPN.Server == cleanTarget.Server || cleanTarget.Server == "")) {
			return true
		}
	}

	// 4. Check if target is the bot and p matches bot's ID or LID
	if client != nil && client.Store != nil {
		botID := client.Store.ID
		botLID := client.Store.LID

		targetIsBot := (botID != nil && !botID.IsEmpty() && IsSameUserRaw(ctx, client, cleanTarget, *botID)) ||
			(!botLID.IsEmpty() && IsSameUserRaw(ctx, client, cleanTarget, botLID))

		if targetIsBot {
			if botID != nil && !botID.IsEmpty() {
				cleanBotID := botID.ToNonAD()
				if (!p.JID.IsEmpty() && p.JID.ToNonAD() == cleanBotID) ||
					(!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == cleanBotID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == cleanBotID) {
					return true
				}
			}
			if !botLID.IsEmpty() {
				cleanBotLID := botLID.ToNonAD()
				if (!p.JID.IsEmpty() && p.JID.ToNonAD() == cleanBotLID) ||
					(!p.LID.IsEmpty() && p.LID.ToNonAD() == cleanBotLID) ||
					(!p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD() == cleanBotLID) {
					return true
				}
			}
		}
	}

	// 5. Cross-check using IsSameUserRaw across all non-empty JIDs on p
	if !p.JID.IsEmpty() && IsSameUserRaw(ctx, client, p.JID, cleanTarget) {
		return true
	}
	if !p.LID.IsEmpty() && IsSameUserRaw(ctx, client, p.LID, cleanTarget) {
		return true
	}
	if !p.PhoneNumber.IsEmpty() && IsSameUserRaw(ctx, client, p.PhoneNumber, cleanTarget) {
		return true
	}

	return false
}

// IsAdminRaw checks if a participant JID is an admin or superadmin in groupInfo.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo, userJID types.JID) bool {
	if groupInfo == nil || userJID.IsEmpty() {
		return false
	}
	for _, p := range groupInfo.Participants {
		if ParticipantMatchesUser(ctx, client, p, userJID) {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// IsBotAdminRaw checks if the bot itself has admin or superadmin privileges in groupInfo.
func IsBotAdminRaw(ctx context.Context, client *whatsmeow.Client, groupInfo *types.GroupInfo) bool {
	if client == nil || client.Store == nil || groupInfo == nil {
		return false
	}
	if client.Store.ID != nil && !client.Store.ID.IsEmpty() && IsAdminRaw(ctx, client, groupInfo, *client.Store.ID) {
		return true
	}
	if !client.Store.LID.IsEmpty() && IsAdminRaw(ctx, client, groupInfo, client.Store.LID) {
		return true
	}
	return false
}

// RecentMessageStore caches recent incoming messages for call responses, target resolution, and LID mapping lookups.
type RecentMessageStore struct {
	mu       sync.RWMutex
	messages map[types.JID]*events.Message
}

var GlobalRecentMessages = &RecentMessageStore{
	messages: make(map[types.JID]*events.Message),
}

// RecordRecentMessage caches an incoming message event indexed by its chat and sender JIDs.
func RecordRecentMessage(evt *events.Message) {
	if evt == nil {
		return
	}
	GlobalRecentMessages.mu.Lock()
	defer GlobalRecentMessages.mu.Unlock()
	if !evt.Info.Chat.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.Chat.ToNonAD()] = evt
	}
	if !evt.Info.Sender.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.Sender.ToNonAD()] = evt
	}
	if !evt.Info.SenderAlt.IsEmpty() {
		GlobalRecentMessages.messages[evt.Info.SenderAlt.ToNonAD()] = evt
	}
}

// GetRecentMessageForJID retrieves the most recent message associated with a JID.
func GetRecentMessageForJID(jid types.JID) *events.Message {
	if jid.IsEmpty() {
		return nil
	}
	GlobalRecentMessages.mu.RLock()
	defer GlobalRecentMessages.mu.RUnlock()
	jidNonAD := jid.ToNonAD()
	if evt, ok := GlobalRecentMessages.messages[jidNonAD]; ok {
		return evt
	}
	for k, v := range GlobalRecentMessages.messages {
		if v != nil && (k.User == jidNonAD.User || v.Info.Sender.ToNonAD().User == jidNonAD.User || v.Info.SenderAlt.ToNonAD().User == jidNonAD.User) {
			return v
		}
	}
	return nil
}

// SettingGetter retrieves a setting value for a client and key from the database store.
type SettingGetter func(ctx context.Context, client *whatsmeow.Client, key string) (string, error)

// SettingSetter updates a setting value for a client and key in the database store.
type SettingSetter func(ctx context.Context, client *whatsmeow.Client, key, value string) error

// SettingDeleter removes a setting value for a client and key from the database store.
type SettingDeleter func(ctx context.Context, client *whatsmeow.Client, key string) error

var (
	GlobalSettingGetter  SettingGetter
	GlobalSettingSetter  SettingSetter
	GlobalSettingDeleter SettingDeleter
)

// GetClientSetting retrieves a configuration value from the global setting getter or identity store fallback.
func GetClientSetting(ctx context.Context, client *whatsmeow.Client, key string) (string, error) {
	if GlobalSettingGetter != nil {
		if val, err := GlobalSettingGetter(ctx, client, key); err == nil && val != "" {
			return val, nil
		}
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			GetSetting(ctx context.Context, key string) (string, error)
		}); ok {
			return s.GetSetting(ctx, key)
		}
	}
	return "", nil
}

// PutClientSetting writes a configuration value to the global setting setter or identity store fallback.
func PutClientSetting(ctx context.Context, client *whatsmeow.Client, key, value string) error {
	if GlobalSettingSetter != nil {
		return GlobalSettingSetter(ctx, client, key, value)
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			PutSetting(ctx context.Context, key, value string) error
		}); ok {
			return s.PutSetting(ctx, key, value)
		}
	}
	return nil
}

// DeleteClientSetting deletes a configuration value using the global setting deleter or identity store fallback.
func DeleteClientSetting(ctx context.Context, client *whatsmeow.Client, key string) error {
	if GlobalSettingDeleter != nil {
		return GlobalSettingDeleter(ctx, client, key)
	}
	if client != nil && client.Store != nil && client.Store.Identities != nil {
		if s, ok := client.Store.Identities.(interface {
			DeleteSetting(ctx context.Context, key string) error
		}); ok {
			return s.DeleteSetting(ctx, key)
		}
	}
	return nil
}

// IsSudoRaw checks if a sender JID has sudo/owner privileges stored in database settings or environment.
func IsSudoRaw(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client == nil || sender.IsEmpty() {
		return false
	}

	// 1. Check if sender is the bot owner (Store.ID or Store.LID)
	if client.Store != nil {
		if client.Store.ID != nil && !client.Store.ID.IsEmpty() && IsSameUserRaw(ctx, client, sender, *client.Store.ID) {
			return true
		}
		if !client.Store.LID.IsEmpty() && IsSameUserRaw(ctx, client, sender, client.Store.LID) {
			return true
		}
	}

	// 2. Check environment variables (SUDOERS, SUDO, OWNER)
	for _, envKey := range []string{"SUDOERS", "SUDO", "OWNER"} {
		if envVal := strings.TrimSpace(os.Getenv(envKey)); envVal != "" {
			for entry := range strings.FieldsSeq(envVal) {
				cleanEntry := strings.TrimPrefix(entry, "+")
				if parsed, err := types.ParseJID(entry); err == nil {
					if IsSameUserRaw(ctx, client, sender, parsed) {
						return true
					}
				} else if cleanEntry != "" && (sender.ToNonAD().User == cleanEntry || strings.TrimPrefix(sender.ToNonAD().User, "+") == cleanEntry) {
					return true
				}
			}
		}
	}

	// 3. Check database settings (database "sudoers" list)
	if raw, err := GetClientSetting(ctx, client, "sudoers"); err == nil && raw != "" {
		// Resolve the sender's contact push name for username-token matching.
		var senderPushName string
		if client.Store != nil && client.Store.Contacts != nil {
			lookupJID := sender.ToNonAD()
			// If sender is a LID, try to get the PN for contact lookup.
			if lookupJID.Server == types.HiddenUserServer && client.Store.LIDs != nil {
				if pn, pnErr := client.Store.LIDs.GetPNForLID(ctx, lookupJID); pnErr == nil && !pn.IsEmpty() {
					lookupJID = pn.ToNonAD()
				}
			}
			if contact, cErr := client.Store.Contacts.GetContact(ctx, lookupJID); cErr == nil && contact.Found {
				if contact.Username != "" {
					senderPushName = strings.ToLower(contact.Username)
				} else if contact.PushName != "" {
					senderPushName = strings.ToLower(contact.PushName)
				} else if contact.FullName != "" {
					senderPushName = strings.ToLower(contact.FullName)
				}
			}
		}
		if senderPushName == "" {
			if recent := GetRecentMessageForJID(sender); recent != nil && recent.Info.PushName != "" {
				senderPushName = strings.ToLower(recent.Info.PushName)
			}
		}

		for sudoerStr := range strings.FieldsSeq(raw) {
			cleanSudoer := strings.TrimPrefix(sudoerStr, "+")
			if strings.Contains(sudoerStr, "@") {
				if sudoerJID, err := types.ParseJID(sudoerStr); err == nil {
					if IsSameUserRaw(ctx, client, sender, sudoerJID) {
						return true
					}
				}
			} else if cleanSudoer != "" {
				// Bare phone number match.
				senderUser := sender.ToNonAD().User
				if senderUser == cleanSudoer || strings.TrimPrefix(senderUser, "+") == cleanSudoer {
					return true
				}
				// Push name / username token match.
				if senderPushName != "" && strings.EqualFold(senderPushName, cleanSudoer) {
					return true
				}
			}
		}
	}

	// Backward compatibility fallback for per-JID key
	if val, err := GetClientSetting(ctx, client, "sudo:"+sender.ToNonAD().String()); err == nil && val == "true" {
		return true
	}
	if val, err := GetClientSetting(ctx, client, "sudo:"+sender.ToNonAD().User); err == nil && val == "true" {
		return true
	}
	if recent := GetRecentMessageForJID(sender); recent != nil && !recent.Info.SenderAlt.IsEmpty() {
		if val, err := GetClientSetting(ctx, client, "sudo:"+recent.Info.SenderAlt.ToNonAD().String()); err == nil && val == "true" {
			return true
		}
		if val, err := GetClientSetting(ctx, client, "sudo:"+recent.Info.SenderAlt.ToNonAD().User); err == nil && val == "true" {
			return true
		}
	}

	return false
}

// IsSameUserRaw compares two JIDs ignoring device and agent AD suffixes,
// resolving and matching Phone Numbers and Linked Identities (LIDs).
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	a = a.ToNonAD()
	b = b.ToNonAD()
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}

	// 1. Direct match (same JID or same server + user)
	if a == b || (a.Server == b.Server && a.User == b.User) {
		return true
	}

	// 2. Direct comparison between client's stored companion ID (PN) and LID
	if client != nil && client.Store != nil {
		id := client.Store.ID
		lid := client.Store.LID
		if id != nil && !id.IsEmpty() && !lid.IsEmpty() {
			idNonAD := id.ToNonAD()
			lidNonAD := lid.ToNonAD()
			if (a == idNonAD && b == lidNonAD) || (a == lidNonAD && b == idNonAD) {
				return true
			}
			if (a.User == idNonAD.User && b.User == lidNonAD.User) || (a.User == lidNonAD.User && b.User == idNonAD.User) {
				return true
			}
		}
	}

	// 3. Resolve LIDs to Phone Numbers via whatsmeow LID mapping store
	aPN := a
	bPN := b
	if a.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, a); err == nil && !pn.IsEmpty() {
			aPN = pn.ToNonAD()
		}
	}
	if b.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, b); err == nil && !pn.IsEmpty() {
			bPN = pn.ToNonAD()
		}
	}

	// Fallback to recent messages for LID->PN resolution if LID store lookup was empty
	if a.Server == types.HiddenUserServer && aPN == a {
		if recent := GetRecentMessageForJID(a); recent != nil && !recent.Info.SenderAlt.IsEmpty() && recent.Info.SenderAlt.Server == types.DefaultUserServer {
			aPN = recent.Info.SenderAlt.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, a, aPN)
			}
		}
	}
	if b.Server == types.HiddenUserServer && bPN == b {
		if recent := GetRecentMessageForJID(b); recent != nil && !recent.Info.SenderAlt.IsEmpty() && recent.Info.SenderAlt.Server == types.DefaultUserServer {
			bPN = recent.Info.SenderAlt.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, b, bPN)
			}
		}
	}

	if !aPN.IsEmpty() && !bPN.IsEmpty() && (aPN == bPN || (aPN.Server == bPN.Server && aPN.User == bPN.User)) {
		return true
	}

	// 4. Resolve Phone Numbers to LIDs via whatsmeow LID mapping store
	aLID := a
	bLID := b
	if a.Server == types.DefaultUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, a); err == nil && !lid.IsEmpty() {
			aLID = lid.ToNonAD()
		}
	}
	if b.Server == types.DefaultUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, b); err == nil && !lid.IsEmpty() {
			bLID = lid.ToNonAD()
		}
	}

	// Fallback to recent messages for PN->LID resolution
	if a.Server == types.DefaultUserServer && aLID == a {
		if recent := GetRecentMessageForJID(a); recent != nil && recent.Info.Sender.Server == types.HiddenUserServer {
			aLID = recent.Info.Sender.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, aLID, a)
			}
		}
	}
	if b.Server == types.DefaultUserServer && bLID == b {
		if recent := GetRecentMessageForJID(b); recent != nil && recent.Info.Sender.Server == types.HiddenUserServer {
			bLID = recent.Info.Sender.ToNonAD()
			if client != nil && client.Store != nil && client.Store.LIDs != nil {
				_ = client.Store.LIDs.PutLIDMapping(ctx, bLID, b)
			}
		}
	}

	if !aLID.IsEmpty() && !bLID.IsEmpty() && (aLID == bLID || (aLID.Server == bLID.Server && aLID.User == bLID.User)) {
		return true
	}

	return !aLID.IsEmpty() && !bLID.IsEmpty() && (aLID == bLID || (aLID.Server == bLID.Server && aLID.User == bLID.User))
}

// ResolveMentionJIDs resolves a participant JID to the full set of JIDs that must be
// included in ContextInfo.MentionedJID, and the parsed user identifier (phone number or
// LID user) to be used as "@" + tagUser in message text. In WhatsApp protocol, interactive
// mentions require the exact user part of the JID/LID (e.g. "@2348012345678"), NOT a push name.
func ResolveMentionJIDs(ctx context.Context, client *whatsmeow.Client, participant types.JID) ([]types.JID, string) {
	resolved := participant.ToNonAD()

	// Build the full set of JIDs: primary + paired LID or PN
	var pnJID, lidJID types.JID
	switch resolved.Server {
	case types.HiddenUserServer:
		lidJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, resolved); err == nil && !pn.IsEmpty() {
				pnJID = pn.ToNonAD()
			}
		}
	default:
		pnJID = resolved
		if client != nil && client.Store != nil && client.Store.LIDs != nil {
			if lid, err := client.Store.LIDs.GetLIDForPN(ctx, resolved); err == nil && !lid.IsEmpty() {
				lidJID = lid.ToNonAD()
			}
		}
	}

	seen := make(map[string]bool)
	var jids []types.JID
	for _, j := range []types.JID{pnJID, lidJID} {
		if !j.IsEmpty() {
			key := j.String()
			if !seen[key] {
				seen[key] = true
				jids = append(jids, j)
			}
		}
	}
	if len(jids) == 0 {
		jids = append(jids, resolved)
	}

	// In WhatsApp protocol, interactive text mentions require @<phone_number> or @<lid_user>
	// (the JID User part). WhatsApp clients match this against ContextInfo.MentionedJID to
	// highlight and render the clickable mention with the user's name on their UI.
	tagUser := pnJID.User
	if tagUser == "" {
		tagUser = resolved.User
	}
	if tagUser == "" {
		tagUser = resolved.String()
	}
	if tagUser == "" {
		tagUser = "User"
	}
	return jids, tagUser
}

// ResolveMentionRaw resolves a participant JID to its primary non-AD JID and user string (without @).
// The returned string is the parsed JID/LID user part (phone number or LID user) required by WhatsApp
// protocol for interactive "@" + user mentions, NOT the contact push name.
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, participant types.JID) (types.JID, string) {
	jids, tagUser := ResolveMentionJIDs(ctx, client, participant)
	if len(jids) > 0 {
		return jids[0], tagUser
	}
	return participant.ToNonAD(), tagUser
}

// ResolveContactName returns the push name or full name of a contact, or falls back to phone number / user ID.
func ResolveContactName(ctx context.Context, client *whatsmeow.Client, participant types.JID) string {
	resolved := participant.ToNonAD()
	if client != nil && client.Store != nil && client.Store.Contacts != nil {
		if contact, err := client.Store.Contacts.GetContact(ctx, resolved); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			} else if contact.FullName != "" {
				return contact.FullName
			}
		}
	}
	return resolved.User
}

// RemoveEmojis strips emoji characters from text strings.
func RemoveEmojis(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x1F000 && r <= 0x1FFFF || r >= 0x2600 && r <= 0x27BF {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ─── Text & Formatting Facades ─────────────────────────────────────────────

var (
	// Bold returns plain text without WhatsApp bold formatting symbols (*).
	Bold = builder.Bold
	// Boldf formats text according to format specifier without bold formatting symbols.
	Boldf = builder.Boldf
	// Italic returns plain text without WhatsApp italic formatting symbols (_).
	Italic = builder.Italic
	// Italicf formats text according to format specifier without italic formatting symbols.
	Italicf = builder.Italicf
	// Code returns plain text without WhatsApp inline code formatting symbols (`).
	Code = builder.Code
	// Codef formats text according to format specifier without inline code formatting symbols.
	Codef = builder.Codef
	// CodeBlock returns plain text without WhatsApp code block formatting symbols (```).
	CodeBlock = builder.CodeBlock
	// Strike returns plain text without WhatsApp strikethrough formatting symbols (~).
	Strike = builder.Strike
	// Strikef formats text according to format specifier without strikethrough formatting symbols.
	Strikef = builder.Strikef
	// Quote returns plain text without WhatsApp quote formatting symbols (>).
	Quote = builder.Quote
	// Quotef formats text according to format specifier without quote formatting symbols.
	Quotef = builder.Quotef
	// NewText creates an interactive text builder instance.
	NewText = builder.NewText
)

// ─── Interactive Loader Engine (No-Op) ─────────────────────────────────────

// Loader represents a no-op loader retained for interface compatibility.
type Loader struct{}

// StartLoader returns a no-op Loader.
func (ctx *PluginContext) StartLoader(initialText ...string) *Loader {
	return &Loader{}
}

// Cancel is a no-op.
func (l *Loader) Cancel() {}

// MessageID returns an empty MessageID.
func (l *Loader) MessageID() types.MessageID { return "" }

// Stop is a no-op.
func (l *Loader) Stop() {}

// Done is a no-op.
func (l *Loader) Done(finalText string) {}

// Delete is a no-op.
func (l *Loader) Delete() {}

// CancelLoader is a no-op loader cancellation function.
func CancelLoader(id string) bool { return false }

// ─── Plugin Context Model ──────────────────────────────────────────────────

// PluginContext captures the invocation execution environment for external/native plugin actions.
type PluginContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Client     *whatsmeow.Client
	Evt        *events.Message

	Command string
	Args    []string
	RawArgs string

	Chat   types.JID
	Sender types.JID

	isOwnerOnce sync.Once
	isOwnerVal  bool
	isSudoOnce  sync.Once
	isSudoVal   bool
}

// Cancel invokes context cancellation if configured.
func (c *PluginContext) Cancel() {
	if c.CancelFunc != nil {
		c.CancelFunc()
	}
}

// GetSendContext returns an active, non-canceled Context suitable for network dispatch.
func (c *PluginContext) GetSendContext() context.Context {
	if c == nil || c.Ctx == nil || c.Ctx.Err() != nil {
		return context.Background()
	}
	return c.Ctx
}

// GetClient returns the underlying whatsmeow client instance.
func (c *PluginContext) GetClient() *whatsmeow.Client {
	if c == nil {
		return nil
	}
	return c.Client
}

// GetChat returns the current chat JID.
func (c *PluginContext) GetChat() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Chat
}

// GetSender returns the triggering sender JID.
func (c *PluginContext) GetSender() types.JID {
	if c == nil {
		return types.EmptyJID
	}
	return c.Sender
}

// FormatTextResponse formats the text response stripping unwanted symbols.
func (c *PluginContext) FormatTextResponse(text string) string {
	return c.formatTextResponse(text)
}

// ReplyContextInfo returns the ContextInfo configured for quoted reply.
func (c *PluginContext) ReplyContextInfo() *waE2E.ContextInfo {
	return c.replyContextInfo()
}

func (c *PluginContext) formatTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = RemoveEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return text
}

func (c *PluginContext) replyContextInfo() *waE2E.ContextInfo {
	if c.Evt == nil {
		return nil
	}
	participant := c.Evt.Info.Sender.ToNonAD().String()
	stanzaID := c.Evt.Info.ID

	var quotedMsg *waE2E.Message
	if c.Evt.Message != nil {
		unwrapped := UnwrapMessageProto(c.Evt.Message)
		if unwrapped != nil {
			if cloned, ok := proto.Clone(unwrapped).(*waE2E.Message); ok && cloned != nil {
				stripContextInfo(cloned)
				quotedMsg = cloned
			}
		}
	}

	ci := &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		QuotedMessage: quotedMsg,
	}
	if c.Evt.Info.IsGroup {
		ci.Participant = &participant
	}
	return ci
}

// stripContextInfo zeroes out any nested ContextInfo within a message protobuf to eliminate cyclic pointer graphs.
func stripContextInfo(msg *waE2E.Message) {
	if msg == nil {
		return
	}
	if ext := msg.ExtendedTextMessage; ext != nil {
		ext.ContextInfo = nil
	}
	if img := msg.ImageMessage; img != nil {
		img.ContextInfo = nil
	}
	if vid := msg.VideoMessage; vid != nil {
		vid.ContextInfo = nil
	}
	if aud := msg.AudioMessage; aud != nil {
		aud.ContextInfo = nil
	}
	if doc := msg.DocumentMessage; doc != nil {
		doc.ContextInfo = nil
	}
	if stk := msg.StickerMessage; stk != nil {
		stk.ContextInfo = nil
	}
	if btn := msg.ButtonsMessage; btn != nil {
		btn.ContextInfo = nil
	}
	if btnResp := msg.ButtonsResponseMessage; btnResp != nil {
		btnResp.ContextInfo = nil
	}
	if list := msg.ListResponseMessage; list != nil {
		list.ContextInfo = nil
	}
	if poll := msg.PollCreationMessage; poll != nil {
		poll.ContextInfo = nil
	}
}

// SendText sends a plain text message without quoting.
func (c *PluginContext) SendText(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, &waE2E.Message{
		Conversation: &formatted,
	})
	return err
}

func (c *PluginContext) resolveMentionJIDStrings(mentions []types.JID) []string {
	if len(mentions) == 0 {
		return nil
	}
	ctx := c.GetSendContext()
	seen := make(map[string]bool)
	var mentionStrs []string
	for _, m := range mentions {
		if m.IsEmpty() {
			continue
		}
		norm := m.ToNonAD()
		key := norm.String()
		if !seen[key] {
			seen[key] = true
			mentionStrs = append(mentionStrs, key)
		}
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.LIDs != nil {
			switch norm.Server {
			case types.HiddenUserServer:
				if pn, err := c.Client.Store.LIDs.GetPNForLID(ctx, norm); err == nil && !pn.IsEmpty() {
					pnStr := pn.ToNonAD().String()
					if !seen[pnStr] {
						seen[pnStr] = true
						mentionStrs = append(mentionStrs, pnStr)
					}
				}
			case types.DefaultUserServer:
				if lid, err := c.Client.Store.LIDs.GetLIDForPN(ctx, norm); err == nil && !lid.IsEmpty() {
					lidStr := lid.ToNonAD().String()
					if !seen[lidStr] {
						seen[lidStr] = true
						mentionStrs = append(mentionStrs, lidStr)
					}
				}
			}
		}
	}
	return mentionStrs
}

// SendTextWithMentions sends a text message with mentioned JIDs without quoting.
func (c *PluginContext) SendTextWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	mentionStrs := c.resolveMentionJIDStrings(mentions)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentionStrs,
			},
		},
	})
	return err
}

// SendImage sends an image without quoting.
func (c *PluginContext) SendImage(data []byte, mimetype, caption string) error {
	return c.SendImageWithMentions(data, mimetype, caption, nil)
}

// SendImageWithMentions sends an image with mentions without quoting.
func (c *PluginContext) SendImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if mentionStrs := c.resolveMentionJIDStrings(mentions); len(mentionStrs) > 0 {
		ci = &waE2E.ContextInfo{
			MentionedJID: mentionStrs,
		}
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendVideo sends a video without quoting.
func (c *PluginContext) SendVideo(data []byte, mimetype, caption string) error {
	return c.sendVideoInternal(data, mimetype, caption, false)
}

// SendVideoGif sends a looping GIF video without quoting.
func (c *PluginContext) SendVideoGif(data []byte, mimetype, caption string) error {
	return c.sendVideoInternal(data, mimetype, caption, true)
}

// SendVideoWithMentions sends a video without quoting, mentioning specific user JIDs.
func (c *PluginContext) SendVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	var ci *waE2E.ContextInfo
	if mentionStrs := c.resolveMentionJIDStrings(mentions); len(mentionStrs) > 0 {
		ci = &waE2E.ContextInfo{
			MentionedJID: mentionStrs,
		}
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

func (c *PluginContext) sendVideoInternal(data []byte, mimetype, caption string, isGif bool) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   new(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendAudio sends an audio file without quoting.
func (c *PluginContext) SendAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendDocument sends a document without quoting.
func (c *PluginContext) SendDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileName:      &filename,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendSticker sends a WebP sticker without quoting.
func (c *PluginContext) SendSticker(data []byte) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}

	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		logger.Error("SendSticker: invalid payload data", "bytes", len(data))
		return fmt.Errorf("invalid sticker data: missing WebP header")
	}

	if meta, err := webp.GetStickerMetadata(data); err != nil || meta == nil {
		pack := c.GetStickerPack()
		author := c.GetStickerAuthor()
		logger.Debug("SendSticker: injecting clean WebP sticker metadata", "pack", pack, "author", author)
		if withMeta, err := webp.AddStickerMetadata(data, pack, author); err == nil {
			data = withMeta
		} else {
			logger.Warn("SendSticker: could not inject sticker metadata", "err", err)
		}
	}

	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload sticker failed: %w", err)
	}

	width := uint32(512)
	height := uint32(512)
	isAnimated := false
	mimetype := "image/webp"

	logger.Debug("SendSticker: outgoing StickerMessage debug info",
		"chat", c.Chat.String(),
		"bytes", len(data),
		"url", uploaded.URL,
		"direct_path", uploaded.DirectPath,
		"file_sha256", hex.EncodeToString(uploaded.FileSHA256),
		"file_enc_sha256", hex.EncodeToString(uploaded.FileEncSHA256),
		"media_key_len", len(uploaded.MediaKey),
		"width", width,
		"height", height,
		"mimetype", mimetype,
	)

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Width:         &width,
			Height:        &height,
			IsAnimated:    &isAnimated,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// SendTextWithGroupMention sends a text message with WhatsApp native group mention.
func (c *PluginContext) SendTextWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				NonJIDMentions: &nonJID,
			},
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// Reply sends a plain text response quoted to the triggering message.
func (c *PluginContext) Reply(text string) error {
	_, err := c.ReplyWithID(text)
	return err
}

// Replyf formats and sends a text response quoted to the triggering message.
func (c *PluginContext) Replyf(format string, args ...any) error {
	return c.Reply(fmt.Sprintf(format, args...))
}

// ReplyWithMentions sends a text message with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithMentions(text string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.MentionedJID = append(ci.MentionedJID, c.resolveMentionJIDStrings(mentions)...)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ci,
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithID sends a text message and returns the sent message ID.
func (c *PluginContext) ReplyWithID(text string) (string, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	ctx := c.GetSendContext()
	formatted := c.formatTextResponse(text)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: c.replyContextInfo(),
		},
	}
	resp, err := c.Client.SendMessage(ctx, c.Chat, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// SendTextWithID sends a text message without quoting and returns the sent message ID.
func (c *PluginContext) SendTextWithID(text string) (string, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return "", fmt.Errorf("client unavailable")
	}
	ctx := c.GetSendContext()
	formatted := c.formatTextResponse(text)
	msg := &waE2E.Message{
		Conversation: &formatted,
	}
	resp, err := c.Client.SendMessage(ctx, c.Chat, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ReplyWithImage uploads and sends an image quoted to the triggering message.
func (c *PluginContext) ReplyWithImage(data []byte, mimetype, caption string) error {
	return c.ReplyWithImageWithMentions(data, mimetype, caption, nil)
}

// ReplyWithImageWithMentions uploads and sends an image with mentions quoted to the triggering message.
func (c *PluginContext) ReplyWithImageWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload image failed: %w", err)
	}

	ci := c.replyContextInfo()
	if ci == nil && len(mentions) > 0 {
		ci = &waE2E.ContextInfo{}
	}
	if ci != nil {
		ci.MentionedJID = append(ci.MentionedJID, c.resolveMentionJIDStrings(mentions)...)
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithVideo uploads and sends a video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideo(data []byte, mimetype, caption string) error {
	return c.replyVideoInternal(data, mimetype, caption, false)
}

// ReplyWithVideoGif uploads and sends a GIF video quoted to the triggering message.
func (c *PluginContext) ReplyWithVideoGif(data []byte, mimetype, caption string) error {
	return c.replyVideoInternal(data, mimetype, caption, true)
}

func (c *PluginContext) replyVideoInternal(data []byte, mimetype, caption string, isGif bool) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			GifPlayback:   new(isGif),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithVideoWithMentions uploads and sends video quoted with mentioned user JIDs.
func (c *PluginContext) ReplyWithVideoWithMentions(data []byte, mimetype, caption string, mentions []types.JID) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaVideo)
	if err != nil {
		return fmt.Errorf("upload video failed: %w", err)
	}

	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.MentionedJID = c.resolveMentionJIDStrings(mentions)

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   ci,
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithAudio uploads and sends audio quoted to the triggering message.
func (c *PluginContext) ReplyWithAudio(data []byte, mimetype string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio failed: %w", err)
	}

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithDocument uploads and sends a file document quoted to the triggering message.
func (c *PluginContext) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document failed: %w", err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileName:      &filename,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Caption:       &caption,
			ContextInfo:   c.replyContextInfo(),
		},
	}
	_, err = c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// ReplyWithSticker uploads and sends a WebP sticker quoted to the triggering message.
func (c *PluginContext) ReplyWithSticker(data []byte) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}

	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		logger.Error("ReplyWithSticker: invalid payload data", "bytes", len(data))
		return fmt.Errorf("invalid sticker data: missing WebP header")
	}

	if meta, err := webp.GetStickerMetadata(data); err != nil || meta == nil {
		pack := c.GetStickerPack()
		author := c.GetStickerAuthor()
		logger.Debug("ReplyWithSticker: injecting clean WebP sticker metadata", "pack", pack, "author", author)
		if withMeta, err := webp.AddStickerMetadata(data, pack, author); err == nil {
			data = withMeta
		} else {
			logger.Warn("ReplyWithSticker: could not inject sticker metadata", "err", err)
		}
	} else {
		logger.Debug("ReplyWithSticker: existing sticker metadata found", "pack", meta.PackName, "publisher", meta.Publisher)
	}

	logger.Debug("ReplyWithSticker: uploading sticker payload", "chat", c.Chat.String(), "bytes", len(data))
	uploaded, err := c.Client.Upload(c.GetSendContext(), data, whatsmeow.MediaImage)
	if err != nil {
		logger.Error("ReplyWithSticker: upload failed", "chat", c.Chat.String(), "err", err)
		return fmt.Errorf("upload sticker failed: %w", err)
	}
	logger.Debug("ReplyWithSticker: upload succeeded", "chat", c.Chat.String(), "url", uploaded.URL)

	width := uint32(512)
	height := uint32(512)
	isAnimated := false
	mimetype := "image/webp"
	ci := c.replyContextInfo()

	var stanzaID string
	if ci != nil && ci.StanzaID != nil {
		stanzaID = *ci.StanzaID
	}

	logger.Debug("ReplyWithSticker: outgoing StickerMessage debug info",
		"chat", c.Chat.String(),
		"bytes", len(data),
		"url", uploaded.URL,
		"direct_path", uploaded.DirectPath,
		"file_sha256", hex.EncodeToString(uploaded.FileSHA256),
		"file_enc_sha256", hex.EncodeToString(uploaded.FileEncSHA256),
		"media_key_len", len(uploaded.MediaKey),
		"width", width,
		"height", height,
		"mimetype", mimetype,
		"quoted_id", stanzaID,
	)

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64(len(data))),
			Width:         &width,
			Height:        &height,
			IsAnimated:    &isAnimated,
			ContextInfo:   ci,
		},
	}
	resp, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	if err != nil {
		logger.Error("ReplyWithSticker: SendMessage failed", "chat", c.Chat.String(), "err", err)
	} else {
		logger.Debug("ReplyWithSticker: SendMessage succeeded", "chat", c.Chat.String(), "id", resp.ID)
	}
	return err
}

// ReplyWithGroupMention sends a text message with group mention quoted to the triggering message.
func (c *PluginContext) ReplyWithGroupMention(text string) error {
	c.StopAutoLoader()
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	formatted := c.formatTextResponse(text)
	var nonJID uint32 = 1
	ci := c.replyContextInfo()
	if ci == nil {
		ci = &waE2E.ContextInfo{}
	}
	ci.NonJIDMentions = &nonJID

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ci,
		},
	}
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, msg)
	return err
}

// Edit edits an existing message.
func (c *PluginContext) Edit(msgID types.MessageID, content any, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	var msg *waE2E.Message
	switch v := content.(type) {
	case string:
		formatted := c.formatTextResponse(v)
		msg = &waE2E.Message{
			Conversation: &formatted,
		}
	case *waE2E.Message:
		msg = v
	default:
		return whatsmeow.SendResponse{}, fmt.Errorf("unsupported content type: %T", content)
	}
	editMsg := c.Client.BuildEdit(c.Chat, msgID, msg)
	return c.Client.SendMessage(c.GetSendContext(), c.Chat, editMsg)
}

// Delete deletes/revokes a message for everyone.
func (c *PluginContext) Delete(msgID types.MessageID, senderJID ...types.JID) (whatsmeow.SendResponse, error) {
	c.StopAutoLoader()
	if c.Client == nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("client unavailable")
	}
	sJID := types.EmptyJID
	if len(senderJID) > 0 {
		sJID = senderJID[0]
	}
	revokeMsg := c.Client.BuildRevoke(c.Chat, sJID, msgID)
	return c.Client.SendMessage(c.GetSendContext(), c.Chat, revokeMsg)
}

// React sends an emoji reaction to the triggering message.
func (c *PluginContext) React(emoji string) error {
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	targetID := ""
	if c.Evt != nil {
		targetID = c.Evt.Info.ID
	}
	if targetID == "" {
		return fmt.Errorf("no target message for reaction")
	}
	reactionMsg := c.Client.BuildReaction(c.Chat, types.EmptyJID, targetID, emoji)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reactionMsg)
	return err
}

// ReactMessage sends an emoji reaction to a specific target message ID.
func (c *PluginContext) ReactMessage(targetID string, emoji string) error {
	if c.Client == nil {
		return fmt.Errorf("client unavailable")
	}
	reactionMsg := c.Client.BuildReaction(c.Chat, types.EmptyJID, targetID, emoji)
	_, err := c.Client.SendMessage(c.GetSendContext(), c.Chat, reactionMsg)
	return err
}

// Text initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) Text(initial ...string) *builder.TextBuilder {
	return builder.NewTextWithSender(c, initial...)
}

// NewText initializes a new TextBuilder bound to this PluginContext.
func (c *PluginContext) NewText(initial ...string) *builder.TextBuilder {
	return builder.NewTextWithSender(c, initial...)
}

// Rook returns a WARook builder engine bound to this PluginContext.
func (c *PluginContext) Rook() *builder.WARook {
	return builder.From(c)
}

// Poll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) Poll(question string) *builder.PollBuilder {
	return c.Rook().NewPoll(question)
}

// NewPoll initializes a new PollBuilder bound to this PluginContext.
func (c *PluginContext) NewPoll(question string) *builder.PollBuilder {
	return c.Rook().NewPoll(question)
}

// StartAutoLoader is a no-op retained for interface compatibility.
func (c *PluginContext) StartAutoLoader(delay ...time.Duration) {}

// StopAutoLoader is a no-op retained for interface compatibility.
func (c *PluginContext) StopAutoLoader() {}

// GetPrefix returns the configured command prefix, default ".".
func (c *PluginContext) GetPrefix() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "prefix"); err == nil && val != "" {
			return val
		}
	}
	return "."
}

// GetBotName returns the configured bot display name, default "WhatsRook".
func (c *PluginContext) GetBotName() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "bot_name"); err == nil && val != "" {
			return val
		}
	}
	return "WhatsRook"
}

// GetStickerPack returns the configured default sticker pack name, defaulting to GetBotName().
func (c *PluginContext) GetStickerPack() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "sticker_pack"); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return c.GetBotName()
}

// GetStickerAuthor returns the configured default sticker author/publisher, defaulting to "WhatsRook".
func (c *PluginContext) GetStickerAuthor() string {
	if c != nil && c.Client != nil {
		if val, err := GetClientSetting(c.GetSendContext(), c.Client, "sticker_author"); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return "WhatsRook"
}

// IsOwner returns true if the sender is the primary bot owner.
func (c *PluginContext) IsOwner() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isOwnerOnce.Do(func() {
		c.isOwnerVal = c.calculateIsOwner()
	})
	return c.isOwnerVal
}

func (c *PluginContext) calculateIsOwner() bool {
	if c.Evt != nil && c.Evt.Info.IsFromMe {
		return true
	}
	if c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Sender, c.Client.Store.LID) {
		return true
	}
	if c.Evt != nil && !c.Evt.Info.SenderAlt.IsEmpty() {
		if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, *c.Client.Store.ID) {
			return true
		}
		if !c.Client.Store.LID.IsEmpty() && IsSameUserRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt, c.Client.Store.LID) {
			return true
		}
	}
	return false
}

// IsSudo returns true if the sender is a sudo user or bot owner.
func (c *PluginContext) IsSudo() bool {
	if c == nil || c.Client == nil {
		return false
	}
	c.isSudoOnce.Do(func() {
		c.isSudoVal = c.calculateIsSudo()
	})
	return c.isSudoVal
}

func (c *PluginContext) calculateIsSudo() bool {
	if c.IsOwner() {
		return true
	}
	if IsSudoRaw(c.GetSendContext(), c.Client, c.Sender) {
		return true
	}
	if c.Evt != nil {
		if !c.Evt.Info.SenderAlt.IsEmpty() {
			if IsSudoRaw(c.GetSendContext(), c.Client, c.Evt.Info.SenderAlt) {
				c.Client.StoreLIDPNMapping(c.GetSendContext(), c.Evt.Info.SenderAlt, c.Sender)
				return true
			}
		}
		if c.Evt.Info.PushName != "" {
			if raw, err := GetClientSetting(c.GetSendContext(), c.Client, "sudoers"); err == nil && raw != "" {
				pushLower := strings.ToLower(strings.TrimSpace(c.Evt.Info.PushName))
				for sudoerStr := range strings.FieldsSeq(raw) {
					if strings.EqualFold(sudoerStr, pushLower) {
						return true
					}
				}
			}
		}
	}
	return false
}

// GetQuotedMessage returns the quoted message proto if the triggering message is a reply.
func (c *PluginContext) GetQuotedMessage() *waE2E.Message {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.QuotedMessage != nil {
		return UnwrapMessageProto(ci.QuotedMessage)
	}
	return nil
}

// GetQuotedSender returns the quoted message sender JID.
func (c *PluginContext) GetQuotedSender() (types.JID, bool) {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return types.EmptyJID, false
	}
	ci := GetContextInfoFromProto(c.Evt.Message)
	if ci != nil && ci.Participant != nil && *ci.Participant != "" {
		if parsed, err := types.ParseJID(*ci.Participant); err == nil && !parsed.IsEmpty() {
			return parsed.ToNonAD(), true
		}
	}
	// Fallback for 1-on-1 (DM) replies where Participant may be omitted by WhatsApp clients:
	if ci != nil && ci.QuotedMessage != nil && !c.Chat.IsEmpty() && c.Chat.Server != "g.us" && c.Chat.Server != "broadcast" {
		if c.Evt.Info.IsFromMe {
			return c.Chat.ToNonAD(), true
		}
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			return c.Client.Store.ID.ToNonAD(), true
		}
	}
	return types.EmptyJID, false
}

// GetContextInfo returns the ContextInfo from the triggering message.
func (c *PluginContext) GetContextInfo() *waE2E.ContextInfo {
	if c == nil || c.Evt == nil || c.Evt.Message == nil {
		return nil
	}
	return GetContextInfoFromProto(c.Evt.Message)
}

// GetMentionedJIDs returns any mentioned JIDs in the triggering message context.
func (c *PluginContext) GetMentionedJIDs() []types.JID {
	ci := c.GetContextInfo()
	if ci == nil || len(ci.MentionedJID) == 0 {
		return nil
	}
	var res []types.JID
	for _, m := range ci.MentionedJID {
		if parsed, err := types.ParseJID(m); err == nil {
			res = append(res, parsed)
		}
	}
	return res
}

// IsSenderAdmin checks if the sender has admin privileges in the provided groupInfo.
func (c *PluginContext) IsSenderAdmin(groupInfo *types.GroupInfo) bool {
	if c == nil || c.Client == nil || groupInfo == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, groupInfo, c.Sender)
}

// ResolveMentionJIDs resolves a JID to its full set of JIDs (phone-number JID + LID)
// needed in ContextInfo.MentionedJID, and the parsed user string (without @) for text mentions.
func (c *PluginContext) ResolveMentionJIDs(jid types.JID) ([]types.JID, string) {
	return ResolveMentionJIDs(c.GetSendContext(), c.Client, jid)
}

// ResolveMention resolves a JID to its primary normalized non-AD JID and user string (without @).
func (c *PluginContext) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(c.GetSendContext(), c.Client, jid)
}

// FormatMention returns "@user" string and the resolved primary JID.
func (c *PluginContext) FormatMention(jid types.JID) (string, types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	resolved := jid.ToNonAD()
	if len(jids) > 0 {
		resolved = jids[0]
	}
	return "@" + tagUser, resolved
}

// FormatMentionJIDs returns "@user" string and all associated JIDs (PN + LID).
func (c *PluginContext) FormatMentionJIDs(jid types.JID) (string, []types.JID) {
	jids, tagUser := c.ResolveMentionJIDs(jid)
	return "@" + tagUser, jids
}

// GetContactName returns the display push name or full name of a contact, or falls back to the phone/user ID.
func (c *PluginContext) GetContactName(jid types.JID) string {
	return ResolveContactName(c.GetSendContext(), c.Client, jid)
}

// ResolvePN returns the normalized non-AD phone number JID.
func (c *PluginContext) ResolvePN(jid types.JID) types.JID {
	return jid.ToNonAD()
}

// IsSameUser compares two JIDs ignoring device suffixes.
func (c *PluginContext) IsSameUser(a, b types.JID) bool {
	return IsSameUserRaw(c.GetSendContext(), c.Client, a, b)
}

// IsTargetSudo checks if a target JID is a sudo user or owner.
func (c *PluginContext) IsTargetSudo(target types.JID) bool {
	if c == nil || c.Client == nil {
		return false
	}
	return IsSudoRaw(c.GetSendContext(), c.Client, target)
}

// IsTargetOwner checks if a target JID is the bot owner.
func (c *PluginContext) IsTargetOwner(target types.JID) bool {
	if c == nil || c.Client == nil || c.Client.Store == nil {
		return false
	}
	if c.Client.Store.ID != nil && !c.Client.Store.ID.IsEmpty() && c.IsSameUser(target, *c.Client.Store.ID) {
		return true
	}
	if !c.Client.Store.LID.IsEmpty() && c.IsSameUser(target, c.Client.Store.LID) {
		return true
	}
	return false
}

// GetTargets resolves target user JIDs from quoted reply, mentions, or arguments.
func (c *PluginContext) GetTargets() []types.JID {
	if c == nil {
		return nil
	}
	if q, ok := c.GetQuotedSender(); ok && !q.IsEmpty() {
		return []types.JID{q.ToNonAD()}
	}
	if m := c.GetMentionedJIDs(); len(m) > 0 {
		var resolved []types.JID
		for _, j := range m {
			if !j.IsEmpty() {
				resolved = append(resolved, j.ToNonAD())
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if len(c.Args) > 0 {
		var resolved []types.JID
		for _, arg := range c.Args {
			if strings.Contains(arg, "@") {
				if parsed, err := types.ParseJID(arg); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			clean := strings.TrimLeft(arg, "@+")
			if strings.Contains(clean, "@") {
				if parsed, err := types.ParseJID(clean); err == nil && !parsed.IsEmpty() {
					resolved = append(resolved, parsed.ToNonAD())
					continue
				}
			}
			if len(clean) >= 5 {
				resolved = append(resolved, types.NewJID(clean, types.DefaultUserServer))
			}
		}
		if len(resolved) > 0 {
			return resolved
		}
	}
	if !c.Chat.IsEmpty() && c.Chat.Server != "g.us" && c.Chat.Server != "broadcast" {
		if c.Client != nil && c.Client.Store != nil && c.Client.Store.ID != nil {
			if !c.IsSameUser(c.Chat, *c.Client.Store.ID) {
				if len(c.Args) == 0 {
					c.Args = []string{c.Chat.ToNonAD().String()}
				}
				return []types.JID{c.Chat.ToNonAD()}
			}
		} else {
			if len(c.Args) == 0 {
				c.Args = []string{c.Chat.ToNonAD().String()}
			}
			return []types.JID{c.Chat.ToNonAD()}
		}
	}
	return nil
}

// GetMedia downloads media bytes and mimetype from the triggering message or quoted message.
func (c *PluginContext) GetMedia() ([]byte, string, error) {
	if c.Client == nil {
		return nil, "", fmt.Errorf("client unavailable")
	}
	extract := func(msg *waE2E.Message) ([]byte, string, bool) {
		msg = UnwrapMessageProto(msg)
		if msg == nil {
			return nil, "", false
		}
		var downloadable whatsmeow.DownloadableMessage
		var mime string
		if img := msg.GetImageMessage(); img != nil {
			downloadable = img
			mime = img.GetMimetype()
		} else if vid := msg.GetVideoMessage(); vid != nil {
			downloadable = vid
			mime = vid.GetMimetype()
		} else if aud := msg.GetAudioMessage(); aud != nil {
			downloadable = aud
			mime = aud.GetMimetype()
		} else if doc := msg.GetDocumentMessage(); doc != nil {
			downloadable = doc
			mime = doc.GetMimetype()
		} else if stk := msg.GetStickerMessage(); stk != nil {
			downloadable = stk
			mime = stk.GetMimetype()
		}
		if downloadable == nil {
			return nil, "", false
		}
		data, err := c.Client.Download(c.GetSendContext(), downloadable)
		if err != nil {
			logger.Warn("PluginContext.GetMedia: download failed", "mime", mime, "err", err)
			return nil, "", false
		}
		return data, mime, true
	}

	if c.Evt != nil && c.Evt.Message != nil {
		if data, mime, ok := extract(c.Evt.Message); ok {
			return data, mime, nil
		}
	}
	if quoted := c.GetQuotedMessage(); quoted != nil {
		if data, mime, ok := extract(quoted); ok {
			return data, mime, nil
		}
	}
	return nil, "", fmt.Errorf("no media found")
}

// IsAdmin checks if a specific JID is a group admin.
func (c *PluginContext) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsAdminRaw(c.GetSendContext(), c.Client, info, jid)
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (c *PluginContext) AmIAdmin(info *types.GroupInfo) bool {
	if c == nil || c.Client == nil || info == nil {
		return false
	}
	return IsBotAdminRaw(c.GetSendContext(), c.Client, info)
}

// Sprintf formats text according to format specifier.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// IsViewOnceMessage returns true if the message is a ViewOnce container or contains a ViewOnce media attachment.
func IsViewOnceMessage(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		return IsViewOnceMessage(msg.EphemeralMessage.Message)
	}
	if msg.ViewOnceMessage != nil || msg.ViewOnceMessageV2 != nil || msg.ViewOnceMessageV2Extension != nil {
		return true
	}
	if img := msg.GetImageMessage(); img != nil && img.GetViewOnce() {
		return true
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		return true
	}
	if aud := msg.GetAudioMessage(); aud != nil && aud.GetViewOnce() {
		return true
	}
	return false
}

// ExtractViewOnceMessage extracts and returns the inner media message from any ViewOnce wrapper.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	msg = UnwrapMessageProto(msg)
	if msg == nil {
		return nil
	}
	res := &waE2E.Message{}
	if img := msg.GetImageMessage(); img != nil {
		cloned := proto.Clone(img).(*waE2E.ImageMessage)
		cloned.ViewOnce = new(false)
		res.ImageMessage = cloned
		return res
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		cloned := proto.Clone(vid).(*waE2E.VideoMessage)
		cloned.ViewOnce = new(false)
		res.VideoMessage = cloned
		return res
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		cloned := proto.Clone(aud).(*waE2E.AudioMessage)
		cloned.ViewOnce = new(false)
		res.AudioMessage = cloned
		return res
	}
	return msg
}

// UnwrapAndSendViewOnceMessage downloads encrypted ViewOnce media, re-uploads it with fresh keys, and sends the clean unwrapped message to target JID.
func UnwrapAndSendViewOnceMessage(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message, senderJID types.JID, pushName string, targetJID types.JID, quoteID string, sourceChat ...types.JID) error {
	if msg == nil || client == nil {
		return fmt.Errorf("invalid arguments")
	}

	unwrapped := ExtractViewOnceMessage(msg)
	if unwrapped == nil {
		return fmt.Errorf("failed to extract inner ViewOnce message")
	}

	if img := unwrapped.GetImageMessage(); img != nil {
		data, err := client.Download(ctx, img)
		if err != nil {
			return fmt.Errorf("download image: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		img.URL = &uploaded.URL
		img.DirectPath = &uploaded.DirectPath
		img.MediaKey = uploaded.MediaKey
		img.FileEncSHA256 = uploaded.FileEncSHA256
		img.FileSHA256 = uploaded.FileSHA256
		img.FileLength = new(uint64(len(data)))
		img.ViewOnce = new(false)
	} else if vid := unwrapped.GetVideoMessage(); vid != nil {
		data, err := client.Download(ctx, vid)
		if err != nil {
			return fmt.Errorf("download video: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return fmt.Errorf("upload video: %w", err)
		}
		vid.URL = &uploaded.URL
		vid.DirectPath = &uploaded.DirectPath
		vid.MediaKey = uploaded.MediaKey
		vid.FileEncSHA256 = uploaded.FileEncSHA256
		vid.FileSHA256 = uploaded.FileSHA256
		vid.FileLength = new(uint64(len(data)))
		vid.ViewOnce = new(false)
	} else if aud := unwrapped.GetAudioMessage(); aud != nil {
		data, err := client.Download(ctx, aud)
		if err != nil {
			return fmt.Errorf("download audio: %w", err)
		}
		uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return fmt.Errorf("upload audio: %w", err)
		}
		aud.URL = &uploaded.URL
		aud.DirectPath = &uploaded.DirectPath
		aud.MediaKey = uploaded.MediaKey
		aud.FileEncSHA256 = uploaded.FileEncSHA256
		aud.FileSHA256 = uploaded.FileSHA256
		aud.FileLength = new(uint64(len(data)))
		aud.ViewOnce = new(false)
	}

	_, err := client.SendMessage(ctx, targetJID, unwrapped)
	return err
}

// FormatTextResponseRaw applies optional global font styles or formatting transformations.
func FormatTextResponseRaw(text string) string {
	return text
}

// EncodeProtoMessage serializes a protobuf message into a hex-encoded string.
func EncodeProtoMessage(msg proto.Message) (string, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// DecodeProtoMessage decodes a hex-encoded or base64-encoded protobuf message.
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	data, err := hex.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		var b64Err error
		data, b64Err = base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if b64Err != nil {
			return nil, fmt.Errorf("decode proto hex: %v, b64: %v", err, b64Err)
		}
	}
	msg := &waE2E.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

var _ builder.Sender = (*PluginContext)(nil)

// DispatchListSelection resolves an interactive list button response.
func DispatchListSelection(ctx any, text, displayText string) bool {
	if sender, ok := ctx.(builder.Sender); ok {
		return builder.DispatchListSelection(sender, text, displayText)
	}
	return false
}

// DispatchPollVoteEvent routes incoming poll votes to reactive action handlers.
func DispatchPollVoteEvent(ctx any, evt *events.Message) bool {
	if sender, ok := ctx.(builder.Sender); ok {
		return builder.DispatchPollVoteEvent(sender, evt)
	}
	return false
}

// ─── Convenience Subpackage Re-exports ─────────────────────────────────────

var (
	// NewMemoryStore initializes a memory cache store.
	NewMemoryStore = cache.NewMemoryStore
	// InitCache initializes global caching.
	InitCache = cache.Init
	// NewWaLogger constructs a Zap protocol logger adapter.
	NewWaLogger = logger.NewWaLogger
	// GetSystemStats retrieves host hardware metrics.
	GetSystemStats = system.GetStats
	// FormatBytes formats byte counts.
	FormatBytes = system.FormatBytes
	// AddStickerMetadata injects EXIF metadata into WebP stickers.
	AddStickerMetadata = webp.AddStickerMetadata
	// WriteStickerMetadata writes EXIF metadata to a WebP file.
	WriteStickerMetadata = webp.WriteStickerMetadata
	// EncodePNG generates a QR code PNG image.
	EncodePNG = qr.EncodePNG
	// StartQRServer starts the local QR web pairing server.
	StartQRServer = qr.StartServer
)
