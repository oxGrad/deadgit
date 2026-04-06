# Scoring formula

Multiplier default value, but configurable

```
score = (
  days_since_last_commit  * 0.4  +
  days_since_last_pr      * 0.2  +
  commit_frequency_decay  * 0.2  +
  stale_branches          * 0.1  +
  no_recent_releases      * 0.1
)
```

# Go CLI Tool - Multi-Org Azure Repo Inactivity Scanner

## Project Overview

Build a Go CLI tool that scans Azure DevOps repositories across multiple organizations,
detects inactive repositories using a configurable weighted scoring system,
and caches API data locally using SQLite via sqlc.

## Tech Stack

- **Language**: Go
- **CLI Framework**: `cobra`
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Query Layer**: `sqlc` for type-safe SQL
- **DB Migrations**: `golang-migrate/migrate`
- **Azure DevOps API**: `microsoft/azure-devops-go-api`
- **Output Formatting**: `olekukonko/tablewriter` or `pterm`
- **Config**: `spf13/viper`

---

## Project Structure

```
.
├── cmd/
│   ├── root.go
│   ├── org.go          # org add / list / remove
│   ├── profile.go      # profile create / list / edit / set-default
│   └── scan.go         # scan command
├── internal/
│   ├── db/
│   │   ├── migrations/
│   │   │   ├── 000001_init.up.sql
│   │   │   └── 000001_init.down.sql
│   │   ├── queries/
│   │   │   ├── orgs.sql
│   │   │   ├── projects.sql
│   │   │   ├── repos.sql
│   │   │   └── profiles.sql
│   │   ├── sqlc.yaml
│   │   └── generated/     # sqlc output goes here
│   │       ├── db.go
│   │       ├── models.go
│   │       ├── orgs.sql.go
│   │       ├── projects.sql.go
│   │       ├── repos.sql.go
│   │       └── profiles.sql.go
│   ├── azure/
│   │   ├── client.go
│   │   └── fetcher.go
│   ├── scoring/
│   │   ├── types.go
│   │   ├── normalizer.go
│   │   └── scorer.go
│   ├── output/
│   │   ├── table.go
│   │   ├── json.go
│   │   └── csv.go
│   └── cache/
│       └── ttl.go
├── main.go
├── sqlc.yaml
└── go.mod
```

---

## Database Schema

### File: `internal/db/migrations/000001_init.up.sql`

