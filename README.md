# deadgit

**Repository inactivity scanner for Azure DevOps and GitHub.**

[![Go Report Card](https://goreportcard.com/badge/github.com/oxGrad/deadgit)](https://goreportcard.com/report/github.com/oxGrad/deadgit)
[![CI](https://github.com/oxGrad/deadgit/actions/workflows/ci.yml/badge.svg)](https://github.com/oxGrad/deadgit/actions/workflows/ci.yml)
[![Release](https://github.com/oxGrad/deadgit/actions/workflows/release.yml/badge.svg)](https://github.com/oxGrad/deadgit/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

deadgit scans repositories across one or more organizations, scores them using configurable weighted metrics, and reports which repos are inactive. Results are cached in a local SQLite database so re-scoring with different profiles is instant.

---

## Quick Start

```bash
# 1. Install
brew install deadgit

# 2. Set your PAT in an env var (name is up to you — you register it per org)
export GITHUB_PAT=ghp_...

# 3. Register an org
deadgit org add mycompany --provider github --pat-env GITHUB_PAT

# 4. Run a scan
deadgit scan --org mycompany
```

---

## Installation

**Homebrew (recommended):**

```bash
brew install deadgit
```

**From source:**

```bash
go install github.com/oxGrad/deadgit@latest
```

---

## Configuration

Two global flags are available on every command:

| Flag       | Default                 | Description                              |
| ---------- | ----------------------- | ---------------------------------------- |
| `--db`     | `~/.deadgit/deadgit.db` | Path to the SQLite database              |
| `--output` | `table`                 | Output format: `table`, `json`, or `csv` |

The database is created automatically on first run. Migrations run on startup — no setup required.

---

## Managing Organizations

### Adding an organization

**GitHub organization (non-interactive):**

```bash
deadgit org add mycompany --name "My Company" --provider github --pat-env GITHUB_PAT
```

**GitHub personal account:**

```bash
deadgit org add myusername --provider github --type personal --pat-env MY_GH_PAT
```

**Azure DevOps:**

```bash
deadgit org add myorg --name "My Org" --provider azure --pat-env AZURE_PAT
```

**Interactive fallback** — any missing flags are prompted when running in a terminal:

```bash
# Slug provided, rest prompted interactively
deadgit org add mycompany

# Fully interactive — all fields prompted
deadgit org add
```

`org add` flags:

| Flag         | Default    | Description                         |
| ------------ | ---------- | ----------------------------------- |
| `--name`     | (slug)     | Display name                        |
| `--provider` | `github`   | Provider: `github` or `azure`       |
| `--type`     | `org`      | Account type: `org` or `personal`   |
| `--pat-env`  | (required) | Name of the env var holding the PAT |

**Note on PAT security:** deadgit never stores the PAT in the database. Only the env var _name_ is stored. The PAT is read from the environment at scan time. If the env var is unset, the scan fails with a clear error.

### Listing organizations

```bash
deadgit org list
```

Prints a table of all registered organizations including provider, account type, status, and which env var holds the PAT.

### Removing an organization

```bash
deadgit org remove mycompany
```

Deactivates the organization (soft delete — data is retained).

---

## Scoring Profiles

deadgit uses weighted scoring to determine whether a repository is inactive. A **scoring profile** defines the weights for each metric and the threshold at which a repo is considered inactive.

### How scoring works

```
score = (days_since_last_commit  * w_last_commit)     # default weight: 0.40
      + (days_since_last_pr      * w_last_pr)         # default weight: 0.20
      + (commit_frequency_decay  * w_commit_frequency) # default weight: 0.20
      + (stale_branch_ratio      * w_branch_staleness) # default weight: 0.10
      + (no_recent_releases      * w_no_releases)      # default weight: 0.10
```

Scores range from 0.0 (very active) to 1.0 (completely inactive). A repo is marked **INACTIVE** when `score >= inactive_score_threshold` (default: 0.65).

Archived and disabled repos always score 1.0 and are always INACTIVE.

### Default profile

A default profile is created automatically on first run with the weights shown above. You can edit it or create additional profiles for different use cases.

### Creating a profile

```bash
# Non-interactive
deadgit profile create aggressive --w-last-commit 0.6 --w-last-pr 0.1 --w-commit-freq 0.1 --w-branch-staleness 0.1 --w-no-releases 0.1

# Interactive (name prompted)
deadgit profile create
```

Weight flags for `profile create` and `profile edit`:

| Flag                   | Default | Description                                |
| ---------------------- | ------- | ------------------------------------------ |
| `--w-last-commit`      | `0.40`  | Weight for days since last commit          |
| `--w-last-pr`          | `0.20`  | Weight for days since last PR              |
| `--w-commit-freq`      | `0.20`  | Weight for commit frequency over 90 days   |
| `--w-branch-staleness` | `0.10`  | Weight for stale branch ratio              |
| `--w-no-releases`      | `0.10`  | Weight for absence of recent releases      |
| `--threshold`          | `90`    | Inactive days threshold                    |
| `--score-min`          | `0.65`  | Score at or above which a repo is INACTIVE |
| `--description`        |         | Optional description                       |

### Listing profiles

```bash
deadgit profile list
```

Shows each profile's name, current version, weights, threshold, and whether it is the default.

### Editing a profile

```bash
# Change only the weights you want to adjust — others are preserved
deadgit profile edit aggressive --w-last-commit 0.5 --w-last-pr 0.2

# Interactive — select profile from list, then adjust
deadgit profile edit
```

Each edit increments the profile version and records the change in history. Profiles are never deleted — they are versioned so that every scan run can be reproduced exactly by referencing the profile name and version.

### Setting the default profile

```bash
deadgit profile set-default aggressive

# Interactive
deadgit profile set-default
```

---

## Scanning

### Scan a single org

```bash
deadgit scan --org mycompany
```

### Scan all registered orgs

```bash
deadgit scan --all-orgs
```

### Use a specific scoring profile

```bash
deadgit scan --org mycompany --profile aggressive
```

Omitting `--profile` uses whichever profile is set as default.

### Cache and refresh

deadgit caches raw repository metrics in SQLite. By default, cached data is reused for 24 hours.

```bash
# Force a fresh API pull regardless of cache age
deadgit scan --org mycompany --refresh

# Change the cache TTL (in hours)
deadgit scan --org mycompany --ttl 48
```

Switching profiles against cached data does not require `--refresh` — scoring runs in memory against whatever is already stored.

### Inline weight overrides

You can override profile weights for a single scan without modifying the saved profile. The table header will show `(overrides active)` to make this visible.

```bash
deadgit scan --org mycompany --w-last-commit 0.6 --w-last-pr 0.1
```

Overrides are never persisted. To save a new weighting permanently, use `profile create` or `profile edit`.

### Output to file

```bash
# Write JSON to a specific path
deadgit scan --org mycompany --output json --outfile results.json

# Default filename when --outfile is omitted: deadgit-report-YYYY-MM-DD.json
deadgit scan --org mycompany --output json
```

### Interactive fallback

When no org is specified and the terminal is interactive, deadgit prompts you to select orgs and a profile:

```bash
deadgit scan
# → multi-select: choose orgs to scan
# → selector: choose scoring profile (name + version shown)
# → confirm refresh? (y/n)
```

### Scan flags

| Flag         | Default           | Description                       |
| ------------ | ----------------- | --------------------------------- |
| `--org`      |                   | Org slug(s) to scan (repeatable)  |
| `--all-orgs` |                   | Scan all active orgs              |
| `--profile`  | (default profile) | Scoring profile name              |
| `--refresh`  | `false`           | Force re-fetch, bypass cache      |
| `--ttl`      | `24`              | Cache TTL in hours                |
| `--outfile`  |                   | Output file path (json/csv modes) |
| `--workers`  | `5`               | Concurrent workers                |

---

## Output Formats

### Table (default)

```
Scan Results  •  Profile: default v2  •  Orgs: myorg, anotherorg
┌──────────────┬──────────────┬───────────────────┬────────┬────────────┬──────────────────────┐
│ Org          │ Project      │ Repository        │ Score  │ Status     │ Reasons              │
├──────────────┼──────────────┼───────────────────┼────────┼────────────┼──────────────────────┤
│ myorg        │ platform     │ legacy-api        │ 0.8821 │ ⚠ INACTIVE │ No commits in 210d   │
│ myorg        │ platform     │ active-service    │ 0.1240 │ ✓ active   │                      │
└──────────────┴──────────────┴───────────────────┴────────┴────────────┴──────────────────────┘
Total: 2 repos  •  1 inactive  •  Cached: 1  •  Fetched: 1  •  Duration: 0.82s
```

If inline weight overrides are active, the header shows `Profile: default v2 (overrides active)`.

### JSON

Written to a file (default: `deadgit-report-YYYY-MM-DD.json`):

```json
{
  "profile": "default",
  "profile_version": 2,
  "scanned_at": "2026-04-02T10:00:00Z",
  "repos": [
    {
      "org": "myorg",
      "project": "platform",
      "repo": "legacy-api",
      "score": 0.8821,
      "is_inactive": true,
      "reasons": ["No commits in 210d"]
    }
  ]
}
```

### CSV

Written to a file (default: `deadgit-report-YYYY-MM-DD.csv`). Includes profile name and version as columns.

---

## Environment Variables

| Variable        | Required      | Description                                                                                            |
| --------------- | ------------- | ------------------------------------------------------------------------------------------------------ |
| `<PAT_ENV_VAR>` | Yes (per org) | PAT token for the org. The variable name is configured per-org via `--pat-env` when running `org add`. |

There are no other required environment variables. All configuration is stored in the SQLite database or passed as CLI flags.

---

## Interactive vs CI Mode

deadgit uses TTY detection to decide when to show interactive prompts. If stdout is not a TTY, all interactive prompts are suppressed and the tool behaves as a pure CLI.

- **Flags always win.** Interactive prompts only fill in what is missing.
- `deadgit scan | jq` works correctly — no prompts, output goes to jq.
- CI pipelines work without any special flags — just provide `--org` or `--all-orgs`.

---

## Azure DevOps Setup

1. Create a Personal Access Token at `https://dev.azure.com/<org>/_usersSettings/tokens`
2. Grant the following permissions:
   - **Code:** Read
   - **Project and Team:** Read
   - **Pull Request Threads:** Read (for PR data)
3. Store the token in an env var and register the org:

```bash
export AZURE_PAT=<your-token>
deadgit org add myorg --provider azure --pat-env AZURE_PAT
```

---

## GitHub Setup

1. Create a Personal Access Token at `https://github.com/settings/tokens`
2. Grant the following permissions:
   - **repo** scope (for private repositories)
   - For public repositories only: **public_repo** is sufficient
3. Store the token in an env var and register the org:

```bash
export GITHUB_PAT=ghp_...
deadgit org add mycompany --provider github --pat-env GITHUB_PAT
```

**GitHub Enterprise:** register with a custom base URL by editing the database directly after `org add`, or set `--provider github` and contact the maintainers for a `--base-url` flag (planned).
