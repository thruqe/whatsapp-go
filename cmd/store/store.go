package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"whatsrook"
	"whatsrook/util/logger"

	"github.com/thruqe/duosql"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

const (
	filterCacheTTL          = 24 * time.Hour
	filterNegativeCacheTTL  = 10 * time.Minute
	settingCacheTTL         = 24 * time.Hour
	settingNegativeCacheTTL = 10 * time.Minute
	cacheSentinelNil        = "__nil__"
)

var (
	duoMu         sync.RWMutex
	duoCache      = make(map[*sql.DB]*duosql.DB)
	tablesInitMu  sync.Mutex
	tablesInitMap = make(map[*sql.DB]bool)
)

// ResolveDatabaseTarget determines the active database dialect and DSN.
// When an argument is missing or invalid, it defaults to a local SQLite database
// configured with WAL journaling and foreign key constraints enabled.
func ResolveDatabaseTarget(dbInput string) (dialect string, dsn string) {
	trimmed := strings.TrimSpace(dbInput)

	defaultSQLitePath := filepath.Join(whatsrook.DefaultDataDir(), "whatsrook.db")
	defaultSQLiteDSN := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", defaultSQLitePath)

	if trimmed == "" {
		ensureParentDir(defaultSQLitePath)
		logger.Debug("No database specified; using default SQLite database", "path", defaultSQLitePath)
		return "sqlite3", defaultSQLiteDSN
	}

	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		return "postgres", trimmed
	}

	if strings.HasPrefix(trimmed, "file:") {
		u, err := url.Parse(trimmed)
		if err == nil && u.Path != "" {
			ensureParentDir(u.Path)
		}
		return "sqlite3", trimmed
	}

	if strings.HasSuffix(trimmed, ".db") || strings.HasSuffix(trimmed, ".sqlite") || strings.HasSuffix(trimmed, ".sqlite3") {
		ensureParentDir(trimmed)
		return "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", trimmed)
	}

	if fi, err := os.Stat(trimmed); err == nil && !fi.IsDir() {
		return "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", trimmed)
	}

	ensureParentDir(defaultSQLitePath)
	logger.Warn("Database target not provided or invalid; falling back to SQLite", "input", dbInput, "fallback", defaultSQLitePath)
	return "sqlite3", defaultSQLiteDSN
}

func ensureParentDir(filePath string) {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0750)
	}
}

// ourJIDStr extracts the normalized non-AD string representation of the bot account JID.
func ourJIDStr(s *sqlstore.SQLStore) string {
	if s == nil {
		return ""
	}
	if s.JID != "" {
		if parsed, err := types.ParseJID(s.JID); err == nil && !parsed.IsEmpty() {
			return parsed.ToNonAD().String()
		}
		return s.JID
	}
	return ""
}

func filterCacheKey(ourJID, trigger string) string {
	return "filter:" + ourJID + ":" + trigger
}

func bgmCacheKey(ourJID, trigger string) string {
	return "bgm:" + ourJID + ":" + trigger
}

func stickerCacheKey(ourJID, shaHex string) string {
	return "stkcmd:" + ourJID + ":" + shaHex
}

func settingCacheKey(ourJID, key string) string {
	return "setting:" + ourJID + ":" + key
}

// sqlQuoteLiteral escapes single quotes in string constants for safe inline SQL clauses.
func sqlQuoteLiteral(val string) string {
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

// getDBFromStore extracts the raw dbutil.Database reference from a sqlstore container.
func getDBFromStore(s *sqlstore.SQLStore) (*dbutil.Database, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	db := s.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	return db, nil
}

// getDuoDBFromDB wraps the database connection in a cached duosql.DB instance,
// preserving connection pooling across all ORM query builder operations.
func getDuoDBFromDB(db *dbutil.Database) (*duosql.DB, error) {
	if db == nil || db.RawDB == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	duoMu.RLock()
	cached, ok := duoCache[db.RawDB]
	duoMu.RUnlock()
	if ok {
		return cached, nil
	}

	duoMu.Lock()
	defer duoMu.Unlock()
	if cached, ok := duoCache[db.RawDB]; ok {
		return cached, nil
	}

	var dialect duosql.Dialect
	if db.Dialect == dbutil.Postgres {
		dialect = duosql.NewPostgresDialect()
	} else {
		dialect = duosql.NewSQLiteDialect()
	}

	wrapped := duosql.Wrap(db.RawDB, dialect)
	duoCache[db.RawDB] = wrapped
	return wrapped, nil
}

// getDuoDB resolves the duosql ORM wrapper from the active WhatsApp sqlstore.
func getDuoDB(s *sqlstore.SQLStore) (*duosql.DB, error) {
	db, err := getDBFromStore(s)
	if err != nil {
		return nil, err
	}
	return getDuoDBFromDB(db)
}

// InitTables applies schema migrations idempotently per database handle and warms memory caches.
func InitTables(ctx context.Context, s *sqlstore.SQLStore) {
	if s == nil {
		return
	}
	db := s.GetDB()
	if db == nil || db.RawDB == nil {
		return
	}

	tablesInitMu.Lock()
	if tablesInitMap[db.RawDB] {
		tablesInitMu.Unlock()
		return
	}
	tablesInitMap[db.RawDB] = true
	tablesInitMu.Unlock()

	if err := RunMigrations(ctx, db); err != nil {
		logger.Error("InitTables: failed to execute schema migrations", "err", err, "dialect", db.Dialect.String())
	}
	_ = PrewarmSettings(ctx, s)
}
