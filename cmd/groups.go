package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"whatsrook/cmd/store"
	"whatsrook/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// GroupManager provides a thread-safe registry, cache, and sync coordinator for WhatsApp groups, communities, and newsletters.
type GroupManager struct {
	mu          sync.RWMutex
	groups      map[types.JID]*store.GroupMetadata
	newsletters map[types.JID]*store.NewsletterMetadata

	syncing sync.Mutex
}

// NewGroupManager creates a new GroupManager.
func NewGroupManager() *GroupManager {
	return &GroupManager{
		groups:      make(map[types.JID]*store.GroupMetadata),
		newsletters: make(map[types.JID]*store.NewsletterMetadata),
	}
}

// LoadFromDB loads previously cached groups and newsletters from the database.
func (gm *GroupManager) LoadFromDB(ctx context.Context, cli *whatsmeow.Client) error {
	if cli == nil || cli.Store == nil || cli.Store.ID == nil {
		return nil
	}
	s, ok := cli.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return nil
	}
	db := s.GetDB()
	if db == nil {
		return nil
	}

	ourJID := cli.Store.ID.ToNonAD().String()

	groups, err := store.LoadAllCachedGroups(ctx, db, ourJID)
	if err != nil {
		logger.Warn("GroupManager: failed to load cached groups from DB", "err", err)
	} else {
		gm.mu.Lock()
		for _, g := range groups {
			gm.groups[g.JID] = g
		}
		gm.mu.Unlock()
		logger.Debug("GroupManager: loaded cached groups from DB", "count", len(groups))
	}

	newsletters, errN := store.LoadAllCachedNewsletters(ctx, db, ourJID)
	if errN != nil {
		logger.Warn("GroupManager: failed to load cached newsletters from DB", "err", errN)
	} else {
		gm.mu.Lock()
		for _, n := range newsletters {
			gm.newsletters[n.JID] = n
		}
		gm.mu.Unlock()
		logger.Debug("GroupManager: loaded cached newsletters from DB", "count", len(newsletters))
	}

	return nil
}

// SyncAll queries all joined groups and subscribed newsletters from WhatsApp servers and caches their full metadata.
func (gm *GroupManager) SyncAll(ctx context.Context, cli *whatsmeow.Client) error {
	if cli == nil || !cli.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	gm.syncing.Lock()
	defer gm.syncing.Unlock()

	logger.Info("GroupManager: starting full sync of groups, communities, and newsletters...")

	ourJID := ""
	var s *sqlstore.SQLStore
	if cli.Store != nil && cli.Store.ID != nil {
		ourJID = cli.Store.ID.ToNonAD().String()
		if storeObj, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok {
			s = storeObj
		}
	}

	// 1. Fetch joined groups and communities
	var syncedGroups []*store.GroupMetadata
	joinedGroups, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		logger.Error("GroupManager: failed to fetch joined groups", "err", err)
	} else {
		for _, g := range joinedGroups {
			meta := gm.convertGroupInfo(ctx, cli, g)
			syncedGroups = append(syncedGroups, meta)

			if s != nil && s.GetDB() != nil {
				_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, meta)
			}
		}

		gm.mu.Lock()
		for _, meta := range syncedGroups {
			gm.groups[meta.JID] = meta
		}
		gm.mu.Unlock()
	}

	// 2. Fetch subscribed newsletters (channels)
	var syncedNewsletters []*store.NewsletterMetadata
	newsletters, errN := cli.GetSubscribedNewsletters(ctx)
	if errN != nil {
		logger.Error("GroupManager: failed to fetch subscribed newsletters", "err", errN)
	} else {
		for _, n := range newsletters {
			meta := gm.convertNewsletterMetadata(n)
			syncedNewsletters = append(syncedNewsletters, meta)

			if s != nil && s.GetDB() != nil {
				_ = store.SaveCachedNewsletter(ctx, s.GetDB(), ourJID, meta)
			}
		}

		gm.mu.Lock()
		for _, meta := range syncedNewsletters {
			gm.newsletters[meta.JID] = meta
		}
		gm.mu.Unlock()
	}

	// Calculate community stats
	communityCount := 0
	for _, g := range syncedGroups {
		if g.IsCommunity {
			communityCount++
		}
	}

	logger.Debug("GroupManager: sync complete",
		"total_groups", len(syncedGroups),
		"communities", communityCount,
		"newsletters", len(syncedNewsletters),
	)

	// 3. Trigger background device pre-warming for all group participants
	if len(joinedGroups) > 0 {
		go gm.WarmupDevices(context.Background(), cli, joinedGroups)
	}

	return nil
}

