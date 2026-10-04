package store

import (
	"time"

	"go.mau.fi/whatsmeow/types"
)

// CallMediaKind defines the media category for automated call rejections.
type CallMediaKind string

const (
	CallMediaAudio CallMediaKind = "audio"
	CallMediaVideo CallMediaKind = "video"
)

// BotSetting stores key-value configuration flags scoped by bot account JID.
type BotSetting struct {
	OurJID string `db:"our_jid"`
	Key    string `db:"key"`
	Value  string `db:"value"`
}

// CallMediaConfig records media payload attachments dispatched when declining incoming calls.
type CallMediaConfig struct {
	OurJID    string `db:"our_jid"`
	JID       string `db:"jid"`
	Kind      string `db:"kind"`
	FilePath  string `db:"file_path"`
	UpdatedAt int64  `db:"updated_at"`
}

// BotFilter represents an auto-responder keyword trigger and its serialized message payload.
type BotFilter struct {
	OurJID       string `db:"our_jid"`
	TriggerWord  string `db:"trigger_word"`
	MessageProto string `db:"message_proto"`
}

// BotBGM represents background music trigger keywords and audio message payloads.
type BotBGM struct {
	OurJID       string `db:"our_jid"`
	TriggerWord  string `db:"trigger_word"`
	MessageProto string `db:"message_proto"`
}

// BotStickerCmd associates sticker SHA-256 hashes with predefined bot commands.
type BotStickerCmd struct {
	OurJID        string `db:"our_jid"`
	StickerSHA256 string `db:"sticker_sha256"`
	CommandName   string `db:"command_name"`
}

// GroupStats tracks daily message metrics per user within group chats.
type GroupStats struct {
	OurJID   string `db:"our_jid"`
	GroupJID string `db:"group_jid"`
	UserJID  string `db:"user_jid"`
	DateStr  string `db:"date_str"`
	MsgCount int    `db:"msg_count"`
}

// BotGroupUserXP tracks scoped leaderboard progress and competitive stats per group chat.
type BotGroupUserXP struct {
	OurJID          string `db:"our_jid"`
	GroupJID        string `db:"group_jid"`
	UserJID         string `db:"user_jid"`
	XP              int64  `db:"xp"`
	TTTWins         int    `db:"ttt_wins"`
	TTTLosses       int    `db:"ttt_losses"`
	TTTDraws        int    `db:"ttt_draws"`
	WCGWins         int    `db:"wcg_wins"`
	WCGGames        int    `db:"wcg_games"`
	WCGRating       int    `db:"wcg_rating"`
	UnscrambleWins  int    `db:"unscramble_wins"`
	UnscrambleScore int    `db:"unscramble_score"`
}

// CachedGroup holds cached metadata and configuration for a WhatsApp group chat.
type CachedGroup struct {
	OurJID                 string    `db:"our_jid"`
	JID                    string    `db:"jid"`
	Name                   string    `db:"name"`
	Topic                  string    `db:"topic"`
	TopicID                string    `db:"topic_id"`
	TopicSetAt             time.Time `db:"topic_set_at"`
	TopicSetBy             string    `db:"topic_set_by"`
	OwnerJID               string    `db:"owner_jid"`
	CreatedAt              time.Time `db:"created_at"`
	IsLocked               bool      `db:"is_locked"`
	IsAnnounce             bool      `db:"is_announce"`
	IsEphemeral            bool      `db:"is_ephemeral"`
	EphemeralDuration      uint32    `db:"ephemeral_duration"`
	MembershipApprovalMode bool      `db:"membership_approval_mode"`
	IsIncognito            bool      `db:"is_incognito"`
	IsCommunity            bool      `db:"is_community"`
	ParentJID              string    `db:"parent_jid"`
	LinkedParentJID        string    `db:"linked_parent_jid"`
	IsDefaultSubgroup      bool      `db:"is_default_subgroup"`
	IsGeneralChat          bool      `db:"is_general_chat"`
	ParticipantCount       int       `db:"participant_count"`
	AdminCount             int       `db:"admin_count"`
	UpdatedAt              time.Time `db:"updated_at"`
}

