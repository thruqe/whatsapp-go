package whatsrook

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/thruqe/whatsrook/util/botctx"
	"github.com/thruqe/whatsrook/util/cache"
	"github.com/thruqe/whatsrook/util/logger"
	"github.com/thruqe/whatsrook/util/message"
	"github.com/thruqe/whatsrook/util/qr"
	"github.com/thruqe/whatsrook/util/send"
	"github.com/thruqe/whatsrook/util/system"
	"github.com/thruqe/whatsrook/util/webp"
)

// ClientType defines the companion platform personality.
type ClientType int

const (
	ClientChrome ClientType = iota
	ClientAndroid
	ClientIos
)

var (
	ErrLoggedOut       = errors.New("logged out from WhatsApp")
	ErrPairingTimedOut = errors.New("pairing timed out")
)

// ParseClientType parses a string into a ClientType.
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

// Config holds client initialization parameters.
type Config struct {
	DataDir         string
	Database        string
	Session         string
	ClientType      ClientType
	Business        bool
	Verbose         bool
	AsyncMessageAck bool
}

// Client wraps whatsmeow.Client with lightweight session management.
type Client struct {
	Config Config

	mu        sync.Mutex
	rawClient *whatsmeow.Client
	container *sqlstore.Container
}

// NewClient creates a new Client instance.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir()
	}
	return &Client{
		Config: cfg,
	}
}

// WAClient returns the underlying whatsmeow.Client.
func (c *Client) WAClient() *whatsmeow.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rawClient
}

// Container returns the underlying sqlstore.Container.
func (c *Client) Container() *sqlstore.Container {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.container
}

// InitSession initializes the database container and WhatsApp client.
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
	ConfigureCompanionPlatform(c.Config.ClientType, isBusiness)

	c.rawClient = cli
	return nil
}

// ConfigureCompanionPlatform sets the companion platform registration payload.
func ConfigureCompanionPlatform(clientType ClientType, isBusiness bool) {
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

// Connect connects to the WhatsApp servers.
func (c *Client) Connect() error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized: call InitSession first")
	}

	return cli.Connect()
}

// Disconnect disconnects from WhatsApp servers and flushes sessions.
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

// WaitForConnection blocks until connected or timeout expires.
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

// SetPresence sends presence state.
func (c *Client) SetPresence(state types.Presence) error {
	c.mu.Lock()
	cli := c.rawClient
	c.mu.Unlock()

	if cli == nil {
		return fmt.Errorf("client not initialized")
	}

	return cli.SendPresence(context.Background(), state)
}

// SetOnline marks presence as available.
func (c *Client) SetOnline() error {
	return c.SetPresence(types.PresenceAvailable)
}

// Close closes the client and release database container resources.
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

// PairPhone initiates pairing using a phone number code.
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

// PairQR returns the QR code channel for authentication.
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

// DefaultDataDir returns the default local data directory for whatsrook.
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

// Re-exports for convenience
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

type (
	PluginContext      = botctx.PluginContext
	SettingGetter      = botctx.SettingGetter
	SettingSetter      = botctx.SettingSetter
	SettingDeleter     = botctx.SettingDeleter
	RecentMessageCache = message.RecentMessageCache
	AlbumMediaItem     = send.AlbumMediaItem
	Sender             = send.Sender
)

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
	Bold      = botctx.Bold
	Boldf     = botctx.Boldf
	Italic    = botctx.Italic
	Italicf   = botctx.Italicf
	Code      = botctx.Code
	Codef     = botctx.Codef
	CodeBlock = botctx.CodeBlock
	Strike    = botctx.Strike
	Strikef   = botctx.Strikef
	Quote     = botctx.Quote
	Quotef    = botctx.Quotef
	NewText   = botctx.NewText
	Sprintf   = botctx.Sprintf

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
	SendWebview              = send.Webview
	SendWebviewWithID        = send.WebviewWithID
	BuildWebviewMessage      = send.BuildWebviewMessage
	BuildWebviewPayload      = send.BuildWebviewPayload

	// Interactive component dispatch re-exports
	DispatchListSelection = botctx.DispatchListSelection
	DispatchPollVoteEvent = botctx.DispatchPollVoteEvent
)
