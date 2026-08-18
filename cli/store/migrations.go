package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mau.fi/util/dbutil"
)

// Migration represents an incremental database schema migration step.
type Migration struct {
	Version     int
	Description string
	Up          func(ctx context.Context, db *dbutil.Database) error
}

const (
	createSchemaVersionTableQuery = `
		CREATE TABLE IF NOT EXISTS cli_schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			description TEXT NOT NULL DEFAULT ''
		);
	`
	getAppliedVersionsQuery = `SELECT version FROM cli_schema_version ORDER BY version ASC`
	recordVersionQuery      = `INSERT INTO cli_schema_version (version, applied_at, description) VALUES ($1, $2, $3)`
)

// getMigrations returns all registered migrations in ascending version order.
func getMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "Initialize base custom bot tables with primary keys and constraints",
			Up:          migration1InitialSchema,
		},
		{
			Version:     2,
			Description: "Repair PostgreSQL and SQLite constraints, column types, and unique indexes",
			Up:          migration2RepairConstraintsAndColumns,
		},
		{
			Version:     3,
			Description: "Add performance and indexing optimization for stats, leaderboard, and stickers",
			Up:          migration3PerformanceIndexes,
		},
		{
			Version:     4,
			Description: "Ensure unique index on call_media_config(jid, kind) for PostgreSQL upserts",
			Up:          migration4CallMediaUniqueIndex,
		},
		{
			Version:     5,
			Description: "Repair call_media_config updated_at default and drop not-null constraint",
			Up:          migration5RepairCallMediaDefaults,
		},
		{
			Version:     6,
			Description: "Add cached groups, communities, participants, and newsletters tables",
			Up:          migration6CachedGroupsAndChannels,
		},
	}
}

// RunMigrations applies any pending schema migrations to the database.
func RunMigrations(ctx context.Context, db *dbutil.Database) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}

	// 1. Ensure cli_schema_version table exists
	if _, err := db.Exec(ctx, createSchemaVersionTableQuery); err != nil {
		return fmt.Errorf("failed to create schema version table: %w", err)
	}

	// 2. Fetch already applied migration versions
	rows, err := db.Query(ctx, getAppliedVersionsQuery)
	if err != nil {
		return fmt.Errorf("failed to query applied schema versions: %w", err)
	}
	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err == nil {
			applied[v] = true
		}
	}
	rows.Close()

	// 3. Execute unapplied migrations in sequence
	migrations := getMigrations()
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}

		slog.Info("Applying CLI database migration...", "version", m.Version, "description", m.Description, "dialect", db.Dialect.String())
		if err := m.Up(ctx, db); err != nil {
			return fmt.Errorf("migration v%d (%s) failed: %w", m.Version, m.Description, err)
		}

		if _, err := db.Exec(ctx, recordVersionQuery, m.Version, time.Now().UTC(), m.Description); err != nil {
			return fmt.Errorf("failed to record migration v%d: %w", m.Version, err)
		}
		slog.Info("Successfully applied CLI database migration", "version", m.Version)
	}

	return nil
}

