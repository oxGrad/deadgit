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
