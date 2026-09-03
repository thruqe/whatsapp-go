package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clistore "whatsrook/cmd/store"

	_ "github.com/lib/pq"
	"go.mau.fi/util/dbutil"
)

func openTestDB(t *testing.T) *dbutil.Database {
	t.Helper()
	pgURL := os.Getenv("DATABASE_URL")
	if pgURL == "" {
		pgURL = os.Getenv("POSTGRES_TEST_URL")
	}
	if pgURL == "" {
		t.Skip("skipping PostgreSQL store test: DATABASE_URL not configured")
		return nil
	}
	if !strings.Contains(pgURL, "sslmode=") {
		if strings.Contains(pgURL, "?") {
			pgURL += "&sslmode=disable"
		} else {
			pgURL += "?sslmode=disable"
		}
	}
	rawDB, err := sql.Open("postgres", pgURL)
	if err != nil {
		t.Fatalf("failed opening test PostgreSQL DB: %v", err)
	}
	if err := rawDB.Ping(); err != nil {
		t.Fatalf("failed pinging test PostgreSQL DB: %v", err)
	}

	schemaName := fmt.Sprintf("test_schema_%d", time.Now().UnixNano())
	if _, err := rawDB.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("failed creating test schema %s: %v", schemaName, err)
	}
	if _, err := rawDB.Exec("SET search_path TO " + schemaName); err != nil {
		t.Fatalf("failed setting search_path to %s: %v", schemaName, err)
	}

	db, err := dbutil.NewWithDB(rawDB, "postgres")
	if err != nil {
		t.Fatalf("failed wrapping db with dbutil: %v", err)
	}
	t.Cleanup(func() {
		_, _ = rawDB.Exec("SET search_path TO public")
		_, _ = rawDB.Exec("DROP SCHEMA " + schemaName + " CASCADE")
		_ = db.Close()
	})
	return db
}

func TestRunMigrations_FreshDatabase(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("failed running migrations on fresh DB: %v", err)
	}

	// Verify cli_schema_version has recorded migrations
	var count int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM cli_schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("failed querying cli_schema_version: %v", err)
	}
	if count < 3 {
		t.Fatalf("expected at least 3 migrations applied, got %d", count)
	}

	// Verify all tables exist
	tables := []string{
		"bot_settings",
		"call_media_config",
		"bot_filters",
		"bot_bgm",
		"group_stats",
		"bot_sticker_cmds",
		"bot_user_xp",
		"bot_group_user_xp",
	}

	for _, table := range tables {
		exists, err := clistore.TableExists(ctx, db, table)
		if err != nil {
			t.Errorf("error checking table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist", table)
		}
	}
}

func TestRunMigrations_Idempotency(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// Run first time
	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}

	// Run second time (should be a no-op without error)
	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
}

func TestBotSettings_ConstraintsAndUpserts(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("failed running migrations: %v", err)
	}

	queryUpsert := `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value
	`

	// Test 1: Session 1 sets prefix
	if _, err := db.Exec(ctx, queryUpsert, "session1@s.whatsapp.net", "prefix", "."); err != nil {
		t.Fatalf("failed upsert for session1: %v", err)
	}

	// Test 2: Session 2 sets same key to different value without conflict
	if _, err := db.Exec(ctx, queryUpsert, "session2@s.whatsapp.net", "prefix", "!"); err != nil {
		t.Fatalf("failed upsert for session2: %v", err)
	}

	// Verify Session 1 gets '.'
	var val1 string
	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2", "session1@s.whatsapp.net", "prefix").Scan(&val1); err != nil {
		t.Fatalf("failed querying session1 setting: %v", err)
	}
	if val1 != "." {
		t.Errorf("expected session1 prefix to be '.', got %q", val1)
	}

	// Verify Session 2 gets '!'
	var val2 string
	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2", "session2@s.whatsapp.net", "prefix").Scan(&val2); err != nil {
		t.Fatalf("failed querying session2 setting: %v", err)
	}
	if val2 != "!" {
		t.Errorf("expected session2 prefix to be '!', got %q", val2)
	}

	// Test 3: Session 1 updates prefix to '?'
	if _, err := db.Exec(ctx, queryUpsert, "session1@s.whatsapp.net", "prefix", "?"); err != nil {
		t.Fatalf("failed updating session1 prefix: %v", err)
	}

	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2", "session1@s.whatsapp.net", "prefix").Scan(&val1); err != nil {
		t.Fatalf("failed querying updated session1 setting: %v", err)
	}
	if val1 != "?" {
		t.Errorf("expected updated session1 prefix to be '?', got %q", val1)
	}

	// Verify Session 2 was NOT affected
	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2", "session2@s.whatsapp.net", "prefix").Scan(&val2); err != nil {
		t.Fatalf("failed querying session2 setting after session1 update: %v", err)
	}
	if val2 != "!" {
		t.Errorf("expected session2 prefix to remain '!', got %q", val2)
	}
}

