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
  description              = ?,
  w_last_commit            = ?,
  w_last_pr                = ?,
  w_commit_frequency       = ?,
  w_branch_staleness       = ?,
  w_no_releases            = ?,
  inactive_days_threshold  = ?,
  inactive_score_threshold = ?,
  version                  = version + 1,
  updated_at               = CURRENT_TIMESTAMP
WHERE name = ?
RETURNING *;

-- name: SetDefaultProfile :exec
UPDATE scoring_profiles SET is_default = CASE WHEN name = ? THEN 1 ELSE 0 END;

-- name: InsertProfileHistory :exec
INSERT INTO scoring_profile_history (profile_id, version, old_values, new_values, changed_by)
VALUES (?, ?, ?, ?, ?);

-- name: ListProfileHistory :many
SELECT * FROM scoring_profile_history
WHERE profile_id = ?
ORDER BY changed_at DESC;
