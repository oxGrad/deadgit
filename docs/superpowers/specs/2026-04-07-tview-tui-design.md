# TUI Design: k9s-style tview interface for deadgit

**Date:** 2026-04-07  
**Status:** Approved

## Overview

Replace the inline `charmbracelet/huh` interactive prompts with a full-screen k9s-style TUI powered by `github.com/rivo/tview`. The TUI launches when `deadgit` is run with no subcommand and no flags. All existing CLI subcommands (`org`, `profile`, `scan` and their flags) are unchanged.

## Architecture

### New package: `internal/tui/`

```
internal/tui/
  app.go        # tview.Application, root layout, navigation, command bar
  scan.go       # scan results table + detail panel
  profiles.go   # profile list table
  orgs.go       # org list table
  forms.go      # profile create/edit form, org add form
  styles.go     # k9s-style color theme and layout constants
```

### New package: `internal/scanner/`

The core scan logic is extracted from `cmd/scan.go` into `internal/scanner/scanner.go` so both the cobra CLI handler and the TUI can call it without duplication.

### `cmd/root.go` change

Add `RunE` to `rootCmd`. When the root command is invoked with no subcommand and no flags, it calls `tui.Run(globalQ)`. The existing `PersistentPreRunE` already opens the DB, so `globalQ` is available.

## Root Layout

```
tview.Flex (vertical, root)
├── header bar          (1 row)   — breadcrumb + nav hints
├── tview.Pages         (fills)   — named pages: scan, profiles, orgs, profile-form, org-form
├── scan progress bar   (2 rows)  — hidden when idle, sticky during active scan
└── help bar            (1 row)   — context-sensitive keybinding hints
```

## Navigation

- **Number keys:** `1` → scan, `2` → profiles, `3` → orgs (shown in header)
- **Command bar:** press `:` to open an `tview.InputField` at the bottom; accepts `scan`, `profiles`, `orgs`; `Esc` dismisses
- `switchTo(name string)` updates `tview.Pages` and refreshes the header breadcrumb

## Views

### Scan view (`scan.go`)

Layout: vertical `tview.Flex`
- Upper: `tview.Table` — columns: ORG, PROJECT, REPO, SCORE, STATUS, CACHED
  - ACTIVE rows: green; DEAD rows: red score; selected row: blue highlight with `▶` prefix
- Lower: detail panel (`tview.TextView`) — toggled with `d`; shows score breakdown, last commit/PR dates, reasons, profile name
  - Hidden by default; appears when `d` is pressed on a selected row

On open: loads last scan results from DB via `globalQ.ListRepositoriesByOrg`.

### Profiles view (`profiles.go`)

`tview.Table` — columns: NAME, VER, COMMIT, PR, FREQ, BRANCH, THRESHOLD, SCORE MIN, DEFAULT  
Default profile row marked with `✓`.

### Orgs view (`orgs.go`)

`tview.Table` — columns: SLUG, PROVIDER, TYPE, STATUS, BASE URL, PAT ENV  
Active orgs: green status; inactive: red.

### Filter (`/` key, all table views)

Pressing `/` opens a single-line `tview.InputField` above the help bar. Typing narrows visible rows by matching against any column value (case-insensitive substring). `Esc` clears the filter and restores all rows.

### Profile form (`forms.go`)

`tview.Form` with fields: Name, Description, Weight: Last Commit, Weight: Last PR, Weight: Commit Freq, Weight: Branch Staleness, Weight: No Releases, Inactive Threshold (days), Score Min, Set as Default.  
Used for both create and edit (pre-populated with existing values on edit).  
No buttons — keybindings only (see below).

### Org add form (`forms.go`)

`tview.Form` with fields: Slug, Display Name, Provider (select: github/azure), Account Type (select: org/personal), PAT Env Var.  
Validates PAT env var is set and connectivity succeeds before saving (same logic as `runOrgAdd`).

## Scan Progress (sticky)

When a scan is triggered (`s` from scan view):
1. A `tview.Modal` prompts org selection (multi-select checklist)
2. A second `tview.Modal` prompts profile selection
3. Scan runs in a background goroutine via `scanner.Run()`
4. Progress bar (2-row panel in root layout) becomes visible and stays visible across all view switches
5. Progress updates sent via channel; `app.QueueUpdateDraw()` used for thread-safe UI updates
6. On completion: progress bar hides; scan view table refreshes with new results

Progress bar format:  
`● scanning myorg · default v3  [████████░░░░░░░░░░] 19/47  legacy-monolith   <esc> cancel`

## Keybindings

| Key | Context | Action |
|-----|---------|--------|
| `1` | global | switch to Scan view |
| `2` | global | switch to Profiles view |
| `3` | global | switch to Orgs view |
| `:` | global | open command bar |
| `q` | global | quit |
| `s` | scan | trigger new scan |
| `d` | scan | toggle detail panel |
| `/` | scan / profiles / orgs | open filter input |
| `n` | profiles / orgs | open create form |
| `e` | profiles | open edit form for selected row |
| `Enter` | profiles | set selected profile as default |
| `Del` / `x` | profiles / orgs | delete / deactivate selected row (confirm modal) |
| `Ctrl+S` | forms | save |
| `Esc` | forms | cancel, return to previous view |
| `Tab` / `Shift+Tab` | forms | next / prev field |

## Color Theme (`styles.go`)

Follows k9s dark palette:
- Background: `#0d1117`
- Header/footer: `#1a1a2e`
- Selected row: `#1f6feb` (blue)
- Active/good: `#3fb950` (green)
- Dead/error: `#f85149` (red)
- Scanning/warning: `#f0883e` (orange)
- Muted text: `#888888`

## What Does NOT Change

- `cmd/org.go`, `cmd/profile.go`, `cmd/scan.go` — cobra commands, all flags, all args
- `cmd/root.go` `PersistentPreRunE` — DB open logic unchanged
- `charmbracelet/huh` forms in `cmd/` — remain for non-interactive / piped CLI use
- All existing tests