// Migration 1: Base custom bot tables.
func migration1InitialSchema(ctx context.Context, db *dbutil.Database) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS bot_settings (
			our_jid TEXT DEFAULT '',
			key     TEXT NOT NULL,
			value   TEXT NOT NULL,
			PRIMARY KEY (our_jid, key)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS bot_settings_key_idx ON bot_settings (key)`,

		`CREATE TABLE IF NOT EXISTS call_media_config (
			our_jid    TEXT DEFAULT '',
			jid        TEXT NOT NULL,
			kind       TEXT NOT NULL DEFAULT 'audio',
			file_path  TEXT NOT NULL,
			updated_at BIGINT DEFAULT 0,
			PRIMARY KEY (jid, kind)
		)`,
		`CREATE INDEX IF NOT EXISTS call_media_config_our_jid_idx ON call_media_config (our_jid)`,

		`CREATE TABLE IF NOT EXISTS bot_filters (
			our_jid       TEXT DEFAULT '',
			trigger_word  TEXT NOT NULL,
			message_proto TEXT NOT NULL,
			PRIMARY KEY (our_jid, trigger_word)
		)`,

		`CREATE TABLE IF NOT EXISTS bot_bgm (
			our_jid       TEXT DEFAULT '',
			trigger_word  TEXT NOT NULL,
			message_proto TEXT NOT NULL,
			PRIMARY KEY (our_jid, trigger_word)
		)`,

		`CREATE TABLE IF NOT EXISTS group_stats (
			group_jid TEXT NOT NULL,
			user_jid  TEXT NOT NULL,
			date_str  TEXT NOT NULL,
			msg_count INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (group_jid, user_jid, date_str)
		)`,

		`CREATE TABLE IF NOT EXISTS bot_sticker_cmds (
			our_jid        TEXT DEFAULT '',
			sticker_sha256 TEXT NOT NULL,
			command_name   TEXT NOT NULL,
			PRIMARY KEY (our_jid, sticker_sha256)
		)`,

		`CREATE TABLE IF NOT EXISTS bot_user_xp (
			user_jid   TEXT PRIMARY KEY,
			xp         INTEGER NOT NULL DEFAULT 0,
			ttt_wins   INTEGER NOT NULL DEFAULT 0,
			ttt_losses INTEGER NOT NULL DEFAULT 0,
			ttt_draws  INTEGER NOT NULL DEFAULT 0,
			wcg_wins   INTEGER NOT NULL DEFAULT 0,
			wcg_games  INTEGER NOT NULL DEFAULT 0,
			wcg_rating INTEGER NOT NULL DEFAULT 1000
		)`,

		`CREATE TABLE IF NOT EXISTS bot_group_user_xp (
			group_jid  TEXT NOT NULL,
			user_jid   TEXT NOT NULL,
			xp         INTEGER NOT NULL DEFAULT 0,
			ttt_wins   INTEGER NOT NULL DEFAULT 0,
			ttt_losses INTEGER NOT NULL DEFAULT 0,
			ttt_draws  INTEGER NOT NULL DEFAULT 0,
			wcg_wins   INTEGER NOT NULL DEFAULT 0,
			wcg_games  INTEGER NOT NULL DEFAULT 0,
			wcg_rating INTEGER NOT NULL DEFAULT 1000,
			PRIMARY KEY (group_jid, user_jid)
		)`,
	}

	for _, query := range schemas {
		if _, err := db.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed executing schema %q: %w", query, err)
		}
	}
	return nil
}

// Migration 2: Repair PostgreSQL and SQLite constraints and legacy differences.
func migration2RepairConstraintsAndColumns(ctx context.Context, db *dbutil.Database) error {
	// 1. Repair bot_settings column & defaults
	_ = EnsureCustomColumnExists(ctx, db, "bot_settings", "our_jid", "TEXT DEFAULT ''")
	if db.Dialect == dbutil.Postgres {
		_, _ = db.Exec(ctx, "ALTER TABLE bot_settings ALTER COLUMN our_jid SET DEFAULT ''")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_settings ALTER COLUMN our_jid DROP NOT NULL")
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN our_jid SET DEFAULT ''")
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN our_jid DROP NOT NULL")
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN updated_at SET DEFAULT 0")
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN updated_at DROP NOT NULL")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_filters ALTER COLUMN our_jid SET DEFAULT ''")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_filters ALTER COLUMN our_jid DROP NOT NULL")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_bgm ALTER COLUMN our_jid SET DEFAULT ''")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_bgm ALTER COLUMN our_jid DROP NOT NULL")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_sticker_cmds ALTER COLUMN our_jid SET DEFAULT ''")
		_, _ = db.Exec(ctx, "ALTER TABLE bot_sticker_cmds ALTER COLUMN our_jid DROP NOT NULL")
	}
	_, _ = db.Exec(ctx, "UPDATE bot_settings SET our_jid = '' WHERE our_jid IS NULL")

	// Ensure unique index on key so that ON CONFLICT (key) works seamlessly in Postgres and SQLite
	if _, err := db.Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS bot_settings_key_idx ON bot_settings (key)"); err != nil {
		slog.Warn("migration2: failed to create unique index on bot_settings(key)", "err", err)
	}

	// 2. Repair call_media_config column naming (sender -> jid) and defaults
	hasSender, _ := TableHasColumn(ctx, db, "call_media_config", "sender")
	hasJID, _ := TableHasColumn(ctx, db, "call_media_config", "jid")
	if hasSender && !hasJID {
		slog.Info("migration2: migrating call_media_config column sender -> jid")
		if _, err := db.Exec(ctx, "ALTER TABLE call_media_config RENAME COLUMN sender TO jid"); err != nil {
			slog.Warn("migration2: failed to rename column sender to jid, attempting fallback column add", "err", err)
			_ = EnsureCustomColumnExists(ctx, db, "call_media_config", "jid", "TEXT DEFAULT ''")
			_, _ = db.Exec(ctx, "UPDATE call_media_config SET jid = sender WHERE jid = '' OR jid IS NULL")
		}
	}
	_ = EnsureCustomColumnExists(ctx, db, "call_media_config", "our_jid", "TEXT DEFAULT ''")
	_ = EnsureCustomColumnExists(ctx, db, "call_media_config", "updated_at", "BIGINT DEFAULT 0")

	// Ensure unique index on (jid, kind) for call_media_config
	if _, err := db.Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS call_media_config_jid_kind_idx ON call_media_config (jid, kind)"); err != nil {
		slog.Warn("migration2: failed to create unique index on call_media_config(jid, kind)", "err", err)
	}

	// 3. Repair XP columns
	_ = EnsureCustomColumnExists(ctx, db, "bot_user_xp", "wcg_wins", "INTEGER DEFAULT 0")
	_ = EnsureCustomColumnExists(ctx, db, "bot_user_xp", "wcg_games", "INTEGER DEFAULT 0")
	_ = EnsureCustomColumnExists(ctx, db, "bot_user_xp", "wcg_rating", "INTEGER DEFAULT 1000")

	_ = EnsureCustomColumnExists(ctx, db, "bot_group_user_xp", "wcg_wins", "INTEGER DEFAULT 0")
	_ = EnsureCustomColumnExists(ctx, db, "bot_group_user_xp", "wcg_games", "INTEGER DEFAULT 0")
	_ = EnsureCustomColumnExists(ctx, db, "bot_group_user_xp", "wcg_rating", "INTEGER DEFAULT 1000")

	// 4. Best-effort column addition for contacts table
	_ = EnsureCustomColumnExists(ctx, db, "whatsmeow_contacts", "username", "TEXT")

	// 5. Decouple blocking foreign key constraints on SQLite legacy setups
	if db.Dialect == dbutil.SQLite {
		botSettingsSchema := `CREATE TABLE IF NOT EXISTS bot_settings (
			our_jid TEXT DEFAULT '',
			key     TEXT NOT NULL,
			value   TEXT NOT NULL,
			PRIMARY KEY (our_jid, key)
		)`
		_ = MigrateSQLiteTableRemovingFK(ctx, db, "bot_settings", botSettingsSchema, "our_jid, key, value")

		callMediaSchema := `CREATE TABLE IF NOT EXISTS call_media_config (
			our_jid    TEXT DEFAULT '',
			jid        TEXT NOT NULL,
			kind       TEXT NOT NULL DEFAULT 'audio',
			file_path  TEXT NOT NULL,
			updated_at BIGINT DEFAULT 0,
			PRIMARY KEY (jid, kind)
		)`
		_ = MigrateSQLiteTableRemovingFK(ctx, db, "call_media_config", callMediaSchema, "our_jid, jid, kind, file_path, updated_at")
	}

	return nil
}

// Migration 3: Performance and lookup indexes.
func migration3PerformanceIndexes(ctx context.Context, db *dbutil.Database) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS group_stats_date_idx ON group_stats (date_str)",
		"CREATE INDEX IF NOT EXISTS group_stats_user_idx ON group_stats (group_jid, user_jid)",
		"CREATE INDEX IF NOT EXISTS bot_group_user_xp_leaderboard_idx ON bot_group_user_xp (group_jid, xp DESC)",
		"CREATE INDEX IF NOT EXISTS bot_sticker_cmds_name_idx ON bot_sticker_cmds (our_jid, command_name)",
		"CREATE INDEX IF NOT EXISTS call_media_config_our_jid_idx ON call_media_config (our_jid)",
	}

	for _, idxQuery := range indexes {
		if _, err := db.Exec(ctx, idxQuery); err != nil {
			slog.Warn("migration3: failed creating index", "query", idxQuery, "err", err)
		}
	}
	return nil
}

// Migration 4: Ensure call_media_config unique index on (jid, kind).
func migration4CallMediaUniqueIndex(ctx context.Context, db *dbutil.Database) error {
	_, err := db.Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS call_media_config_jid_kind_idx ON call_media_config (jid, kind)")
	return err
}

// Migration 5: Ensure call_media_config updated_at default and drop not-null constraint.
func migration5RepairCallMediaDefaults(ctx context.Context, db *dbutil.Database) error {
	if db.Dialect == dbutil.Postgres {
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN updated_at SET DEFAULT 0")
		_, _ = db.Exec(ctx, "ALTER TABLE call_media_config ALTER COLUMN updated_at DROP NOT NULL")
	}
	_, _ = db.Exec(ctx, "UPDATE call_media_config SET updated_at = 0 WHERE updated_at IS NULL")
	return nil
}

// Migration 6: Tables for caching groups, communities, participants, and newsletters.
func migration6CachedGroupsAndChannels(ctx context.Context, db *dbutil.Database) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS cached_groups (
			our_jid                   TEXT DEFAULT '',
			jid                       TEXT NOT NULL,
			name                      TEXT NOT NULL DEFAULT '',
			topic                     TEXT NOT NULL DEFAULT '',
			topic_id                  TEXT DEFAULT '',
			topic_set_at              TIMESTAMP,
			topic_set_by              TEXT DEFAULT '',
			owner_jid                 TEXT NOT NULL DEFAULT '',
			created_at                TIMESTAMP,
			is_locked                 BOOLEAN DEFAULT FALSE,
			is_announce               BOOLEAN DEFAULT FALSE,
			is_ephemeral              BOOLEAN DEFAULT FALSE,
			ephemeral_duration        INTEGER DEFAULT 0,
			membership_approval_mode  BOOLEAN DEFAULT FALSE,
			is_incognito              BOOLEAN DEFAULT FALSE,
			is_community              BOOLEAN DEFAULT FALSE,
			parent_jid                TEXT DEFAULT '',
			linked_parent_jid         TEXT DEFAULT '',
			is_default_subgroup       BOOLEAN DEFAULT FALSE,
			participant_count         INTEGER DEFAULT 0,
			admin_count               INTEGER DEFAULT 0,
			updated_at                TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (our_jid, jid)
		)`,
		`CREATE INDEX IF NOT EXISTS cached_groups_parent_idx ON cached_groups (our_jid, parent_jid)`,
		`CREATE INDEX IF NOT EXISTS cached_groups_community_idx ON cached_groups (our_jid, is_community)`,

		`CREATE TABLE IF NOT EXISTS cached_group_participants (
			our_jid        TEXT DEFAULT '',
			group_jid      TEXT NOT NULL,
			user_jid       TEXT NOT NULL,
			lid            TEXT DEFAULT '',
			is_admin       BOOLEAN DEFAULT FALSE,
			is_super_admin BOOLEAN DEFAULT FALSE,
			display_name   TEXT DEFAULT '',
			PRIMARY KEY (our_jid, group_jid, user_jid)
		)`,
		`CREATE INDEX IF NOT EXISTS cached_participants_user_idx ON cached_group_participants (our_jid, user_jid)`,

		`CREATE TABLE IF NOT EXISTS cached_newsletters (
			our_jid           TEXT DEFAULT '',
			jid               TEXT NOT NULL,
			name              TEXT NOT NULL DEFAULT '',
			description       TEXT NOT NULL DEFAULT '',
			invite_code       TEXT DEFAULT '',
			subscribers_count BIGINT DEFAULT 0,
			verification      TEXT DEFAULT '',
			role              TEXT DEFAULT '',
			mute_state        TEXT DEFAULT '',
			picture_url       TEXT DEFAULT '',
			created_at        TIMESTAMP,
			updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (our_jid, jid)
		)`,
	}

	for _, s := range schemas {
		if _, err := db.Exec(ctx, s); err != nil {
			return fmt.Errorf("failed executing schema %q: %w", s, err)
		}
	}
	return nil
}