// WarmupDevices extracts all group participants from the synced groups and pre-fetches their companion device lists into memory.
func (gm *GroupManager) WarmupDevices(ctx context.Context, cli *whatsmeow.Client, groups []*types.GroupInfo) {
	if cli == nil || !cli.IsConnected() || len(groups) == 0 {
		return
	}

	seen := make(map[types.JID]struct{})
	var uniqueJIDs []types.JID
	for _, g := range groups {
		if g == nil {
			continue
		}
		for _, p := range g.Participants {
			targetJID := p.JID
			if targetJID.IsEmpty() {
				targetJID = p.PhoneNumber
			}
			if targetJID.IsEmpty() || targetJID.IsBot() {
				continue
			}
			if _, ok := seen[targetJID]; !ok {
				seen[targetJID] = struct{}{}
				uniqueJIDs = append(uniqueJIDs, targetJID)
			}
		}
	}

	if len(uniqueJIDs) == 0 {
		return
	}

	logger.Debug("GroupManager: warming up participant device cache...", "unique_participants", len(uniqueJIDs))
	start := time.Now()

	// Chunk queries into batches of 150 to keep USync requests optimal
	const batchSize = 150
	totalCached := 0
	var allDeviceJIDs []types.JID
	for i := 0; i < len(uniqueJIDs); i += batchSize {
		end := min(i+batchSize, len(uniqueJIDs))
		batch := uniqueJIDs[i:end]
		devices, err := cli.GetUserDevices(ctx, batch)
		if err != nil {
			logger.Debug("GroupManager: device warmup batch error", "batch_start", i, "err", err)
		} else {
			totalCached += len(devices)
			allDeviceJIDs = append(allDeviceJIDs, devices...)
		}
	}

	// Pre-warm in-memory L1 Signal sessions for all companion devices
	if len(allDeviceJIDs) > 0 && cli.Store != nil {
		var addrs []string
		for _, d := range allDeviceJIDs {
			addrs = append(addrs, d.SignalAddress().String())
		}
		_, _, _ = cli.Store.WithCachedSessions(ctx, addrs)
	}

	logger.Info("GroupManager: device and session cache warm up complete",
		"total_participants", len(uniqueJIDs),
		"companion_devices", totalCached,
		"duration", time.Since(start),
	)
}

