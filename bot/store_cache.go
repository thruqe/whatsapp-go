package store

import (
	"context"
	"fmt"
	"time"

	"github.com/thruqe/duosql"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/types"
)

// SaveCachedGroup serializes group metadata and its roster of participants into the cache store.
func SaveCachedGroup(ctx context.Context, db *dbutil.Database, ourJID string, g *GroupMetadata) error {
	if db == nil || g == nil {
		return nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return err
	}

	g.UpdatedAt = time.Now().UTC()

	cg := &CachedGroup{
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
		IsGeneralChat:          g.IsGeneralChat,
		ParticipantCount:       g.ParticipantCount,
		AdminCount:             g.AdminCount,
	}

	_, err = duosql.Insert[CachedGroup](duo).
		Values(cg).
		OnConflictDoUpdateAll("our_jid", "jid").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("SaveCachedGroup failed for %s: %w", g.JID.String(), err)
	}

	if len(g.Participants) > 0 {
		_ = SaveCachedGroupParticipants(ctx, db, ourJID, g.JID.String(), g.Participants)
	}
	return nil
}

// SaveCachedGroupParticipants replaces existing participant state for a group with fresh roster records.
func SaveCachedGroupParticipants(ctx context.Context, db *dbutil.Database, ourJID, groupJID string, participants []GroupParticipantMetadata) error {
	if db == nil {
		return nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return err
	}

	_, _ = duosql.Delete[CachedGroupParticipant](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("group_jid", groupJID),
		).
		Exec(ctx)

	if len(participants) == 0 {
		return nil
	}

	seenUsers := make(map[string]bool, len(participants))
	var validRows []*CachedGroupParticipant
	for _, p := range participants {
		uStr := p.JID.String()
		if uStr == "" || seenUsers[uStr] {
			continue
		}
		seenUsers[uStr] = true
		validRows = append(validRows, &CachedGroupParticipant{
			OurJID:       ourJID,
			GroupJID:     groupJID,
			UserJID:      uStr,
			LID:          p.LID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		})
	}

	const chunkSize = 50
	for i := 0; i < len(validRows); i += chunkSize {
		end := min(i+chunkSize, len(validRows))
		chunk := validRows[i:end]

		_, _ = duosql.Insert[CachedGroupParticipant](duo).
			Values(chunk...).
			OnConflictDoUpdateAll("our_jid", "group_jid", "user_jid").
			Exec(ctx)
	}
	return nil
}

// SaveCachedNewsletter upserts a broadcast newsletter's metadata and channel status.
func SaveCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID string, n *NewsletterMetadata) error {
	if db == nil || n == nil {
		return nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return err
	}

	n.UpdatedAt = time.Now().UTC()

	cn := &CachedNewsletter{
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
	}

	_, err = duosql.Insert[CachedNewsletter](duo).
		Values(cn).
		OnConflictDoUpdateAll("our_jid", "jid").
		Exec(ctx)
	return err
}

// DeleteCachedGroup purges a group and all its participant cache rows.
func DeleteCachedGroup(ctx context.Context, db *dbutil.Database, ourJID, groupJID string) error {
	if db == nil {
		return nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return err
	}

	_, _ = duosql.Delete[CachedGroupParticipant](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("group_jid", groupJID),
		).
		Exec(ctx)

	_, err = duosql.Delete[CachedGroup](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("jid", groupJID),
		).
		Exec(ctx)
	return err
}

// DeleteCachedNewsletter clears a channel entry from the cache.
func DeleteCachedNewsletter(ctx context.Context, db *dbutil.Database, ourJID, newsletterJID string) error {
	if db == nil {
		return nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return err
	}

	_, err = duosql.Delete[CachedNewsletter](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("jid", newsletterJID),
		).
		Exec(ctx)
	return err
}

// LoadAllCachedGroups loads all cached group snapshots and their participant rosters.
func LoadAllCachedGroups(ctx context.Context, db *dbutil.Database, ourJID string) ([]*GroupMetadata, error) {
	if db == nil {
		return nil, nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return nil, err
	}

	normJID := ourJID
	if parsed, parseErr := types.ParseJID(ourJID); parseErr == nil && !parsed.IsEmpty() {
		normJID = parsed.ToNonAD().String()
	}

	partRows, err := duosql.Select[CachedGroupParticipant](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", normJID),
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	partMap := make(map[string][]GroupParticipantMetadata, len(partRows))
	for _, p := range partRows {
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

	cachedGroups, err := duosql.Select[CachedGroup](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", normJID),
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		OrderBy(duosql.Asc("name")).
		All(ctx)
	if err != nil {
		return nil, err
	}

	groups := make([]*GroupMetadata, 0, len(cachedGroups))
	for _, cg := range cachedGroups {
		gJID, _ := types.ParseJID(cg.JID)
		topicBy, _ := types.ParseJID(cg.TopicSetBy)
		oJID, _ := types.ParseJID(cg.OwnerJID)
		pJID, _ := types.ParseJID(cg.ParentJID)
		lpJID, _ := types.ParseJID(cg.LinkedParentJID)

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
			OwnerJID:               oJID,
			CreatedAt:              cg.CreatedAt,
			IsLocked:               cg.IsLocked,
			IsAnnounce:             cg.IsAnnounce,
			IsEphemeral:            cg.IsEphemeral,
			EphemeralDuration:      cg.EphemeralDuration,
			MembershipApprovalMode: cg.MembershipApprovalMode,
			IsIncognito:            cg.IsIncognito,
			IsCommunity:            cg.IsCommunity,
			ParentJID:              pJID,
			LinkedParentJID:        lpJID,
			IsDefaultSubgroup:      cg.IsDefaultSubgroup,
			IsGeneralChat:          cg.IsGeneralChat,
			Participants:           parts,
			ParticipantCount:       pCount,
			AdminCount:             cg.AdminCount,
			UpdatedAt:              cg.UpdatedAt,
		})
	}

	return groups, nil
}

// LoadAllCachedNewsletters returns all cached newsletter channel metadata.
func LoadAllCachedNewsletters(ctx context.Context, db *dbutil.Database, ourJID string) ([]*NewsletterMetadata, error) {
	if db == nil {
		return nil, nil
	}
	duo, err := getDuoDBFromDB(db)
	if err != nil {
		return nil, err
	}

	normJID := ourJID
	if parsed, parseErr := types.ParseJID(ourJID); parseErr == nil && !parsed.IsEmpty() {
		normJID = parsed.ToNonAD().String()
	}

	rows, err := duosql.Select[CachedNewsletter](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", normJID),
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		OrderBy(duosql.Asc("name")).
		All(ctx)
	if err != nil {
		return nil, err
	}

	newsletters := make([]*NewsletterMetadata, 0, len(rows))
	for _, cn := range rows {
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
