# deadgit v2 Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite deadgit from a stateless single-org Azure DevOps scanner into a multi-provider (Azure + GitHub), SQLite-backed, cobra-driven CLI with configurable weighted scoring profiles and interactive TUI prompts.

**Architecture:** Cobra CLI with `org`, `profile`, and `scan` subcommands drives a provider-abstracted fetch layer (`internal/providers/azure` and `internal/providers/github`). Raw metrics are stored in SQLite (via sqlc-generated queries + embedded migrations); scores are computed at runtime from stored data + a versioned scoring profile. Interactive prompts (`charmbracelet/huh`) activate only when required flags are absent and stdout is a TTY.

**Tech Stack:** Go 1.25, cobra, charmbracelet/huh, modernc.org/sqlite, sqlc (code-gen tool), golang-migrate concept via custom embed.FS runner, tablewriter, fatih/color, uber/zap, golang.org/x/term

---

## File Map

### Created
```
cmd/root.go                              cobra root, --db / --output flags, DB init
cmd/org.go                               org add/list/remove + interactive
cmd/profile.go                           profile create/list/edit/set-default + interactive
cmd/scan.go                              scan command, fetch + score + render
internal/db/migrations/000001_init.up.sql    full schema
internal/db/migrations/000001_init.down.sql  drop all tables
internal/db/queries/orgs.sql             sqlc query source
internal/db/queries/projects.sql         sqlc query source
internal/db/queries/repos.sql            sqlc query source
internal/db/queries/profiles.sql         sqlc query source
internal/db/generated/db.go              sqlc output (checked in)
internal/db/generated/models.go          sqlc output (checked in)
internal/db/generated/orgs.sql.go        sqlc output (checked in)
internal/db/generated/projects.sql.go    sqlc output (checked in)
internal/db/generated/repos.sql.go       sqlc output (checked in)
internal/db/generated/profiles.sql.go    sqlc output (checked in)
internal/db/db.go                        Open(), embedded migration runner
internal/scoring/types.go                RepoMetrics, ScoringProfile, ScoringResult
internal/scoring/normalizer.go           NormalizeLinear, NormalizeCommitFrequency, NormalizeBranchStaleness
internal/scoring/normalizer_test.go
internal/scoring/scorer.go               Score() pure function
internal/scoring/scorer_test.go
internal/cache/ttl.go                    IsStale()
internal/cache/ttl_test.go
internal/providers/provider.go           Provider interface, Organization, Project, RepoData types
internal/providers/azure/client.go       HTTP client ported from azuredevops/client.go
internal/providers/azure/fetcher.go      Azure DevOps Provider implementation
internal/providers/azure/fetcher_test.go
internal/providers/github/client.go      GitHub HTTP client (Bearer auth)
internal/providers/github/fetcher.go     GitHub Provider implementation
internal/providers/github/fetcher_test.go
internal/output/table.go                 PrintTable with profile name+version in header
internal/output/table_test.go
internal/output/json.go                  WriteJSON with profile envelope
internal/output/csv.go                   WriteCSV with profile column
sqlc.yaml
main.go                                  updated: calls cmd.Execute()
README.md
ROADMAP.md
```

### Deleted
```
azuredevops/         (all files)
report/              (all files)
pipeline/            (all files)
cmd/cli.go
```

### Modified
```
go.mod               add cobra, huh, modernc.org/sqlite, golang.org/x/term; remove yaml.v3
PRD.md               updated to reflect v2 scope
```

---

## Task 1: Create branch and remove v1 code

**Files:** delete `azuredevops/`, `report/`, `pipeline/`, `cmd/cli.go`

- [ ] **Step 1: Create feature branch**

```bash
git checkout -b feat/v2-rewrite
```

- [ ] **Step 2: Delete v1 packages**

```bash
git rm -r azuredevops/ report/ pipeline/ cmd/cli.go
```

- [ ] **Step 3: Verify deletion**

```bash
git status
# Should show all azuredevops/, report/, pipeline/, cmd/cli.go as deleted
```

- [ ] **Step 4: Commit**

```bash
git commit -m "chore: remove v1 packages in preparation for v2 rewrite"
```

---

## Task 2: Update go.mod dependencies

**Files:** `go.mod`

- [ ] **Step 1: Add new dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/huh@latest
go get modernc.org/sqlite@latest
go get golang.org/x/term@latest
```

- [ ] **Step 2: Remove unused dependency**

```bash
go get gopkg.in/yaml.v3@none
go mod tidy
```

- [ ] **Step 3: Verify go.mod contains these lines**

```bash
grep -E "cobra|huh|sqlite|term" go.mod
# Expected:
# github.com/charmbracelet/huh v0.x.x
# github.com/spf13/cobra v1.x.x
# golang.org/x/term v0.x.x
# modernc.org/sqlite v1.x.x
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: update dependencies for v2 (cobra, huh, sqlite, term)"
```

---

## Task 3: DB migration SQL

**Files:** `internal/db/migrations/000001_init.up.sql`, `internal/db/migrations/000001_init.down.sql`

- [ ] **Step 1: Create directories**

```bash
mkdir -p internal/db/migrations internal/db/queries internal/db/generated
```

- [ ] **Step 2: Write up migration**

Create `internal/db/migrations/000001_init.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS organizations (
  id           INTEGER  PRIMARY KEY AUTOINCREMENT,
  name         TEXT     NOT NULL,
  slug         TEXT     NOT NULL UNIQUE,
  provider     TEXT     NOT NULL DEFAULT 'github',
  account_type TEXT     NOT NULL DEFAULT 'org',
  base_url     TEXT     NOT NULL DEFAULT 'https://api.github.com',
  pat_env      TEXT     NOT NULL,
  is_active    INTEGER  NOT NULL DEFAULT 1,
  last_synced  DATETIME,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id           INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name             TEXT    NOT NULL,
  external_id      TEXT,
  last_synced      DATETIME,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(org_id, name)
);

CREATE TABLE IF NOT EXISTS repositories (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name                TEXT    NOT NULL,
  remote_url          TEXT    NOT NULL,
  external_id         TEXT    UNIQUE,
  default_branch      TEXT,
  is_archived         INTEGER NOT NULL DEFAULT 0,
  is_disabled         INTEGER NOT NULL DEFAULT 0,
  last_commit_at      DATETIME,
  last_push_at        DATETIME,
  last_pr_merged_at   DATETIME,
  last_pr_created_at  DATETIME,
  commit_count_90d    INTEGER,
  active_branch_count INTEGER,
  contributor_count   INTEGER,
  last_fetched        DATETIME,
  raw_api_blob        TEXT,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, name)
);