func (gm *GroupManager) convertGroupInfo(ctx context.Context, cli *whatsmeow.Client, g *types.GroupInfo) *store.GroupMetadata {
	if g == nil {
		return nil
	}

	name := g.Name
	if name == "" && g.GroupName.Name != "" {
		name = g.GroupName.Name
	}
	topic := g.Topic
	if topic == "" && g.GroupTopic.Topic != "" {
		topic = g.GroupTopic.Topic
	}

	var topicSetAt time.Time
	var topicSetBy types.JID
	topicID := ""
	if g.GroupTopic.Topic != "" {
		topicSetAt = g.GroupTopic.TopicSetAt
		topicSetBy = g.GroupTopic.TopicSetBy
		topicID = g.GroupTopic.TopicID
	}

	var isLocked, isAnnounce, isEphemeral, isApproval, isIncognito bool
	var ephDuration uint32
	if g.GroupLocked.IsLocked {
		isLocked = true
	}
	if g.GroupAnnounce.IsAnnounce {
		isAnnounce = true
	}
	if g.GroupEphemeral.IsEphemeral {
		isEphemeral = true
		ephDuration = g.GroupEphemeral.DisappearingTimer
	}
	if g.GroupMembershipApprovalMode.IsJoinApprovalRequired {
		isApproval = true
	}
	if g.GroupIncognito.IsIncognito {
		isIncognito = true
	}

	// Community metadata
	isCommunity := g.GroupParent.IsParent
	linkedParentJID := g.GroupLinkedParent.LinkedParentJID
	parentJID := linkedParentJID
	isDefaultSubgroup := g.GroupIsDefaultSub.IsDefaultSubGroup
	isGeneralChat := g.GroupGeneralChat.IsGeneralChat

	var participants []store.GroupParticipantMetadata
	adminCount := 0
	for _, p := range g.Participants {
		pnJID := p.JID.ToNonAD()
		pm := store.GroupParticipantMetadata{
			JID:          pnJID,
			LID:          p.LID,
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		}
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		}
		participants = append(participants, pm)
	}

	return &store.GroupMetadata{
		JID:                    g.JID,
		Name:                   name,
		Topic:                  topic,
		TopicID:                topicID,
		TopicSetAt:             topicSetAt,
		TopicSetBy:             topicSetBy,
		OwnerJID:               g.OwnerJID,
		CreatedAt:              g.GroupCreated,
		IsLocked:               isLocked,
		IsAnnounce:             isAnnounce,
		IsEphemeral:            isEphemeral,
		EphemeralDuration:      ephDuration,
		MembershipApprovalMode: isApproval,
		IsIncognito:            isIncognito,
		IsCommunity:            isCommunity,
		ParentJID:              parentJID,
		LinkedParentJID:        linkedParentJID,
		IsDefaultSubgroup:      isDefaultSubgroup,
		IsGeneralChat:          isGeneralChat,
		Participants:           participants,
		ParticipantCount:       len(participants),
		AdminCount:             adminCount,
		UpdatedAt:              time.Now().UTC(),
	}
}

func (gm *GroupManager) convertNewsletterMetadata(n *types.NewsletterMetadata) *store.NewsletterMetadata {
	if n == nil {
		return nil
	}

	name := n.ThreadMeta.Name.Text
	desc := n.ThreadMeta.Description.Text

	picURL := ""
	if n.ThreadMeta.Picture != nil {
		picURL = n.ThreadMeta.Picture.URL
		if picURL == "" {
			picURL = n.ThreadMeta.Picture.DirectPath
		}
	}

	role := ""
	mute := ""
	if n.ViewerMeta != nil {
		role = string(n.ViewerMeta.Role)
		mute = string(n.ViewerMeta.Mute)
	}

	return &store.NewsletterMetadata{
		JID:              n.ID,
		Name:             name,
		Description:      desc,
		InviteCode:       n.ThreadMeta.InviteCode,
		SubscribersCount: int64(n.ThreadMeta.SubscriberCount),
		Verification:     string(n.ThreadMeta.VerificationState),
		Role:             role,
		MuteState:        mute,
		PictureURL:       picURL,
		CreatedAt:        n.ThreadMeta.CreationTime.Time,
		UpdatedAt:        time.Now().UTC(),
	}
}

