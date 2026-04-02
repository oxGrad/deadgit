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
