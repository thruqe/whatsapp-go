package store

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm/clause"
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

// SaveCachedGroup persists a group's metadata into the database via GORM.
func SaveCachedGroup(ctx context.Context, db *dbutil.Database, ourJID string, g *GroupMetadata) error {
	if db == nil || g == nil {
		return nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	g.UpdatedAt = now

	cg := CachedGroup{
		OurJID:                 ourJID,
		JID:                    g.JID.String(),
		Name:                   g.Name,
		Topic:                  g.Topic,
		TopicID:                g.TopicID,
		TopicSetAt:             g.TopicSetAt,
		TopicSetBy:             g.TopicSetBy.String(),
		OwnerJID:               g.OwnerJID.String(),
		CreatedAt:              g.CreatedAt,
		IsLocked:               g.IsLocked,
		IsAnnounce:             g.IsAnnounce,
		IsEphemeral:            g.IsEphemeral,
		EphemeralDuration:      g.EphemeralDuration,
		MembershipApprovalMode: g.MembershipApprovalMode,
		IsIncognito:            g.IsIncognito,
		IsCommunity:            g.IsCommunity,
		ParentJID:              g.ParentJID.String(),
		LinkedParentJID:        g.LinkedParentJID.String(),
		IsDefaultSubgroup:      g.IsDefaultSubgroup,
		ParticipantCount:       g.ParticipantCount,
		AdminCount:             g.AdminCount,
		UpdatedAt:              now,
	}

	if err := gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "jid"}},
		UpdateAll: true,
	}).Create(&cg).Error; err != nil {
		return fmt.Errorf("SaveCachedGroup failed for %s: %w", g.JID.String(), err)
	}

	if len(g.Participants) > 0 {
		_ = SaveCachedGroupParticipants(ctx, db, ourJID, g.JID.String(), g.Participants)
	}
	return nil
}

// SaveCachedGroupParticipants replaces all participant records for a group via GORM.
func SaveCachedGroupParticipants(ctx context.Context, db *dbutil.Database, ourJID, groupJID string, participants []GroupParticipantMetadata) error {
	if db == nil {
		return nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return err
	}

	// Delete obsolete participant records for this group
	_ = gdb.WithContext(ctx).
		Where("our_jid = ? AND group_jid = ?", ourJID, groupJID).
		Delete(&CachedGroupParticipant{}).Error

	if len(participants) == 0 {
		return nil
	}

	var records []CachedGroupParticipant
	for _, p := range participants {
		records = append(records, CachedGroupParticipant{
			OurJID:       ourJID,
			GroupJID:     groupJID,
			UserJID:      p.JID.String(),
			LID:          p.LID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		})
	}

	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "group_jid"}, {Name: "user_jid"}},
		UpdateAll: true,
	}).Create(&records).Error
}

// SaveCachedNewsletter persists a newsletter/channel metadata into the database via GORM.
func SaveCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID string, n *NewsletterMetadata) error {
	if db == nil || n == nil {
		return nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	n.UpdatedAt = now

	cn := CachedNewsletter{
		OurJID:           ourJID,
		JID:              n.JID.String(),
		Name:             n.Name,
		Description:      n.Description,
		InviteCode:       n.InviteCode,
		SubscribersCount: n.SubscribersCount,
		Verification:     n.Verification,
		Role:             n.Role,
		MuteState:        n.MuteState,
		PictureURL:       n.PictureURL,
		CreatedAt:        n.CreatedAt,
		UpdatedAt:        now,
	}

	return gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "our_jid"}, {Name: "jid"}},
		UpdateAll: true,
	}).Create(&cn).Error
}

// DeleteCachedGroup removes a group and its participants from cached tables.
func DeleteCachedGroup(ctx context.Context, db *dbutil.Database, ourJID, groupJID string) error {
	if db == nil {
		return nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return err
	}

	_ = gdb.WithContext(ctx).
		Where("our_jid = ? AND group_jid = ?", ourJID, groupJID).
		Delete(&CachedGroupParticipant{}).Error

	return gdb.WithContext(ctx).
		Where("our_jid = ? AND jid = ?", ourJID, groupJID).
		Delete(&CachedGroup{}).Error
}

