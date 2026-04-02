# deadgit v2 — Design Spec

**Date:** 2026-04-02  
**Status:** Approved

---

## Overview

Rewrite `deadgit` from a stateless single-org Azure DevOps scanner into a multi-provider (Azure DevOps + GitHub), SQLite-backed repository inactivity scanner with configurable weighted scoring profiles.

Pipeline YAML template detection from v1 is **dropped**.

---

## Architecture

### Package Structure

```
deadgit/
├── cmd/
│   ├── root.go          # cobra root, --db / --output persistent flags
│   ├── org.go           # org add / list / remove
│   ├── profile.go       # profile create / list / edit / set-default
│   └── scan.go          # scan command + inline weight overrides
├── internal/
│   ├── providers/
│   │   ├── provider.go          # Provider interface
│   │   ├── azure/
│   │   │   ├── client.go        # HTTP client ported from azuredevops/client.go
│   │   │   └── fetcher.go       # Azure DevOps implementation of Provider
│   │   └── github/
│   │       ├── client.go        # GitHub HTTP client (Bearer token auth)
│   │       └── fetcher.go       # GitHub implementation of Provider
│   ├── db/
│   │   ├── migrations/          # golang-migrate SQL files, embedded via embed.FS
│   │   │   ├── 000001_init.up.sql
│   │   │   └── 000001_init.down.sql
│   │   ├── queries/             # sqlc .sql source files
│   │   │   ├── orgs.sql
│   │   │   ├── projects.sql
│   │   │   ├── repos.sql
│   │   │   └── profiles.sql
│   │   └── generated/           # sqlc output, checked in (no runtime codegen)
│   │       ├── db.go
│   │       ├── models.go
│   │       └── *.sql.go
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
├── go.mod
├── README.md
└── ROADMAP.md
```

**Removed from v1:** `azuredevops/`, `report/`, `pipeline/`, `cmd/cli.go`

### Binary Distribution

Both migration SQL files (via `embed.FS`) and sqlc-generated Go code are compiled into the binary. No external files required at runtime — suitable for Homebrew distribution.

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS
```

Migrations run automatically on startup via `golang-migrate` with the `iofs` source driver.

---

## Provider Abstraction

```go
// internal/providers/provider.go
type Provider interface {
    ListProjects(org Organization) ([]Project, error)
    ListRepositories(org Organization, project Project) ([]Repository, error)
    GetRepoMetrics(org Organization, project Project, repo Repository) (RepoMetrics, error)
}
```

A `ProviderFor(org Organization) Provider` factory in `provider.go` returns the correct implementation based on `org.Provider` (`"azure"` | `"github"`).

### GitHub Hierarchy

GitHub has no projects layer. The GitHub fetcher auto-creates a single stub `Project` per org (name = org slug) to keep the DB schema uniform. No schema changes needed for this.

### GitHub Account Types

GitHub supports both organization and personal accounts:

```bash
# GitHub organization
deadgit org add mycompany --provider github --pat-env GITHUB_PAT