// UpdateFromEvent updates in-memory cache and database from live WhatsApp events.
func (gm *GroupManager) UpdateFromEvent(ctx context.Context, cli *whatsmeow.Client, evt any) {
	ourJID := ""
	var s *sqlstore.SQLStore
	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		ourJID = cli.Store.ID.ToNonAD().String()
		if storeObj, ok := cli.Store.Identities.(*sqlstore.SQLStore); ok {
			s = storeObj
		}
	}

	switch v := evt.(type) {
	case *events.GroupInfo:
		gm.handleGroupInfoEvent(ctx, cli, v, ourJID, s)

	case *events.JoinedGroup:
		gm.handleJoinedGroupEvent(ctx, cli, v, ourJID, s)

	case *events.Picture:
		gm.handlePictureEvent(ctx, v, ourJID, s)

	case *events.DisappearingMode:
		if v.Chat.Server == types.GroupServer {
			gm.mu.Lock()
			meta, exists := gm.groups[v.Chat]
			if exists && meta != nil {
				meta.IsEphemeral = v.IsEphemeral
				meta.EphemeralDuration = uint32(v.Timer.Seconds())
				if s != nil && s.GetDB() != nil {
					_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, meta)
				}
			}
			gm.mu.Unlock()
		}

	case *events.NewsletterJoin:
		meta := gm.convertNewsletterMetadata(&v.NewsletterMetadata)
		gm.mu.Lock()
		gm.newsletters[meta.JID] = meta
		gm.mu.Unlock()
		if s != nil && s.GetDB() != nil {
			_ = store.SaveCachedNewsletter(ctx, s.GetDB(), ourJID, meta)
		}

	case *events.NewsletterLeave:
		gm.mu.Lock()
		delete(gm.newsletters, v.ID)
		gm.mu.Unlock()
		if s != nil && s.GetDB() != nil {
			_ = store.DeleteCachedNewsletter(ctx, s.GetDB(), ourJID, v.ID.String())
		}

	case *events.NewsletterMuteChange:
		gm.mu.Lock()
		if n, ok := gm.newsletters[v.ID]; ok {
			n.MuteState = string(v.Mute)
			n.UpdatedAt = time.Now().UTC()
			if s != nil && s.GetDB() != nil {
				_ = store.SaveCachedNewsletter(ctx, s.GetDB(), ourJID, n)
			}
		}
		gm.mu.Unlock()

	case *events.NewsletterLiveUpdate:
		gm.mu.Lock()
		if n, ok := gm.newsletters[v.JID]; ok {
			n.UpdatedAt = v.Time
		}
		gm.mu.Unlock()
	}
}

func (gm *GroupManager) handleGroupInfoEvent(ctx context.Context, cli *whatsmeow.Client, g *events.GroupInfo, ourJID string, s *sqlstore.SQLStore) {
	if g == nil || g.JID.IsEmpty() {
		return
	}

	gm.mu.Lock()
	meta, exists := gm.groups[g.JID]
	if !exists {
		gm.mu.Unlock()
		// Group not in cache yet: fetch complete info from server
		if cli != nil {
			if info, err := cli.GetGroupInfo(ctx, g.JID); err == nil && info != nil {
				meta = gm.convertGroupInfo(ctx, cli, info)
				gm.mu.Lock()
				gm.groups[g.JID] = meta
				gm.mu.Unlock()
				if s != nil && s.GetDB() != nil {
					_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, meta)
				}
			}
		}
		return
	}

	// 1. Group deleted or left
	if g.Delete != nil && g.Delete.Deleted {
		delete(gm.groups, g.JID)
		gm.mu.Unlock()
		if s != nil && s.GetDB() != nil {
			_ = store.DeleteCachedGroup(ctx, s.GetDB(), ourJID, g.JID.String())
		}
		return
	}

	// 2. Name update
	if g.Name != nil && g.Name.Name != "" {
		meta.Name = g.Name.Name
	}

	// 3. Topic update
	if g.Topic != nil {
		meta.Topic = g.Topic.Topic
		meta.TopicID = g.Topic.TopicID
		meta.TopicSetAt = g.Topic.TopicSetAt
		meta.TopicSetBy = g.Topic.TopicSetBy
	}

	// 4. Locked / Announce / Ephemeral / Approval updates
	if g.Locked != nil {
		meta.IsLocked = g.Locked.IsLocked
	}
	if g.Announce != nil {
		meta.IsAnnounce = g.Announce.IsAnnounce
	}
	if g.Ephemeral != nil {
		meta.IsEphemeral = g.Ephemeral.IsEphemeral
		meta.EphemeralDuration = g.Ephemeral.DisappearingTimer
	}
	if g.MembershipApprovalMode != nil {
		meta.MembershipApprovalMode = g.MembershipApprovalMode.IsJoinApprovalRequired
	}

	// 5. Community Link / Unlink updates
	if g.Link != nil {
		switch g.Link.Type {
		case types.GroupLinkChangeTypeParent:
			meta.ParentJID = g.Link.Group.JID
			meta.LinkedParentJID = g.Link.Group.JID
		case types.GroupLinkChangeTypeSub:
			meta.IsCommunity = true
		}
	}
	if g.Unlink != nil {
		if g.Unlink.Type == types.GroupLinkChangeTypeParent {
			meta.ParentJID = types.EmptyJID
			meta.LinkedParentJID = types.EmptyJID
		}
	}

	// 6. Participants Joins
	if len(g.Join) > 0 {
		for _, j := range g.Join {
			resolvedJID := j.ToNonAD()
			already := false
			for _, p := range meta.Participants {
				if p.JID == resolvedJID {
					already = true
					break
				}
			}
			if !already {
				meta.Participants = append(meta.Participants, store.GroupParticipantMetadata{
					JID:          resolvedJID,
					IsAdmin:      false,
					IsSuperAdmin: false,
				})
			}
		}
	}

	// 7. Participants Leaves
	if len(g.Leave) > 0 {
		for _, l := range g.Leave {
			resolvedJID := l.ToNonAD()
			newParticipants := make([]store.GroupParticipantMetadata, 0, len(meta.Participants))
			for _, p := range meta.Participants {
				if p.JID != resolvedJID {
					newParticipants = append(newParticipants, p)
				}
			}
			meta.Participants = newParticipants
		}
	}

	// 8. Admin Promotions
	if len(g.Promote) > 0 {
		for _, prom := range g.Promote {
			resolvedJID := prom.ToNonAD()
			for i := range meta.Participants {
				if meta.Participants[i].JID == resolvedJID {
					meta.Participants[i].IsAdmin = true
					break
				}
			}
		}
	}

	// 9. Admin Demotions
	if len(g.Demote) > 0 {
		for _, dem := range g.Demote {
			resolvedJID := dem.ToNonAD()
			for i := range meta.Participants {
				if meta.Participants[i].JID == resolvedJID {
					meta.Participants[i].IsAdmin = false
					meta.Participants[i].IsSuperAdmin = false
					break
				}
			}
		}
	}

	// Recompute counts
	adminCount := 0
	for _, p := range meta.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			adminCount++
		}
	}
	meta.ParticipantCount = len(meta.Participants)
	meta.AdminCount = adminCount
	meta.UpdatedAt = time.Now().UTC()

	gm.mu.Unlock()

	if s != nil && s.GetDB() != nil {
		_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, meta)
	}
}

