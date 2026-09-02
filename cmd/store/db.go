package store

import (
	"context"
	"fmt"
	"sync"

	Logger "whatsrook/logger"

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
		return val.(*gorm.DB), nil
	}

	gormMu.Lock()
	defer gormMu.Unlock()

	// Double check after acquiring mutex
	if val, ok := gormDBMap.Load(db.RawDB); ok {
		return val.(*gorm.DB), nil
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

	// AutoMigrate all custom tables
	if err := AutoMigrateAll(ctx, gdb); err != nil {
		Logger.Warn("GORM AutoMigrate encountered issue", "err", err)
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
	models := []any{
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
	for _, m := range models {
		if !migrator.HasTable(m) {
			if err := migrator.CreateTable(m); err != nil {
				return err
			}
		}
	}
	return nil
}