// DeleteCachedNewsletter removes a newsletter from cached tables.
func DeleteCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID, newsletterJID string) error {
	if db == nil {
		return nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return err
	}

	return gdb.WithContext(ctx).
		Where("our_jid = ? AND jid = ?", ourJID, newsletterJID).
		Delete(&CachedNewsletter{}).Error
}

// LoadAllCachedGroups loads all cached groups and their participants from the database via GORM.
func LoadAllCachedGroups(ctx context.Context, db *dbutil.Database, ourJID string) ([]*GroupMetadata, error) {
	if db == nil {
		return nil, nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return nil, err
	}

	var cgs []CachedGroup
	if err := gdb.WithContext(ctx).
		Where("our_jid = ?", ourJID).
		Order("name ASC").
		Find(&cgs).Error; err != nil {
		return nil, err
	}

	var allParticipants []CachedGroupParticipant
	_ = gdb.WithContext(ctx).
		Where("our_jid = ?", ourJID).
		Find(&allParticipants).Error

	partMap := make(map[string][]GroupParticipantMetadata)
	for _, p := range allParticipants {
		uJID, _ := types.ParseJID(p.UserJID)
		lJID, _ := types.ParseJID(p.LID)
		partMap[p.GroupJID] = append(partMap[p.GroupJID], GroupParticipantMetadata{
			JID:          uJID,
			LID:          lJID,
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		})
	}

	var groups []*GroupMetadata
	for _, cg := range cgs {
		gJID, _ := types.ParseJID(cg.JID)
		topicBy, _ := types.ParseJID(cg.TopicSetBy)
		ownerJID, _ := types.ParseJID(cg.OwnerJID)
		parentJID, _ := types.ParseJID(cg.ParentJID)
		linkedParentJID, _ := types.ParseJID(cg.LinkedParentJID)

		parts := partMap[cg.JID]
		pCount := cg.ParticipantCount
		if len(parts) > 0 {
			pCount = len(parts)
		}

		groups = append(groups, &GroupMetadata{
			JID:                    gJID,
			Name:                   cg.Name,
			Topic:                  cg.Topic,
			TopicID:                cg.TopicID,
			TopicSetAt:             cg.TopicSetAt,
			TopicSetBy:             topicBy,
			OwnerJID:               ownerJID,
			CreatedAt:              cg.CreatedAt,
			IsLocked:               cg.IsLocked,
			IsAnnounce:             cg.IsAnnounce,
			IsEphemeral:            cg.IsEphemeral,
			EphemeralDuration:      cg.EphemeralDuration,
			MembershipApprovalMode: cg.MembershipApprovalMode,
			IsIncognito:            cg.IsIncognito,
			IsCommunity:            cg.IsCommunity,
			ParentJID:              parentJID,
			LinkedParentJID:        linkedParentJID,
			IsDefaultSubgroup:      cg.IsDefaultSubgroup,
			Participants:           parts,
			ParticipantCount:       pCount,
			AdminCount:             cg.AdminCount,
			UpdatedAt:              cg.UpdatedAt,
		})
	}

	return groups, nil
}

// LoadAllCachedNewsletters loads all cached newsletters from the database via GORM.
func LoadAllCachedNewsletters(ctx context.Context, db *dbutil.Database, ourJID string) ([]*NewsletterMetadata, error) {
	if db == nil {
		return nil, nil
	}
	gdb, err := GetORMFromDB(ctx, db)
	if err != nil {
		return nil, err
	}

	var cns []CachedNewsletter
	if err := gdb.WithContext(ctx).
		Where("our_jid = ?", ourJID).
		Order("name ASC").
		Find(&cns).Error; err != nil {
		return nil, err
	}

	var newsletters []*NewsletterMetadata
	for _, cn := range cns {
		nJID, _ := types.ParseJID(cn.JID)
		newsletters = append(newsletters, &NewsletterMetadata{
			JID:              nJID,
			Name:             cn.Name,
			Description:      cn.Description,
			InviteCode:       cn.InviteCode,
			SubscribersCount: cn.SubscribersCount,
			Verification:     cn.Verification,
			Role:             cn.Role,
			MuteState:        cn.MuteState,
			PictureURL:       cn.PictureURL,
			CreatedAt:        cn.CreatedAt,
			UpdatedAt:        cn.UpdatedAt,
		})
	}

	return newsletters, nil
}