func (gm *GroupManager) handleJoinedGroupEvent(ctx context.Context, cli *whatsmeow.Client, j *events.JoinedGroup, ourJID string, s *sqlstore.SQLStore) {
	if j == nil || j.JID.IsEmpty() {
		return
	}
	var info *types.GroupInfo
	if cli != nil {
		var err error
		info, err = cli.GetGroupInfo(ctx, j.JID)
		if err != nil || info == nil {
			info = &j.GroupInfo
		}
	} else {
		info = &j.GroupInfo
	}
	meta := gm.convertGroupInfo(ctx, cli, info)

	gm.mu.Lock()
	gm.groups[meta.JID] = meta
	gm.mu.Unlock()

	if s != nil && s.GetDB() != nil {
		_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, meta)
	}
}

func (gm *GroupManager) handlePictureEvent(ctx context.Context, p *events.Picture, ourJID string, s *sqlstore.SQLStore) {
	if p == nil || p.JID.IsEmpty() {
		return
	}

	switch p.JID.Server {
	case types.GroupServer:
		gm.mu.Lock()
		if g, ok := gm.groups[p.JID]; ok {
			g.UpdatedAt = time.Now().UTC()
			if s != nil && s.GetDB() != nil {
				_ = store.SaveCachedGroup(ctx, s.GetDB(), ourJID, g)
			}
		}
		gm.mu.Unlock()
	case types.NewsletterServer:
		gm.mu.Lock()
		if n, ok := gm.newsletters[p.JID]; ok {
			n.PictureURL = p.PictureID
			n.UpdatedAt = time.Now().UTC()
			if s != nil && s.GetDB() != nil {
				_ = store.SaveCachedNewsletter(ctx, s.GetDB(), ourJID, n)
			}
		}
		gm.mu.Unlock()
	}
}

// IsAdmin checks if a user is an admin in the specified group using cached metadata.
func (gm *GroupManager) IsAdmin(groupJID, userJID types.JID) bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	g, ok := gm.groups[groupJID]
	if !ok {
		return false
	}
	cleanUser := userJID.ToNonAD()
	for _, p := range g.Participants {
		if p.JID == cleanUser || p.LID == cleanUser {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}