// CachedGroupParticipant records participant permissions and identity within cached groups.
type CachedGroupParticipant struct {
	OurJID       string `db:"our_jid"`
	GroupJID     string `db:"group_jid"`
	UserJID      string `db:"user_jid"`
	LID          string `db:"lid"`
	IsAdmin      bool   `db:"is_admin"`
	IsSuperAdmin bool   `db:"is_super_admin"`
	DisplayName  string `db:"display_name"`
}

// CachedNewsletter holds channel/newsletter metadata and subscription numbers.
type CachedNewsletter struct {
	OurJID           string    `db:"our_jid"`
	JID              string    `db:"jid"`
	Name             string    `db:"name"`
	Description      string    `db:"description"`
	InviteCode       string    `db:"invite_code"`
	SubscribersCount int64     `db:"subscribers_count"`
	Verification     string    `db:"verification"`
	Role             string    `db:"role"`
	MuteState        string    `db:"mute_state"`
	PictureURL       string    `db:"picture_url"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// GroupParticipantMetadata represents API participant details exchanged with group handlers.
type GroupParticipantMetadata struct {
	JID          types.JID `json:"jid"`
	LID          types.JID `json:"lid"`
	IsAdmin      bool      `json:"is_admin"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	DisplayName  string    `json:"display_name,omitempty"`
}

// GroupMetadata represents the consolidated group snapshot, including participants.
type GroupMetadata struct {
	JID                    types.JID                  `json:"jid"`
	Name                   string                     `json:"name"`
	Topic                  string                     `json:"topic"`
	TopicID                string                     `json:"topic_id,omitempty"`
	TopicSetAt             time.Time                  `json:"topic_set_at"`
	TopicSetBy             types.JID                  `json:"topic_set_by"`
	OwnerJID               types.JID                  `json:"owner_jid"`
	CreatedAt              time.Time                  `json:"created_at"`
	IsLocked               bool                       `json:"is_locked"`
	IsAnnounce             bool                       `json:"is_announce"`
	IsEphemeral            bool                       `json:"is_ephemeral"`
	EphemeralDuration      uint32                     `json:"ephemeral_duration"`
	MembershipApprovalMode bool                       `json:"membership_approval_mode"`
	IsIncognito            bool                       `json:"is_incognito"`
	IsCommunity            bool                       `json:"is_community"`
	ParentJID              types.JID                  `json:"parent_jid"`
	LinkedParentJID        types.JID                  `json:"linked_parent_jid"`
	IsDefaultSubgroup      bool                       `json:"is_default_subgroup"`
	IsGeneralChat          bool                       `json:"is_general_chat"`
	Participants           []GroupParticipantMetadata `json:"participants,omitempty"`
	ParticipantCount       int                        `json:"participant_count"`
	AdminCount             int                        `json:"admin_count"`
	UpdatedAt              time.Time                  `json:"updated_at"`
}

// NewsletterMetadata describes newsletter broadcast channel information.
type NewsletterMetadata struct {
	JID              types.JID `json:"jid"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	InviteCode       string    `json:"invite_code,omitempty"`
	SubscribersCount int64     `json:"subscribers_count"`
	Verification     string    `json:"verification,omitempty"`
	Role             string    `json:"role,omitempty"`
	MuteState        string    `json:"mute_state,omitempty"`
	PictureURL       string    `json:"picture_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BotPlatformCookie struct {
	OurJID    string    `db:"our_jid" json:"our_jid"`
	Platform  string    `db:"platform" json:"platform"`
	Domain    string    `db:"domain" json:"domain"`
	Cookies   string    `db:"cookies" json:"cookies"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (BotSetting) TableName() string             { return "bot_settings" }
func (CallMediaConfig) TableName() string        { return "call_media_config" }
func (BotFilter) TableName() string              { return "bot_filters" }
func (BotBGM) TableName() string                 { return "bot_bgm" }
func (BotStickerCmd) TableName() string          { return "bot_sticker_cmds" }
func (GroupStats) TableName() string             { return "group_stats" }
func (BotGroupUserXP) TableName() string         { return "bot_group_user_xp" }
func (CachedGroup) TableName() string            { return "cached_groups" }
func (CachedGroupParticipant) TableName() string { return "cached_group_participants" }
func (CachedNewsletter) TableName() string       { return "cached_newsletters" }
func (BotPlatformCookie) TableName() string      { return "bot_platform_cookies" }