```sql
CREATE TABLE IF NOT EXISTS organizations (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL,
  slug        TEXT    NOT NULL UNIQUE,
  provider    TEXT    NOT NULL DEFAULT 'azure',
  base_url    TEXT    NOT NULL DEFAULT 'https://dev.azure.com',
  pat_token   TEXT    NOT NULL,
  is_active   INTEGER NOT NULL DEFAULT 1,
  last_synced DATETIME,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id           INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name             TEXT    NOT NULL,
  azure_project_id TEXT,
  last_synced      DATETIME,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(org_id, name)
);

CREATE TABLE IF NOT EXISTS repositories (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name                TEXT    NOT NULL,
  remote_url          TEXT    NOT NULL,
  azure_repo_id       TEXT    UNIQUE,
  default_branch      TEXT,
  is_archived         INTEGER NOT NULL DEFAULT 0,
  is_disabled         INTEGER NOT NULL DEFAULT 0,
  -- Raw metrics from API (never computed values)
  last_commit_at      DATETIME,
  last_push_at        DATETIME,
  last_pr_merged_at   DATETIME,
  last_pr_created_at  DATETIME,
  commit_count_90d    INTEGER,
  active_branch_count INTEGER,
  contributor_count   INTEGER,
  -- Cache control
  last_fetched        DATETIME,
  raw_api_blob        TEXT,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, name)
);

CREATE TABLE IF NOT EXISTS scoring_profiles (
  id                        INTEGER PRIMARY KEY AUTOINCREMENT,
  name                      TEXT    NOT NULL UNIQUE,
  description               TEXT,
  is_default                INTEGER NOT NULL DEFAULT 0,
  -- Weights (must sum to 1.0, enforced at app layer)
  w_last_commit             REAL    NOT NULL DEFAULT 0.40,
  w_last_pr                 REAL    NOT NULL DEFAULT 0.20,
  w_commit_frequency        REAL    NOT NULL DEFAULT 0.20,
  w_branch_staleness        REAL    NOT NULL DEFAULT 0.10,
  w_no_releases             REAL    NOT NULL DEFAULT 0.10,
  -- Thresholds
  inactive_days_threshold   INTEGER NOT NULL DEFAULT 90,
  inactive_score_threshold  REAL    NOT NULL DEFAULT 0.65,
  created_at                DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at                DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scoring_profile_history (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id  INTEGER NOT NULL REFERENCES scoring_profiles(id) ON DELETE CASCADE,
  changed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  old_values  TEXT     NOT NULL,
  new_values  TEXT     NOT NULL,
  changed_by  TEXT     NOT NULL DEFAULT 'cli'
);

CREATE TABLE IF NOT EXISTS scan_runs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id         INTEGER REFERENCES organizations(id) ON DELETE SET NULL,
  profile_id     INTEGER REFERENCES scoring_profiles(id) ON DELETE SET NULL,
  profile_snapshot TEXT NOT NULL,   -- JSON snapshot of weights used
  total_repos    INTEGER NOT NULL DEFAULT 0,
  inactive_count INTEGER NOT NULL DEFAULT 0,
  scanned_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Insert default scoring profile on init
INSERT OR IGNORE INTO scoring_profiles (
  name, description, is_default,
  w_last_commit, w_last_pr, w_commit_frequency,
  w_branch_staleness, w_no_releases,
  inactive_days_threshold, inactive_score_threshold
) VALUES (
  'default', 'Default balanced scoring profile', 1,
  0.40, 0.20, 0.20, 0.10, 0.10,
  90, 0.65
);
```

---

## SQLC Configuration

### File: `sqlc.yaml`

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

---

## SQL Queries

### File: `internal/db/queries/orgs.sql`

```sql
-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, provider, base_url, pat_token)
VALUES (?, ?, ?, ?, ?)
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

### File: `internal/db/queries/projects.sql`

```sql
-- name: UpsertProject :one
INSERT INTO projects (org_id, name, azure_project_id)
VALUES (?, ?, ?)
ON CONFLICT(org_id, name) DO UPDATE SET
  azure_project_id = excluded.azure_project_id,
  last_synced = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListProjectsByOrg :many
SELECT * FROM projects WHERE org_id = ? ORDER BY name;

