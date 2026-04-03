-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, provider, account_type, base_url, pat_env)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = ? AND is_active = 1 LIMIT 1;

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