CREATE TABLE IF NOT EXISTS scoring_profiles (
  id                       INTEGER PRIMARY KEY AUTOINCREMENT,
  name                     TEXT    NOT NULL UNIQUE,
  description              TEXT,
  version                  INTEGER NOT NULL DEFAULT 1,
  is_default               INTEGER NOT NULL DEFAULT 0,
  w_last_commit            REAL    NOT NULL DEFAULT 0.40,
  w_last_pr                REAL    NOT NULL DEFAULT 0.20,
  w_commit_frequency       REAL    NOT NULL DEFAULT 0.20,
  w_branch_staleness       REAL    NOT NULL DEFAULT 0.10,
  w_no_releases            REAL    NOT NULL DEFAULT 0.10,
  inactive_days_threshold  INTEGER NOT NULL DEFAULT 90,
  inactive_score_threshold REAL    NOT NULL DEFAULT 0.65,
  created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scoring_profile_history (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id INTEGER NOT NULL REFERENCES scoring_profiles(id) ON DELETE CASCADE,
  version    INTEGER NOT NULL,
  changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  old_values TEXT     NOT NULL,
  new_values TEXT     NOT NULL,
  changed_by TEXT     NOT NULL DEFAULT 'cli'
);

CREATE TABLE IF NOT EXISTS scan_runs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id           INTEGER REFERENCES organizations(id) ON DELETE SET NULL,
  profile_id       INTEGER REFERENCES scoring_profiles(id) ON DELETE SET NULL,
  profile_name     TEXT    NOT NULL,
  profile_version  INTEGER NOT NULL,
  profile_snapshot TEXT    NOT NULL,
  total_repos      INTEGER NOT NULL DEFAULT 0,
  inactive_count   INTEGER NOT NULL DEFAULT 0,
  scanned_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO scoring_profiles (
  name, description, version, is_default,
  w_last_commit, w_last_pr, w_commit_frequency,
  w_branch_staleness, w_no_releases,
  inactive_days_threshold, inactive_score_threshold
) VALUES (
  'default', 'Default balanced scoring profile', 1, 1,
  0.40, 0.20, 0.20, 0.10, 0.10, 90, 0.65
);
```

- [ ] **Step 3: Write down migration**

Create `internal/db/migrations/000001_init.down.sql`:

```sql
DROP TABLE IF EXISTS scan_runs;
DROP TABLE IF EXISTS scoring_profile_history;
DROP TABLE IF EXISTS scoring_profiles;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS organizations;
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/
git commit -m "feat: add initial DB migration schema"
```

---

## Task 4: sqlc configuration and query files

**Files:** `sqlc.yaml`, `internal/db/queries/*.sql`

- [ ] **Step 1: Write sqlc.yaml at repo root**

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "internal/db/queries"
    schema: "internal/db/migrations"
    gen:
      go:
        package: "db"
        out: "internal/db/generated"
        emit_json_tags: true
        emit_pointers_for_null_fields: true
        null_str: "sql"
```

- [ ] **Step 2: Write orgs.sql**

Create `internal/db/queries/orgs.sql`:

```sql
-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, provider, account_type, base_url, pat_env)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = ? LIMIT 1;

-- name: ListOrganizations :many
SELECT * FROM organizations WHERE is_active = 1 ORDER BY name;

-- name: ListAllOrganizations :many
SELECT * FROM organizations ORDER BY name;

-- name: UpdateOrganizationLastSynced :exec
UPDATE organizations SET last_synced = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeactivateOrganization :exec
UPDATE organizations SET is_active = 0 WHERE slug = ?;

-- name: DeleteOrganization :exec
DELETE FROM organizations WHERE slug = ?;
```

- [ ] **Step 3: Write projects.sql**

Create `internal/db/queries/projects.sql`:

```sql
-- name: UpsertProject :one
INSERT INTO projects (org_id, name, external_id)
VALUES (?, ?, ?)
ON CONFLICT(org_id, name) DO UPDATE SET
  external_id = excluded.external_id,
  last_synced = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListProjectsByOrg :many
SELECT * FROM projects WHERE org_id = ? ORDER BY name;

-- name: GetProjectByName :one
SELECT * FROM projects WHERE org_id = ? AND name = ? LIMIT 1;
```

- [ ] **Step 4: Write repos.sql**

Create `internal/db/queries/repos.sql`:

```sql
-- name: UpsertRepository :one
INSERT INTO repositories (
  project_id, name, remote_url, external_id, default_branch,
  is_archived, is_disabled,
  last_commit_at, last_push_at, last_pr_merged_at, last_pr_created_at,
  commit_count_90d, active_branch_count, contributor_count,
  last_fetched, raw_api_blob
) VALUES (
  ?, ?, ?, ?, ?, ?, ?,
  ?, ?, ?, ?,
  ?, ?, ?,
  CURRENT_TIMESTAMP, ?
)
ON CONFLICT(project_id, name) DO UPDATE SET
  remote_url          = excluded.remote_url,
  default_branch      = excluded.default_branch,
  is_archived         = excluded.is_archived,
  is_disabled         = excluded.is_disabled,
  last_commit_at      = excluded.last_commit_at,
  last_push_at        = excluded.last_push_at,
  last_pr_merged_at   = excluded.last_pr_merged_at,
  last_pr_created_at  = excluded.last_pr_created_at,
  commit_count_90d    = excluded.commit_count_90d,
  active_branch_count = excluded.active_branch_count,
  contributor_count   = excluded.contributor_count,
  last_fetched        = CURRENT_TIMESTAMP,
  raw_api_blob        = excluded.raw_api_blob
RETURNING *;

-- name: ListRepositoriesByProject :many
SELECT r.*, p.name AS project_name, o.slug AS org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE r.project_id = ?
ORDER BY r.name;

-- name: ListRepositoriesByOrg :many
SELECT r.*, p.name AS project_name, o.slug AS org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE o.slug = ?
ORDER BY p.name, r.name;

-- name: ListAllRepositories :many
SELECT r.*, p.name AS project_name, o.slug AS org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
ORDER BY o.slug, p.name, r.name;

-- name: ListStaleRepositories :many
SELECT r.*, p.name AS project_name, o.slug AS org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE r.last_fetched IS NULL
   OR r.last_fetched < datetime('now', '-' || ? || ' hours')
ORDER BY r.last_fetched ASC;
```

- [ ] **Step 5: Write profiles.sql**

Create `internal/db/queries/profiles.sql`:

```sql
-- name: CreateScoringProfile :one
INSERT INTO scoring_profiles (
  name, description, is_default,
  w_last_commit, w_last_pr, w_commit_frequency,
  w_branch_staleness, w_no_releases,
  inactive_days_threshold, inactive_score_threshold
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDefaultProfile :one
SELECT * FROM scoring_profiles WHERE is_default = 1 LIMIT 1;

-- name: GetProfileByName :one
SELECT * FROM scoring_profiles WHERE name = ? LIMIT 1;

-- name: ListProfiles :many
SELECT * FROM scoring_profiles ORDER BY is_default DESC, name;

-- name: UpdateProfile :one
UPDATE scoring_profiles SET
  description              = ?,
  w_last_commit            = ?,
  w_last_pr                = ?,
  w_commit_frequency       = ?,
  w_branch_staleness       = ?,
  w_no_releases            = ?,
  inactive_days_threshold  = ?,
  inactive_score_threshold = ?,
  version                  = version + 1,
  updated_at               = CURRENT_TIMESTAMP
WHERE name = ?
RETURNING *;

-- name: SetDefaultProfile :exec
UPDATE scoring_profiles SET is_default = CASE WHEN name = ? THEN 1 ELSE 0 END;

-- name: InsertProfileHistory :exec
INSERT INTO scoring_profile_history (profile_id, version, old_values, new_values, changed_by)
VALUES (?, ?, ?, ?, ?);

-- name: ListProfileHistory :many
SELECT * FROM scoring_profile_history
WHERE profile_id = ?
ORDER BY changed_at DESC;
```

- [ ] **Step 6: Commit query files and sqlc.yaml**

```bash
git add sqlc.yaml internal/db/queries/
git commit -m "feat: add sqlc config and query files"
```

---

## Task 5: Run sqlc and commit generated code

**Files:** `internal/db/generated/*.go`

- [ ] **Step 1: Install sqlc if not present**

```bash
which sqlc || go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

- [ ] **Step 2: Generate code**

```bash
sqlc generate
```

Expected: no errors, files created in `internal/db/generated/`.

- [ ] **Step 3: Verify generated files exist**

```bash
ls internal/db/generated/
# Expected: db.go  models.go  orgs.sql.go  profiles.sql.go  projects.sql.go  repos.sql.go
```

- [ ] **Step 4: Commit generated code**

```bash
git add internal/db/generated/ sqlc.yaml
git commit -m "feat: add sqlc-generated DB query layer"
```

---

## Task 6: DB package — connection and migration runner

**Files:** Create `internal/db/db.go`

- [ ] **Step 1: Write internal/db/db.go**

```go
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

var migrations = []struct {
	version int
	upFile  string
}{
	{1, "migrations/000001_init.up.sql"},
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
```

- [ ] **Step 2: Write integration test**

Create `internal/db/db_test.go`:

```go
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
	defer sqlDB.Close()

	// schema_migrations should have version 1
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
	defer sqlDB.Close()

	q := dbgen.New(sqlDB)
	profile, err := q.GetDefaultProfile(t.Context())
	if err != nil {
		t.Fatalf("GetDefaultProfile: %v", err)
	}
	if profile.Name != "default" {
		t.Errorf("expected profile name 'default', got %q", profile.Name)
	}
	if profile.Version != 1 {
		t.Errorf("expected version 1, got %d", profile.Version)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 3; i++ {
		sqlDB, err := deaddb.Open(path)
		if err != nil {
			t.Fatalf("Open attempt %d: %v", i, err)
		}
		sqlDB.Close()
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/db/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go
git commit -m "feat: add DB open + embedded migration runner"
```

---

## Task 7: Scoring types, normalizer (TDD)

**Files:** `internal/scoring/types.go`, `internal/scoring/normalizer.go`, `internal/scoring/normalizer_test.go`

- [ ] **Step 1: Write types.go**

Create `internal/scoring/types.go`:

```go
package scoring

// RepoMetrics holds normalized inputs derived from stored raw timestamps.
// Computed at runtime — never stored in DB.
type RepoMetrics struct {
	DaysSinceLastCommit float64
	DaysSinceLastPR     float64
	CommitCount90d      int
	ActiveBranchCount   int
	HasRecentRelease    bool
	IsArchived          bool
	IsDisabled          bool
}

// ScoringProfile is a pure value object derived from the DB row.
type ScoringProfile struct {
	Name                   string
	Version                int
	WLastCommit            float64
	WLastPR                float64
	WCommitFrequency       float64
	WBranchStaleness       float64
	WNoReleases            float64
	InactiveDaysThreshold  int
	InactiveScoreThreshold float64
}

// ScoreBreakdown holds per-component scores for transparency.
type ScoreBreakdown struct {
	LastCommitScore      float64 `json:"last_commit_score"`
	LastPRScore          float64 `json:"last_pr_score"`
	CommitFrequencyScore float64 `json:"commit_frequency_score"`
	BranchStalenessScore float64 `json:"branch_staleness_score"`
	ReleaseScore         float64 `json:"release_score"`
}

// ScoringResult is the final computed output. Never persisted.
type ScoringResult struct {
	TotalScore float64        `json:"total_score"`
	IsInactive bool           `json:"is_inactive"`
	Breakdown  ScoreBreakdown `json:"breakdown"`
	Reasons    []string       `json:"reasons"`
}
```

- [ ] **Step 2: Write failing normalizer tests**

Create `internal/scoring/normalizer_test.go`:

```go
package scoring_test

import (
	"testing"

	"github.com/oxGrad/deadgit/internal/scoring"
)

func TestNormalizeLinear(t *testing.T) {
	tests := []struct {
		days      float64
		threshold int
		want      float64
	}{
		{0, 90, 0.0},
		{45, 90, 0.5},
		{90, 90, 1.0},
		{200, 90, 1.0}, // capped at 1.0
		{0, 0, 0.0},    // zero threshold safe
	}
	for _, tc := range tests {
		got := scoring.NormalizeLinear(tc.days, tc.threshold)
		if got != tc.want {
			t.Errorf("NormalizeLinear(%v, %v) = %v, want %v", tc.days, tc.threshold, got, tc.want)
		}
	}
}

func TestNormalizeCommitFrequency(t *testing.T) {
	tests := []struct {
		commits   int
		threshold int
		wantHigh  bool // score >= 0.8 means "inactive-ish"
	}{
		{0, 90, true},   // zero commits → fully inactive
		{100, 90, false}, // many commits → active
		{1, 90, true},   // very few commits → inactive
	}
	for _, tc := range tests {
		got := scoring.NormalizeCommitFrequency(tc.commits, tc.threshold)
		if got < 0 || got > 1 {
			t.Errorf("NormalizeCommitFrequency(%d, %d) = %v out of range", tc.commits, tc.threshold, got)
		}
		isHigh := got >= 0.8
		if isHigh != tc.wantHigh {
			t.Errorf("NormalizeCommitFrequency(%d, %d) = %v, wantHigh=%v", tc.commits, tc.threshold, got, tc.wantHigh)
		}
	}
}

func TestNormalizeBranchStaleness(t *testing.T) {
	tests := []struct {
		branches int
		want     float64
	}{
		{0, 1.0},
		{3, 0.0},
		{10, 0.0}, // capped at 0.0
		{1, 0.6667},
	}
	for _, tc := range tests {
		got := scoring.NormalizeBranchStaleness(tc.branches)
		if got < 0 || got > 1 {
			t.Errorf("NormalizeBranchStaleness(%d) = %v out of range", tc.branches, got)
		}
		if tc.want != 0.6667 && got != tc.want {
			t.Errorf("NormalizeBranchStaleness(%d) = %v, want %v", tc.branches, got, tc.want)
		}
	}
}
```

- [ ] **Step 3: Run tests — verify FAIL**

```bash
go test ./internal/scoring/... -v
# Expected: FAIL — normalizer.go does not exist yet
```

- [ ] **Step 4: Write normalizer.go**

Create `internal/scoring/normalizer.go`:

```go
package scoring

import "math"

// NormalizeLinear maps days to 0.0–1.0. 0 days = 0.0 (active), >= threshold = 1.0 (inactive).
func NormalizeLinear(days float64, threshold int) float64 {
	if threshold <= 0 {
		return 0
	}
	return math.Min(days/float64(threshold), 1.0)
}

// NormalizeCommitFrequency returns a 0–1 inactivity score. More commits = lower score.
func NormalizeCommitFrequency(commitCount90d int, threshold int) float64 {
	if commitCount90d <= 0 {
		return 1.0
	}
	baseline := float64(threshold) / 30.0 * 4.0 // ~4 commits/month as "active"
	return math.Max(1.0-math.Min(float64(commitCount90d)/baseline, 1.0), 0.0)
}

// NormalizeBranchStaleness returns 0–1. 0 branches = 1.0 (stale), ≥3 branches = 0.0.
func NormalizeBranchStaleness(activeBranches int) float64 {
	if activeBranches <= 0 {
		return 1.0
	}
	return math.Max(1.0-float64(activeBranches)/3.0, 0.0)
}
```

- [ ] **Step 5: Run tests — verify PASS**

```bash
go test ./internal/scoring/... -run TestNormalize -v
# Expected: PASS
```

- [ ] **Step 6: Commit**

```bash
git add internal/scoring/
git commit -m "feat: add scoring types and normalizers (TDD)"
```

---

## Task 8: Scoring engine — Score() function (TDD)

**Files:** `internal/scoring/scorer.go`, `internal/scoring/scorer_test.go`

- [ ] **Step 1: Write failing scorer test**

Create `internal/scoring/scorer_test.go`:

```go
package scoring_test

import (
	"testing"

	"github.com/oxGrad/deadgit/internal/scoring"
)

var defaultProfile = scoring.ScoringProfile{
	Name:                   "default",
	Version:                1,
	WLastCommit:            0.40,
	WLastPR:                0.20,
	WCommitFrequency:       0.20,
	WBranchStaleness:       0.10,
	WNoReleases:            0.10,
	InactiveDaysThreshold:  90,
	InactiveScoreThreshold: 0.65,
}

func TestScore_ArchivedAlwaysInactive(t *testing.T) {
	metrics := scoring.RepoMetrics{IsArchived: true}
	result := scoring.Score(metrics, defaultProfile)
	if result.TotalScore != 1.0 {
		t.Errorf("archived: TotalScore = %v, want 1.0", result.TotalScore)
	}
	if !result.IsInactive {
		t.Error("archived: IsInactive should be true")
	}
	if len(result.Reasons) == 0 {
		t.Error("archived: expected at least one reason")
	}
}

func TestScore_DisabledAlwaysInactive(t *testing.T) {
	metrics := scoring.RepoMetrics{IsDisabled: true}
	result := scoring.Score(metrics, defaultProfile)
	if !result.IsInactive {
		t.Error("disabled: IsInactive should be true")
	}
}

func TestScore_ActiveRepo(t *testing.T) {
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 5,
		DaysSinceLastPR:     10,
		CommitCount90d:      30,
		ActiveBranchCount:   3,
		HasRecentRelease:    true,
	}
	result := scoring.Score(metrics, defaultProfile)
	if result.IsInactive {
		t.Errorf("active repo scored inactive: score=%v", result.TotalScore)
	}
	if result.TotalScore >= defaultProfile.InactiveScoreThreshold {
		t.Errorf("active repo score too high: %v", result.TotalScore)
	}
}

func TestScore_InactiveRepo(t *testing.T) {
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 300,
		DaysSinceLastPR:     300,
		CommitCount90d:      0,
		ActiveBranchCount:   0,
		HasRecentRelease:    false,
	}
	result := scoring.Score(metrics, defaultProfile)
	if !result.IsInactive {
		t.Errorf("inactive repo not flagged: score=%v", result.TotalScore)
	}
	if len(result.Reasons) == 0 {
		t.Error("inactive repo: expected reasons")
	}
}

func TestScore_WeightSumRespected(t *testing.T) {
	// All components at 1.0 → total = sum of weights = 1.0
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 9999,
		DaysSinceLastPR:     9999,
		CommitCount90d:      0,
		ActiveBranchCount:   0,
		HasRecentRelease:    false,
	}
	result := scoring.Score(metrics, defaultProfile)
	if result.TotalScore != 1.0 {
		t.Errorf("max score = %v, want 1.0", result.TotalScore)
	}
}
```

- [ ] **Step 2: Run to verify FAIL**

```bash
go test ./internal/scoring/... -run TestScore -v
# Expected: FAIL — scorer.go does not exist
```

- [ ] **Step 3: Write scorer.go**

Create `internal/scoring/scorer.go`:

```go
package scoring

import (
	"fmt"
	"math"
)

// Score computes an inactivity score from raw metrics and a profile.
// Pure function: no DB access, no side effects.
func Score(metrics RepoMetrics, profile ScoringProfile) ScoringResult {
	if metrics.IsArchived || metrics.IsDisabled {
		return ScoringResult{
			TotalScore: 1.0,
			IsInactive: true,
			Breakdown:  ScoreBreakdown{1, 1, 1, 1, 1},
			Reasons:    buildOverrideReasons(metrics),
		}
	}

	threshold := profile.InactiveDaysThreshold
	lastCommitScore := NormalizeLinear(metrics.DaysSinceLastCommit, threshold)
	lastPRScore := NormalizeLinear(metrics.DaysSinceLastPR, threshold)
	commitFreqScore := NormalizeCommitFrequency(metrics.CommitCount90d, threshold)
	branchScore := NormalizeBranchStaleness(metrics.ActiveBranchCount)
	releaseScore := releaseInactivityScore(metrics.HasRecentRelease)

	total := lastCommitScore*profile.WLastCommit +
		lastPRScore*profile.WLastPR +
		commitFreqScore*profile.WCommitFrequency +
		branchScore*profile.WBranchStaleness +
		releaseScore*profile.WNoReleases
	total = math.Round(total*10000) / 10000

	bd := ScoreBreakdown{
		LastCommitScore:      lastCommitScore,
		LastPRScore:          lastPRScore,
		CommitFrequencyScore: commitFreqScore,
		BranchStalenessScore: branchScore,
		ReleaseScore:         releaseScore,
	}
	return ScoringResult{
		TotalScore: total,
		IsInactive: total >= profile.InactiveScoreThreshold,
		Breakdown:  bd,
		Reasons:    buildReasons(metrics, bd),
	}
}

func releaseInactivityScore(hasRecent bool) float64 {
	if hasRecent {
		return 0.0
	}
	return 1.0
}

func buildReasons(metrics RepoMetrics, b ScoreBreakdown) []string {
	var r []string
	if b.LastCommitScore >= 0.8 {
		r = append(r, fmt.Sprintf("No commits in %.0fd", metrics.DaysSinceLastCommit))
	}
	if b.LastPRScore >= 0.8 {
		r = append(r, fmt.Sprintf("No PR activity in %.0fd", metrics.DaysSinceLastPR))
	}
	if b.CommitFrequencyScore >= 0.8 {
		r = append(r, "Commit frequency near zero (90d)")
	}
	if b.BranchStalenessScore >= 0.8 {
		r = append(r, "No active branches")
	}
	if b.ReleaseScore == 1.0 {
		r = append(r, "No recent releases")
	}
	return r
}

func buildOverrideReasons(metrics RepoMetrics) []string {
	var r []string
	if metrics.IsArchived {
		r = append(r, "Repository is archived")
	}
	if metrics.IsDisabled {
		r = append(r, "Repository is disabled")
	}
	return r
}
```

- [ ] **Step 4: Run tests — verify PASS**

```bash
go test ./internal/scoring/... -v
# Expected: all PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/scoring/scorer.go internal/scoring/scorer_test.go
git commit -m "feat: add scoring engine Score() function (TDD)"
```

---

## Task 9: Cache TTL helper (TDD)

**Files:** `internal/cache/ttl.go`, `internal/cache/ttl_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/cache/ttl_test.go`:

```go
package cache_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/internal/cache"
)

func TestIsStale_NullTime(t *testing.T) {
	if !cache.IsStale(sql.NullTime{Valid: false}, 24) {
		t.Error("null time should be stale")
	}
}

func TestIsStale_Fresh(t *testing.T) {
	fresh := sql.NullTime{Valid: true, Time: time.Now().Add(-1 * time.Hour)}
	if cache.IsStale(fresh, 24) {
		t.Error("1h old with 24h TTL should not be stale")
	}
}

func TestIsStale_Expired(t *testing.T) {
	old := sql.NullTime{Valid: true, Time: time.Now().Add(-25 * time.Hour)}
	if !cache.IsStale(old, 24) {
		t.Error("25h old with 24h TTL should be stale")
	}
}
```

- [ ] **Step 2: Run — verify FAIL**

```bash
go test ./internal/cache/... -v
# Expected: FAIL
```

- [ ] **Step 3: Write ttl.go**

Create `internal/cache/ttl.go`:

```go
package cache

import (
	"database/sql"
	"time"
)

const DefaultTTLHours = 24

// IsStale returns true if lastFetched is null or older than ttlHours.
func IsStale(lastFetched sql.NullTime, ttlHours int) bool {
	if !lastFetched.Valid {
		return true
	}
	return time.Since(lastFetched.Time) > time.Duration(ttlHours)*time.Hour
}
```

- [ ] **Step 4: Run — verify PASS**

```bash
go test ./internal/cache/... -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat: add cache TTL helper (TDD)"
```

---

## Task 10: Provider interface and shared types

**Files:** Create `internal/providers/provider.go`

- [ ] **Step 1: Create directory**

```bash
mkdir -p internal/providers/azure internal/providers/github
```

- [ ] **Step 2: Write provider.go**

Create `internal/providers/provider.go`:

```go
package providers

import "time"

// Organization mirrors the DB organizations row for provider use.
type Organization struct {
	ID          int64
	Slug        string
	Name        string
	Provider    string // "azure" | "github"
	AccountType string // "org" | "personal"
	BaseURL     string
	PatEnv      string
}

// Project mirrors the DB projects row.
type Project struct {
	ID         int64
	Name       string
	ExternalID string // azure project ID, or "" for GitHub
}

// RepoData holds raw API data for one repository.
// This is upserted to the repositories table as-is; no computed values.
type RepoData struct {
	Name               string
	RemoteURL          string
	ExternalID         string // azure_repo_id or GitHub repo node ID
	DefaultBranch      string
	IsArchived         bool
	IsDisabled         bool
	LastCommitAt       *time.Time
	LastPushAt         *time.Time
	LastPRMergedAt     *time.Time
	LastPRCreatedAt    *time.Time
	CommitCount90d     int
	ActiveBranchCount  int
	ContributorCount   int
	RawAPIBlob         string // full JSON response for reprocessing
}

// Provider is the interface both Azure and GitHub fetchers implement.
type Provider interface {
	// ListProjects returns all projects for the org.
	// GitHub fetchers return a single stub project (org slug as name).
	ListProjects(org Organization) ([]Project, error)

	// FetchRepos returns full RepoData for every repository in a project.
	// All API calls (branches, commits, PRs) happen inside this call.
	FetchRepos(org Organization, project Project) ([]RepoData, error)
}

// ProviderFor returns the correct Provider implementation based on org.Provider.
// Returns an error if the provider is unknown.
func ProviderFor(org Organization, pat string) (Provider, error) {
	switch org.Provider {
	case "azure":
		return newAzureProvider(org.BaseURL, pat), nil
	case "github":
		return newGitHubProvider(org.BaseURL, pat), nil
	default:
		return nil, fmt.Errorf("unknown provider %q for org %q", org.Provider, org.Slug)
	}
}
```

> **Note:** `newAzureProvider` and `newGitHubProvider` are constructors defined in the `azure/` and `github/` sub-packages. Add the import `"fmt"` and the two constructor calls after those packages exist. For now the file will not compile — that is fixed in Tasks 11 and 13.

- [ ] **Step 3: Add fmt import and placeholder stubs so it compiles**

Replace `provider.go` content with the same code but add the import and stubs at the bottom:

```go
package providers

import (
	"fmt"
	"time"
)

// ... (same structs as above) ...

// ProviderFor returns the correct Provider implementation.
func ProviderFor(org Organization, pat string) (Provider, error) {
	switch org.Provider {
	case "azure":
		return nil, fmt.Errorf("azure provider not yet implemented")
	case "github":
		return nil, fmt.Errorf("github provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider %q for org %q", org.Provider, org.Slug)
	}
}
```

- [ ] **Step 4: Verify it compiles**

```bash
go build ./internal/providers/...
# Expected: no errors
```

- [ ] **Step 5: Commit**

```bash
git add internal/providers/provider.go
git commit -m "feat: add Provider interface and shared types"
```

---

## Task 11: Azure HTTP client

**Files:** Create `internal/providers/azure/client.go`

- [ ] **Step 1: Write client.go** (ported from `azuredevops/client.go`, adapted for new package)

Create `internal/providers/azure/client.go`:

```go
package azure

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries     = 3
	baseBackoff    = 500 * time.Millisecond
	requestTimeout = 30 * time.Second
)

type client struct {
	http    *http.Client
	authHdr string
}

func newClient(pat string) *client {
	encoded := base64.StdEncoding.EncodeToString([]byte(":" + pat))
	return &client{
		http:    &http.Client{Timeout: requestTimeout},
		authHdr: "Basic " + encoded,
	}
}

// get fetches url, decodes JSON into out. Retries on 429 and 5xx.
func (c *client) get(url string, out interface{}) error {
	body, err := c.getRaw(url)
	if err != nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(out)
}

// getRaw fetches url and returns raw bytes. Retries on 429 and 5xx.
func (c *client) getRaw(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * baseBackoff
			time.Sleep(backoff)
		}
		body, retry, err := c.doRequest(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *client) doRequest(url string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", c.authHdr)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				time.Sleep(time.Duration(secs) * time.Second)
			}
		}
		return nil, true, fmt.Errorf("HTTP 429: rate limited")
	}
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("HTTP %d: server error", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	return body, false, err
}
```

- [ ] **Step 2: Write client test**

Create `internal/providers/azure/client_test.go`:

```go
package azure_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func Test429Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	c := newClientForTest(srv.URL)
	var out struct{ Value []interface{} }
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func Test404NoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClientForTest(srv.URL)
	_, err := c.getRaw(srv.URL)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if calls.Load() != 1 {
		t.Errorf("404 should not retry, got %d calls", calls.Load())
	}
}
```

Add `newClientForTest` as an exported-for-test helper in `internal/providers/azure/export_test.go`:

```go
package azure

// newClientForTest exposes newClient for external tests.
func newClientForTest(baseURL string) *client { return newClient("test-pat") }
```

- [ ] **Step 3: Run client tests**

```bash
go test ./internal/providers/azure/... -run TestClient -v 2>/dev/null || go test ./internal/providers/azure/... -v
# Expected: PASS (or compile errors to fix before fetcher is written)
```

- [ ] **Step 4: Commit**

```bash
git add internal/providers/azure/
git commit -m "feat: add Azure HTTP client with retry logic"
```

---

## Task 12: Azure fetcher

**Files:** `internal/providers/azure/fetcher.go`, `internal/providers/azure/fetcher_test.go`

- [ ] **Step 1: Write fetcher.go**

Create `internal/providers/azure/fetcher.go`:

```go
package azure

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oxGrad/deadgit/internal/providers"
)

const apiVersion = "api-version=7.0"

type azureProvider struct {
	baseURL string
	client  *client
}

func newAzureProvider(baseURL, pat string) providers.Provider {
	return &azureProvider{baseURL: strings.TrimRight(baseURL, "/"), client: newClient(pat)}
}

// --- types for Azure API responses ---

type azProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type azProjectList struct {
	Value             []azProject `json:"value"`
	Count             int         `json:"count"`
	ContinuationToken string      `json:"continuationToken"`
}

type azRepo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RemoteURL     string `json:"remoteUrl"`
	DefaultBranch string `json:"defaultBranch"`
	IsDisabled    bool   `json:"isDisabled"`
	Size          int    `json:"size"`
}

type azRepoList struct {
	Value []azRepo `json:"value"`
}

type azCommit struct {
	CommitID string `json:"commitId"`
	Author   struct {
		Date time.Time `json:"date"`
	} `json:"author"`
}

type azCommitList struct {
	Value []azCommit `json:"value"`
}

type azRef struct {
	Name   string `json:"name"`
	ObjectID string `json:"objectId"`
}

type azRefList struct {
	Value []azRef `json:"value"`
}

type azPRList struct {
	Count int `json:"count"`
}

// --- Provider implementation ---

func (p *azureProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	var result []providers.Project
	skip := 0
	top := 100
	for {
		url := fmt.Sprintf("%s/%s/_apis/projects?$top=%d&$skip=%d&%s",
			p.baseURL, org.Slug, top, skip, apiVersion)
		var list azProjectList
		if err := p.client.get(url, &list); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		for _, p := range list.Value {
			result = append(result, providers.Project{Name: p.Name, ExternalID: p.ID})
		}
		if len(list.Value) < top {
			break
		}
		skip += top
	}
	return result, nil
}

func (p *azureProvider) FetchRepos(org providers.Organization, project providers.Project) ([]providers.RepoData, error) {
	url := fmt.Sprintf("%s/%s/%s/_apis/git/repositories?%s",
		p.baseURL, org.Slug, project.Name, apiVersion)
	var list azRepoList
	if err := p.client.get(url, &list); err != nil {
		return nil, fmt.Errorf("list repos for %s/%s: %w", org.Slug, project.Name, err)
	}

	var result []providers.RepoData
	for _, repo := range list.Value {
		data, err := p.fetchRepoData(org, project, repo)
		if err != nil {
			// log error but continue — don't fail the whole project scan
			data = providers.RepoData{
				Name:       repo.Name,
				RemoteURL:  repo.RemoteURL,
				ExternalID: repo.ID,
				IsDisabled: repo.IsDisabled,
			}
		}
		result = append(result, data)
	}
	return result, nil
}

func (p *azureProvider) fetchRepoData(org providers.Organization, project providers.Project, repo azRepo) (providers.RepoData, error) {
	defaultBranch := normalizeBranch(repo.DefaultBranch)

	// Fetch branches
	branchURL := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?filter=heads/&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, apiVersion)
	var refs azRefList
	p.client.get(branchURL, &refs) // ignore error — best effort

	activeBranchCount := len(refs.Value)

	// Fetch last commit on default branch
	var lastCommitAt *time.Time
	if defaultBranch != "" {
		commitURL := fmt.Sprintf(
			"%s/%s/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=1&%s",
			p.baseURL, org.Slug, project.Name, repo.ID, defaultBranch, apiVersion)
		var commits azCommitList
		if err := p.client.get(commitURL, &commits); err == nil && len(commits.Value) > 0 {
			t := commits.Value[0].Author.Date
			lastCommitAt = &t
		}
	}

	// Fetch 90-day commit count
	since := time.Now().AddDate(0, 0, -90).Format(time.RFC3339)
	countURL := fmt.Sprintf(
		"%s/%s/%s/_apis/git/repositories/%s/commits?searchCriteria.fromDate=%s&searchCriteria.$top=1000&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, since, apiVersion)
	var recent azCommitList
	p.client.get(countURL, &recent) // best effort
	commitCount90d := len(recent.Value)

	// Fetch open PR count
	prURL := fmt.Sprintf(
		"%s/%s/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=active&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, apiVersion)
	var prs azPRList
	p.client.get(prURL, &prs) // best effort — count field in response

	// Build raw blob
	blob, _ := json.Marshal(map[string]interface{}{
		"repo":    repo,
		"refs":    refs,
		"commits": recent,
	})

	return providers.RepoData{
		Name:              repo.Name,
		RemoteURL:         repo.RemoteURL,
		ExternalID:        repo.ID,
		DefaultBranch:     defaultBranch,
		IsDisabled:        repo.IsDisabled,
		LastCommitAt:      lastCommitAt,
		CommitCount90d:    commitCount90d,
		ActiveBranchCount: activeBranchCount,
		RawAPIBlob:        string(blob),
	}, nil
}

func normalizeBranch(branch string) string {
	return strings.TrimPrefix(branch, "refs/heads/")
}
```

Note: Azure DevOps PR count endpoint returns a list, not a `count` field. Replace the `azPRList` struct and PR fetch with:

```go
type azPRListFull struct {
	Value []struct{} `json:"value"`
}
// In fetchRepoData:
var prs azPRListFull
p.client.get(prURL, &prs)
openPRCount := len(prs.Value)
```

Add `openPRCount` to the `RepoData` returned (add `LastPRCreatedAt` from the first PR if available).

- [ ] **Step 2: Write fetcher test**

Create `internal/providers/azure/fetcher_test.go`:

```go
package azure_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/internal/providers"
)

func TestListProjects_Pagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if page == 0 {
			page++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]string{{"id": "p1", "name": "Project1"}},
				"count": 1,
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []interface{}{},
				"count": 0,
			})
		}
	}))
	defer srv.Close()

	p := newAzureProviderForTest(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", Provider: "azure", BaseURL: srv.URL}
	projects, err := p.ListProjects(org)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "Project1" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestFetchRepos_EmptyProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"value": []interface{}{}})
	}))
	defer srv.Close()

	p := newAzureProviderForTest(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj", ExternalID: "pid"}
	repos, err := p.FetchRepos(org, proj)
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}
```

Add export helper in `internal/providers/azure/export_test.go`:

```go
package azure

func newAzureProviderForTest(baseURL, pat string) Provider {
	return newAzureProvider(baseURL, pat)
}
```

- [ ] **Step 3: Update ProviderFor in provider.go to wire Azure**

Edit `internal/providers/provider.go` — replace the azure stub:

```go
import (
	"fmt"
	"time"

	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
)

func ProviderFor(org Organization, pat string) (Provider, error) {
	switch org.Provider {
	case "azure":
		return azure.New(org.BaseURL, pat), nil
	case "github":
		return github.New(org.BaseURL, pat, org.AccountType), nil
	default:
		return nil, fmt.Errorf("unknown provider %q for org %q", org.Provider, org.Slug)
	}
}
```

Export `newAzureProvider` as `New` in `internal/providers/azure/fetcher.go`:

```go
// New creates an Azure DevOps provider.
func New(baseURL, pat string) providers.Provider {
	return newAzureProvider(baseURL, pat)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/providers/azure/... -v
# Expected: PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/providers/azure/ internal/providers/provider.go
git commit -m "feat: add Azure DevOps provider (list projects + fetch repos)"
```

---

## Task 13: GitHub HTTP client

**Files:** Create `internal/providers/github/client.go`

- [ ] **Step 1: Write client.go**

Create `internal/providers/github/client.go`:

```go
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries     = 3
	baseBackoff    = 500 * time.Millisecond
	requestTimeout = 30 * time.Second
)

type client struct {
	http    *http.Client
	authHdr string
	baseURL string
}

func newClient(baseURL, pat string) *client {
	return &client{
		http:    &http.Client{Timeout: requestTimeout},
		authHdr: "Bearer " + pat,
		baseURL: baseURL,
	}
}

func (c *client) get(url string, out interface{}) error {
	body, err := c.getRaw(url)
	if err != nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(out)
}

func (c *client) getRaw(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(math.Pow(2, float64(attempt-1))) * baseBackoff)
		}
		body, retry, err := c.doRequest(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *client) doRequest(url string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", c.authHdr)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 403 {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, _ := strconv.Atoi(ra); secs > 0 {
				time.Sleep(time.Duration(secs) * time.Second)
			}
		}
		return nil, true, fmt.Errorf("HTTP %d: rate limited", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("HTTP %d: server error", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	return body, false, err
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/providers/github/client.go
git commit -m "feat: add GitHub HTTP client"
```

---

## Task 14: GitHub fetcher

**Files:** `internal/providers/github/fetcher.go`, `internal/providers/github/fetcher_test.go`

- [ ] **Step 1: Write fetcher.go**

Create `internal/providers/github/fetcher.go`:

```go
package github

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/oxGrad/deadgit/internal/providers"
)

type ghProvider struct {
	client      *client
	accountType string // "org" | "personal"
}

// New creates a GitHub provider.
func New(baseURL, pat, accountType string) providers.Provider {
	return &ghProvider{client: newClient(baseURL, pat), accountType: accountType}
}

type ghRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	CloneURL      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
	Archived      bool      `json:"archived"`
	Disabled      bool      `json:"disabled"`
	PushedAt      time.Time `json:"pushed_at"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type ghBranch struct {
	Name string `json:"name"`
}

type ghPR struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProjects returns a single stub project for GitHub (no project concept).
func (p *ghProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	return []providers.Project{{Name: org.Slug, ExternalID: ""}}, nil
}

// FetchRepos lists all repos for the org/user and fetches metrics for each.
func (p *ghProvider) FetchRepos(org providers.Organization, project providers.Project) ([]providers.RepoData, error) {
	repos, err := p.listRepos(org)
	if err != nil {
		return nil, err
	}

	var result []providers.RepoData
	for _, repo := range repos {
		data, err := p.fetchRepoData(org, repo)
		if err != nil {
			data = providers.RepoData{
				Name:       repo.Name,
				RemoteURL:  repo.CloneURL,
				ExternalID: fmt.Sprintf("%d", repo.ID),
				IsArchived: repo.Archived,
				IsDisabled: repo.Disabled,
			}
		}
		result = append(result, data)
	}
	return result, nil
}

func (p *ghProvider) listRepos(org providers.Organization) ([]ghRepo, error) {
	var all []ghRepo
	page := 1
	for {
		var url string
		if p.accountType == "personal" {
			url = fmt.Sprintf("%s/user/repos?per_page=100&page=%d&type=owner", org.BaseURL, page)
		} else {
			url = fmt.Sprintf("%s/orgs/%s/repos?per_page=100&page=%d&type=all", org.BaseURL, org.Slug, page)
		}
		var repos []ghRepo
		if err := p.client.get(url, &repos); err != nil {
			return nil, fmt.Errorf("list repos page %d: %w", page, err)
		}
		all = append(all, repos...)
		if len(repos) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func (p *ghProvider) fetchRepoData(org providers.Organization, repo ghRepo) (providers.RepoData, error) {
	owner := org.Slug

	// Last commit on default branch
	var lastCommitAt *time.Time
	if repo.DefaultBranch != "" {
		commitURL := fmt.Sprintf("%s/repos/%s/%s/commits?sha=%s&per_page=1",
			org.BaseURL, owner, repo.Name, repo.DefaultBranch)
		var commits []ghCommit
		if err := p.client.get(commitURL, &commits); err == nil && len(commits) > 0 {
			t := commits[0].Commit.Author.Date
			lastCommitAt = &t
		}
	}

	// Branch count
	branchURL := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", org.BaseURL, owner, repo.Name)
	var branches []ghBranch
	p.client.get(branchURL, &branches) // best effort

	// Commit count in last 90 days
	since := time.Now().AddDate(0, 0, -90).Format(time.RFC3339)
	countURL := fmt.Sprintf("%s/repos/%s/%s/commits?since=%s&per_page=100",
		org.BaseURL, owner, repo.Name, since)
	var recentCommits []ghCommit
	p.client.get(countURL, &recentCommits) // best effort

	// Open PR count + last PR created
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", org.BaseURL, owner, repo.Name)
	var prs []ghPR
	p.client.get(prURL, &prs) // best effort

	var lastPRCreatedAt *time.Time
	if len(prs) > 0 {
		t := prs[0].CreatedAt
		lastPRCreatedAt = &t
	}

	// Push time from top-level repo metadata
	pushedAt := repo.PushedAt
	var lastPushAt *time.Time
	if !pushedAt.IsZero() {
		lastPushAt = &pushedAt
	}

	blob, _ := json.Marshal(map[string]interface{}{
		"repo":    repo,
		"commits": recentCommits,
	})

	return providers.RepoData{
		Name:               repo.Name,
		RemoteURL:          repo.CloneURL,
		ExternalID:         fmt.Sprintf("%d", repo.ID),
		DefaultBranch:      repo.DefaultBranch,
		IsArchived:         repo.Archived,
		IsDisabled:         repo.Disabled,
		LastCommitAt:       lastCommitAt,
		LastPushAt:         lastPushAt,
		LastPRCreatedAt:    lastPRCreatedAt,
		CommitCount90d:     len(recentCommits),
		ActiveBranchCount:  len(branches),
		RawAPIBlob:         string(blob),
	}, nil
}
```

- [ ] **Step 2: Write fetcher test**

Create `internal/providers/github/fetcher_test.go`:

```go
package github_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/internal/providers"
)

func TestListProjects_ReturnsStub(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	p := newGitHubProviderForTest(srv.URL, "token", "org")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	projects, err := p.ListProjects(org)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "myorg" {
		t.Errorf("expected stub project 'myorg', got %+v", projects)
	}
}

func TestFetchRepos_PersonalAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/user/repos" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": 1, "name": "my-repo", "clone_url": "https://github.com/user/my-repo.git",
					"default_branch": "main", "archived": false, "disabled": false, "pushed_at": "2024-01-01T00:00:00Z"},
			})
		} else {
			json.NewEncoder(w).Encode([]interface{}{})
		}
	}))
	defer srv.Close()

	p := newGitHubProviderForTest(srv.URL, "token", "personal")
	org := providers.Organization{Slug: "myuser", BaseURL: srv.URL, AccountType: "personal"}
	proj := providers.Project{Name: "myuser"}
	repos, err := p.FetchRepos(org, proj)
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "my-repo" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}
```

Add export helper:

```go
// internal/providers/github/export_test.go
package github

func newGitHubProviderForTest(baseURL, pat, accountType string) Provider { return New(baseURL, pat, accountType) }
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/providers/github/... -v
# Expected: PASS
```

- [ ] **Step 4: Commit**

```bash
git add internal/providers/github/
git commit -m "feat: add GitHub provider (org + personal account support)"
```

---

## Task 15: Output — table

**Files:** `internal/output/table.go`, `internal/output/table_test.go`

- [ ] **Step 1: Write table.go**

Create `internal/output/table.go`:

```go
package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// RepoRow is a fully computed row ready for display.
type RepoRow struct {
	OrgSlug    string
	Project    string
	Repo       string
	Score      float64
	IsInactive bool
	Reasons    []string
	Cached     bool
}

// TableOptions controls what appears in the header line.
type TableOptions struct {
	ProfileName    string
	ProfileVersion int
	OrgSlugs       []string
	HasOverrides   bool
	TotalRepos     int
	InactiveCount  int
	CachedCount    int
	FetchedCount   int
	DurationSec    float64
}

// PrintTable renders repos to w as a formatted table.
func PrintTable(w io.Writer, rows []RepoRow, opts TableOptions) {
	profileLabel := fmt.Sprintf("%s v%d", opts.ProfileName, opts.ProfileVersion)
	if opts.HasOverrides {
		profileLabel += " (overrides active)"
	}
	fmt.Fprintf(w, "\nScan Results  •  Profile: %s  •  Orgs: %s\n",
		profileLabel, strings.Join(opts.OrgSlugs, ", "))

	tbl := tablewriter.NewTable(w)
	tbl.Header([]string{"Org", "Project", "Repository", "Score", "Status", "Reasons"})

	active := color.New(color.FgGreen).SprintFunc()
	inactive := color.New(color.FgYellow).SprintFunc()

	for _, r := range rows {
		status := active("✓ active")
		if r.IsInactive {
			status = inactive("⚠ INACTIVE")
		}
		tbl.Append([]string{
			r.OrgSlug,
			r.Project,
			r.Repo,
			fmt.Sprintf("%.4f", r.Score),
			status,
			strings.Join(r.Reasons, "; "),
		})
	}
	tbl.Render()

	fmt.Fprintf(w, "Total: %d repos  •  %d inactive  •  Cached: %d  •  Fetched: %d  •  Duration: %.2fs\n",
		opts.TotalRepos, opts.InactiveCount, opts.CachedCount, opts.FetchedCount, opts.DurationSec)
}
```

- [ ] **Step 2: Write table test**

Create `internal/output/table_test.go`:

```go
package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/internal/output"
)

func TestPrintTable_ContainsProfileVersion(t *testing.T) {
	var buf bytes.Buffer
	rows := []output.RepoRow{
		{OrgSlug: "myorg", Project: "proj", Repo: "repo1", Score: 0.82, IsInactive: true, Reasons: []string{"No commits in 210d"}},
		{OrgSlug: "myorg", Project: "proj", Repo: "repo2", Score: 0.12, IsInactive: false},
	}
	opts := output.TableOptions{
		ProfileName: "default", ProfileVersion: 2,
		OrgSlugs: []string{"myorg"}, TotalRepos: 2, InactiveCount: 1,
		CachedCount: 1, FetchedCount: 1, DurationSec: 0.5,
	}
	output.PrintTable(&buf, rows, opts)
	out := buf.String()
	if !strings.Contains(out, "default v2") {
		t.Errorf("expected 'default v2' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "INACTIVE") {
		t.Errorf("expected 'INACTIVE' in output")
	}
	if !strings.Contains(out, "No commits in 210d") {
		t.Errorf("expected reason in output")
	}
}

func TestPrintTable_OverrideLabel(t *testing.T) {
	var buf bytes.Buffer
	opts := output.TableOptions{
		ProfileName: "default", ProfileVersion: 1,
		HasOverrides: true, OrgSlugs: []string{"myorg"},
	}
	output.PrintTable(&buf, nil, opts)
	if !strings.Contains(buf.String(), "(overrides active)") {
		t.Errorf("expected overrides label in output")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/output/... -run TestPrintTable -v
# Expected: PASS
```

- [ ] **Step 4: Commit**

```bash
git add internal/output/table.go internal/output/table_test.go
git commit -m "feat: add table output with profile name+version header"
```

---

## Task 16: Output — JSON and CSV

**Files:** `internal/output/json.go`, `internal/output/csv.go`

- [ ] **Step 1: Write json.go**

Create `internal/output/json.go`:

```go
package output

import (
	"encoding/json"
	"os"
	"time"
)

// JSONReport is the top-level envelope written to file.
type JSONReport struct {
	Profile        string    `json:"profile"`
	ProfileVersion int       `json:"profile_version"`
	ScannedAt      time.Time `json:"scanned_at"`
	Repos          []RepoRow `json:"repos"`
}

// WriteJSON writes repos as pretty JSON to path with profile metadata in the envelope.
func WriteJSON(path string, rows []RepoRow, profileName string, profileVersion int) error {
	report := JSONReport{
		Profile:        profileName,
		ProfileVersion: profileVersion,
		ScannedAt:      time.Now().UTC(),
		Repos:          rows,
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
```

- [ ] **Step 2: Write csv.go**

Create `internal/output/csv.go`:

```go
package output

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WriteCSV writes repos as CSV to path. Profile name+version appear as columns.
func WriteCSV(path string, rows []RepoRow, profileName string, profileVersion int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"org", "project", "repository", "score", "is_inactive", "status", "reasons", "profile", "profile_version"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		status := "active"
		if r.IsInactive {
			status = "INACTIVE"
		}
		record := []string{
			r.OrgSlug, r.Project, r.Repo,
			fmt.Sprintf("%.4f", r.Score),
			strconv.FormatBool(r.IsInactive),
			status,
			strings.Join(r.Reasons, "|"),
			profileName,
			strconv.Itoa(profileVersion),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.Error()
}
```

- [ ] **Step 3: Quick smoke test**

```bash
go build ./internal/output/...
# Expected: no errors
```

- [ ] **Step 4: Commit**

```bash
git add internal/output/json.go internal/output/csv.go
git commit -m "feat: add JSON and CSV output with profile version"
```

---

## Task 17: cmd/root.go

**Files:** Create `cmd/root.go`

- [ ] **Step 1: Write root.go**

Create `cmd/root.go`:

```go
package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var (
	dbPath     string
	outputFmt  string
	globalDB   *sql.DB
	globalQ    *dbgen.Queries
)

var rootCmd = &cobra.Command{
	Use:   "deadgit",
	Short: "Scan GitHub and Azure DevOps repositories for inactivity",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip DB init for help commands
		if cmd.Name() == "help" {
			return nil
		}
		sqlDB, err := deaddb.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		globalDB = sqlDB
		globalQ = dbgen.New(sqlDB)
		return nil
	},
}

// Execute is the main entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	if globalDB != nil {
		globalDB.Close()
	}
}

func init() {
	defaultDB := filepath.Join(mustHomeDir(), ".deadgit", "deadgit.db")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "Path to SQLite database")
	rootCmd.PersistentFlags().StringVar(&outputFmt, "output", "table", "Output format: table | json | csv")

	rootCmd.AddCommand(orgCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(scanCmd)
}

func mustHomeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}
```

- [ ] **Step 2: Stub org/profile/scan commands so it compiles**

Create minimal stubs (will be replaced in subsequent tasks):

```go
// cmd/org.go (stub)
package cmd
import "github.com/spf13/cobra"
var orgCmd = &cobra.Command{Use: "org", Short: "Manage organizations"}

// cmd/profile.go (stub)
package cmd
import "github.com/spf13/cobra"
var profileCmd = &cobra.Command{Use: "profile", Short: "Manage scoring profiles"}

// cmd/scan.go (stub)
package cmd
import "github.com/spf13/cobra"
var scanCmd = &cobra.Command{Use: "scan", Short: "Scan repositories"}
```

- [ ] **Step 3: Update main.go**

Replace `main.go` content:

```go
package main

import "github.com/oxGrad/deadgit/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 4: Verify build**

```bash
go build -o deadgit .
./deadgit --help
# Expected: usage output with org, profile, scan subcommands
```

- [ ] **Step 5: Commit**

```bash
git add cmd/root.go cmd/org.go cmd/profile.go cmd/scan.go main.go
git commit -m "feat: add cobra root command and main entry point"
```

---

## Task 18: cmd/org.go — org management with interactive fallback

**Files:** Replace stub `cmd/org.go`

- [ ] **Step 1: Write full org.go**

Replace `cmd/org.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "Manage organizations",
}

var orgAddCmd = &cobra.Command{
	Use:   "add [slug]",
	Short: "Register an organization",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOrgAdd,
}

var orgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered organizations",
	RunE:  runOrgList,
}

var orgRemoveCmd = &cobra.Command{
	Use:   "remove <slug>",
	Short: "Deactivate an organization",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgRemove,
}

var (
	orgAddName        string
	orgAddProvider    string
	orgAddType        string
	orgAddPatEnv      string
)

func init() {
	orgAddCmd.Flags().StringVar(&orgAddName, "name", "", "Display name")
	orgAddCmd.Flags().StringVar(&orgAddProvider, "provider", "github", "Provider: github | azure")
	orgAddCmd.Flags().StringVar(&orgAddType, "type", "org", "Account type: org | personal (GitHub only)")
	orgAddCmd.Flags().StringVar(&orgAddPatEnv, "pat-env", "", "Env var name holding the PAT")
	orgCmd.AddCommand(orgAddCmd, orgListCmd, orgRemoveCmd)
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func runOrgAdd(cmd *cobra.Command, args []string) error {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	// Interactive fallback for missing required values
	if isInteractive() {
		if slug == "" || orgAddName == "" || orgAddPatEnv == "" {
			if err := runOrgAddInteractive(&slug); err != nil {
				return err
			}
		}
	}

	// Validate required fields (non-interactive path)
	if slug == "" {
		return fmt.Errorf("slug is required (arg or interactive prompt)")
	}
	if orgAddPatEnv == "" {
		return fmt.Errorf("--pat-env is required")
	}
	if orgAddName == "" {
		orgAddName = slug
	}

	// Validate PAT env var is set
	pat := os.Getenv(orgAddPatEnv)
	if pat == "" {
		return fmt.Errorf("PAT env var %q is not set — set it before adding the org", orgAddPatEnv)
	}

	// Resolve base URL from provider
	baseURL := baseURLForProvider(orgAddProvider)

	ctx := context.Background()
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name:        orgAddName,
		Slug:        slug,
		Provider:    orgAddProvider,
		AccountType: orgAddType,
		BaseURL:     baseURL,
		PatEnv:      orgAddPatEnv,
	})
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	fmt.Printf("Organization %q added (id=%d, provider=%s)\n", org.Slug, org.ID, org.Provider)
	return nil
}

func runOrgAddInteractive(slug *string) error {
	var name, provider, accountType, patEnv string
	if *slug != "" {
		name = *slug
	}
	provider = orgAddProvider
	accountType = orgAddType
	patEnv = orgAddPatEnv

	fields := []huh.Field{
		huh.NewInput().Title("Slug (unique short name)").Value(slug),
		huh.NewInput().Title("Display name").Value(&name),
		huh.NewSelect[string]().
			Title("Provider").
			Options(
				huh.NewOption("GitHub", "github"),
				huh.NewOption("Azure DevOps", "azure"),
			).Value(&provider),
		huh.NewSelect[string]().
			Title("Account type").
			Options(
				huh.NewOption("Organization / Team", "org"),
				huh.NewOption("Personal account", "personal"),
			).Value(&accountType),
		huh.NewInput().Title("PAT env var name (e.g. GITHUB_PAT)").Value(&patEnv),
	}

	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}

	orgAddName = name
	orgAddProvider = provider
	orgAddType = accountType
	orgAddPatEnv = patEnv
	return nil
}

func runOrgList(cmd *cobra.Command, args []string) error {
	orgs, err := globalQ.ListAllOrganizations(context.Background())
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		fmt.Println("No organizations registered. Run: deadgit org add")
		return nil
	}
	for _, o := range orgs {
		status := "active"
		if o.IsActive == 0 {
			status = "inactive"
		}
		fmt.Printf("  %-20s  %-8s  %-10s  %-8s  pat-env=%-15s  [%s]\n",
			o.Slug, o.Provider, o.AccountType, status, o.PatEnv, o.BaseUrl)
	}
	return nil
}

func runOrgRemove(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := globalQ.DeactivateOrganization(context.Background(), slug); err != nil {
		return err
	}
	fmt.Printf("Organization %q deactivated.\n", slug)
	return nil
}

func baseURLForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "azure":
		return "https://dev.azure.com"
	default:
		return "https://api.github.com"
	}
}
```

- [ ] **Step 2: Build and smoke-test**

```bash
go build -o deadgit . && ./deadgit org --help
# Expected: add / list / remove subcommands shown
```

- [ ] **Step 3: Commit**

```bash
git add cmd/org.go
git commit -m "feat: implement org add/list/remove with interactive fallback"
```

---

## Task 19: cmd/profile.go — profile management with interactive fallback

**Files:** Replace stub `cmd/profile.go`

- [ ] **Step 1: Write full profile.go**

Replace `cmd/profile.go`:

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage scoring profiles",
}

var profileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new scoring profile",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProfileCreate,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scoring profiles",
	RunE:  runProfileList,
}

var profileEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a scoring profile (increments version)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProfileEdit,
}

var profileSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set a profile as the default",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProfileSetDefault,
}

var (
	profileDesc      string
	profileWCommit   float64 = -1
	profileWPR       float64 = -1
	profileWFreq     float64 = -1
	profileWBranch   float64 = -1
	profileWRelease  float64 = -1
	profileThreshold int     = -1
	profileScoreMin  float64 = -1
	profileIsDefault bool
)

func init() {
	for _, c := range []*cobra.Command{profileCreateCmd, profileEditCmd} {
		c.Flags().StringVar(&profileDesc, "description", "", "Profile description")
		c.Flags().Float64Var(&profileWCommit, "w-last-commit", -1, "Weight: last commit (0.0-1.0)")
		c.Flags().Float64Var(&profileWPR, "w-last-pr", -1, "Weight: last PR (0.0-1.0)")
		c.Flags().Float64Var(&profileWFreq, "w-commit-freq", -1, "Weight: commit frequency (0.0-1.0)")
		c.Flags().Float64Var(&profileWBranch, "w-branch-staleness", -1, "Weight: branch staleness (0.0-1.0)")
		c.Flags().Float64Var(&profileWRelease, "w-no-releases", -1, "Weight: no releases (0.0-1.0)")
		c.Flags().IntVar(&profileThreshold, "threshold", -1, "Inactive days threshold")
		c.Flags().Float64Var(&profileScoreMin, "score-min", -1, "Inactive score threshold (0.0-1.0)")
	}
	profileCreateCmd.Flags().BoolVar(&profileIsDefault, "default", false, "Set as default profile")
	profileCmd.AddCommand(profileCreateCmd, profileListCmd, profileEditCmd, profileSetDefaultCmd)
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	// Defaults
	wCommit, wPR, wFreq, wBranch, wRelease := 0.40, 0.20, 0.20, 0.10, 0.10
	threshold, scoreMin := 90, 0.65

	if isInteractive() && (name == "" || !cmd.Flags().Changed("w-last-commit")) {
		if err := runProfileInteractive(&name, &profileDesc, &wCommit, &wPR, &wFreq, &wBranch, &wRelease, &threshold, &scoreMin); err != nil {
			return err
		}
	} else {
		applyWeightOverrides(&wCommit, &wPR, &wFreq, &wBranch, &wRelease, &threshold, &scoreMin)
	}

	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	isDefault := 0
	if profileIsDefault {
		isDefault = 1
	}

	ctx := context.Background()
	p, err := globalQ.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name:                   name,
		Description:            &profileDesc,
		IsDefault:              int64(isDefault),
		WLastCommit:            wCommit,
		WLastPr:                wPR,
		WCommitFrequency:       wFreq,
		WBranchStaleness:       wBranch,
		WNoReleases:            wRelease,
		InactiveDaysThreshold:  int64(threshold),
		InactiveScoreThreshold: scoreMin,
	})
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	fmt.Printf("Profile %q created (v%d)\n", p.Name, p.Version)
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	profiles, err := globalQ.ListProfiles(context.Background())
	if err != nil {
		return err
	}
	for _, p := range profiles {
		def := ""
		if p.IsDefault == 1 {
			def = " [default]"
		}
		fmt.Printf("  %-20s v%-3d  commit=%.2f pr=%.2f freq=%.2f branch=%.2f release=%.2f  threshold=%dd%s\n",
			p.Name, p.Version,
			p.WLastCommit, p.WLastPr, p.WCommitFrequency, p.WBranchStaleness, p.WNoReleases,
			p.InactiveDaysThreshold, def)
	}
	return nil
}

func runProfileEdit(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	ctx := context.Background()

	if isInteractive() && name == "" {
		profiles, err := globalQ.ListProfiles(ctx)
		if err != nil {
			return err
		}
		opts := make([]huh.Option[string], len(profiles))
		for i, p := range profiles {
			opts[i] = huh.NewOption(fmt.Sprintf("%s v%d", p.Name, p.Version), p.Name)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Select profile to edit").Options(opts...).Value(&name),
		)).Run(); err != nil {
			return err
		}
	}
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	existing, err := globalQ.GetProfileByName(ctx, name)
	if err != nil {
		return fmt.Errorf("profile %q not found: %w", name, err)
	}

	wCommit := existing.WLastCommit
	wPR := existing.WLastPr
	wFreq := existing.WCommitFrequency
	wBranch := existing.WBranchStaleness
	wRelease := existing.WNoReleases
	threshold := int(existing.InactiveDaysThreshold)
	scoreMin := existing.InactiveScoreThreshold
	desc := ""
	if existing.Description != nil {
		desc = *existing.Description
	}

	applyWeightOverrides(&wCommit, &wPR, &wFreq, &wBranch, &wRelease, &threshold, &scoreMin)

	// Snapshot old values for history
	oldJSON, _ := json.Marshal(existing)

	updated, err := globalQ.UpdateProfile(ctx, dbgen.UpdateProfileParams{
		Description:            &desc,
		WLastCommit:            wCommit,
		WLastPr:                wPR,
		WCommitFrequency:       wFreq,
		WBranchStaleness:       wBranch,
		WNoReleases:            wRelease,
		InactiveDaysThreshold:  existing.InactiveDaysThreshold,
		InactiveScoreThreshold: scoreMin,
		Name:                   name,
	})
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}

	newJSON, _ := json.Marshal(updated)
	globalQ.InsertProfileHistory(ctx, dbgen.InsertProfileHistoryParams{
		ProfileID: updated.ID,
		Version:   updated.Version,
		OldValues: string(oldJSON),
		NewValues: string(newJSON),
		ChangedBy: "cli",
	})

	fmt.Printf("Profile %q updated to v%d\n", updated.Name, updated.Version)
	return nil
}

func runProfileSetDefault(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	ctx := context.Background()

	if isInteractive() && name == "" {
		profiles, err := globalQ.ListProfiles(ctx)
		if err != nil {
			return err
		}
		opts := make([]huh.Option[string], len(profiles))
		for i, p := range profiles {
			opts[i] = huh.NewOption(fmt.Sprintf("%s v%d", p.Name, p.Version), p.Name)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Select default profile").Options(opts...).Value(&name),
		)).Run(); err != nil {
			return err
		}
	}
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	if err := globalQ.SetDefaultProfile(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Default profile set to %q\n", name)
	return nil
}

func applyWeightOverrides(wCommit, wPR, wFreq, wBranch, wRelease *float64, threshold *int, scoreMin *float64) {
	if profileWCommit >= 0 {
		*wCommit = profileWCommit
	}
	if profileWPR >= 0 {
		*wPR = profileWPR
	}
	if profileWFreq >= 0 {
		*wFreq = profileWFreq
	}
	if profileWBranch >= 0 {
		*wBranch = profileWBranch
	}
	if profileWRelease >= 0 {
		*wRelease = profileWRelease
	}
	if profileThreshold >= 0 {
		*threshold = profileThreshold
	}
	if profileScoreMin >= 0 {
		*scoreMin = profileScoreMin
	}
}

func runProfileInteractive(name, desc *string, wCommit, wPR, wFreq, wBranch, wRelease *float64, threshold *int, scoreMin *float64) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Profile name").Value(name),
		huh.NewInput().Title("Description (optional)").Value(desc),
	)).Run()
	// Weight editing via interactive prompts is advanced — users can use flags for precise values.
	// Interactive mode sets name/description; weights use defaults unless overridden by flags.
}
```

- [ ] **Step 2: Build**

```bash
go build -o deadgit . && ./deadgit profile --help
# Expected: create / list / edit / set-default subcommands
```

- [ ] **Step 3: Commit**

```bash
git add cmd/profile.go
git commit -m "feat: implement profile create/list/edit/set-default with interactive fallback"
```

---

## Task 20: cmd/scan.go — full scan flow

**Files:** Replace stub `cmd/scan.go`

- [ ] **Step 1: Write full scan.go**

Replace `cmd/scan.go`:

```go
package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/cache"
	"github.com/oxGrad/deadgit/internal/output"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/scoring"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan repositories for inactivity",
	RunE:  runScan,
}

var (
	scanOrgs       []string
	scanAllOrgs    bool
	scanProfile    string
	scanRefresh    bool
	scanTTL        int
	scanOutFile    string
	scanWorkers    int
	scanWCommit    float64 = -1
	scanWPR        float64 = -1
	scanWFreq      float64 = -1
	scanWBranch    float64 = -1
	scanWRelease   float64 = -1
	scanThreshold  int     = -1
	scanScoreMin   float64 = -1
)

func init() {
	scanCmd.Flags().StringSliceVar(&scanOrgs, "org", nil, "Org slugs to scan (repeatable)")
	scanCmd.Flags().BoolVar(&scanAllOrgs, "all-orgs", false, "Scan all active orgs")
	scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scoring profile name (uses default if omitted)")
	scanCmd.Flags().BoolVar(&scanRefresh, "refresh", false, "Force re-fetch from API ignoring cache")
	scanCmd.Flags().IntVar(&scanTTL, "ttl", cache.DefaultTTLHours, "Cache TTL in hours")
	scanCmd.Flags().StringVar(&scanOutFile, "outfile", "", "Output file path (json/csv modes)")
	scanCmd.Flags().IntVar(&scanWorkers, "workers", 5, "Concurrent workers")
	scanCmd.Flags().Float64Var(&scanWCommit, "w-last-commit", -1, "Override weight: last commit")
	scanCmd.Flags().Float64Var(&scanWPR, "w-last-pr", -1, "Override weight: last PR")
	scanCmd.Flags().Float64Var(&scanWFreq, "w-commit-freq", -1, "Override weight: commit frequency")
	scanCmd.Flags().Float64Var(&scanWBranch, "w-branch-staleness", -1, "Override weight: branch staleness")
	scanCmd.Flags().Float64Var(&scanWRelease, "w-no-releases", -1, "Override weight: no releases")
	scanCmd.Flags().IntVar(&scanThreshold, "threshold", -1, "Override inactive days threshold")
	scanCmd.Flags().Float64Var(&scanScoreMin, "score-min", -1, "Override inactive score threshold")
}

func runScan(cmd *cobra.Command, args []string) error {
	start := time.Now()
	ctx := context.Background()
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	// 1. Load scoring profile
	var dbProfile dbgen.ScoringProfile
	var err error
	if scanProfile != "" {
		dbProfile, err = globalQ.GetProfileByName(ctx, scanProfile)
	} else {
		dbProfile, err = globalQ.GetDefaultProfile(ctx)
	}
	if err != nil {
		return fmt.Errorf("load scoring profile: %w", err)
	}

	profile := dbProfileToScoringProfile(dbProfile)

	// 2. Apply inline overrides (in-memory only)
	hasOverrides := applyInlineOverrides(&profile, cmd)

	// 3. Warn if weights don't sum to ~1.0
	wSum := profile.WLastCommit + profile.WLastPR + profile.WCommitFrequency +
		profile.WBranchStaleness + profile.WNoReleases
	if math.Abs(wSum-1.0) > 0.01 {
		fmt.Fprintf(os.Stderr, "warning: weights sum to %.4f (expected ~1.0)\n", wSum)
	}

	// 4. Determine orgs to scan
	orgsToScan, err := resolveOrgs(ctx, cmd)
	if err != nil {
		return err
	}
	if len(orgsToScan) == 0 {
		return fmt.Errorf("no orgs to scan — use --org <slug> or --all-orgs")
	}

	// 5. Collect all repos from DB + fetch stale ones
	type repoJob struct {
		org     dbgen.Organization
		project dbgen.Project
		repo    dbgen.ListRepositoriesByOrgRow
		isStale bool
	}

	var allJobs []repoJob
	for _, org := range orgsToScan {
		projects, err := globalQ.ListProjectsByOrg(ctx, org.ID)
		if err != nil {
			log.Warn("list projects failed", zap.String("org", org.Slug), zap.Error(err))
			continue
		}
		for _, proj := range projects {
			repos, err := globalQ.ListRepositoriesByOrg(ctx, org.Slug)
			if err != nil {
				log.Warn("list repos failed", zap.String("project", proj.Name), zap.Error(err))
				continue
			}
			for _, r := range repos {
				stale := scanRefresh || cache.IsStale(r.LastFetched, scanTTL)
				allJobs = append(allJobs, repoJob{org: org, project: proj, repo: r, isStale: stale})
			}
		}
		// If no repos in DB yet, fetch all
		if len(allJobs) == 0 {
			if err := fetchAndStoreOrg(ctx, org, log); err != nil {
				log.Error("initial fetch failed", zap.String("org", org.Slug), zap.Error(err))
			}
			// Reload
			repos, _ := globalQ.ListRepositoriesByOrg(ctx, org.Slug)
			for _, r := range repos {
				allJobs = append(allJobs, repoJob{org: org, repo: r, isStale: false})
			}
		}
	}

	// 6. Fetch stale repos concurrently
	type fetchResult struct {
		job   repoJob
		fetched bool
	}
	fetchResults := make(chan fetchResult, len(allJobs))
	jobCh := make(chan repoJob, len(allJobs))
	var wg sync.WaitGroup

	for i := 0; i < scanWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if job.isStale {
					if err := refreshRepo(ctx, job.org, job.project, job.repo, log); err != nil {
						log.Warn("refresh failed", zap.String("repo", job.repo.Name), zap.Error(err))
					}
					fetchResults <- fetchResult{job: job, fetched: true}
				} else {
					fetchResults <- fetchResult{job: job, fetched: false}
				}
			}
		}()
	}
	for _, j := range allJobs {
		jobCh <- j
	}
	close(jobCh)
	go func() { wg.Wait(); close(fetchResults) }()

	// 7. Collect results and score
	var rows []output.RepoRow
	cachedCount, fetchedCount := 0, 0
	for fr := range fetchResults {
		if fr.fetched {
			fetchedCount++
		} else {
			cachedCount++
		}
		// Re-read repo from DB after potential refresh
		repo := fr.job.repo
		metrics := repoToMetrics(repo)
		result := scoring.Score(metrics, profile)
		rows = append(rows, output.RepoRow{
			OrgSlug:    fr.job.org.Slug,
			Project:    repo.ProjectName,
			Repo:       repo.Name,
			Score:      result.TotalScore,
			IsInactive: result.IsInactive,
			Reasons:    result.Reasons,
			Cached:     !fr.fetched,
		})
	}

	// 8. Count inactive
	inactiveCount := 0
	for _, r := range rows {
		if r.IsInactive {
			inactiveCount++
		}
	}

	// 9. Render output
	orgSlugs := make([]string, len(orgsToScan))
	for i, o := range orgsToScan {
		orgSlugs[i] = o.Slug
	}

	opts := output.TableOptions{
		ProfileName:    profile.Name,
		ProfileVersion: profile.Version,
		OrgSlugs:       orgSlugs,
		HasOverrides:   hasOverrides,
		TotalRepos:     len(rows),
		InactiveCount:  inactiveCount,
		CachedCount:    cachedCount,
		FetchedCount:   fetchedCount,
		DurationSec:    time.Since(start).Seconds(),
	}

	today := time.Now().Format("2006-01-02")
	switch outputFmt {
	case "json":
		path := scanOutFile
		if path == "" {
			path = fmt.Sprintf("deadgit-report-%s.json", today)
		}
		return output.WriteJSON(path, rows, profile.Name, profile.Version)
	case "csv":
		path := scanOutFile
		if path == "" {
			path = fmt.Sprintf("deadgit-report-%s.csv", today)
		}
		return output.WriteCSV(path, rows, profile.Name, profile.Version)
	default:
		output.PrintTable(os.Stdout, rows, opts)
	}
	return nil
}

func resolveOrgs(ctx context.Context, cmd *cobra.Command) ([]dbgen.Organization, error) {
	if scanAllOrgs {
		return globalQ.ListOrganizations(ctx)
	}
	if len(scanOrgs) > 0 {
		var result []dbgen.Organization
		for _, slug := range scanOrgs {
			o, err := globalQ.GetOrganizationBySlug(ctx, slug)
			if err != nil {
				return nil, fmt.Errorf("org %q not found: %w", slug, err)
			}
			result = append(result, o)
		}
		return result, nil
	}
	// Interactive: pick from registered orgs
	if isInteractive() {
		allOrgs, err := globalQ.ListOrganizations(ctx)
		if err != nil || len(allOrgs) == 0 {
			return nil, fmt.Errorf("no orgs registered — run: deadgit org add")
		}
		opts := make([]huh.Option[string], len(allOrgs))
		for i, o := range allOrgs {
			opts[i] = huh.NewOption(fmt.Sprintf("%s (%s)", o.Slug, o.Provider), o.Slug)
		}
		var selected []string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().Title("Select orgs to scan").Options(opts...).Value(&selected),
		)).Run(); err != nil {
			return nil, err
		}
		scanOrgs = selected
		return resolveOrgs(ctx, cmd)
	}
	return nil, nil
}

func fetchAndStoreOrg(ctx context.Context, org dbgen.Organization, log *zap.Logger) error {
	pat := os.Getenv(org.PatEnv)
	if pat == "" {
		return fmt.Errorf("PAT env var %q is not set for org %q", org.PatEnv, org.Slug)
	}
	provOrg := providers.Organization{
		ID: org.ID, Slug: org.Slug, Name: org.Name,
		Provider: org.Provider, AccountType: org.AccountType,
		BaseURL: org.BaseUrl, PatEnv: org.PatEnv,
	}
	prov, err := providers.ProviderFor(provOrg, pat)
	if err != nil {
		return err
	}
	projects, err := prov.ListProjects(provOrg)
	if err != nil {
		return err
	}
	for _, proj := range projects {
		dbProj, err := globalQ.UpsertProject(ctx, dbgen.UpsertProjectParams{
			OrgID: org.ID, Name: proj.Name, ExternalID: &proj.ExternalID,
		})
		if err != nil {
			log.Warn("upsert project failed", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		repos, err := prov.FetchRepos(provOrg, proj)
		if err != nil {
			log.Warn("fetch repos failed", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		for _, r := range repos {
			upsertRepoData(ctx, dbProj.ID, r, log)
		}
	}
	globalQ.UpdateOrganizationLastSynced(ctx, org.ID)
	return nil
}

func refreshRepo(ctx context.Context, org dbgen.Organization, proj dbgen.Project, repo dbgen.ListRepositoriesByOrgRow, log *zap.Logger) error {
	pat := os.Getenv(org.PatEnv)
	if pat == "" {
		return fmt.Errorf("PAT env var %q not set", org.PatEnv)
	}
	provOrg := providers.Organization{
		ID: org.ID, Slug: org.Slug, Name: org.Name,
		Provider: org.Provider, AccountType: org.AccountType,
		BaseURL: org.BaseUrl, PatEnv: org.PatEnv,
	}
	prov, err := providers.ProviderFor(provOrg, pat)
	if err != nil {
		return err
	}
	provProj := providers.Project{ID: proj.ID, Name: proj.Name, ExternalID: ""}
	repos, err := prov.FetchRepos(provOrg, provProj)
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.Name == repo.Name {
			upsertRepoData(ctx, proj.ID, r, log)
			break
		}
	}
	return nil
}

func upsertRepoData(ctx context.Context, projectID int64, r providers.RepoData, log *zap.Logger) {
	toNullTime := func(t *time.Time) sql.NullTime {
		if t == nil {
			return sql.NullTime{}
		}
		return sql.NullTime{Valid: true, Time: *t}
	}
	toNullInt := func(v int) *int64 {
		i := int64(v)
		return &i
	}
	_, err := globalQ.UpsertRepository(context.Background(), dbgen.UpsertRepositoryParams{
		ProjectID:         projectID,
		Name:              r.Name,
		RemoteUrl:         r.RemoteURL,
		ExternalID:        &r.ExternalID,
		DefaultBranch:     &r.DefaultBranch,
		IsArchived:        boolToInt(r.IsArchived),
		IsDisabled:        boolToInt(r.IsDisabled),
		LastCommitAt:      toNullTime(r.LastCommitAt),
		LastPushAt:        toNullTime(r.LastPushAt),
		LastPrMergedAt:    toNullTime(r.LastPRMergedAt),
		LastPrCreatedAt:   toNullTime(r.LastPRCreatedAt),
		CommitCount_90d:   toNullInt(r.CommitCount90d),
		ActiveBranchCount: toNullInt(r.ActiveBranchCount),
		ContributorCount:  toNullInt(r.ContributorCount),
		RawApiBlob:        &r.RawAPIBlob,
	})
	if err != nil {
		log.Warn("upsert repo failed", zap.String("repo", r.Name), zap.Error(err))
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func repoToMetrics(r dbgen.ListRepositoriesByOrgRow) scoring.RepoMetrics {
	daysSince := func(t sql.NullTime) float64 {
		if !t.Valid {
			return 9999
		}
		return time.Since(t.Time).Hours() / 24
	}
	commitCount := 0
	if r.CommitCount_90d != nil {
		commitCount = int(*r.CommitCount_90d)
	}
	branchCount := 0
	if r.ActiveBranchCount != nil {
		branchCount = int(*r.ActiveBranchCount)
	}
	return scoring.RepoMetrics{
		DaysSinceLastCommit: daysSince(r.LastCommitAt),
		DaysSinceLastPR:     daysSince(r.LastPrCreatedAt),
		CommitCount90d:      commitCount,
		ActiveBranchCount:   branchCount,
		HasRecentRelease:    false, // future: add releases table
		IsArchived:          r.IsArchived == 1,
		IsDisabled:          r.IsDisabled == 1,
	}
}

func dbProfileToScoringProfile(p dbgen.ScoringProfile) scoring.ScoringProfile {
	return scoring.ScoringProfile{
		Name:                   p.Name,
		Version:                int(p.Version),
		WLastCommit:            p.WLastCommit,
		WLastPR:                p.WLastPr,
		WCommitFrequency:       p.WCommitFrequency,
		WBranchStaleness:       p.WBranchStaleness,
		WNoReleases:            p.WNoReleases,
		InactiveDaysThreshold:  int(p.InactiveDaysThreshold),
		InactiveScoreThreshold: p.InactiveScoreThreshold,
	}
}

func applyInlineOverrides(p *scoring.ScoringProfile, cmd *cobra.Command) bool {
	changed := false
	if cmd.Flags().Changed("w-last-commit") {
		p.WLastCommit = scanWCommit; changed = true
	}
	if cmd.Flags().Changed("w-last-pr") {
		p.WLastPR = scanWPR; changed = true
	}
	if cmd.Flags().Changed("w-commit-freq") {
		p.WCommitFrequency = scanWFreq; changed = true
	}
	if cmd.Flags().Changed("w-branch-staleness") {
		p.WBranchStaleness = scanWBranch; changed = true
	}
	if cmd.Flags().Changed("w-no-releases") {
		p.WNoReleases = scanWRelease; changed = true
	}
	if cmd.Flags().Changed("threshold") {
		p.InactiveDaysThreshold = scanThreshold; changed = true
	}
	if cmd.Flags().Changed("score-min") {
		p.InactiveScoreThreshold = scanScoreMin; changed = true
	}
	return changed
}

// recordScanRun writes a scan_runs row for audit purposes.
func recordScanRun(ctx context.Context, org dbgen.Organization, profile scoring.ScoringProfile, total, inactive int) {
	snapshot, _ := json.Marshal(profile)
	globalQ.InsertScanRun(ctx, dbgen.InsertScanRunParams{
		OrgID:           &org.ID,
		ProfileID:       nil,
		ProfileName:     profile.Name,
		ProfileVersion:  int64(profile.Version),
		ProfileSnapshot: string(snapshot),
		TotalRepos:      int64(total),
		InactiveCount:   int64(inactive),
	})
}
```

> **Note:** `InsertScanRun` requires a matching sqlc query. Add this to `internal/db/queries/profiles.sql` (or a new `scan_runs.sql`):
```sql
-- name: InsertScanRun :exec
INSERT INTO scan_runs (org_id, profile_id, profile_name, profile_version, profile_snapshot, total_repos, inactive_count)
VALUES (?, ?, ?, ?, ?, ?, ?);
```
Then re-run `sqlc generate` and commit.

- [ ] **Step 2: Build**

```bash
go build -o deadgit .
# Iterate on compile errors — field names from sqlc-generated types must match exactly.
# Check internal/db/generated/models.go for exact struct field names.
```

- [ ] **Step 3: Smoke test with --help**

```bash
./deadgit scan --help
# Expected: all flags shown
```

- [ ] **Step 4: Commit**

```bash
git add cmd/scan.go internal/db/queries/ internal/db/generated/
git commit -m "feat: implement scan command with full fetch+score+render flow"
```

---

## Task 21: Add scan_runs query and regenerate sqlc

**Files:** `internal/db/queries/scan_runs.sql`, re-run sqlc

- [ ] **Step 1: Create scan_runs.sql**

Create `internal/db/queries/scan_runs.sql`:

```sql
-- name: InsertScanRun :exec
INSERT INTO scan_runs (org_id, profile_id, profile_name, profile_version, profile_snapshot, total_repos, inactive_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListScanRuns :many
SELECT * FROM scan_runs ORDER BY scanned_at DESC LIMIT 50;
```

- [ ] **Step 2: Regenerate sqlc**

```bash
sqlc generate
```

- [ ] **Step 3: Build**

```bash
go build -o deadgit .
# Expected: no errors
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/scan_runs.sql internal/db/generated/
git commit -m "feat: add scan_runs query and regenerate sqlc"
```

---

## Task 22: Full build, tests, and integration smoke test

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v 2>&1 | tail -40
# Expected: all PASS, no FAIL
```

- [ ] **Step 2: Build final binary**

```bash
go build -o deadgit .
```

- [ ] **Step 3: Smoke test — help output**

```bash
./deadgit --help
./deadgit org --help
./deadgit profile --help
./deadgit scan --help
# Expected: all commands and flags documented
```

- [ ] **Step 4: Smoke test — org add (non-interactive)**

```bash
GITHUB_PAT=dummy ./deadgit org add testorg --name "Test Org" --provider github --pat-env GITHUB_PAT 2>&1 || true
# Expected: either succeeds or fails with clear error (PAT validation or DB error)
```

- [ ] **Step 5: Smoke test — profile list**

```bash
./deadgit profile list
# Expected: 'default v1' shown
```

- [ ] **Step 6: Fix any failures, then commit**

```bash
git add -A
git commit -m "fix: resolve build and test failures from integration"
```

---

## Task 23: README.md and ROADMAP.md

**Files:** Create `README.md`, `ROADMAP.md`

- [ ] **Step 1: Write README.md**

Create `README.md`:

````markdown
# deadgit

Scan GitHub and Azure DevOps repositories for inactivity. Detects dead repos using a configurable weighted scoring system and caches results locally in SQLite.

## Installation

```bash
# Homebrew (coming soon)
brew install oxGrad/tap/deadgit

# From source
go install github.com/oxGrad/deadgit@latest
```

## Quick Start

```bash
# Register a GitHub org (PAT needs repo scope)
GITHUB_PAT=ghp_xxx deadgit org add mycompany --name "My Company" --pat-env GITHUB_PAT

# Scan it
GITHUB_PAT=ghp_xxx deadgit scan --org mycompany

# Scan all registered orgs
GITHUB_PAT=ghp_xxx deadgit scan --all-orgs
```

## Environment Variables

| Variable | Purpose |
|---|---|
| `<PAT_ENV>` | PAT for a specific org — name configured per org via `--pat-env` |

## Commands

### `deadgit org`

```bash
# Add a GitHub organization
deadgit org add mycompany --name "My Company" --provider github --pat-env GITHUB_PAT

# Add a GitHub personal account
deadgit org add myusername --provider github --type personal --pat-env GITHUB_PAT

# Add an Azure DevOps organization
deadgit org add myazureorg --provider azure --pat-env AZURE_PAT

# List all orgs
deadgit org list

# Deactivate an org
deadgit org remove mycompany
```

When running interactively (TTY), omitting flags opens guided prompts. For CI, always provide all flags.

### `deadgit profile`

```bash
# List profiles (default profile is always present)
deadgit profile list

# Create a custom profile
deadgit profile create aggressive \
  --w-last-commit 0.5 --w-last-pr 0.2 --w-commit-freq 0.2 \
  --w-branch-staleness 0.05 --w-no-releases 0.05 \
  --threshold 60 --score-min 0.5

# Edit a profile (version is incremented automatically)
deadgit profile edit aggressive --w-last-commit 0.6

# Set as default
deadgit profile set-default aggressive
```

Profiles are **immutable-versioned**: editing increments the version and records history. Profiles cannot be deleted.

### `deadgit scan`

```bash
# Scan specific orgs with default profile
deadgit scan --org mycompany --org myotheorg

# Scan all orgs, output as JSON
deadgit scan --all-orgs --output json --outfile report.json

# Use a different scoring profile
deadgit scan --all-orgs --profile aggressive

# Force refresh (bypass cache)
deadgit scan --all-orgs --refresh

# Experiment with weights without saving a profile
deadgit scan --all-orgs --w-last-commit 0.6 --w-last-pr 0.1

# Custom cache TTL (hours)
deadgit scan --all-orgs --ttl 48
```

**Profile switching with cached data:**

Once data is fetched and cached, switching profiles is instant — no API calls:

```bash
# First scan: fetches from API, scores with 'default'
deadgit scan --all-orgs

# Re-score from cache with 'aggressive' profile — zero API calls
deadgit scan --all-orgs --profile aggressive

# Force fresh data + rescore
deadgit scan --all-orgs --refresh --profile aggressive
```

## Scoring

Each repository receives an inactivity score from 0.0 (fully active) to 1.0 (fully inactive):

```
score = (
  days_since_last_commit  × w_last_commit       (default 0.40)
  days_since_last_pr      × w_last_pr           (default 0.20)
  commit_frequency_decay  × w_commit_frequency  (default 0.20)
  stale_branches          × w_branch_staleness  (default 0.10)
  no_recent_releases      × w_no_releases       (default 0.10)
)
```

A repo is **INACTIVE** when `score ≥ inactive_score_threshold` (default 0.65).

Archived and disabled repos always score 1.0.

## Output Formats

```bash
# Table (default — colorized, shown in terminal)
deadgit scan --all-orgs

# JSON (full metrics + profile metadata)
deadgit scan --all-orgs --output json

# CSV (one row per repo + profile columns)
deadgit scan --all-orgs --output csv
```

## Global Flags

```
--db <path>       SQLite database path (default: ~/.deadgit/deadgit.db)
--output <fmt>    Output format: table | json | csv (default: table)
```

## Database

deadgit stores raw API metrics in `~/.deadgit/deadgit.db` (SQLite). The database is created automatically on first run. Migrations run automatically — no setup required.

The binary is fully self-contained: migrations are embedded and require no external files. This makes it suitable for Homebrew distribution.
````

- [ ] **Step 2: Write ROADMAP.md**

Create `ROADMAP.md`:

```markdown
# deadgit Roadmap

## Planned

### OS Keychain Integration (`go-keyring`)

**Status:** Not started  
**Motivation:** Currently PAT tokens are read from environment variables at runtime and their names are stored in the DB. This is secure (no plaintext PAT on disk) but requires the caller to manage env vars for each org.

**Planned approach:**
- Use `github.com/zalando/go-keyring` to store PATs in the OS native keychain:
  - macOS → Keychain Access (transparent, no user setup)
  - Windows → Windows Credential Manager
  - Linux → Secret Service / GNOME Keyring (requires daemon)
- `deadgit org add` would store the PAT in the keychain under key `deadgit/<slug>`
- At scan time, PAT is retrieved from keychain automatically
- Fall back to env var if keychain lookup fails (for CI/headless environments)
- The `pat_env` column would become optional (used for override/CI bypass)

**Blockers:** Linux headless environments require a running Secret Service daemon. Need a graceful fallback story before shipping.

### GitHub Releases tracking

Track `last_release_at` per repository to improve the `no_recent_releases` scoring component. Currently defaults to "no recent release" (score = 1.0) for all repos.

### GitLab provider

Add `internal/providers/gitlab/` implementing the `Provider` interface for GitLab SaaS and self-hosted instances.

### Web UI

A read-only web dashboard served locally (`deadgit serve`) showing scan history, score trends, and org/profile comparisons.
```

- [ ] **Step 3: Commit**

```bash
git add README.md ROADMAP.md
git commit -m "docs: add README with full usage guide and ROADMAP"
```

---

## Task 24: Update PRD.md

**Files:** Overwrite `PRD.md`

- [ ] **Step 1: Update PRD.md to reflect v2**

Replace `PRD.md` with a concise v2 summary:

```markdown
# deadgit — Product Requirements (v2)

## Overview

deadgit is a Go CLI tool that scans GitHub and Azure DevOps repositories across
multiple organizations, detects inactive repositories using a configurable
weighted scoring system, and caches raw API data locally in SQLite.

## Tech Stack

- **Language**: Go 1.21+
- **CLI**: `cobra`
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Query Layer**: `sqlc` (type-safe SQL, generated code checked in)
- **Interactive UI**: `charmbracelet/huh` (TTY-only, CI-safe)
- **Output**: `olekukonko/tablewriter`, `fatih/color`
- **Logging**: `uber/zap`

## Supported Providers

- GitHub (organization and personal accounts)
- Azure DevOps

## Key Design Rules

1. **Scores are never stored** — always computed at runtime from raw metrics + profile
2. **Profiles are versioned, not deleted** — `edit` increments version, history is recorded
3. **PAT tokens are never stored** — env var name stored; PAT read at runtime
4. **Migrations are embedded** — binary is self-contained for Homebrew distribution
5. **Interactive mode is TTY-only** — pipes and CI always behave non-interactively
6. **GitHub is the default provider** — `--provider` defaults to `github`

## Scoring Formula

```
score = (
  days_since_last_commit  × w_last_commit       (default 0.40)
  days_since_last_pr      × w_last_pr           (default 0.20)
  commit_frequency_decay  × w_commit_frequency  (default 0.20)
  stale_branches          × w_branch_staleness  (default 0.10)
  no_recent_releases      × w_no_releases       (default 0.10)
)
```

`score ≥ inactive_score_threshold` (default 0.65) → INACTIVE

Archived/disabled repos always score 1.0.

## See Also

- `docs/superpowers/specs/2026-04-02-deadgit-v2-design.md` — full design spec
- `ROADMAP.md` — planned features (keyring, GitLab, web UI)
```

- [ ] **Step 2: Commit**

```bash
git add PRD.md
git commit -m "docs: update PRD.md to reflect v2 scope and design"
```

---

## Self-Review Checklist

| Spec requirement | Covered in task |
|---|---|
| Multi-org SQLite-backed storage | Tasks 3, 6 |
| GitHub org + personal account | Tasks 13, 14 |
| Azure DevOps provider | Tasks 11, 12 |
| Cobra CLI with org/profile/scan | Tasks 17–20 |
| Scoring profiles — create/list/edit/set-default | Task 19 |
| Profile versioning — no delete, version increment | Tasks 4, 19 |
| Profile name + version in all output (table, JSON, CSV) | Tasks 15, 16 |
| Interactive TUI via huh with TTY detection | Tasks 18, 19, 20 |
| Flags always win (CI-safe) | Tasks 18, 19, 20 |
| PAT via env var — name stored, never PAT itself | Tasks 3, 18 |
| Embedded migrations (embed.FS) | Task 6 |
| sqlc-generated code checked in | Task 5 |
| Cache TTL + --refresh flag | Tasks 9, 20 |
| Inline --w-* overrides (not persisted) | Task 20 |
| README.md with full usage guide | Task 23 |
| ROADMAP.md with keyring plans | Task 23 |
| Updated PRD.md | Task 24 |
| Drop pipeline scanning | Task 1 |
| GitHub as default provider | Tasks 3, 18 |
| base_url auto-set from provider | Task 18 |
