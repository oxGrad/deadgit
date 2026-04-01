# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o deadgit .

# Run all tests
go test ./...

# Run tests for a specific package
go test ./azuredevops/...
go test ./report/...
go test ./pipeline/...

# Run a single test
go test ./azuredevops/... -run TestClientGet_Retry429 -v

# Run the binary
AZURE_DEVOPS_ORG=myorg AZURE_DEVOPS_PAT=mytoken ./deadgit --output=table
AZURE_DEVOPS_ORG=myorg AZURE_DEVOPS_PAT=mytoken ./deadgit --output=json --project=MyProject
AZURE_DEVOPS_ORG=myorg AZURE_DEVOPS_PAT=mytoken ./deadgit --output=csv --inactive-days=30 --workers=10
```

## Architecture

The program scans all Azure DevOps repositories in an organization and produces an activity report. It runs a concurrent worker pool where each worker processes one repo at a time.

**Data flow:**
1. `cmd/cli.go` — entry point, parses config, lists all projects → repos, feeds a job channel to `Workers` goroutines
2. Each worker calls `scanRepo()` in `cmd/cli.go`, which calls the `azuredevops/` package functions and assembles a `report.RepoReport`
3. Results are collected and dispatched to one of three output formats via `report/`

**Key design decisions:**

- `azuredevops/client.go` — `Client.Get` (JSON decode) and `Client.GetRaw` (bytes) share identical retry logic: 3 attempts, exponential backoff starting at 500ms, honors `Retry-After` header on 429. Non-retriable errors (non-429, non-5xx) return immediately. **404 is not retried** — callers like `items.go` check `strings.Contains(err.Error(), "HTTP 404")` to handle missing pipeline folders gracefully.

- `azuredevops/items.go` — `ListPipelineFolder` calls the Items API with `scopePath=/pipeline&recursionLevel=OneLevel`, then filters results through `pipeline.MatchesPipelineGlob` to find `*.pipeline.yaml` / `*.pipeline.yml` blobs only.

- `pipeline/parser.go` — YAML parsing never returns an error; bad YAML yields an empty `ExtendsPipeline` string. `MatchesPipelineGlob` requires the path to be exactly `/pipeline/<name>` (not nested).

- `report/activity.go` — `DetermineActivityStatus` priority: DISABLED → DORMANT (nil LastCommitAnyBranch) → ACTIVE (commit within threshold OR OpenPRCount > 0) → INACTIVE.

- `report/table.go` — uses `tablewriter` v1.1.4 API (`Header()` / `Append()` / `Render()`), which differs from the v0.x API (`SetHeader()` used in most online examples).

- All tests use `net/http/httptest` — no mocking libraries. Tests are in `_test` packages (external test style).

## Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `AZURE_DEVOPS_ORG` | Organization name (required) | — |
| `AZURE_DEVOPS_PAT` | Personal Access Token (required) | — |
| `INACTIVE_DAYS_THRESHOLD` | Override `--inactive-days` flag | 90 |
| `WORKER_COUNT` | Override `--workers` flag | 5 |