-- name: GetProjectByName :one
SELECT * FROM projects WHERE org_id = ? AND name = ? LIMIT 1;
```

### File: `internal/db/queries/repos.sql`

```sql
-- name: UpsertRepository :one
INSERT INTO repositories (
  project_id, name, remote_url, azure_repo_id, default_branch,
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
SELECT r.*, p.name as project_name, o.slug as org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE r.project_id = ?
ORDER BY r.name;

-- name: ListAllRepositories :many
SELECT r.*, p.name as project_name, o.slug as org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
ORDER BY o.slug, p.name, r.name;

-- name: ListRepositoriesByOrg :many
SELECT r.*, p.name as project_name, o.slug as org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE o.slug = ?
ORDER BY p.name, r.name;

-- name: GetRepositoryByAzureID :one
SELECT * FROM repositories WHERE azure_repo_id = ? LIMIT 1;

-- name: ListStaleRepositories :many
SELECT r.*, p.name as project_name, o.slug as org_slug
FROM repositories r
JOIN projects p ON r.project_id = p.id
JOIN organizations o ON p.org_id = o.id
WHERE r.last_fetched IS NULL
   OR r.last_fetched < datetime('now', '-' || ? || ' hours')
ORDER BY r.last_fetched ASC;
```

### File: `internal/db/queries/profiles.sql`

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
  description               = ?,
  w_last_commit             = ?,
  w_last_pr                 = ?,
  w_commit_frequency        = ?,
  w_branch_staleness        = ?,
  w_no_releases             = ?,
  inactive_days_threshold   = ?,
  inactive_score_threshold  = ?,
  updated_at                = CURRENT_TIMESTAMP
WHERE name = ?
RETURNING *;

-- name: SetDefaultProfile :exec
UPDATE scoring_profiles SET is_default = CASE WHEN name = ? THEN 1 ELSE 0 END;

-- name: InsertProfileHistory :exec
INSERT INTO scoring_profile_history (profile_id, old_values, new_values, changed_by)
VALUES (?, ?, ?, ?);

-- name: ListProfileHistory :many
SELECT * FROM scoring_profile_history
WHERE profile_id = ?
ORDER BY changed_at DESC;

-- name: DeleteProfile :exec
DELETE FROM scoring_profiles WHERE name = ? AND is_default = 0;
```

---

## Scoring Engine

### File: `internal/scoring/types.go`

```go
package scoring

// RepoMetrics holds raw normalized inputs derived from stored API data.
// All values are computed at runtime from the DB, never stored.
type RepoMetrics struct {
    DaysSinceLastCommit float64
    DaysSinceLastPR     float64
    CommitCount90d      int
    ActiveBranchCount   int
    HasRecentRelease    bool
    IsArchived          bool
    IsDisabled          bool
}

// ScoringProfile mirrors the DB row but is used as a pure value object.
type ScoringProfile struct {
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

### File: `internal/scoring/normalizer.go`

```go
package scoring

import "math"

// NormalizeLinear maps days to a 0.0-1.0 score.
// 0 days = 0.0 (fully active), >= threshold days = 1.0 (fully inactive).
func NormalizeLinear(days float64, threshold int) float64 {
    if threshold <= 0 {
        return 0
    }
    return math.Min(days/float64(threshold), 1.0)
}

// NormalizeCommitFrequency returns a 0-1 inactivity score based on commit count.
// More commits in 90d = lower (more active) score.
func NormalizeCommitFrequency(commitCount90d int, threshold int) float64 {
    if commitCount90d <= 0 {
        return 1.0
    }
    // Treat N commits per threshold days as baseline for "active"
    baseline := float64(threshold) / 30.0 * 4.0 // ~4 commits/month as active baseline
    score := 1.0 - math.Min(float64(commitCount90d)/baseline, 1.0)
    return math.Max(score, 0.0)
}

// NormalizeBranchStaleness returns a 0-1 score.
// 0 active branches = fully stale (1.0).
func NormalizeBranchStaleness(activeBranches int) float64 {
    if activeBranches <= 0 {
        return 1.0
    }
    // More than 3 branches considered fully active
    return math.Max(1.0-float64(activeBranches)/3.0, 0.0)
}
```

### File: `internal/scoring/scorer.go`

```go
package scoring

import (
    "fmt"
    "math"
)

// Score computes an inactivity score dynamically from raw metrics and a profile.
// This function is pure: no DB access, no side effects, fully testable.
func Score(metrics RepoMetrics, profile ScoringProfile) ScoringResult {
    // Hard overrides: archived or disabled repos are always inactive
    if metrics.IsArchived || metrics.IsDisabled {
        return ScoringResult{
            TotalScore: 1.0,
            IsInactive: true,
            Breakdown:  ScoreBreakdown{1, 1, 1, 1, 1},
            Reasons:    buildOverrideReasons(metrics),
        }
    }

    threshold := profile.InactiveDaysThreshold

    lastCommitScore  := NormalizeLinear(metrics.DaysSinceLastCommit, threshold)
    lastPRScore      := NormalizeLinear(metrics.DaysSinceLastPR, threshold)
    commitFreqScore  := NormalizeCommitFrequency(metrics.CommitCount90d, threshold)
    branchScore      := NormalizeBranchStaleness(metrics.ActiveBranchCount)
    releaseScore     := releaseInactivityScore(metrics.HasRecentRelease)

    total :=
        lastCommitScore * profile.WLastCommit     +
        lastPRScore     * profile.WLastPR         +
        commitFreqScore * profile.WCommitFrequency +
        branchScore     * profile.WBranchStaleness +
        releaseScore    * profile.WNoReleases

    total = math.Round(total*10000) / 10000

    return ScoringResult{
        TotalScore: total,
        IsInactive: total >= profile.InactiveScoreThreshold,
        Breakdown: ScoreBreakdown{
            LastCommitScore:      lastCommitScore,
            LastPRScore:          lastPRScore,
            CommitFrequencyScore: commitFreqScore,
            BranchStalenessScore: branchScore,
            ReleaseScore:         releaseScore,
        },
        Reasons: buildReasons(metrics, ScoreBreakdown{
            LastCommitScore:      lastCommitScore,
            LastPRScore:          lastPRScore,
            CommitFrequencyScore: commitFreqScore,
            BranchStalenessScore: branchScore,
            ReleaseScore:         releaseScore,
        }),
    }
}

func releaseInactivityScore(hasRecentRelease bool) float64 {
    if hasRecentRelease {
        return 0.0
    }
    return 1.0
}

func buildReasons(metrics RepoMetrics, b ScoreBreakdown) []string {
    var reasons []string
    if b.LastCommitScore >= 0.8 {
        reasons = append(reasons, fmt.Sprintf(
            "No commits in %.0f days", metrics.DaysSinceLastCommit,
        ))
    }
    if b.LastPRScore >= 0.8 {
        reasons = append(reasons, fmt.Sprintf(
            "No PR activity in %.0f days", metrics.DaysSinceLastPR,
        ))
    }
    if b.CommitFrequencyScore >= 0.8 {
        reasons = append(reasons, "Commit frequency near zero in last 90 days")
    }
    if b.BranchStalenessScore >= 0.8 {
        reasons = append(reasons, "No active branches detected")
    }
    if b.ReleaseScore == 1.0 {
        reasons = append(reasons, "No recent releases or tags")
    }
    return reasons
}

func buildOverrideReasons(metrics RepoMetrics) []string {
    var reasons []string
    if metrics.IsArchived {
        reasons = append(reasons, "Repository is archived")
    }
    if metrics.IsDisabled {
        reasons = append(reasons, "Repository is disabled")
    }
    return reasons
}
```

---

## Cache TTL

### File: `internal/cache/ttl.go`

```go
package cache

import (
    "database/sql"
    "time"
)

const DefaultTTLHours = 24

// IsStale returns true if last_fetched is null or older than TTL hours.
func IsStale(lastFetched sql.NullTime, ttlHours int) bool {
    if !lastFetched.Valid {
        return true
    }
    return time.Since(lastFetched.Time) > time.Duration(ttlHours)*time.Hour
}
```

---

## CLI Commands

### File: `cmd/root.go`

```go
package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
    Use:   "repo-scan",
    Short: "Scan Azure DevOps repos for inactivity",
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func init() {
    rootCmd.PersistentFlags().String("db", "repo-scan.db", "Path to SQLite database")
    rootCmd.PersistentFlags().String("output", "table", "Output format: table | json | csv")
    viper.BindPFlag("db", rootCmd.PersistentFlags().Lookup("db"))
    viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))

    rootCmd.AddCommand(orgCmd)
    rootCmd.AddCommand(profileCmd)
    rootCmd.AddCommand(scanCmd)
}
```

### File: `cmd/scan.go`

```go
package cmd

