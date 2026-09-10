package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"whatsrook/logger"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

var tablesInitOnce sync.Once

// TableHasColumn checks if a given column exists in a PostgreSQL database table.
func TableHasColumn(ctx context.Context, db *dbutil.Database, table, column string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("nil database")
	}

	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = $1 AND column_name = $2
		)
	`, strings.ToLower(table), strings.ToLower(column)).Scan(&exists)
	return exists, err
}

// EnsureCustomColumnExists safely adds a column to a table if it does not already exist.
func EnsureCustomColumnExists(ctx context.Context, db *dbutil.Database, table, column, colDef string) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}

	hasCol, err := TableHasColumn(ctx, db, table, column)
	if err == nil && hasCol {
		if db.Dialect == dbutil.Postgres {
			upperDef := strings.ToUpper(colDef)
			if strings.Contains(upperDef, "DEFAULT") {
				parts := strings.SplitN(upperDef, "DEFAULT", 2)
				if len(parts) == 2 {
					defaultVal := strings.TrimSpace(colDef[len(parts[0])+7:])
					_, _ = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", table, column, defaultVal))
				}
			}
			if !strings.Contains(upperDef, "NOT NULL") {
				_, _ = db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", table, column))
			}
		}
		return nil
	}

	alterCmd := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef)
	_, err = db.Exec(ctx, alterCmd)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate column") ||
			strings.Contains(errStr, "already exists") ||
			strings.Contains(errStr, "42701") { // Postgres duplicate_column code
			return nil
		}
		return err
	}
	return nil
}

// InitTables initializes and migrates all CLI bot custom database tables, indexes, and constraints.
func InitTables(ctx context.Context, s *sqlstore.SQLStore) {
	if s == nil {
		return
	}
	tablesInitOnce.Do(func() {
		db := s.GetDB()
		if db == nil {
			return
		}

		if err := RunMigrations(ctx, db); err != nil {
			logger.Error("InitTables: failed to execute schema migrations", "err", err, "dialect", db.Dialect.String())
		}
	})
}
