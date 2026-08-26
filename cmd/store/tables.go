package store

import (
	"context"
	"fmt"
	"strings"
	"sync"

	Logger "whatsrook/src/logger"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

var tablesInitOnce sync.Once

// TableExists checks if a given table exists in the database.
func TableExists(ctx context.Context, db *dbutil.Database, table string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("nil database")
	}

	var exists bool
	if db.Dialect == dbutil.SQLite {
		var count int
		err := db.QueryRow(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=$1", table).Scan(&count)
		return count > 0, err
	}

	// PostgreSQL / Standard SQL
	err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table).Scan(&exists)
	return exists, err
}

// TableHasColumn checks if a given column exists in a database table.
func TableHasColumn(ctx context.Context, db *dbutil.Database, table, column string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("nil database")
	}

	if db.Dialect == dbutil.SQLite {
		var count int
		// Check pragma_table_info (SQLite 3.16+) or fallback to raw query
		err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name=$1", table), column).Scan(&count)
		if err == nil {
			return count > 0, nil
		}

		// Fallback: query PRAGMA table_info directly
		rows, errQuery := db.Query(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
		if errQuery != nil {
			return false, errQuery
		}
		defer rows.Close()

		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dfltValue any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
				if strings.EqualFold(name, column) {
					return true, nil
				}
			}
		}
		return false, nil
	}

	// PostgreSQL / Standard ANSI SQL
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

// EnsureIndex creates an index if it does not already exist.
func EnsureIndex(ctx context.Context, db *dbutil.Database, indexName, table, columns string, unique bool) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	uniqueClause := ""
	if unique {
		uniqueClause = "UNIQUE "
	}
	query := fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)", uniqueClause, indexName, table, columns)
	_, err := db.Exec(ctx, query)
	return err
}

// MigrateTableRemovingFK migrates legacy tables to remove blocking foreign key constraints (backward compatibility wrapper).
func MigrateTableRemovingFK(ctx context.Context, db *dbutil.Database, tableName, createSchema, selectCols string) {
	if db == nil {
		return
	}
	if db.Dialect == dbutil.SQLite {
		_ = MigrateSQLiteTableRemovingFK(ctx, db, tableName, createSchema, selectCols)
	}
}

// MigrateSQLiteTableRemovingFK decouples SQLite tables from foreign key constraints safely.
func MigrateSQLiteTableRemovingFK(ctx context.Context, db *dbutil.Database, tableName, createSchema, selectCols string) error {
	if db == nil || db.Dialect != dbutil.SQLite {
		return nil
	}

	var tableSql string
	err := db.QueryRow(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name=$1", tableName).Scan(&tableSql)
	if err != nil {
		return nil
	}
	if !strings.Contains(strings.ToUpper(tableSql), "FOREIGN KEY") {
		return nil
	}

	Logger.Info("Migrating SQLite table to decouple from foreign key constraint...", "table", tableName)
	tempTable := tableName + "_fk_migrated"
	var newSchema string
	if strings.Contains(createSchema, "CREATE TABLE IF NOT EXISTS "+tableName) {
		newSchema = strings.Replace(createSchema, "CREATE TABLE IF NOT EXISTS "+tableName, "CREATE TABLE "+tempTable, 1)
	} else {
		newSchema = strings.Replace(createSchema, "CREATE TABLE "+tableName, "CREATE TABLE "+tempTable, 1)
	}

	// Disable foreign keys temporarily during table swap
	_, _ = db.Exec(ctx, "PRAGMA foreign_keys = OFF")
	defer func() {
		_, _ = db.Exec(ctx, "PRAGMA foreign_keys = ON")
	}()

	if _, err := db.Exec(ctx, newSchema); err != nil {
		return fmt.Errorf("failed to create temp table %s: %w", tempTable, err)
	}

	insertCmd := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) SELECT %s FROM %s", tempTable, selectCols, selectCols, tableName)
	if _, err := db.Exec(ctx, insertCmd); err != nil {
		_, _ = db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTable))
		return fmt.Errorf("failed to copy rows to %s: %w", tempTable, err)
	}

	if _, err := db.Exec(ctx, fmt.Sprintf("DROP TABLE %s", tableName)); err != nil {
		return fmt.Errorf("failed to drop table %s: %w", tableName, err)
	}

	if _, err := db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tempTable, tableName)); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tempTable, tableName, err)
	}

	return nil
}

// MigrateSQLiteTableToCompositePK migrates a legacy SQLite table if its primary key is single-column instead of composite (our_jid, ...).
func MigrateSQLiteTableToCompositePK(ctx context.Context, db *dbutil.Database, tableName, createSchema, targetCols, selectCols string) error {
	if db == nil || db.Dialect != dbutil.SQLite {
		return nil
	}

	var tableSql string
	err := db.QueryRow(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name=$1", tableName).Scan(&tableSql)
	if err != nil {
		return nil
	}
	upper := strings.ToUpper(tableSql)
	// If already composite PK with our_jid, nothing to do
	if strings.Contains(upper, "PRIMARY KEY (OUR_JID") || strings.Contains(upper, "PRIMARY KEY(OUR_JID") {
		return nil
	}

	Logger.Info("Migrating SQLite table to composite primary key...", "table", tableName)
	tempTable := tableName + "_pk_migrated"
	var newSchema string
	if strings.Contains(createSchema, "CREATE TABLE IF NOT EXISTS "+tableName) {
		newSchema = strings.Replace(createSchema, "CREATE TABLE IF NOT EXISTS "+tableName, "CREATE TABLE "+tempTable, 1)
	} else {
		newSchema = strings.Replace(createSchema, "CREATE TABLE "+tableName, "CREATE TABLE "+tempTable, 1)
	}

	_, _ = db.Exec(ctx, "PRAGMA foreign_keys = OFF")
	defer func() {
		_, _ = db.Exec(ctx, "PRAGMA foreign_keys = ON")
	}()

	if _, err := db.Exec(ctx, newSchema); err != nil {
		Logger.Error("MigrateSQLiteTableToCompositePK: failed to create temp table", "table", tempTable, "err", err, "schema", newSchema)
		return fmt.Errorf("failed to create temp table %s: %w", tempTable, err)
	}

	if selectCols == "" {
		selectCols = targetCols
	}

	// Clean up any NULL our_jid values in the old table before copy
	_, _ = db.Exec(ctx, fmt.Sprintf("UPDATE %s SET our_jid = '' WHERE our_jid IS NULL", tableName))

	insertCmd := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) SELECT %s FROM %s", tempTable, targetCols, selectCols, tableName)
	if _, err := db.Exec(ctx, insertCmd); err != nil {
		Logger.Error("MigrateSQLiteTableToCompositePK: failed to copy rows", "table", tempTable, "err", err, "cmd", insertCmd)
		_, _ = db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTable))
		return fmt.Errorf("failed to copy rows to %s: %w", tempTable, err)
	}

	if _, err := db.Exec(ctx, fmt.Sprintf("DROP TABLE %s", tableName)); err != nil {
		Logger.Error("MigrateSQLiteTableToCompositePK: failed to drop table", "table", tableName, "err", err)
		return fmt.Errorf("failed to drop table %s: %w", tableName, err)
	}

	if _, err := db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tempTable, tableName)); err != nil {
		Logger.Error("MigrateSQLiteTableToCompositePK: failed to rename table", "table", tempTable, "err", err)
		return fmt.Errorf("failed to rename %s to %s: %w", tempTable, tableName, err)
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
			Logger.Error("InitTables: failed to execute schema migrations", "err", err, "dialect", db.Dialect.String())
		}
	})
}
