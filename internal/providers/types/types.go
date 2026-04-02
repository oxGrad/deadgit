package types

import "time"

// Organization mirrors the DB organizations row for provider use.
type Organization struct {
	ID          int64
	Slug        string
	Name        string
	Provider    string // "azure" | "github"
	AccountType string // "org" | "personal"
	BaseURL     string
	PatEnv      string
}

// Project mirrors the DB projects row.
type Project struct {
	ID         int64
	Name       string
	ExternalID string
}

// RepoData holds raw API data for one repository.
// Upserted to DB as-is — no computed values.
type RepoData struct {
	Name              string
	RemoteURL         string
	ExternalID        string
	DefaultBranch     string
	IsArchived        bool
	IsDisabled        bool
	LastCommitAt      *time.Time
	LastPushAt        *time.Time
	LastPRMergedAt    *time.Time
	LastPRCreatedAt   *time.Time
	CommitCount90d    int
	ActiveBranchCount int
	ContributorCount  int
	RawAPIBlob        string
}

// Provider is the interface both Azure and GitHub fetchers implement.
type Provider interface {
	// ListProjects returns all projects for the org.
	// GitHub fetchers return a single stub project (org slug as name).
	ListProjects(org Organization) ([]Project, error)

	// FetchRepos returns full RepoData for every repository in a project.
	FetchRepos(org Organization, project Project) ([]RepoData, error)
}