# GitHub personal account
deadgit org add myusername --provider github --type personal --pat-env MY_GH_PAT
```

Personal accounts use `GET /user/repos` (authenticated user's own repos).  
Org accounts use `GET /orgs/{org}/repos`.

---

## Database Schema

### Changes from PRD-update.md

- `pat_token` column **removed** — PAT lives in env vars only
- `pat_env` column **added** — stores the name of the env var holding the PAT (e.g. `"GITHUB_PAT"`)
- `account_type` column **added** to `organizations` — `'org'` (default) | `'personal'`

### organizations table (final)

```sql
CREATE TABLE IF NOT EXISTS organizations (
  id           INTEGER  PRIMARY KEY AUTOINCREMENT,
  name         TEXT     NOT NULL,
  slug         TEXT     NOT NULL UNIQUE,
  provider     TEXT     NOT NULL DEFAULT 'azure',   -- 'azure' | 'github'
  account_type TEXT     NOT NULL DEFAULT 'org',     -- 'org'  | 'personal'
  base_url     TEXT     NOT NULL DEFAULT 'https://dev.azure.com',
  pat_env      TEXT     NOT NULL,                   -- env var name, e.g. "MYORG_PAT"
  is_active    INTEGER  NOT NULL DEFAULT 1,
  last_synced  DATETIME,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

The `base_url` column is set automatically by `org add` based on `--provider`:
- `azure` → `https://dev.azure.com` (default)
- `github` → `https://api.github.com`

`--provider` defaults to `azure` if omitted.

All other tables (`projects`, `repositories`, `scoring_profiles`, `scoring_profile_history`, `scan_runs`) match PRD-update.md exactly.

---

## CLI Commands

### Global flags
```
--db <path>       Path to SQLite database (default: ~/.deadgit/deadgit.db)
--output <fmt>    Output format: table | json | csv (default: table)
```

### org

```bash
deadgit org add <slug> --name <display-name> --pat-env <ENV_VAR_NAME> [--provider azure|github] [--type org|personal]
deadgit org list
deadgit org remove <slug>
```

`org add` reads the PAT from the named env var **at add time** to validate connectivity, then stores only the env var name in the DB. At scan time, the PAT is re-read from the env var.

### profile

```bash
deadgit profile create <name> [weight flags] [--description "..."]
deadgit profile list
deadgit profile edit <name> [weight flags]
deadgit profile set-default <name>
```

Weight flags: `--w-last-commit`, `--w-last-pr`, `--w-commit-freq`, `--w-branch-staleness`, `--w-no-releases`, `--threshold`, `--score-min`

### scan

```bash
deadgit scan [--org <slug>] [--all-orgs] [--profile <name>] [--refresh] [--ttl 24] [weight flag overrides] [--outfile <path>]
```

**Scoring flow:**
1. Load profile from DB (by name or default)
2. Apply any inline `--w-*` overrides in-memory only (never persisted)
3. Warn if weights don't sum to ~1.0 (tolerance ±0.01); don't hard-fail
4. For each org → project → repo: check cache TTL, fetch from API if stale (or `--refresh`), upsert raw metrics to DB
5. Compute `scoring.Score(metrics, profile)` at runtime — scores are **never stored**
6. Render output

**`--refresh` flag:** bypasses TTL, forces new API pull, updates raw metrics, then re-scores.

**Profile switching:** re-run `scan` with `--profile <name>` against cached data — instant re-score, no API call (unless `--refresh`).

---

## PAT Handling

PAT tokens are **never stored in the database**. The `pat_env` column stores the env var name. At runtime:

```
MYORG_PAT=xxx deadgit scan --org myorg
```

If the env var is unset at scan time, the command fails with a clear error:
```
error: PAT env var "MYORG_PAT" is not set for org "myorg"
```

**ROADMAP:** OS keychain integration via `go-keyring` — see `ROADMAP.md`.

---

## Scoring Engine

Straight from PRD-update.md. Pure functions, no DB access, fully testable.

```
score = (
  days_since_last_commit  * w_last_commit       (default 0.40)
  days_since_last_pr      * w_last_pr           (default 0.20)
  commit_frequency_decay  * w_commit_frequency  (default 0.20)
  stale_branches          * w_branch_staleness  (default 0.10)
  no_recent_releases      * w_no_releases       (default 0.10)
)
```

Hard overrides: archived or disabled repos always score 1.0 (always INACTIVE).

`IsInactive = score >= inactive_score_threshold` (default 0.65).

---

## Output

### Table (default)

```
Scan Results  •  Profile: default  •  Orgs: myorg, anotherorg
┌──────────────┬──────────────┬───────────────────┬────────┬────────────┬──────────────────────┐
│ Org          │ Project      │ Repository        │ Score  │ Status     │ Reasons              │
├──────────────┼──────────────┼───────────────────┼────────┼────────────┼──────────────────────┤
│ myorg        │ platform     │ legacy-api        │ 0.8821 │ ⚠ INACTIVE │ No commits in 210d   │
│ myorg        │ platform     │ active-service    │ 0.1240 │ ✓ active   │                      │
└──────────────┴──────────────┴───────────────────┴────────┴────────────┴──────────────────────┘
Total: 2 repos  •  1 inactive  •  Cached: 1  •  Fetched: 1  •  Duration: 0.82s
```

### JSON / CSV

Full repo metrics + score breakdown written to file. Default filenames:
- `deadgit-report-YYYY-MM-DD.json`
- `deadgit-report-YYYY-MM-DD.csv`

---

## Testing Strategy

- All tests use `net/http/httptest` — no mocking libraries (matches v1 convention)
- External test packages (`_test` suffix)
- Scoring functions tested with table-driven tests (pure functions, trivial to test)
- Provider fetchers tested against local test HTTP servers
- DB layer: integration tests using an in-memory SQLite DB

---

## Deliverables

- [ ] New branch `feat/v2-rewrite`
- [ ] All packages under `internal/` and `cmd/`
- [ ] Migrations embedded via `embed.FS`
- [ ] sqlc-generated code checked in
- [ ] `README.md` — full usage guide (commands, env vars, Azure + GitHub examples, scoring profiles)
- [ ] `ROADMAP.md` — keyring integration, future provider ideas
- [ ] Updated `PRD.md` to reflect v2 scope
- [ ] All existing v1 packages removed
