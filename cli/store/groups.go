package store

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/types"
)

// GroupParticipantMetadata contains details of a group participant.
type GroupParticipantMetadata struct {
	JID          types.JID `json:"jid"`
	LID          types.JID `json:"lid"`
	IsAdmin      bool      `json:"is_admin"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	DisplayName  string    `json:"display_name,omitempty"`
}

// GroupMetadata contains complete metadata for a WhatsApp group or Community.
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
	Participants           []GroupParticipantMetadata `json:"participants,omitempty"`
	ParticipantCount       int                        `json:"participant_count"`
	AdminCount             int                        `json:"admin_count"`
	UpdatedAt              time.Time                  `json:"updated_at"`
}

// NewsletterMetadata contains complete metadata for a WhatsApp Newsletter / Channel.
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

const (
	upsertCachedGroupQuery = `
		INSERT INTO cached_groups (
			our_jid, jid, name, topic, topic_id, topic_set_at, topic_set_by,
			owner_jid, created_at, is_locked, is_announce, is_ephemeral,
			ephemeral_duration, membership_approval_mode, is_incognito,
			is_community, parent_jid, linked_parent_jid, is_default_subgroup,
			participant_count, admin_count, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (our_jid, jid) DO UPDATE SET
			name = EXCLUDED.name,
			topic = EXCLUDED.topic,
			topic_id = EXCLUDED.topic_id,
			topic_set_at = EXCLUDED.topic_set_at,
			topic_set_by = EXCLUDED.topic_set_by,
			owner_jid = EXCLUDED.owner_jid,
			created_at = EXCLUDED.created_at,
			is_locked = EXCLUDED.is_locked,
			is_announce = EXCLUDED.is_announce,
			is_ephemeral = EXCLUDED.is_ephemeral,
			ephemeral_duration = EXCLUDED.ephemeral_duration,
			membership_approval_mode = EXCLUDED.membership_approval_mode,
			is_incognito = EXCLUDED.is_incognito,
			is_community = EXCLUDED.is_community,
			parent_jid = EXCLUDED.parent_jid,
			linked_parent_jid = EXCLUDED.linked_parent_jid,
			is_default_subgroup = EXCLUDED.is_default_subgroup,
			participant_count = EXCLUDED.participant_count,
			admin_count = EXCLUDED.admin_count,
			updated_at = EXCLUDED.updated_at
	`

	upsertCachedParticipantQuery = `
		INSERT INTO cached_group_participants (
			our_jid, group_jid, user_jid, lid, is_admin, is_super_admin, display_name
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (our_jid, group_jid, user_jid) DO UPDATE SET
			lid = EXCLUDED.lid,
			is_admin = EXCLUDED.is_admin,
			is_super_admin = EXCLUDED.is_super_admin,
			display_name = EXCLUDED.display_name
	`

	upsertCachedNewsletterQuery = `
		INSERT INTO cached_newsletters (
			our_jid, jid, name, description, invite_code, subscribers_count,
			verification, role, mute_state, picture_url, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (our_jid, jid) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			invite_code = EXCLUDED.invite_code,
			subscribers_count = EXCLUDED.subscribers_count,
			verification = EXCLUDED.verification,
			role = EXCLUDED.role,
			mute_state = EXCLUDED.mute_state,
			picture_url = EXCLUDED.picture_url,
			created_at = EXCLUDED.created_at,
			updated_at = EXCLUDED.updated_at
	`
)

// SaveCachedGroup persists a group's metadata into the database.
func SaveCachedGroup(ctx context.Context, db *dbutil.Database, ourJID string, g *GroupMetadata) error {
	if db == nil || g == nil {
		return nil
	}
	now := time.Now().UTC()
	g.UpdatedAt = now

	_, err := db.Exec(ctx, upsertCachedGroupQuery,
		ourJID,
		g.JID.String(),
		g.Name,
		g.Topic,
		g.TopicID,
		g.TopicSetAt,
		g.TopicSetBy.String(),
		g.OwnerJID.String(),
		g.CreatedAt,
		g.IsLocked,
		g.IsAnnounce,
		g.IsEphemeral,
		g.EphemeralDuration,
		g.MembershipApprovalMode,
		g.IsIncognito,
		g.IsCommunity,
		g.ParentJID.String(),
		g.LinkedParentJID.String(),
		g.IsDefaultSubgroup,
		g.ParticipantCount,
		g.AdminCount,
		now,
	)
	if err != nil {
		return fmt.Errorf("SaveCachedGroup failed for %s: %w", g.JID.String(), err)
	}

	if len(g.Participants) > 0 {
		_ = SaveCachedGroupParticipants(ctx, db, ourJID, g.JID.String(), g.Participants)
	}

	return nil
}

// SaveCachedGroupParticipants replaces all participant records for a group.
func SaveCachedGroupParticipants(ctx context.Context, db *dbutil.Database, ourJID, groupJID string, participants []GroupParticipantMetadata) error {
	if db == nil {
		return nil
	}

	// Delete obsolete participant records for this group
	_, _ = db.Exec(ctx, "DELETE FROM cached_group_participants WHERE our_jid = $1 AND group_jid = $2", ourJID, groupJID)

	for _, p := range participants {
		_, err := db.Exec(ctx, upsertCachedParticipantQuery,
			ourJID,
			groupJID,
			p.JID.String(),
			p.LID.String(),
			p.IsAdmin,
			p.IsSuperAdmin,
			p.DisplayName,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveCachedNewsletter persists a newsletter/channel metadata into the database.
func SaveCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID string, n *NewsletterMetadata) error {
	if db == nil || n == nil {
		return nil
	}
	now := time.Now().UTC()
	n.UpdatedAt = now

	_, err := db.Exec(ctx, upsertCachedNewsletterQuery,
		ourJID,
		n.JID.String(),
		n.Name,
		n.Description,
		n.InviteCode,
		n.SubscribersCount,
		n.Verification,
		n.Role,
		n.MuteState,
		n.PictureURL,
		n.CreatedAt,
		now,
	)
	return err
}

// DeleteCachedGroup removes a group from the cached tables.
func DeleteCachedGroup(ctx context.Context, db *dbutil.Database, ourJID, groupJID string) error {
	if db == nil {
		return nil
	}
	_, _ = db.Exec(ctx, "DELETE FROM cached_group_participants WHERE our_jid = $1 AND group_jid = $2", ourJID, groupJID)
	_, err := db.Exec(ctx, "DELETE FROM cached_groups WHERE our_jid = $1 AND jid = $2", ourJID, groupJID)
	return err
}

// DeleteCachedNewsletter removes a newsletter from the cached tables.
func DeleteCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID, newsletterJID string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(ctx, "DELETE FROM cached_newsletters WHERE our_jid = $1 AND jid = $2", ourJID, newsletterJID)
	return err
}

// LoadAllCachedGroups loads all cached groups and their participants from the database.
func LoadAllCachedGroups(ctx context.Context, db *dbutil.Database, ourJID string) ([]*GroupMetadata, error) {
	if db == nil {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT jid, name, topic, topic_id, topic_set_at, topic_set_by,
		       owner_jid, created_at, is_locked, is_announce, is_ephemeral,
		       ephemeral_duration, membership_approval_mode, is_incognito,
		       is_community, parent_jid, linked_parent_jid, is_default_subgroup,
		       participant_count, admin_count, updated_at
		FROM cached_groups
		WHERE our_jid = $1
		ORDER BY name ASC
	`, ourJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*GroupMetadata
	for rows.Next() {
		var g GroupMetadata
		var jidStr, topicSetByStr, ownerStr, parentStr, linkedParentStr string
		var topicSetAt, createdAt, updatedAt *time.Time

		err := rows.Scan(
			&jidStr, &g.Name, &g.Topic, &g.TopicID, &topicSetAt, &topicSetByStr,
			&ownerStr, &createdAt, &g.IsLocked, &g.IsAnnounce, &g.IsEphemeral,
			&g.EphemeralDuration, &g.MembershipApprovalMode, &g.IsIncognito,
			&g.IsCommunity, &parentStr, &linkedParentStr, &g.IsDefaultSubgroup,
			&g.ParticipantCount, &g.AdminCount, &updatedAt,
		)
		if err != nil {
			continue
		}

		g.JID, _ = types.ParseJID(jidStr)
		g.TopicSetBy, _ = types.ParseJID(topicSetByStr)
		g.OwnerJID, _ = types.ParseJID(ownerStr)
		g.ParentJID, _ = types.ParseJID(parentStr)
		g.LinkedParentJID, _ = types.ParseJID(linkedParentStr)
		if topicSetAt != nil {
			g.TopicSetAt = *topicSetAt
		}
		if createdAt != nil {
			g.CreatedAt = *createdAt
		}
		if updatedAt != nil {
			g.UpdatedAt = *updatedAt
		}

		groups = append(groups, &g)
	}

	// Load participants for each group
	for _, g := range groups {
		pRows, errP := db.Query(ctx, `
			SELECT user_jid, lid, is_admin, is_super_admin, display_name
			FROM cached_group_participants
			WHERE our_jid = $1 AND group_jid = $2
		`, ourJID, g.JID.String())
		if errP != nil {
			continue
		}
		for pRows.Next() {
			var p GroupParticipantMetadata
			var userStr, lidStr string
			if errS := pRows.Scan(&userStr, &lidStr, &p.IsAdmin, &p.IsSuperAdmin, &p.DisplayName); errS == nil {
				p.JID, _ = types.ParseJID(userStr)
				p.LID, _ = types.ParseJID(lidStr)
				g.Participants = append(g.Participants, p)
			}
		}
		pRows.Close()
		g.ParticipantCount = len(g.Participants)
	}

	return groups, nil
}

// LoadAllCachedNewsletters loads all cached newsletters from the database.
func LoadAllCachedNewsletters(ctx context.Context, db *dbutil.Database, ourJID string) ([]*NewsletterMetadata, error) {
	if db == nil {
		return nil, nil
	}

	rows, err := db.Query(ctx, `
		SELECT jid, name, description, invite_code, subscribers_count,
		       verification, role, mute_state, picture_url, created_at, updated_at
		FROM cached_newsletters
		WHERE our_jid = $1
		ORDER BY name ASC
	`, ourJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var newsletters []*NewsletterMetadata
	for rows.Next() {
		var n NewsletterMetadata
		var jidStr string
		var createdAt, updatedAt *time.Time

		err := rows.Scan(
			&jidStr, &n.Name, &n.Description, &n.InviteCode, &n.SubscribersCount,
			&n.Verification, &n.Role, &n.MuteState, &n.PictureURL, &createdAt, &updatedAt,
		)
		if err != nil {
			continue
		}

		n.JID, _ = types.ParseJID(jidStr)
		if createdAt != nil {
			n.CreatedAt = *createdAt
		}
		if updatedAt != nil {
			n.UpdatedAt = *updatedAt
		}

		newsletters = append(newsletters, &n)
	}

	return newsletters, nil
}
