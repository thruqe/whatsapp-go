package store

import (
	"time"
)

// BotSetting stores custom bot configuration key-value pairs scoped to our_jid.
type BotSetting struct {
	OurJID string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	Key    string `gorm:"column:key;primaryKey;size:255;not null"`
	Value  string `gorm:"column:value;type:text;not null"`
}

func (BotSetting) TableName() string {
	return "bot_settings"
}

// CallMediaConfig stores custom audio/video media paths for VoIP call auto-reply.
type CallMediaConfig struct {
	OurJID    string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	JID       string `gorm:"column:jid;primaryKey;size:255;not null"`
	Kind      string `gorm:"column:kind;primaryKey;size:32;not null;default:'audio'"`
	FilePath  string `gorm:"column:file_path;type:text;not null"`
	UpdatedAt int64  `gorm:"column:updated_at;default:0"`
}

func (CallMediaConfig) TableName() string {
	return "call_media_config"
}

// BotFilter stores custom text/media trigger-response pairs.
type BotFilter struct {
	OurJID       string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	TriggerWord  string `gorm:"column:trigger_word;primaryKey;size:255;not null"`
	MessageProto string `gorm:"column:message_proto;type:text;not null"`
}

func (BotFilter) TableName() string {
	return "bot_filters"
}

// BotBGM stores custom audio background music triggers.
type BotBGM struct {
	OurJID       string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	TriggerWord  string `gorm:"column:trigger_word;primaryKey;size:255;not null"`
	MessageProto string `gorm:"column:message_proto;type:text;not null"`
}

func (BotBGM) TableName() string {
	return "bot_bgm"
}

// BotStickerCmd maps sticker image hashes to executable bot commands.
type BotStickerCmd struct {
	OurJID        string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	StickerSHA256 string `gorm:"column:sticker_sha256;primaryKey;size:128;not null"`
	CommandName   string `gorm:"column:command_name;size:255;not null"`
}

func (BotStickerCmd) TableName() string {
	return "bot_sticker_cmds"
}

// GroupStats tracks daily message volume per user per group.
type GroupStats struct {
	OurJID   string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	GroupJID string `gorm:"column:group_jid;primaryKey;size:255;not null"`
	UserJID  string `gorm:"column:user_jid;primaryKey;size:255;not null"`
	DateStr  string `gorm:"column:date_str;primaryKey;size:32;not null"`
	MsgCount int    `gorm:"column:msg_count;not null;default:1"`
}

func (GroupStats) TableName() string {
	return "group_stats"
}

// BotUserXP tracks global user experience, levels, and activity counters.
type BotUserXP struct {
	OurJID    string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	UserJID   string `gorm:"column:user_jid;primaryKey;size:255;not null"`
	XP        int64  `gorm:"column:xp;not null;default:0"`
	Level     int    `gorm:"column:level;not null;default:1"`
	Messages  int64  `gorm:"column:messages;default:0"`
	Stickers  int64  `gorm:"column:stickers;default:0"`
	Commands  int64  `gorm:"column:commands;default:0"`
	UpdatedAt int64  `gorm:"column:updated_at;default:0"`
	TTTWins   int    `gorm:"column:ttt_wins;default:0"`
	TTTLosses int    `gorm:"column:ttt_losses;default:0"`
	TTTDraws  int    `gorm:"column:ttt_draws;default:0"`
	WCGWins   int    `gorm:"column:wcg_wins;default:0"`
	WCGGames  int    `gorm:"column:wcg_games;default:0"`
	WCGRating int    `gorm:"column:wcg_rating;default:1000"`
}

func (BotUserXP) TableName() string {
	return "bot_user_xp"
}

