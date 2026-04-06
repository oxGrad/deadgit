package db_test

import (
	"path/filepath"
	"testing"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

func TestOpen_RunsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := deaddb.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected migration 1 to be applied, got count=%d", n)
	}
}

func TestOpen_DefaultProfileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := deaddb.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sqlDB.Close() //nolint:errcheck

	q := dbgen.New(sqlDB)
	ctx := t.Context()
	profile, err := q.GetDefaultProfile(ctx)
	if err != nil {
		t.Fatalf("GetDefaultProfile: %v", err)
	}
	if profile.Name != "default" {
		t.Errorf("expected profile name 'default', got %q", profile.Name)
	}
	if profile.Version != 1 {
		t.Errorf("expected version 1, got %d", profile.Version)
	}
	if profile.IsDefault != 1 {
		t.Errorf("expected is_default=1, got %d", profile.IsDefault)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := range 3 {
		sqlDB, err := deaddb.Open(path)
		if err != nil {
			t.Fatalf("Open attempt %d: %v", i, err)
		}
		sqlDB.Close() //nolint:errcheck
	}
}
