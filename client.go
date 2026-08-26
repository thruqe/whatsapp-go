// package whatsrook implements the core session manager, database coordinator, and lifecycle bridge
// for the whatsmeow whatsapp protocol engine.
//
// architectural mechanics:
// this is the central orchestrator coordinating multi-backend persistent storage (shared sqlite / postgresql),
// device registration identity resolution, companion hardware profile emulation (chrome, android, ios),
// and event-driven message dispatching. it encapsulates raw connection primitives inside a thread-safe
// client abstraction, providing structured fallback strategies and integrated caching layers.
package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"whatsrook/src"
	Logger "whatsrook/src/logger"

	"whatsrook/src/cache"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// clienttype specifies the companion operating system and hardware profile to emulate during registration.
type ClientType int

var (
	// errloggedout is the sentinel error indicating that the active session has been explicitly
	// terminated by the remote whatsapp server or unlinked by the primary device.
	ErrLoggedOut = errors.New("logged out from WhatsApp")
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
		"chrome":  ClientChrome,
		"android": ClientAndroid,
		"ios":     ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// config defines the operational parameters, storage directories, and runtime flags for a client instance.
type Config struct {
	// session holds the primary identifier (e.g., phone number or session token) for the device.
	Session string

	// datadir specifies the base filesystem path for logs and local database storage.
	DataDir string

	// database defines the connection uri or storage driver selector (e.g., "sqlite", postgres connection string).
	Database string

	// clienttype defines the companion device platform signature emulated during pairing.
	ClientType ClientType

	// verbose toggles debug-level tracing across whatsmeow protocol logs and internal drivers.
	Verbose bool

	// skipoldmessages instructs the client to ignore backlog history during initial handshake synchronization.
	SkipOldMessages bool

	// asyncmessageack enables non-blocking message dispatch where write operations return immediately
	// upon socket flush without blocking on server-side message receipt acknowledgments.
	AsyncMessageAck bool
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
	c := &Client{Config: cfg}
	c.applyDefaults()
	return c
}

// defaultdatadir resolves the working directory adjacent to the executing binary.
// this handles execution under testing environments or `go run` by falling back to the current working directory.
func DefaultDataDir() string {
	if exePath, err := os.Executable(); err == nil {
		if !strings.Contains(exePath, "go-build") && !strings.Contains(exePath, "/tmp/") && !strings.Contains(exePath, `\Temp\`) {
			return filepath.Dir(exePath)
		}
	}
	return "."
}

// defaultauthdir is a legacy delegation wrapper maintained strictly for backward compatibility.
func DefaultAuthDir() string { return DefaultDataDir() }

// applydefaults verifies and fills missing filesystem paths in the client configuration.
func (c *Client) applyDefaults() {
	if c.Config.DataDir == "" {
		c.Config.DataDir = DefaultDataDir()
	}
}

// waclient returns the underlying whatsmeow client instance under a mutex read lock.
// this prevents data races if the underlying client is reassigned or cleared during runtime reconnects.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// container returns the shared sqlstore container under mutex synchronization.
func (c *Client) Container() *sqlstore.Container {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.container
}

// initsession establishes the persistent database container, configures logging subsystems,
// resolves or creates device credentials, and bootstraps the core whatsmeow protocol client.
//
// initialization flow:
// 1. configuration validation and logging hierarchy setup.
// 2. database initialization (postgresql with automatic ssl fallback, or wal-mode sqlite).
// 3. identity resolution via getorcreatedevice matching the session phone number.
// 4. external cache injection (attaching the default cache layer to reduce database lookup pressure).
// 5. companion platform payload construction (emulating chrome, android, or ios user-agent headers).
// todo: implement structured health-check probes prior to completing initialization.
func (c *Client) InitSession(ctx context.Context) error {
	c.applyDefaults()

	if c.Config.Session == "" {
		return errors.New("session phone number is required")
	}

	// initialize centralized logger writing to datadir/logs
	if err := src.InitLogger(c.Config.DataDir, c.Config.Verbose); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	waLevel := "INFO"
	if c.Config.Verbose {
		waLevel = "DEBUG"
	}

	// resolve shared sqlite database path
	dbPath := filepath.Join(c.Config.DataDir, "whatsrook.db")

	container, err := c.initStore(ctx, dbPath, waLevel)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	// retrieve existing cryptographic keys or allocate a clean unpaired device container
	deviceStore, err := c.getOrCreateDevice(ctx, container)
	if err != nil {
		_ = container.Close()
		return fmt.Errorf("failed to get device: %w", err)
	}
	deviceStore.ExternalCache = cache.Default()

	clientLog := src.WhatsmeowStyle("Client", "INFO", true)
	rawClient := whatsmeow.NewClient(deviceStore, clientLog)
	rawClient.SetCallLogger(src.ZerologStyle("wacaller"))
	rawClient.AsyncMessageAck = c.Config.AsyncMessageAck

	// configure companion platform registration headers and os version payloads
	switch c.Config.ClientType {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("16")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_ANDROID.Enum()
		store.BaseClientPayload.UserAgent.OsVersion = new("16.0.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("16.0.0")
		store.BaseClientPayload.WebInfo = nil
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("18.0")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_IOS.Enum()
		store.BaseClientPayload.UserAgent.OsVersion = new("18.0")
		store.BaseClientPayload.UserAgent.OsBuildNumber = new("18.0")
		store.BaseClientPayload.WebInfo = nil
	default:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		store.DeviceProps.Os = new("Linux")
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
	}

	c.mu.Lock()
	c.container = container
	c.rawClient = rawClient
	c.mu.Unlock()

	return nil
}

// getorcreatedevice scans the sql container for an existing device record matching the session identifier.
// this strips leading plus prefixes to match jid user parts directly.
// if no matching record exists, this creates and returns a new unpaired device.
func (c *Client) getOrCreateDevice(ctx context.Context, container *sqlstore.Container) (*store.Device, error) {
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	phone := c.Config.Session
	// strip leading '+' so prefix comparison aligns with stored jid user parts (e.g., "447911123456.0")
	phone = strings.TrimPrefix(phone, "+")

	for _, dev := range devices {
		if dev.ID != nil && strings.HasPrefix(dev.ID.User, phone) {
			return dev, nil
		}
	}

	// no matching credentials found; allocate a new container for qr/pairing code flows
	return container.NewDevice(), nil
}

// close cleanly closes the underlying sql store container and resets internal pointers.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.container != nil {
		err := c.container.Close()
		c.container = nil
		return err
	}
	return nil
}

// addeventhandler registers an event listener callback with the active whatsmeow client.
func (c *Client) AddEventHandler(handler func(evt any)) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli != nil {
		cli.AddEventHandler(handler)
	}
}

// connect initiates the websocket connection handshake with the whatsapp infrastructure.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return errors.New("client not initialized")
	}
	return cli.Connect()
}

// disconnect terminates the active websocket transport connection cleanly without unlinking the session.
func (c *Client) Disconnect() {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli != nil {
		cli.Disconnect()
	}
}

// logout revokes the companion pairing token on whatsapp servers and invalidates the session keys.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()
	if cli == nil {
		return errors.New("client not initialized")
	}
	return cli.Logout(ctx)
}

// wipesession is a deprecated no-op retained for API backward compatibility.
func WipeSession(_ string) {}

// clearsessiondb deletes the specific device record associated with this session from the shared database,
// leaving all other session records untouched, and closes the client container.
func (c *Client) ClearSessionDB(ctx context.Context, _ string) {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli != nil && cli.Store != nil {
		if err := cli.Store.Delete(ctx); err != nil {
			Logger.Warn("failed to delete session device store", "err", err)
		}
	}

	_ = c.Close()
}

// sanitizedburl redacts database password credentials from connection strings prior to log emission.
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

// ensuressldisabled injects or overrides sslmode=disable on postgresql connection strings during fallback attempts.
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

// initstore resolves database configuration precedence (explicit config -> per-session env -> global env -> sqlite)
// and handles automatic connection fallbacks between postgresql and high-performance sqlite instances.
//
// database resilience strategy:
//  1. attempt postgresql connection using provided credentials.
//  2. if connection fails with ssl requirements, retry with sslmode=disable.
//  3. if postgresql is entirely unreachable, gracefully fall back to local sqlite.
//  4. sqlite is initialized with explicit pragma directives (wal mode, normal sync, busy timeout, cache sizing)
//     to optimize concurrent read/write throughput and prevent database locking panics.
//
// todo: consider adding connection pool tuning parameters (max open/idle connections) to config.
func (c *Client) initStore(ctx context.Context, dbPath, waLevel string) (*sqlstore.Container, error) {
	dbLog := src.WhatsmeowStyle("Database", waLevel, true)

	// resolve database connection uri with hierarchical environment variable overrides
	dbConn := c.Config.Database
	if dbConn == "" {
		phone := strings.TrimPrefix(c.Config.Session, "+")
		if phone != "" {
			dbConn = os.Getenv("DATABASE_URL_" + phone)
		}
	}
	if dbConn == "" {
		dbConn = os.Getenv("DATABASE_URL")
	}
	if dbConn == "" {
		dbConn = "sqlite"
	}

	// postgresql initialization path
	if dbConn != "sqlite" && dbConn != "none" && (strings.HasPrefix(dbConn, "postgres://") || strings.HasPrefix(dbConn, "postgresql://")) {
		Logger.Info("attempting connection to PostgreSQL database...", "url", sanitizeDBURL(dbConn))
		container, err := sqlstore.New(ctx, "postgres", dbConn, dbLog)
		if err == nil && container != nil {
			Logger.Info("successfully connected to PostgreSQL database")
			return container, nil
		}

		// ssl fallback retry logic
		if !strings.HasSuffix(dbConn, "?sslmode=disable") {
			disableURL := ensureSSLDisabled(dbConn)
			Logger.Warn("PostgreSQL SSL connection failed, attempting reconnection with sslmode=disable...", "err", err, "url", sanitizeDBURL(disableURL))
			container, errDisable := sqlstore.New(ctx, "postgres", disableURL, dbLog)
			if errDisable == nil && container != nil {
				Logger.Info("successfully connected to PostgreSQL database with sslmode=disable")
				return container, nil
			}
			Logger.Warn("PostgreSQL connection failed with sslmode=disable", "err", errDisable)
		} else {
			Logger.Warn("PostgreSQL connection failed", "err", err)
		}
		Logger.Warn("falling back to SQLite after PostgreSQL connection failure")
	}

	// sqlite fallback / primary storage path with optimized performance pragmas
	Logger.Info("initializing SQLite database store", "path", dbPath)
	sqliteURI := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout=5000&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL&_pragma=foreign_keys=on&_pragma=cache_size=-2000",
		dbPath,
	)
	container, err := sqlstore.New(ctx, "sqlite", sqliteURI, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	return container, nil
}

// getjoinedgroups retrieves all group chats currently joined by the active account.
func (c *Client) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetJoinedGroups(ctx)
}

// getsubscribednewsletters retrieves all channels and newsletters subscribed to by the active account.
func (c *Client) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetSubscribedNewsletters(ctx)
}

// getgroupinfo fetches full metadata, settings, and participant lists for a specific group jid.
func (c *Client) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetGroupInfo(ctx, jid)
}

// getnewsletterinfo fetches complete metadata and subscriber details for a specific newsletter jid.
func (c *Client) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	wa := c.WAClient()
	if wa == nil {
		return nil, errors.New("client not initialized")
	}
	return wa.GetNewsletterInfo(ctx, jid)
}