import (
    "github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
    Use:   "scan",
    Short: "Scan repositories for inactivity",
    RunE:  runScan,
}

func init() {
    scanCmd.Flags().StringSlice("org", nil, "Org slugs to scan (repeatable)")
    scanCmd.Flags().Bool("all-orgs", false, "Scan all registered active orgs")
    scanCmd.Flags().String("profile", "", "Scoring profile name (uses default if omitted)")
    scanCmd.Flags().Bool("refresh", false, "Force re-fetch from API even if cache is fresh")
    scanCmd.Flags().Int("ttl", 24, "Cache TTL in hours before re-fetching")

    // Inline weight overrides (do NOT persist to DB, used for experimentation)
    scanCmd.Flags().Float64("w-last-commit", -1, "Override weight: last commit (0.0-1.0)")
    scanCmd.Flags().Float64("w-last-pr", -1, "Override weight: last PR (0.0-1.0)")
    scanCmd.Flags().Float64("w-commit-freq", -1, "Override weight: commit frequency (0.0-1.0)")
    scanCmd.Flags().Float64("w-branch-staleness", -1, "Override weight: branch staleness (0.0-1.0)")
    scanCmd.Flags().Float64("w-no-releases", -1, "Override weight: no releases (0.0-1.0)")
    scanCmd.Flags().Int("threshold", -1, "Override inactive days threshold")
    scanCmd.Flags().Float64("score-min", -1, "Override inactive score threshold (0.0-1.0)")
}

