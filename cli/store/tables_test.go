package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	clistore "whatsrook/cli/store"

	"go.mau.fi/util/dbutil"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *dbutil.Database {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	rawDB, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys=on")
	if err != nil {
		t.Fatalf("failed opening test SQLite DB: %v", err)
	}
	db, err := dbutil.NewWithDB(rawDB, "sqlite")
	if err != nil {
		t.Fatalf("failed wrapping db with dbutil: %v", err)
	}
	t.Cleanup(func() {
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

	// Test 1: Upsert providing our_jid explicitly (matching updated PutSetting)
	queryWithOurJID := `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value=excluded.value, our_jid=CASE WHEN excluded.our_jid != '' THEN excluded.our_jid ELSE bot_settings.our_jid END
	`
	if _, err := db.Exec(ctx, queryWithOurJID, "258256953950323@lid", "sudoers", "258256953950323@lid"); err != nil {
		t.Fatalf("failed upsert with explicit our_jid: %v", err)
	}

	// Test 2: Single-key upsert without our_jid (relies on column default '')
	querySingleKey := `
		INSERT INTO bot_settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value=excluded.value
	`
	if _, err := db.Exec(ctx, querySingleKey, "prefix", "."); err != nil {
		t.Fatalf("failed upsert with ON CONFLICT (key): %v", err)
	}

	// Update the same key
	if _, err := db.Exec(ctx, querySingleKey, "prefix", "!"); err != nil {
		t.Fatalf("failed updating with ON CONFLICT (key): %v", err)
	}

	var val string
	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE key=$1", "prefix").Scan(&val); err != nil {
		t.Fatalf("failed querying setting: %v", err)
	}
	if val != "!" {
		t.Errorf("expected '!', got %q", val)
	}

	// Test 3: Composite key upsert (matching filter.go ON CONFLICT (our_jid, key))
	queryComposite := `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, 'mention_proto', $2)
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value
	`
	if _, err := db.Exec(ctx, queryComposite, "123456@s.whatsapp.net", "proto_bytes_data"); err != nil {
		t.Fatalf("failed upsert with ON CONFLICT (our_jid, key): %v", err)
	}

	var mentionVal string
	if err := db.QueryRow(ctx, "SELECT value FROM bot_settings WHERE our_jid=$1 AND key='mention_proto'", "123456@s.whatsapp.net").Scan(&mentionVal); err != nil {
		t.Fatalf("failed querying composite setting: %v", err)
	}
	if mentionVal != "proto_bytes_data" {
		t.Errorf("expected 'proto_bytes_data', got %q", mentionVal)
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

	// Verify ON CONFLICT (key) works after repair
	queryUpsert := `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ('258256953950323@lid', 'sudoers', '123456@lid 789@lid')
		ON CONFLICT (key) DO UPDATE SET value=excluded.value
	`
	if _, err := db.Exec(ctx, queryUpsert); err != nil {
		t.Fatalf("ON CONFLICT (key) failed after legacy repair: %v", err)
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
		INSERT INTO group_stats (group_jid, user_jid, date_str, msg_count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT(group_jid, user_jid, date_str) DO UPDATE SET msg_count = group_stats.msg_count + 1
	`
	if _, err := db.Exec(ctx, statsQuery, "group1@g.us", "user1@s.whatsapp.net", "2026-08-13"); err != nil {
		t.Fatalf("group stats insert failed: %v", err)
	}
	if _, err := db.Exec(ctx, statsQuery, "group1@g.us", "user1@s.whatsapp.net", "2026-08-13"); err != nil {
		t.Fatalf("group stats update failed: %v", err)
	}

	var count int
	if err := db.QueryRow(ctx, "SELECT msg_count FROM group_stats WHERE group_jid=$1 AND user_jid=$2 AND date_str=$3", "group1@g.us", "user1@s.whatsapp.net", "2026-08-13").Scan(&count); err != nil {
		t.Fatalf("querying group_stats failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected msg_count 2, got %d", count)
	}

	// Test bot_group_user_xp upsert with CASE WHEN logic
	xpQuery := `
		INSERT INTO bot_group_user_xp (group_jid, user_jid, xp, ttt_wins, ttt_losses, ttt_draws)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(group_jid, user_jid) DO UPDATE SET
			xp = CASE WHEN bot_group_user_xp.xp + EXCLUDED.xp < 0 THEN 0 ELSE bot_group_user_xp.xp + EXCLUDED.xp END,
			ttt_wins = bot_group_user_xp.ttt_wins + EXCLUDED.ttt_wins,
			ttt_losses = bot_group_user_xp.ttt_losses + EXCLUDED.ttt_losses,
			ttt_draws = bot_group_user_xp.ttt_draws + EXCLUDED.ttt_draws
	`
	if _, err := db.Exec(ctx, xpQuery, "group1@g.us", "user1@s.whatsapp.net", 50, 1, 0, 0); err != nil {
		t.Fatalf("bot_group_user_xp insert failed: %v", err)
	}
	if _, err := db.Exec(ctx, xpQuery, "group1@g.us", "user1@s.whatsapp.net", 25, 1, 0, 0); err != nil {
		t.Fatalf("bot_group_user_xp update failed: %v", err)
	}

	var totalXP, tttWins int
	if err := db.QueryRow(ctx, "SELECT xp, ttt_wins FROM bot_group_user_xp WHERE group_jid=$1 AND user_jid=$2", "group1@g.us", "user1@s.whatsapp.net").Scan(&totalXP, &tttWins); err != nil {
		t.Fatalf("querying bot_group_user_xp failed: %v", err)
	}
	if totalXP != 75 || tttWins != 2 {
		t.Errorf("expected xp=75 and ttt_wins=2, got xp=%d, ttt_wins=%d", totalXP, tttWins)
	}
}
