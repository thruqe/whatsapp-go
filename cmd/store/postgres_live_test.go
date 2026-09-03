package store_test

import (
	"context"
	"testing"
	"time"

	clistore "whatsrook/cmd/store"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

func TestPostgresLiveStoreIntegration(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	container := sqlstore.NewWithDB(db.RawDB, "postgres", nil)
	deviceJID := types.NewADJID("100000000000001", 0, 88)
	s := sqlstore.NewSQLStore(container, deviceJID)

	// 1. Test PutSetting and GetSetting for sudoers
	t.Run("Settings_Sudoers", func(t *testing.T) {
		sudoersVal := "100000000000002@lid 100000000000003@s.whatsapp.net"
		if err := clistore.PutSetting(ctx, s, "sudoers", sudoersVal); err != nil {
			t.Fatalf("PutSetting('sudoers') failed: %v", err)
		}
		got, err := clistore.GetSetting(ctx, s, "sudoers")
		if err != nil {
			t.Fatalf("GetSetting('sudoers') failed: %v", err)
		}
		if got != sudoersVal {
			t.Errorf("GetSetting('sudoers') = %q, expected %q", got, sudoersVal)
		}
	})

	// 2. Test Setting isolation across AD-JID and different sessions
	t.Run("Settings_Isolation_AD_JID", func(t *testing.T) {
		if err := clistore.PutSetting(ctx, s, "prefix", "!"); err != nil {
			t.Fatalf("PutSetting prefix failed: %v", err)
		}

		// Read using non-AD store representation
		nonADJID := types.NewJID("100000000000001", types.DefaultUserServer)
		sNonAD := sqlstore.NewSQLStore(container, nonADJID)
		got, err := clistore.GetSetting(ctx, sNonAD, "prefix")
		if err != nil || got != "!" {
			t.Errorf("GetSetting on non-AD store = %q (err: %v), expected '!'", got, err)
		}

		// Verify different session does not see prefix
		diffJID := types.NewJID("2349099999999", types.DefaultUserServer)
		sDiff := sqlstore.NewSQLStore(container, diffJID)
		gotDiff, _ := clistore.GetSetting(ctx, sDiff, "prefix")
		if gotDiff != "" {
			t.Errorf("expected empty prefix for different session, got %q", gotDiff)
		}
	})

	// 3. Test Call Media Config
	t.Run("CallMediaConfig", func(t *testing.T) {
		callerJID := types.NewJID("1234567890", types.DefaultUserServer)
		audioPath := "/media/custom_voicemail.opus"
		if err := clistore.PutCallMediaConfig(ctx, s, callerJID, clistore.CallMediaAudio, audioPath); err != nil {
			t.Fatalf("PutCallMediaConfig failed: %v", err)
		}

		gotPath, err := clistore.GetCallMediaConfig(ctx, s, callerJID, clistore.CallMediaAudio)
		if err != nil || gotPath != audioPath {
			t.Errorf("GetCallMediaConfig audio = %q (err: %v), expected %q", gotPath, err, audioPath)
		}

		// Update to new path
		newPath := "/media/updated.opus"
		if err := clistore.PutCallMediaConfig(ctx, s, callerJID, clistore.CallMediaAudio, newPath); err != nil {
			t.Fatalf("PutCallMediaConfig update failed: %v", err)
		}
		gotPath, err = clistore.GetCallMediaConfig(ctx, s, callerJID, clistore.CallMediaAudio)
		if err != nil || gotPath != newPath {
			t.Errorf("GetCallMediaConfig after update = %q, expected %q", gotPath, newPath)
		}
	})

	// 4. Test Filters and BGM
	t.Run("FiltersAndBGM", func(t *testing.T) {
		if err := clistore.PutFilter(ctx, s, "hello", "proto_hello_world"); err != nil {
			t.Fatalf("PutFilter failed: %v", err)
		}
		gotFilter, err := clistore.GetFilter(ctx, s, "hello")
		if err != nil || gotFilter != "proto_hello_world" {
			t.Errorf("GetFilter('hello') = %q, expected 'proto_hello_world'", gotFilter)
		}

		filters, err := clistore.ListFilters(ctx, s)
		if err != nil || len(filters) == 0 {
			t.Errorf("ListFilters failed or empty: %v", err)
		}

		if err := clistore.PutBGM(ctx, s, "theme", "proto_theme_audio"); err != nil {
			t.Fatalf("PutBGM failed: %v", err)
		}
		gotBGM, err := clistore.GetBGM(ctx, s, "theme")
		if err != nil || gotBGM != "proto_theme_audio" {
			t.Errorf("GetBGM('theme') = %q, expected 'proto_theme_audio'", gotBGM)
		}

		bgms, err := clistore.ListBGMs(ctx, s)
		if err != nil || len(bgms) == 0 {
			t.Errorf("ListBGMs failed or empty: %v", err)
		}
	})

	// 5. Test Sticker Commands
	t.Run("StickerCommands", func(t *testing.T) {
		stickerHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
		if err := clistore.PutStickerCmd(ctx, s, stickerHash, "menu"); err != nil {
			t.Fatalf("PutStickerCmd failed: %v", err)
		}
		cmdName, err := clistore.GetStickerCmd(ctx, s, stickerHash)
		if err != nil || cmdName != "menu" {
			t.Errorf("GetStickerCmd = %q (err: %v), expected 'menu'", cmdName, err)
		}

		list, err := clistore.ListStickerCmds(ctx, s)
		if err != nil || len(list) == 0 {
			t.Errorf("ListStickerCmds failed or empty: %v", err)
		}

		if err := clistore.DeleteStickerCmdBySHA(ctx, s, stickerHash); err != nil {
			t.Fatalf("DeleteStickerCmdBySHA failed: %v", err)
		}
		cmdName, _ = clistore.GetStickerCmd(ctx, s, stickerHash)
		if cmdName != "" {
			t.Errorf("expected sticker cmd to be deleted, got %q", cmdName)
		}
	})

	// 6. Test Group Stats
	t.Run("GroupStats", func(t *testing.T) {
		groupJID := types.NewJID("120363000000001", types.GroupServer)
		userJID := types.NewJID("2348011111111", types.DefaultUserServer)

		clistore.LogGroupMessage(ctx, s, groupJID, userJID)
		clistore.LogGroupMessage(ctx, s, groupJID, userJID)

		var count int
		dateStr := time.Now().Format("2006-01-02")
		ourJID := "100000000000001@s.whatsapp.net"
		err := db.QueryRow(ctx, "SELECT msg_count FROM group_stats WHERE our_jid=$1 AND group_jid=$2 AND user_jid=$3 AND date_str=$4", ourJID, groupJID.String(), userJID.String(), dateStr).Scan(&count)
		if err != nil || count != 2 {
			t.Errorf("group_stats msg_count = %d (err: %v), expected 2", count, err)
		}
	})

	// 7. Test Group XP: TTT, WCG, Unscramble
	t.Run("GroupXP_Games", func(t *testing.T) {
		groupJID := "120363000000001@g.us"
		userJID := "2348011111111@s.whatsapp.net"

		// TTT win
		if err := clistore.AddGroupUserTTTXP(ctx, s, groupJID, userJID, 50, 1, 0, 0); err != nil {
			t.Fatalf("AddGroupUserTTTXP failed: %v", err)
		}

		// WCG win
		if err := clistore.AddGroupUserWCGXP(ctx, s, groupJID, userJID, 30, 1, 1, 15); err != nil {
			t.Fatalf("AddGroupUserWCGXP failed: %v", err)
		}

		// Unscramble win
		if err := clistore.AddGroupUserUnscrambleXP(ctx, s, groupJID, userJID, 40, 1, 20); err != nil {
			t.Fatalf("AddGroupUserUnscrambleXP failed: %v", err)
		}

		entry, err := clistore.GetGroupUserXP(ctx, s, groupJID, userJID)
		if err != nil {
			t.Fatalf("GetGroupUserXP failed: %v", err)
		}
		if entry.XP != 120 {
			t.Errorf("expected total XP to be 120, got %d", entry.XP)
		}
		if entry.TTTWins != 1 {
			t.Errorf("expected TTTWins 1, got %d", entry.TTTWins)
		}
		if entry.WCGWins != 1 {
			t.Errorf("expected WCGWins 1, got %d", entry.WCGWins)
		}
		if entry.UnscrambleWins != 1 {
			t.Errorf("expected UnscrambleWins 1, got %d", entry.UnscrambleWins)
		}
		if entry.UnscrambleScore != 20 {
			t.Errorf("expected UnscrambleScore 20, got %d", entry.UnscrambleScore)
		}

		lb, err := clistore.GetGroupLeaderboard(ctx, s, groupJID)
		if err != nil || len(lb) == 0 {
			t.Fatalf("GetGroupLeaderboard failed: %v", err)
		}
		if lb[0].UserJID != userJID {
			t.Errorf("expected user to be top of leaderboard, got %s", lb[0].UserJID)
		}
	})

	// 8. Test Global User XP
	t.Run("GlobalUserXP", func(t *testing.T) {
		userJID := "2348011111111@s.whatsapp.net"
		if err := clistore.AddUserXP(ctx, s, userJID, 100, 10, 2, 5); err != nil {
			t.Fatalf("AddUserXP failed: %v", err)
		}
		entry, err := clistore.GetUserXP(ctx, s, userJID)
		if err != nil {
			t.Fatalf("GetUserXP failed: %v", err)
		}
		if entry.XP != 100 || entry.Messages != 10 || entry.Stickers != 2 || entry.Commands != 5 {
			t.Errorf("GetUserXP unexpected values: %+v", entry)
		}

		// Increment
		if err := clistore.AddUserXP(ctx, s, userJID, 50, 5, 1, 2); err != nil {
			t.Fatalf("AddUserXP increment failed: %v", err)
		}
		entry, err = clistore.GetUserXP(ctx, s, userJID)
		if err != nil || entry.XP != 150 || entry.Messages != 15 {
			t.Errorf("GetUserXP after increment: %+v (err: %v)", entry, err)
		}
	})

	// 9. Test Cached Groups & Newsletters
	t.Run("CachedGroupsAndNewsletters", func(t *testing.T) {
		ourJID := "100000000000001@s.whatsapp.net"
		groupJID := types.NewJID("120363000000002", types.GroupServer)
		u1 := types.NewJID("2348011111111", types.DefaultUserServer)
		u2 := types.NewJID("2348022222222", types.DefaultUserServer)

		meta := &clistore.GroupMetadata{
			JID:              groupJID,
			Name:             "Test Group One",
			Topic:            "Live testing group caching",
			ParticipantCount: 2,
			AdminCount:       1,
			Participants: []clistore.GroupParticipantMetadata{
				{JID: u1, IsAdmin: true, DisplayName: "Admin User"},
				{JID: u2, IsAdmin: false, DisplayName: "Member User"},
				// Deliberate duplicate participant to verify ON CONFLICT deduplication
				{JID: u2, IsAdmin: false, DisplayName: "Member User Duplicate"},
			},
		}

		if err := clistore.SaveCachedGroup(ctx, db, ourJID, meta); err != nil {
			t.Fatalf("SaveCachedGroup failed: %v", err)
		}

		groups, err := clistore.LoadAllCachedGroups(ctx, db, ourJID)
		if err != nil || len(groups) == 0 {
			t.Fatalf("LoadAllCachedGroups failed: %v", err)
		}
		if groups[0].Name != "Test Group One" {
			t.Errorf("expected group name 'Test Group One', got %q", groups[0].Name)
		}
		if len(groups[0].Participants) != 2 {
			t.Errorf("expected 2 deduplicated participants, got %d", len(groups[0].Participants))
		}

		// Newsletters
		nJID := types.NewJID("120363000000003", types.NewsletterServer)
		nMeta := &clistore.NewsletterMetadata{
			JID:              nJID,
			Name:             "Daily Tech News",
			Description:      "Latest updates in AI and Go",
			SubscribersCount: 500,
		}
		if err := clistore.SaveCachedNewsletter(ctx, db, ourJID, nMeta); err != nil {
			t.Fatalf("SaveCachedNewsletter failed: %v", err)
		}

		newsletters, err := clistore.LoadAllCachedNewsletters(ctx, db, ourJID)
		if err != nil || len(newsletters) == 0 {
			t.Fatalf("LoadAllCachedNewsletters failed: %v", err)
		}
		if newsletters[0].Name != "Daily Tech News" {
			t.Errorf("expected newsletter name 'Daily Tech News', got %q", newsletters[0].Name)
		}
	})
}
