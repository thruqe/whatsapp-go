package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"whatsrook"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

var (
	testStore *sqlstore.SQLStore
	testCtx   context.Context
	testDB    *dbutil.Database
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "store_test_*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	container, err := whatsrook.OpenStoreContainer(ctx, tempDir, "")
	if err != nil {
		panic(err)
	}
	defer container.Close()

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	botPN := types.NewJID("2348000000000", types.DefaultUserServer)
	deviceStore.ID = &botPN

	testCtx = ctx
	testStore = sqlstore.NewSQLStore(container, botPN)
	testDB = container.Database()
	InitTables(ctx, testStore)

	os.Exit(m.Run())
}

func TestResolveDatabaseTarget(t *testing.T) {
	dialect, dsn := ResolveDatabaseTarget("")
	if dialect != "sqlite3" || !strings.Contains(dsn, "whatsrook.db") {
		t.Fatalf("expected default sqlite target, got %s: %s", dialect, dsn)
	}

	dialect, dsn = ResolveDatabaseTarget("postgres://user:pass@localhost:5432/db")
	if dialect != "postgres" || dsn != "postgres://user:pass@localhost:5432/db" {
		t.Fatalf("expected postgres dialect, got %s", dialect)
	}

	dialect, dsn = ResolveDatabaseTarget("/tmp/custom.db")
	if dialect != "sqlite3" || !strings.Contains(dsn, "custom.db") {
		t.Fatalf("expected sqlite dialect for file extension, got %s", dialect)
	}
}

func TestFiltersAndBGM(t *testing.T) {
	if err := PutFilter(testCtx, testStore, "!ping", "pong proto"); err != nil {
		t.Fatalf("PutFilter failed: %v", err)
	}

	msg, err := GetFilter(testCtx, testStore, "!ping")
	if err != nil {
		t.Fatalf("GetFilter failed: %v", err)
	}
	if msg != "pong proto" {
		t.Errorf("GetFilter expected 'pong proto', got %q", msg)
	}

	filters, err := ListFilters(testCtx, testStore)
	if err != nil {
		t.Fatalf("ListFilters failed: %v", err)
	}
	if len(filters) == 0 {
		t.Errorf("ListFilters expected non-empty result")
	}

	if err := DeleteFilter(testCtx, testStore, "!ping"); err != nil {
		t.Fatalf("DeleteFilter failed: %v", err)
	}
	msgAfter, _ := GetFilter(testCtx, testStore, "!ping")
	if msgAfter != "" {
		t.Errorf("expected empty after delete, got %q", msgAfter)
	}

	// BGM operations
	if err := PutBGM(testCtx, testStore, "intro", "audio proto"); err != nil {
		t.Fatalf("PutBGM failed: %v", err)
	}
	bgm, err := GetBGM(testCtx, testStore, "intro")
	if err != nil || bgm != "audio proto" {
		t.Fatalf("GetBGM failed: %v, got %q", err, bgm)
	}
	bgmList, err := ListBGMs(testCtx, testStore)
	if err != nil || len(bgmList) == 0 {
		t.Fatalf("ListBGMs expected non-empty result")
	}
	if err := DeleteBGM(testCtx, testStore, "intro"); err != nil {
		t.Fatalf("DeleteBGM failed: %v", err)
	}
}

func TestStickerCommands(t *testing.T) {
	sha := "abcdef1234567890"
	if err := PutStickerCmd(testCtx, testStore, sha, "kick"); err != nil {
		t.Fatalf("PutStickerCmd failed: %v", err)
	}

	cmd, err := GetStickerCmd(testCtx, testStore, sha)
	if err != nil || cmd != "kick" {
		t.Fatalf("GetStickerCmd failed: %v, got %q", err, cmd)
	}

	list, err := ListStickerCmds(testCtx, testStore)
	if err != nil || len(list) != 1 || list[0].CommandName != "kick" {
		t.Fatalf("ListStickerCmds unexpected: %v", list)
	}

	if err := DeleteStickerCmdBySHA(testCtx, testStore, sha); err != nil {
		t.Fatalf("DeleteStickerCmdBySHA failed: %v", err)
	}

	cmdAfter, _ := GetStickerCmd(testCtx, testStore, sha)
	if cmdAfter != "" {
		t.Errorf("expected empty sticker cmd after delete, got %q", cmdAfter)
	}
}

func TestBotSettings(t *testing.T) {
	if err := PutSetting(testCtx, testStore, "prefix", "."); err != nil {
		t.Fatalf("PutSetting failed: %v", err)
	}
	if err := PutSetting(testCtx, testStore, "mode_private", "true"); err != nil {
		t.Fatalf("PutSetting failed: %v", err)
	}

	val, err := GetSetting(testCtx, testStore, "prefix")
	if err != nil || val != "." {
		t.Fatalf("GetSetting failed: %v, got %q", err, val)
	}

	if err := PrewarmSettings(testCtx, testStore); err != nil {
		t.Fatalf("PrewarmSettings failed: %v", err)
	}

	prefixed, err := ListSettingsWithPrefixes(testCtx, testStore, "mode_")
	if err != nil || len(prefixed) != 1 || prefixed[0].Key != "mode_private" {
		t.Fatalf("ListSettingsWithPrefixes unexpected: %v", prefixed)
	}

	if err := DeleteSetting(testCtx, testStore, "prefix"); err != nil {
		t.Fatalf("DeleteSetting failed: %v", err)
	}
}

func TestCallMediaConfig(t *testing.T) {
	callerJID := types.NewJID("123456789", types.DefaultUserServer)
	filePath := "/path/to/media.mp3"

	if err := PutCallMediaConfig(testCtx, testStore, callerJID, CallMediaAudio, filePath); err != nil {
		t.Fatalf("PutCallMediaConfig failed: %v", err)
	}

	gotPath, err := GetCallMediaConfig(testCtx, testStore, callerJID, CallMediaAudio)
	if err != nil || gotPath != filePath {
		t.Fatalf("GetCallMediaConfig failed: %v, got %q", err, gotPath)
	}
}