// BotGroupUserXP tracks per-group user experience, game scores, and wins.
type BotGroupUserXP struct {
	OurJID          string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	GroupJID        string `gorm:"column:group_jid;primaryKey;size:255;not null"`
	UserJID         string `gorm:"column:user_jid;primaryKey;size:255;not null"`
	XP              int64  `gorm:"column:xp;not null;default:0"`
	TTTWins         int    `gorm:"column:ttt_wins;default:0"`
	TTTLosses       int    `gorm:"column:ttt_losses;default:0"`
	TTTDraws        int    `gorm:"column:ttt_draws;default:0"`
	WCGWins         int    `gorm:"column:wcg_wins;default:0"`
	WCGGames        int    `gorm:"column:wcg_games;default:0"`
	WCGRating       int    `gorm:"column:wcg_rating;default:1000"`
	UnscrambleWins  int    `gorm:"column:unscramble_wins;default:0"`
	UnscrambleScore int    `gorm:"column:unscramble_score;default:0"`
}

func (BotGroupUserXP) TableName() string {
	return "bot_group_user_xp"
}

// CachedGroup stores WhatsApp group & community metadata.
type CachedGroup struct {
	OurJID                 string    `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	JID                    string    `gorm:"column:jid;primaryKey;size:255;not null"`
	Name                   string    `gorm:"column:name;size:255;not null;default:''"`
	Topic                  string    `gorm:"column:topic;type:text"`
	TopicID                string    `gorm:"column:topic_id;size:255"`
	TopicSetAt             time.Time `gorm:"column:topic_set_at;type:timestamp"`
	TopicSetBy             string    `gorm:"column:topic_set_by;size:255"`
	OwnerJID               string    `gorm:"column:owner_jid;size:255"`
	CreatedAt              time.Time `gorm:"column:created_at;type:timestamp"`
	IsLocked               bool      `gorm:"column:is_locked"`
	IsAnnounce             bool      `gorm:"column:is_announce"`
	IsEphemeral            bool      `gorm:"column:is_ephemeral"`
	EphemeralDuration      uint32    `gorm:"column:ephemeral_duration"`
	MembershipApprovalMode bool      `gorm:"column:membership_approval_mode"`
	IsIncognito            bool      `gorm:"column:is_incognito"`
	IsCommunity            bool      `gorm:"column:is_community"`
	ParentJID              string    `gorm:"column:parent_jid;size:255"`
	LinkedParentJID        string    `gorm:"column:linked_parent_jid;size:255"`
	IsDefaultSubgroup      bool      `gorm:"column:is_default_subgroup"`
	ParticipantCount       int       `gorm:"column:participant_count"`
	AdminCount             int       `gorm:"column:admin_count"`
	UpdatedAt              time.Time `gorm:"column:updated_at;type:timestamp"`
}

func (CachedGroup) TableName() string {
	return "cached_groups"
}

// CachedGroupParticipant stores group participant roster entries.
type CachedGroupParticipant struct {
	OurJID       string `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	GroupJID     string `gorm:"column:group_jid;primaryKey;size:255;not null"`
	UserJID      string `gorm:"column:user_jid;primaryKey;size:255;not null"`
	LID          string `gorm:"column:lid;size:255"`
	IsAdmin      bool   `gorm:"column:is_admin"`
	IsSuperAdmin bool   `gorm:"column:is_super_admin"`
	DisplayName  string `gorm:"column:display_name;size:255"`
}

func (CachedGroupParticipant) TableName() string {
	return "cached_group_participants"
}

// CachedNewsletter stores WhatsApp Newsletter / Channel metadata.
type CachedNewsletter struct {
	OurJID           string    `gorm:"column:our_jid;primaryKey;size:255;not null;default:''"`
	JID              string    `gorm:"column:jid;primaryKey;size:255;not null"`
	Name             string    `gorm:"column:name;size:255;not null;default:''"`
	Description      string    `gorm:"column:description;type:text"`
	InviteCode       string    `gorm:"column:invite_code;size:255"`
	SubscribersCount int64     `gorm:"column:subscribers_count"`
	Verification     string    `gorm:"column:verification;size:64"`
	Role             string    `gorm:"column:role;size:64"`
	MuteState        string    `gorm:"column:mute_state;size:64"`
	PictureURL       string    `gorm:"column:picture_url;type:text"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamp"`
	UpdatedAt        time.Time `gorm:"column:updated_at;type:timestamp"`
}

func (CachedNewsletter) TableName() string {
	return "cached_newsletters"
}