func runScan(cmd *cobra.Command, args []string) error {
    // 1. Load profile from DB (by name or default)
    // 2. Apply any inline flag overrides to profile (in-memory only, not saved)
    // 3. Validate weights sum to ~1.0 (warn if not)
    // 4. Determine orgs to scan (--org flags or --all-orgs)
    // 5. For each org -> project -> repo:
    //    a. Check cache TTL (skip fetch if fresh and --refresh not set)
    //    b. Fetch from Azure API if stale
    //    c. Upsert raw metrics into DB
    //    d. Compute RepoMetrics from stored data
    //    e. Call scoring.Score(metrics, profile)  ← dynamic, not stored
    // 6. Collect ScoringResult per repo
    // 7. Render output (table/json/csv)
    return nil
}
```

---

## Key Design Rules for Implementation

```
1. NEVER store computed scores in the database
   - Scores are always derived at runtime from raw metrics + active profile
   - This allows re-scoring historical data with new weights instantly

2. raw_api_blob stores the full JSON API response
   - Allows reprocessing without hitting the API again
   - Useful when adding new metrics later

3. Inline --w-* flags override profile weights in-memory only
   - Useful for experimentation without corrupting saved profiles
   - Always log which profile + overrides were used in scan output

4. Weight validation (app layer, not DB)
   - Warn if weights do not sum to ~1.0 (tolerance ±0.01)
   - Do not hard-fail, allow flexible experimentation

5. PAT tokens
   - Consider integrating with OS keychain (zalando/go-keyring)
   - Or encrypt at rest with a local master key stored in config

6. Migrations
   - Use golang-migrate/migrate with embed.FS for migration files
   - Run migrations automatically on app startup

7. Azure DevOps hierarchy
   - Organization → Projects → Repositories
   - Always sync at project level before repo level
   - Store azure_project_id and azure_repo_id as stable references
```

---

## Example Output

```
Scan Results  •  Profile: default  •  Orgs: myorg, anotherorg
┌────────────────┬──────────────┬──────────────────────┬────────┬────────────┬──────────────────────────────┐
│ Org            │ Project      │ Repository           │ Score  │ Status     │ Reasons                      │
├────────────────┼──────────────┼──────────────────────┼────────┼────────────┼──────────────────────────────┤
│ myorg          │ platform     │ legacy-api           │ 0.8821 │ ⚠ INACTIVE │ No commits in 210 days       │
│ myorg          │ platform     │ active-service       │ 0.1240 │ ✓ active   │                              │
│ anotherorg     │ backend      │ old-monolith         │ 0.9100 │ ⚠ INACTIVE │ Archived, No commits 400d    │
└────────────────┴──────────────┴──────────────────────┴────────┴────────────┴──────────────────────────────┘
Total: 3 repos  •  2 inactive  •  Cached: 1  •  Fetched: 2  •  Duration: 1.23s
```
