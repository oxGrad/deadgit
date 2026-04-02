package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var migrations = []struct {
	version int
	upFile  string
}{
	{1, "migrations/000001_init.up.sql"},
}

// Open opens (or creates) the SQLite database at path and runs any pending migrations.
// The parent directory is created if it does not exist.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite is single-writer
	if err := runMigrations(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return sqlDB, nil
}

func runMigrations(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER  PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var n int
		if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile(m.upFile)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", m.version, err)
		}
		if _, err := sqlDB.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			return err
		}
	}
	return nil
}