func TestLegacySchemaRepairs(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// Simulate legacy tables created with old schemas before migration
	legacySettings := `CREATE TABLE bot_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`
	if _, err := db.Exec(ctx, legacySettings); err != nil {
		t.Fatalf("failed creating legacy bot_settings: %v", err)
	}
	_, _ = db.Exec(ctx, "INSERT INTO bot_settings (key, value) VALUES ('sudoers', '123456@lid')")

	legacyCallMedia := `CREATE TABLE call_media_config (
		our_jid TEXT,
		sender TEXT NOT NULL,
		kind TEXT NOT NULL,
		file_path TEXT NOT NULL,
		PRIMARY KEY (sender, kind)
	)`
	if _, err := db.Exec(ctx, legacyCallMedia); err != nil {
		t.Fatalf("failed creating legacy call_media_config: %v", err)
	}
	_, _ = db.Exec(ctx, "INSERT INTO call_media_config (our_jid, sender, kind, file_path) VALUES ('', '234@s.whatsapp.net', 'audio', '/path/to/audio.mp3')")

	legacyXP := `CREATE TABLE bot_user_xp (
		user_jid TEXT PRIMARY KEY,
		xp INTEGER DEFAULT 0
	)`
	if _, err := db.Exec(ctx, legacyXP); err != nil {
		t.Fatalf("failed creating legacy bot_user_xp: %v", err)
	}

	// Now run migrations to repair legacy tables
	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("migration failed on legacy DB: %v", err)
	}

	// Verify our_jid column was added and populated in bot_settings
	hasOurJID, err := clistore.TableHasColumn(ctx, db, "bot_settings", "our_jid")
	if err != nil || !hasOurJID {
		t.Errorf("expected bot_settings to have our_jid column, err: %v", err)
	}

	// Verify ON CONFLICT (our_jid, key) works after repair
	queryUpsert := `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ('100000000000001@lid', 'sudoers', '123456@lid 789@lid')
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value
	`
	if _, err := db.Exec(ctx, queryUpsert); err != nil {
		t.Fatalf("ON CONFLICT (our_jid, key) failed after legacy repair: %v", err)
	}

	// Verify call_media_config column migration
	hasJID, err := clistore.TableHasColumn(ctx, db, "call_media_config", "jid")
	if err != nil || !hasJID {
		t.Errorf("expected call_media_config to have jid column, err: %v", err)
	}

	// Verify XP columns were added
	hasWCGWins, _ := clistore.TableHasColumn(ctx, db, "bot_user_xp", "wcg_wins")
	hasWCGGames, _ := clistore.TableHasColumn(ctx, db, "bot_user_xp", "wcg_games")
	hasWCGRating, _ := clistore.TableHasColumn(ctx, db, "bot_user_xp", "wcg_rating")
	if !hasWCGWins || !hasWCGGames || !hasWCGRating {
		t.Errorf("expected XP table to have wcg_wins, wcg_games, wcg_rating")
	}
}

func TestGroupStatsAndLeaderboardOperations(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("failed running migrations: %v", err)
	}

	// Test group stats increment
	statsQuery := `
		INSERT INTO group_stats (our_jid, group_jid, user_jid, date_str, msg_count)
		VALUES ($1, $2, $3, $4, 1)
		ON CONFLICT(our_jid, group_jid, user_jid, date_str) DO UPDATE SET msg_count = group_stats.msg_count + 1
	`
	if _, err := db.Exec(ctx, statsQuery, "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net", "2026-08-13"); err != nil {
		t.Fatalf("group stats insert failed: %v", err)
	}
	if _, err := db.Exec(ctx, statsQuery, "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net", "2026-08-13"); err != nil {
		t.Fatalf("group stats update failed: %v", err)
	}

	var count int
	if err := db.QueryRow(ctx, "SELECT msg_count FROM group_stats WHERE our_jid=$1 AND group_jid=$2 AND user_jid=$3 AND date_str=$4", "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net", "2026-08-13").Scan(&count); err != nil {
		t.Fatalf("querying group_stats failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected msg_count 2, got %d", count)
	}

	// Test bot_group_user_xp upsert with CASE WHEN logic
	xpQuery := `
		INSERT INTO bot_group_user_xp (our_jid, group_jid, user_jid, xp, ttt_wins, ttt_losses, ttt_draws)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(our_jid, group_jid, user_jid) DO UPDATE SET
			xp = CASE WHEN bot_group_user_xp.xp + EXCLUDED.xp < 0 THEN 0 ELSE bot_group_user_xp.xp + EXCLUDED.xp END,
			ttt_wins = bot_group_user_xp.ttt_wins + EXCLUDED.ttt_wins,
			ttt_losses = bot_group_user_xp.ttt_losses + EXCLUDED.ttt_losses,
			ttt_draws = bot_group_user_xp.ttt_draws + EXCLUDED.ttt_draws
	`
	if _, err := db.Exec(ctx, xpQuery, "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net", 50, 1, 0, 0); err != nil {
		t.Fatalf("bot_group_user_xp insert failed: %v", err)
	}
	if _, err := db.Exec(ctx, xpQuery, "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net", 25, 1, 0, 0); err != nil {
		t.Fatalf("bot_group_user_xp update failed: %v", err)
	}

	var totalXP, tttWins int
	if err := db.QueryRow(ctx, "SELECT xp, ttt_wins FROM bot_group_user_xp WHERE our_jid=$1 AND group_jid=$2 AND user_jid=$3", "our_session@s.whatsapp.net", "group1@g.us", "user1@s.whatsapp.net").Scan(&totalXP, &tttWins); err != nil {
		t.Fatalf("querying bot_group_user_xp failed: %v", err)
	}
	if totalXP != 75 || tttWins != 2 {
		t.Errorf("expected xp=75 and ttt_wins=2, got xp=%d, ttt_wins=%d", totalXP, tttWins)
	}
}