func TestGroupStatsAndXP(t *testing.T) {
	chatJID := types.NewJID("group123", types.GroupServer)
	senderJID := types.NewJID("user456", types.DefaultUserServer)

	StoreGroupMessage(testCtx, testStore, chatJID, senderJID)
	StoreGroupMessage(testCtx, testStore, chatJID, senderJID)

	groupStr := chatJID.String()
	userStr := senderJID.ToNonAD().String()

	if err := AddGroupUserTTTXP(testCtx, testStore, groupStr, userStr, 50, 1, 0, 0); err != nil {
		t.Fatalf("AddGroupUserTTTXP failed: %v", err)
	}
	if err := AddGroupUserWCGXP(testCtx, testStore, groupStr, userStr, 30, 1, 1, 15); err != nil {
		t.Fatalf("AddGroupUserWCGXP failed: %v", err)
	}
	if err := AddGroupUserUnscrambleXP(testCtx, testStore, groupStr, userStr, 20, 1, 100); err != nil {
		t.Fatalf("AddGroupUserUnscrambleXP failed: %v", err)
	}

	leaderboard, err := GetGroupLeaderboard(testCtx, testStore, groupStr)
	if err != nil || len(leaderboard) != 1 {
		t.Fatalf("GetGroupLeaderboard failed: %v, len=%d", err, len(leaderboard))
	}

	entry := leaderboard[0]
	if entry.XP != 100 || entry.TTTWins != 1 || entry.WCGWins != 1 || entry.UnscrambleWins != 1 {
		t.Errorf("Unexpected leaderboard entry metrics: %+v", entry)
	}
}

func TestCachedGroupsAndNewsletters(t *testing.T) {
	ourJID := ourJIDStr(testStore)

	groupJID := types.NewJID("testgroup", types.GroupServer)
	userJID := types.NewJID("member1", types.DefaultUserServer)

	g := &GroupMetadata{
		JID:              groupJID,
		Name:             "Test Group Alpha",
		Topic:            "Discussions",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
		ParticipantCount: 1,
		Participants: []GroupParticipantMetadata{
			{
				JID:         userJID,
				DisplayName: "Member One",
				IsAdmin:     true,
			},
		},
	}

	if err := SaveCachedGroup(testCtx, testDB, ourJID, g); err != nil {
		t.Fatalf("SaveCachedGroup failed: %v", err)
	}

	groups, err := LoadAllCachedGroups(testCtx, testDB, ourJID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("LoadAllCachedGroups unexpected: %v, len=%d", err, len(groups))
	}
	if groups[0].Name != "Test Group Alpha" || len(groups[0].Participants) != 1 {
		t.Errorf("Unexpected cached group data: %+v", groups[0])
	}

	if err := DeleteCachedGroup(testCtx, testDB, ourJID, groupJID.String()); err != nil {
		t.Fatalf("DeleteCachedGroup failed: %v", err)
	}
	groupsAfter, _ := LoadAllCachedGroups(testCtx, testDB, ourJID)
	if len(groupsAfter) != 0 {
		t.Errorf("expected 0 groups after delete, got %d", len(groupsAfter))
	}

	// Newsletters
	newsJID := types.NewJID("1234567890", types.NewsletterServer)
	n := &NewsletterMetadata{
		JID:              newsJID,
		Name:             "Alpha Channel",
		Description:      "Daily updates",
		SubscribersCount: 500,
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}
	if err := SaveCachedNewsletter(testCtx, testDB, ourJID, n); err != nil {
		t.Fatalf("SaveCachedNewsletter failed: %v", err)
	}

	newsletters, err := LoadAllCachedNewsletters(testCtx, testDB, ourJID)
	if err != nil || len(newsletters) != 1 || newsletters[0].Name != "Alpha Channel" {
		t.Fatalf("LoadAllCachedNewsletters unexpected: %v", newsletters)
	}

	if err := DeleteCachedNewsletter(testCtx, testDB, ourJID, newsJID.String()); err != nil {
		t.Fatalf("DeleteCachedNewsletter failed: %v", err)
	}
}

func TestPlatformCookies(t *testing.T) {
	cookiesText := "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1799999999\tSID\tabc123\n"
	if err := PutPlatformCookie(testCtx, testStore, "youtube", ".youtube.com", cookiesText); err != nil {
		t.Fatalf("PutPlatformCookie failed: %v", err)
	}

	gotCookies, err := GetPlatformCookie(testCtx, testStore, "youtube")
	if err != nil || gotCookies != cookiesText {
		t.Fatalf("GetPlatformCookie failed: %v, got %q", err, gotCookies)
	}

	list, err := ListPlatformCookies(testCtx, testStore)
	if err != nil || len(list) != 1 || list[0].Platform != "youtube" {
		t.Fatalf("ListPlatformCookies unexpected: %v", list)
	}

	merged, err := GetAllPlatformCookiesMerged(testCtx, testStore)
	if err != nil || !strings.Contains(merged, "SID\tabc123") {
		t.Fatalf("GetAllPlatformCookiesMerged unexpected: %v", merged)
	}

	if err := DeletePlatformCookie(testCtx, testStore, "youtube"); err != nil {
		t.Fatalf("DeletePlatformCookie failed: %v", err)
	}

	emptyList, err := ListPlatformCookies(testCtx, testStore)
	if err != nil || len(emptyList) != 0 {
		t.Errorf("expected 0 cookies after delete, got %d", len(emptyList))
	}
}
