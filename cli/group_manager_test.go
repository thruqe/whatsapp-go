package main

import (
	"context"
	"testing"
	"time"

	"whatsrook/cli/store"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestGroupManagerInMem(t *testing.T) {
	gm := NewGroupManager()

	groupJID := types.NewJID("1234567890", types.GroupServer)
	user1 := types.NewJID("1111111111", types.DefaultUserServer)
	user2 := types.NewJID("2222222222", types.DefaultUserServer)
	ownerJID := types.NewJID("3333333333", types.DefaultUserServer)

	meta := &store.GroupMetadata{
		JID:      groupJID,
		Name:     "Test Group",
		Topic:    "Test Topic",
		OwnerJID: ownerJID,
		Participants: []store.GroupParticipantMetadata{
			{JID: user1, IsAdmin: true},
			{JID: user2, IsAdmin: false},
		},
		ParticipantCount: 2,
		AdminCount:       1,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	gm.mu.Lock()
	gm.groups[groupJID] = meta
	gm.mu.Unlock()

	// 1. GetGroup
	g, ok := gm.GetGroup(groupJID)
	if !ok || g == nil {
		t.Fatalf("expected to get group %s", groupJID)
	}
	if g.Name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %s", g.Name)
	}

	// 2. IsAdmin check
	if !gm.IsAdmin(groupJID, user1) {
		t.Errorf("expected user1 to be admin")
	}
	if gm.IsAdmin(groupJID, user2) {
		t.Errorf("expected user2 not to be admin")
	}

	// 3. Update from event: GroupInfo Name change & Promote
	gm.UpdateFromEvent(context.Background(), nil, &events.GroupInfo{
		JID: groupJID,
		Name: &types.GroupName{
			Name: "Updated Test Group",
		},
		Promote: []types.JID{user2},
	})

	g, _ = gm.GetGroup(groupJID)
	if g.Name != "Updated Test Group" {
		t.Errorf("expected updated name, got %s", g.Name)
	}
	if !gm.IsAdmin(groupJID, user2) {
		t.Errorf("expected user2 to be promoted to admin")
	}
	if g.AdminCount != 2 {
		t.Errorf("expected admin count to be 2, got %d", g.AdminCount)
	}

	// 4. Update from event: Demote & Leave
	gm.UpdateFromEvent(context.Background(), nil, &events.GroupInfo{
		JID:    groupJID,
		Demote: []types.JID{user1},
		Leave:  []types.JID{user1},
	})

	g, _ = gm.GetGroup(groupJID)
	if gm.IsAdmin(groupJID, user1) {
		t.Errorf("expected user1 not to be admin after demote")
	}
	if g.ParticipantCount != 1 {
		t.Errorf("expected participant count to be 1, got %d", g.ParticipantCount)
	}

	// 5. Test Newsletters / Channels in manager
	channelJID := types.NewJID("120363000000000000", types.NewsletterServer)
	nMeta := &store.NewsletterMetadata{
		JID:              channelJID,
		Name:             "Alpha Channel",
		Description:      "Official Updates",
		InviteCode:       "abc12345",
		SubscribersCount: 5000,
		Role:             "OWNER",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	gm.mu.Lock()
	gm.newsletters[channelJID] = nMeta
	gm.mu.Unlock()

	n, okN := gm.GetNewsletter(channelJID)
	if !okN || n == nil {
		t.Fatalf("expected to get newsletter %s", channelJID)
	}
	if n.SubscribersCount != 5000 {
		t.Errorf("expected subscribers count 5000, got %d", n.SubscribersCount)
	}

	// 6. Test NewsletterLeave event
	gm.UpdateFromEvent(context.Background(), nil, &events.NewsletterLeave{
		ID: channelJID,
	})

	if _, exists := gm.GetNewsletter(channelJID); exists {
		t.Errorf("expected newsletter to be removed after leave event")
	}
}