func TestGORM_Settings_Filters_BGM_Stickers_XP(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("failed running migrations: %v", err)
	}

	gdb, err := clistore.GetORMFromDB(ctx, db)
	if err != nil {
		t.Fatalf("failed getting GORM instance: %v", err)
	}
	if gdb == nil {
		t.Fatalf("expected non-nil GORM db")
	}

	// 1. Settings via GORM
	setting := clistore.BotSetting{
		OurJID: "sess1@s.whatsapp.net",
		Key:    "prefix",
		Value:  "!",
	}
	if err := gdb.Create(&setting).Error; err != nil {
		t.Fatalf("failed creating BotSetting via GORM: %v", err)
	}

	var fetched clistore.BotSetting
	if err := gdb.Where("our_jid = ? AND key = ?", "sess1@s.whatsapp.net", "prefix").First(&fetched).Error; err != nil {
		t.Fatalf("failed fetching BotSetting via GORM: %v", err)
	}
	if fetched.Value != "!" {
		t.Errorf("expected prefix '!', got %q", fetched.Value)
	}

	// 2. Filters via GORM
	filter := clistore.BotFilter{
		OurJID:       "sess1@s.whatsapp.net",
		TriggerWord:  "ping",
		MessageProto: "pong",
	}
	if err := gdb.Create(&filter).Error; err != nil {
		t.Fatalf("failed creating BotFilter via GORM: %v", err)
	}

	var filterFetched clistore.BotFilter
	if err := gdb.Where("our_jid = ? AND trigger_word = ?", "sess1@s.whatsapp.net", "ping").First(&filterFetched).Error; err != nil {
		t.Fatalf("failed fetching BotFilter: %v", err)
	}
	if filterFetched.MessageProto != "pong" {
		t.Errorf("expected filter response 'pong', got %q", filterFetched.MessageProto)
	}

	// 3. BGMs via GORM
	bgm := clistore.BotBGM{
		OurJID:       "sess1@s.whatsapp.net",
		TriggerWord:  "theme",
		MessageProto: "audio_data",
	}
	if err := gdb.Create(&bgm).Error; err != nil {
		t.Fatalf("failed creating BotBGM via GORM: %v", err)
	}

	var bgmFetched clistore.BotBGM
	if err := gdb.Where("our_jid = ? AND trigger_word = ?", "sess1@s.whatsapp.net", "theme").First(&bgmFetched).Error; err != nil {
		t.Fatalf("failed fetching BotBGM: %v", err)
	}
	if bgmFetched.MessageProto != "audio_data" {
		t.Errorf("expected bgm response 'audio_data', got %q", bgmFetched.MessageProto)
	}

	// 4. Sticker Commands via GORM
	stk := clistore.BotStickerCmd{
		OurJID:        "sess1@s.whatsapp.net",
		StickerSHA256: "aabbccdd11223344",
		CommandName:   "menu",
	}
	if err := gdb.Create(&stk).Error; err != nil {
		t.Fatalf("failed creating BotStickerCmd: %v", err)
	}

	var stkFetched clistore.BotStickerCmd
	if err := gdb.Where("our_jid = ? AND sticker_sha256 = ?", "sess1@s.whatsapp.net", "aabbccdd11223344").First(&stkFetched).Error; err != nil {
		t.Fatalf("failed fetching BotStickerCmd: %v", err)
	}
	if stkFetched.CommandName != "menu" {
		t.Errorf("expected command 'menu', got %q", stkFetched.CommandName)
	}

	// 5. XP & Leaderboard via GORM
	userXP := clistore.BotUserXP{
		OurJID:   "sess1@s.whatsapp.net",
		UserJID:  "user_one@s.whatsapp.net",
		XP:       150,
		Level:    2,
		Messages: 20,
	}
	if err := gdb.Create(&userXP).Error; err != nil {
		t.Fatalf("failed creating BotUserXP: %v", err)
	}

	var xpFetched clistore.BotUserXP
	if err := gdb.Where("our_jid = ? AND user_jid = ?", "sess1@s.whatsapp.net", "user_one@s.whatsapp.net").First(&xpFetched).Error; err != nil {
		t.Fatalf("failed fetching BotUserXP: %v", err)
	}
	if xpFetched.XP != 150 || xpFetched.Level != 2 {
		t.Errorf("expected XP 150 and level 2, got %d, %d", xpFetched.XP, xpFetched.Level)
	}
}
