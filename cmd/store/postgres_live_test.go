package store_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	clistore "whatsrook/cmd/store"

	_ "github.com/lib/pq"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

func TestPostgresLiveIntegration(t *testing.T) {
	connStr := os.Getenv("TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = os.Getenv("DATABASE_URL")
	}
	if connStr == "" {
		t.Skip("Skipping live PostgreSQL integration test: TEST_POSTGRES_URL / DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Log("Connecting to PostgreSQL...")
	rawDB, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to open raw Postgres connection: %v", err)
	}
	defer rawDB.Close()

	if err := rawDB.PingContext(ctx); err != nil {
		t.Logf("Ping failed: %v, retrying with sslmode=disable...", err)
		if !strings.Contains(connStr, "sslmode=") {
			connStr += "?sslmode=disable"
		}
		rawDB, err = sql.Open("postgres", connStr)
		if err != nil || rawDB.PingContext(ctx) != nil {
			t.Fatalf("failed to connect to PostgreSQL: %v", err)
		}
	}

	container := sqlstore.NewWithDB(rawDB, "postgres", nil)
	if err := container.Upgrade(ctx); err != nil {
		t.Fatalf("container.Upgrade failed: %v", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatalf("GetFirstDevice failed: %v", err)
	}
	if device.ID == nil {
		testJID := types.NewJID("100000000000001", types.DefaultUserServer)
		device.ID = &testJID
		if err := container.PutDevice(ctx, device); err != nil {
			t.Fatalf("PutDevice failed: %v", err)
		}
	}

	s := sqlstore.NewSQLStore(container, *device.ID)
	db := s.GetDB()
	if db == nil {
		t.Fatalf("GetDB returned nil")
	}

	t.Log("Running clistore migrations on PostgreSQL...")
	if err := clistore.RunMigrations(ctx, db); err != nil {
		t.Fatalf("clistore.RunMigrations failed on PostgreSQL: %v", err)
	}

	// Also call InitTables to verify idempotency
	clistore.InitTables(ctx, s)

	// ─── 1. Test PutSetting for sudoers (The exact failing case) ───
	t.Log("Testing PutSetting sudoers...")
	sudoersVal := "100000000000001@lid 123456789@lid"
	if err := clistore.PutSetting(ctx, s, "sudoers", sudoersVal); err != nil {
		t.Fatalf("PutSetting('sudoers') FAILED: %v", err)
	}
	gotSudoers, err := clistore.GetSetting(ctx, s, "sudoers")
	if err != nil {
		t.Fatalf("GetSetting('sudoers') FAILED: %v", err)
	}
	if gotSudoers != sudoersVal {
		t.Errorf("GetSetting('sudoers') = %q, expected %q", gotSudoers, sudoersVal)
	}

	// ─── 2. Test generic PutSetting & GetSetting & AD-JID persistence ───
	t.Log("Testing PutSetting prefix & BotNamePromptDismissed across AD-JID instances...")
	if err := clistore.PutSetting(ctx, s, "prefix", "!"); err != nil {
		t.Fatalf("PutSetting('prefix') FAILED: %v", err)
	}
	gotPrefix, err := clistore.GetSetting(ctx, s, "prefix")
	if err != nil || gotPrefix != "!" {
		t.Errorf("GetSetting('prefix') = %q (err: %v), expected '!'", gotPrefix, err)
	}

	// Test Dismissed Prompt persistence across AD-JID (e.g. :88) and new store instances
	adJID := types.NewADJID("100000000000002", 0, 88)
	sAD := sqlstore.NewSQLStore(container, adJID)
	if err := clistore.PutSetting(ctx, sAD, "botname_prompt_dismissed", "true"); err != nil {
		t.Fatalf("PutSetting('botname_prompt_dismissed') on AD store failed: %v", err)
	}
	// Verify reading from non-AD store
	nonADJID := types.NewJID("100000000000002", types.DefaultUserServer)
	sNonAD := sqlstore.NewSQLStore(container, nonADJID)
	val, err := clistore.GetSetting(ctx, sNonAD, "botname_prompt_dismissed")
	if err != nil || val != "true" {
		t.Errorf("GetSetting on sNonAD = %q (err: %v), expected 'true'", val, err)
	}

	// ─── 3. Test filter.go composite key query ───
	t.Log("Testing composite bot_settings (filter.go)...")
	ourJID := device.ID.ToNonAD().String()
	_, err = db.Exec(ctx, `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, 'mention_proto', $2)
		ON CONFLICT(our_jid, key) DO UPDATE SET value=excluded.value
	`, ourJID, "proto_sample_data")
	if err != nil {
		t.Fatalf("filter.go composite bot_settings insert failed: %v", err)
	}

	var mentionProto string
	err = db.QueryRow(ctx, `SELECT value FROM bot_settings WHERE our_jid=$1 AND key='mention_proto'`, ourJID).Scan(&mentionProto)
	if err != nil || mentionProto != "proto_sample_data" {
		t.Errorf("mention_proto scan failed: %q (err: %v)", mentionProto, err)
	}

	// ─── 4. Test Call Media Config ───
	t.Log("Testing PutCallMediaConfig & GetCallMediaConfig...")
	targetJID := types.NewJID("1234567890", types.DefaultUserServer)
	if err := clistore.PutCallMediaConfig(ctx, s, targetJID, clistore.CallMediaAudio, "/media/audio.opus"); err != nil {
		t.Fatalf("PutCallMediaConfig audio failed: %v", err)
	}
	gotMedia, err := clistore.GetCallMediaConfig(ctx, s, targetJID, clistore.CallMediaAudio)
	if err != nil || gotMedia != "/media/audio.opus" {
		t.Errorf("GetCallMediaConfig audio = %q (err: %v)", gotMedia, err)
	}

	// ─── 5. Test bot_filters ───
	t.Log("Testing bot_filters...")
	_, err = db.Exec(ctx, `
		INSERT INTO bot_filters (our_jid, trigger_word, message_proto)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, trigger_word) DO UPDATE SET message_proto=excluded.message_proto
	`, ourJID, "hello", "proto_hello_data")
	if err != nil {
		t.Fatalf("bot_filters insert failed: %v", err)
	}

	// ─── 6. Test bot_bgm ───
	t.Log("Testing bot_bgm...")
	_, err = db.Exec(ctx, `
		INSERT INTO bot_bgm (our_jid, trigger_word, message_proto)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, trigger_word) DO UPDATE SET message_proto=excluded.message_proto
	`, ourJID, "bgm1", "proto_bgm_data")
	if err != nil {
		t.Fatalf("bot_bgm insert failed: %v", err)
	}

	// ─── 7. Test bot_sticker_cmds ───
	t.Log("Testing bot_sticker_cmds...")
	_, err = db.Exec(ctx, `
		INSERT INTO bot_sticker_cmds (our_jid, sticker_sha256, command_name)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, sticker_sha256) DO UPDATE SET command_name=excluded.command_name
	`, ourJID, "abcdef123456", "ping")
	if err != nil {
		t.Fatalf("bot_sticker_cmds insert failed: %v", err)
	}

	// ─── 8. Test group_stats ───
	t.Log("Testing LogGroupMessage & group_stats...")
	groupJID := types.NewJID("120363025@g.us", types.GroupServer)
	senderJID := types.NewJID("2348012345678", types.DefaultUserServer)
	clistore.LogGroupMessage(ctx, s, groupJID, senderJID)
	clistore.LogGroupMessage(ctx, s, groupJID, senderJID)

	var msgCount int
	dateStr := time.Now().Format("2006-01-02")
	err = db.QueryRow(ctx, "SELECT msg_count FROM group_stats WHERE our_jid=$1 AND group_jid=$2 AND user_jid=$3 AND date_str=$4", s.JID, groupJID.String(), senderJID.String(), dateStr).Scan(&msgCount)
	if err != nil || msgCount < 2 {
		t.Errorf("group_stats msg_count = %d (err: %v), expected >= 2", msgCount, err)
	}

	// ─── 9. Test bot_group_user_xp with CASE WHEN ───
	t.Log("Testing bot_group_user_xp (tictactoe / unscramble / wcg)...")
	_, err = db.Exec(ctx, `
		INSERT INTO bot_group_user_xp (our_jid, group_jid, user_jid, xp, ttt_wins, ttt_losses, ttt_draws, wcg_wins, wcg_games, wcg_rating)
		VALUES ($1, $2, $3, 100, 1, 0, 0, 1, 1, 1050)
		ON CONFLICT(our_jid, group_jid, user_jid) DO UPDATE SET
			xp = CASE WHEN bot_group_user_xp.xp + EXCLUDED.xp < 0 THEN 0 ELSE bot_group_user_xp.xp + EXCLUDED.xp END,
			ttt_wins = bot_group_user_xp.ttt_wins + EXCLUDED.ttt_wins,
			ttt_losses = bot_group_user_xp.ttt_losses + EXCLUDED.ttt_losses,
			ttt_draws = bot_group_user_xp.ttt_draws + EXCLUDED.ttt_draws,
			wcg_wins = bot_group_user_xp.wcg_wins + EXCLUDED.wcg_wins,
			wcg_games = bot_group_user_xp.wcg_games + EXCLUDED.wcg_games,
			wcg_rating = CASE WHEN bot_group_user_xp.wcg_rating + 50 < 100 THEN 100 ELSE bot_group_user_xp.wcg_rating + 50 END
	`, s.JID, groupJID.String(), senderJID.String())
	if err != nil {
		t.Fatalf("bot_group_user_xp insert failed: %v", err)
	}

	// Query leaderboard
	rows, err := db.Query(ctx, `
		SELECT user_jid, xp, ttt_wins, ttt_losses, ttt_draws, COALESCE(wcg_wins, 0), COALESCE(wcg_games, 0), COALESCE(wcg_rating, 1000) 
		FROM bot_group_user_xp 
		WHERE our_jid=$1 AND group_jid=$2 
		ORDER BY xp DESC 
		LIMIT 10
	`, s.JID, groupJID.String())
	if err != nil {
		t.Fatalf("bot_group_user_xp query failed: %v", err)
	}
	defer rows.Close()

	var rowFound bool
	for rows.Next() {
		var uJID string
		var xp, tw, tl, td, ww, wg, wr int
		if err := rows.Scan(&uJID, &xp, &tw, &tl, &td, &ww, &wg, &wr); err != nil {
			t.Errorf("leaderboard scan failed: %v", err)
		} else if uJID == senderJID.String() {
			rowFound = true
		}
	}
	if !rowFound {
		t.Errorf("expected user row to be returned in leaderboard")
	}

	t.Log("All PostgreSQL live integration tests PASSED successfully!")
}
