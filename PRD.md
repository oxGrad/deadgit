# deadgit v2 — Product Requirements Document

**Status:** Implemented  
**Design spec:** `docs/superpowers/specs/2026-04-02-deadgit-v2-design.md`

---

## Overview

deadgit v2 is a multi-provider repository inactivity scanner with a local SQLite cache and configurable weighted scoring profiles.

**What changed from v1:**

- **Dropped:** pipeline YAML template detection, single-org Azure-only model, stateless CLI
- **Added:** GitHub provider, multi-org support, SQLite-backed metrics cache, versioned scoring profiles, interactive TUI via `charmbracelet/huh`, Homebrew-ready binary distribution

---

## Requirements

### Providers

- **R1.** Support Azure DevOps and GitHub as providers.
- **R2.** Support GitHub organization accounts (`GET /orgs/{org}/repos`) and personal accounts (`GET /user/repos`).
- **R3.** All provider-specific logic is isolated behind a `Provider` interface. Adding a new provider must not require changes to the CLI or scoring layers.

### Database

- **R4.** All raw repository metrics are stored in a local SQLite database (default: `~/.deadgit/deadgit.db`).
- **R5.** PAT tokens are never stored. Only the env var name (`pat_env`) is stored per org.
- **R6.** The database schema is managed by embedded SQL migrations that run automatically on startup. No manual setup is required.
- **R7.** Scores are computed at runtime and never persisted.

### CLI Commands

- **R8.** `org add [slug]` — registers an org with provider, account type, display name, and PAT env var name. Falls back to interactive prompts when required fields are missing and stdout is a TTY.
- **R9.** `org list` — lists all registered orgs.
- **R10.** `org remove <slug>` — soft-deactivates an org (data retained).
- **R11.** `profile create [name]` — creates a scoring profile with configurable weights, threshold, and score minimum. Falls back to interactive prompts when name is missing and stdout is a TTY.
- **R12.** `profile list` — lists all profiles with version and weights.
- **R13.** `profile edit [name]` — updates an existing profile in place, incrementing its version and recording a history row. Profiles cannot be deleted.
- **R14.** `profile set-default [name]` — sets the default profile used by `scan` when `--profile` is omitted.
- **R15.** `scan` — scans repos for inactivity. Accepts `--org`, `--all-orgs`, `--profile`, `--refresh`, `--ttl`, `--outfile`, and inline weight override flags. Falls back to interactive org/profile selection when no org is specified and stdout is a TTY.

### Caching

- **R16.** Raw metrics fetched from the API are cached in SQLite with a configurable TTL (default: 24 hours).
- **R17.** `--refresh` bypasses the TTL and forces a new API fetch.
- **R18.** Changing the scoring profile or weights never requires `--refresh` — re-scoring runs entirely against cached data.

### Scoring

- **R19.** A scoring profile defines five weights (`w_last_commit`, `w_last_pr`, `w_commit_freq`, `w_branch_staleness`, `w_no_releases`), an inactive days threshold, and an inactive score threshold.
- **R20.** Default weights: 0.40 / 0.20 / 0.20 / 0.10 / 0.10. Default score threshold: 0.65.
- **R21.** Archived and disabled repos always score 1.0 (always INACTIVE).
- **R22.** Inline weight overrides via `scan --w-*` flags apply in memory only and are never persisted.
- **R23.** A default profile is created automatically on first run.

### Output Formats

- **R24.** Table output (default): renders to stdout with profile name, version, org slugs, inactive count, cache stats, and duration in the header/footer.
- **R25.** JSON output: writes a document with a `profile`, `profile_version`, `scanned_at`, and `repos` array to a file.
- **R26.** CSV output: writes all fields including profile name and version to a file.
- **R27.** Default filenames for file-based formats: `deadgit-report-YYYY-MM-DD.json` / `.csv`.

### Interactive Mode

- **R28.** Interactive prompts (via `charmbracelet/huh`) activate only when required args are missing and stdout is a TTY.
- **R29.** Flags always take precedence over interactive input.
- **R30.** Piping output (`deadgit scan | jq`) suppresses all interactive prompts automatically.

### PAT Handling

- **R31.** PATs are read from environment variables at runtime. The env var name is stored per org.
- **R32.** If the env var is unset at scan time, the command fails immediately with a clear error message identifying the missing variable and org.

---

## Out of Scope (v2)

- Pipeline YAML template detection (dropped from v1)
- OS keychain integration (planned for v2.x — see `ROADMAP.md`)
- GitLab, Bitbucket (planned for v2.x)
- Web dashboard (planned for v2.x)
- Scheduled / automated scans (planned for v3.x)
