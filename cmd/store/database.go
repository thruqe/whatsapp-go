package store

import (
	"context"
	"fmt"
	"sync"

	"whatsrook/logger"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	gormDBMap sync.Map // map[*sql.DB]*gorm.DB
	gormMu    sync.Mutex
)

// autoMigrateModels is the canonical schema list applied by AutoMigrateAll.
var autoMigrateModels = []any{
	&BotSetting{},
	&CallMediaConfig{},
	&BotFilter{},
	&BotBGM{},
	&BotStickerCmd{},
	&GroupStats{},
	&BotUserXP{},
	&BotGroupUserXP{},
	&CachedGroup{},
	&CachedGroupParticipant{},
	&CachedNewsletter{},
}

// GetORM retrieves or initializes the *gorm.DB instance for the given sqlstore.SQLStore.
func GetORM(ctx context.Context, s *sqlstore.SQLStore) (*gorm.DB, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	db := s.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	return GetORMFromDB(ctx, db)
}

// GetORMFromDB wraps the dbutil.Database with a GORM instance, running AutoMigrate.
func GetORMFromDB(ctx context.Context, db *dbutil.Database) (*gorm.DB, error) {
	if db == nil || db.RawDB == nil {
		return nil, fmt.Errorf("database handle or RawDB is nil")
	}

	if val, ok := gormDBMap.Load(db.RawDB); ok {
		if gdb, ok := val.(*gorm.DB); ok {
			return gdb, nil
		}
	}

	gormMu.Lock()
	defer gormMu.Unlock()

	// Re-check under lock: gorm.Open + AutoMigrate are expensive, so we only
	// hold the mutex for the (rare) first initialization of a given DB handle.
	if val, ok := gormDBMap.Load(db.RawDB); ok {
		if gdb, ok := val.(*gorm.DB); ok {
			return gdb, nil
		}
	}

	dialector := postgres.New(postgres.Config{
		Conn: db.RawDB,
	})

	gdb, err := gorm.Open(dialector, &gorm.Config{
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
		SkipDefaultTransaction:                   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open GORM database: %w", err)
	}

	// AutoMigrate all custom tables. Failures are logged but do not prevent
	// caching the handle: settings.go's hot path (GetSetting/PutSetting/etc.)
	// must not re-run gorm.Open + migrate on every call. Versioned schema
	// correctness is owned by RunMigrations/InitTables; this is a best-effort
	// net and must not contradict the hand-written DDL there.
	if err := AutoMigrateAll(ctx, gdb); err != nil {
		logger.Warn("GORM AutoMigrate encountered issue", "err", err)
	}

	gormDBMap.Store(db.RawDB, gdb)
	return gdb, nil
}

// AutoMigrateAll applies GORM schema initialization for custom tables when they do not already exist.
func AutoMigrateAll(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil GORM instance")
	}
	migrator := db.WithContext(ctx).Migrator()
	for _, m := range autoMigrateModels {
		if !migrator.HasTable(m) {
			if err := migrator.CreateTable(m); err != nil {
				return err
			}
		}
	}
	return nil
}
