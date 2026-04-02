-- name: InsertScanRun :exec
INSERT INTO scan_runs (org_id, profile_id, profile_name, profile_version, profile_snapshot, total_repos, inactive_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListScanRuns :many
SELECT * FROM scan_runs ORDER BY scanned_at DESC LIMIT 50;
