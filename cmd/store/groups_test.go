package store_test

import (
	"context"
	"testing"
	"time"

	clistore "whatsrook/cmd/store"

	"go.mau.fi/whatsmeow/types"
)

func TestSaveAndLoadCachedGroupsAndNewsletters(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("failed running migrations: %v", err)
	}

	ourJID := "100000000000001@s.whatsapp.net"
	groupJID := types.NewJID("120363025000000", types.GroupServer)
	parentJID := types.NewJID("120363025999999", types.GroupServer)
	ownerJID := types.NewJID("2348011111111", types.DefaultUserServer)
	user1 := types.NewJID("2348022222222", types.DefaultUserServer)
	user2 := types.NewJID("2348033333333", types.DefaultUserServer)

	groupMeta := &clistore.GroupMetadata{
		JID:                    groupJID,
		Name:                   "Alpha Community Subgroup",
		Topic:                  "Discussion Topic",
		OwnerJID:               ownerJID,
		CreatedAt:              time.Now().UTC().Truncate(time.Second),
		IsLocked:               true,
		IsAnnounce:             false,
		IsEphemeral:            true,
		EphemeralDuration:      86400,
		MembershipApprovalMode: true,
		IsIncognito:            false,
		IsCommunity:            false,
		ParentJID:              parentJID,
		LinkedParentJID:        parentJID,
		IsDefaultSubgroup:      false,
		IsGeneralChat:          true,
		ParticipantCount:       2,
		AdminCount:             1,
		Participants: []clistore.GroupParticipantMetadata{
			{JID: user1, IsAdmin: true, DisplayName: "Admin One"},
			{JID: user2, IsAdmin: false, DisplayName: "Member Two"},
		},
	}

	// 1. Save group
	if err := clistore.SaveCachedGroup(ctx, db, ourJID, groupMeta); err != nil {
		t.Fatalf("failed saving cached group: %v", err)
	}

	// 2. Load group
	groups, err := clistore.LoadAllCachedGroups(ctx, db, ourJID)
	if err != nil {
		t.Fatalf("failed loading cached groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 cached group, got %d", len(groups))
	}
	g := groups[0]
	if g.JID != groupJID || g.Name != "Alpha Community Subgroup" || !g.IsLocked || g.ParentJID != parentJID || !g.IsGeneralChat {
		t.Errorf("group metadata mismatch: %+v", g)
	}
	if len(g.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(g.Participants))
	}

	// 3. Delete group
	if err := clistore.DeleteCachedGroup(ctx, db, ourJID, groupJID.String()); err != nil {
		t.Fatalf("failed deleting cached group: %v", err)
	}
	groupsAfter, _ := clistore.LoadAllCachedGroups(ctx, db, ourJID)
	if len(groupsAfter) != 0 {
		t.Errorf("expected 0 groups after deletion, got %d", len(groupsAfter))
	}

	// 4. Save and load newsletter
	nJID := types.NewJID("120363099999999", types.NewsletterServer)
	nMeta := &clistore.NewsletterMetadata{
		JID:              nJID,
		Name:             "Alpha Announcements",
		Description:      "Official announcements channel",
		InviteCode:       "alpha123",
		SubscribersCount: 1500,
		Role:             "admin",
	}
	if err := clistore.SaveCachedNewsletter(ctx, db, ourJID, nMeta); err != nil {
		t.Fatalf("failed saving cached newsletter: %v", err)
	}

	newsletters, err := clistore.LoadAllCachedNewsletters(ctx, db, ourJID)
	if err != nil {
		t.Fatalf("failed loading cached newsletters: %v", err)
	}
	if len(newsletters) != 1 {
		t.Fatalf("expected 1 cached newsletter, got %d", len(newsletters))
	}
	n := newsletters[0]
	if n.JID != nJID || n.Name != "Alpha Announcements" || n.SubscribersCount != 1500 {
		t.Errorf("newsletter metadata mismatch: %+v", n)
	}

	// 5. Delete newsletter
	if err := clistore.DeleteCachedNewsletter(ctx, db, ourJID, nJID.String()); err != nil {
		t.Fatalf("failed deleting cached newsletter: %v", err)
	}
	newslettersAfter, _ := clistore.LoadAllCachedNewsletters(ctx, db, ourJID)
	if len(newslettersAfter) != 0 {
		t.Errorf("expected 0 newsletters after deletion, got %d", len(newslettersAfter))
	}
}
