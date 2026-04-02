CREATE TABLE IF NOT EXISTS organizations (
  id           INTEGER  PRIMARY KEY AUTOINCREMENT,
  name         TEXT     NOT NULL,
  slug         TEXT     NOT NULL UNIQUE,
  provider     TEXT     NOT NULL DEFAULT 'github',
  account_type TEXT     NOT NULL DEFAULT 'org',
  base_url     TEXT     NOT NULL DEFAULT 'https://api.github.com',
  pat_env      TEXT     NOT NULL,
  is_active    INTEGER  NOT NULL DEFAULT 1,
  last_synced  DATETIME,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id           INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name             TEXT    NOT NULL,
  external_id      TEXT,
  last_synced      DATETIME,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(org_id, name)
);

CREATE TABLE IF NOT EXISTS repositories (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name                TEXT    NOT NULL,
  remote_url          TEXT    NOT NULL,
  external_id         TEXT    UNIQUE,
  default_branch      TEXT,
  is_archived         INTEGER NOT NULL DEFAULT 0,
  is_disabled         INTEGER NOT NULL DEFAULT 0,
  last_commit_at      DATETIME,
  last_push_at        DATETIME,
  last_pr_merged_at   DATETIME,
  last_pr_created_at  DATETIME,
  commit_count_90d    INTEGER,
  active_branch_count INTEGER,
  contributor_count   INTEGER,
  last_fetched        DATETIME,
  raw_api_blob        TEXT,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, name)
);

CREATE TABLE IF NOT EXISTS scoring_profiles (
  id                       INTEGER PRIMARY KEY AUTOINCREMENT,
  name                     TEXT    NOT NULL UNIQUE,
  description              TEXT,
  version                  INTEGER NOT NULL DEFAULT 1,
  is_default               INTEGER NOT NULL DEFAULT 0,
  w_last_commit            REAL    NOT NULL DEFAULT 0.40,
  w_last_pr                REAL    NOT NULL DEFAULT 0.20,
  w_commit_frequency       REAL    NOT NULL DEFAULT 0.20,
  w_branch_staleness       REAL    NOT NULL DEFAULT 0.10,
  w_no_releases            REAL    NOT NULL DEFAULT 0.10,
  inactive_days_threshold  INTEGER NOT NULL DEFAULT 90,
  inactive_score_threshold REAL    NOT NULL DEFAULT 0.65,
  created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scoring_profile_history (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id INTEGER NOT NULL REFERENCES scoring_profiles(id) ON DELETE CASCADE,
  version    INTEGER NOT NULL,
  changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  old_values TEXT     NOT NULL,
  new_values TEXT     NOT NULL,
  changed_by TEXT     NOT NULL DEFAULT 'cli'
);

CREATE TABLE IF NOT EXISTS scan_runs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id           INTEGER REFERENCES organizations(id) ON DELETE SET NULL,
  profile_id       INTEGER REFERENCES scoring_profiles(id) ON DELETE SET NULL,
  profile_name     TEXT    NOT NULL,
  profile_version  INTEGER NOT NULL,
  profile_snapshot TEXT    NOT NULL,
  total_repos      INTEGER NOT NULL DEFAULT 0,
  inactive_count   INTEGER NOT NULL DEFAULT 0,
  scanned_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO scoring_profiles (
  name, description, version, is_default,
  w_last_commit, w_last_pr, w_commit_frequency,
  w_branch_staleness, w_no_releases,
  inactive_days_threshold, inactive_score_threshold
) VALUES (
  'default', 'Default balanced scoring profile', 1, 1,
  0.40, 0.20, 0.20, 0.10, 0.10, 90, 0.65
);
