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

	ourJID := "1234567890@s.whatsapp.net"
	groupJID := types.NewJID("987654321", types.GroupServer)
	parentJID := types.NewJID("1122334455", types.GroupServer)
	ownerJID := types.NewJID("5544332211", types.DefaultUserServer)
	user1 := types.NewJID("1111111111", types.DefaultUserServer)
	user2 := types.NewJID("2222222222", types.DefaultUserServer)

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
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.JID != groupJID || g.Name != "Alpha Community Subgroup" || g.ParentJID != parentJID {
		t.Errorf("group metadata mismatch: %+v", g)
	}
	if len(g.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(g.Participants))
	}

	// 3. Save Newsletter
	channelJID := types.NewJID("120363000000000000", types.NewsletterServer)
	newsletterMeta := &clistore.NewsletterMetadata{
		JID:              channelJID,
		Name:             "Daily News",
		Description:      "Latest updates and alerts",
		InviteCode:       "invitecode123",
		SubscribersCount: 12500,
		Verification:     "VERIFIED",
		Role:             "ADMIN",
		MuteState:        "OFF",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}

	if err := clistore.SaveCachedNewsletter(ctx, db, ourJID, newsletterMeta); err != nil {
		t.Fatalf("failed saving newsletter: %v", err)
	}

	// 4. Load Newsletters
	newsletters, errN := clistore.LoadAllCachedNewsletters(ctx, db, ourJID)
	if errN != nil {
		t.Fatalf("failed loading newsletters: %v", errN)
	}
	if len(newsletters) != 1 {
		t.Fatalf("expected 1 newsletter, got %d", len(newsletters))
	}
	n := newsletters[0]
	if n.JID != channelJID || n.Name != "Daily News" || n.SubscribersCount != 12500 || n.Verification != "VERIFIED" {
		t.Errorf("newsletter metadata mismatch: %+v", n)
	}

	// 5. Delete group and newsletter
	if err := clistore.DeleteCachedGroup(ctx, db, ourJID, groupJID.String()); err != nil {
		t.Fatalf("failed deleting cached group: %v", err)
	}
	if err := clistore.DeleteCachedNewsletter(ctx, db, ourJID, channelJID.String()); err != nil {
		t.Fatalf("failed deleting cached newsletter: %v", err)
	}

	groupsAfter, _ := clistore.LoadAllCachedGroups(ctx, db, ourJID)
	if len(groupsAfter) != 0 {
		t.Errorf("expected 0 groups after delete, got %d", len(groupsAfter))
	}
}
