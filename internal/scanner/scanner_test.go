package scanner_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/scanner"
	"github.com/oxGrad/deadgit/internal/scoring"
)

func setupDB(t *testing.T) *dbgen.Queries {
	t.Helper()
	sqlDB, err := deaddb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() }) //nolint:errcheck
	return dbgen.New(sqlDB)
}

// stubProvider returns repos for the given project without making HTTP calls.
func stubProvider(repos map[string][]providers.RepoData) scanner.ProviderFunc {
	return func(org dbgen.Organization) (providers.Provider, error) {
		return &fakeProvider{repos: repos}, nil
	}
}

type fakeProvider struct {
	repos map[string][]providers.RepoData
}

func (f *fakeProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	projects := make([]providers.Project, 0, len(f.repos))
	for name := range f.repos {
		projects = append(projects, providers.Project{Name: name})
	}
	return projects, nil
}

func (f *fakeProvider) FetchRepos(org providers.Organization, proj providers.Project) ([]providers.RepoData, error) {
	return f.repos[proj.Name], nil
}

func TestRun_ScoresRepos(t *testing.T) {
	q := setupDB(t)
	ctx := context.Background()

	org, err := q.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "testorg", Slug: "testorg", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "FAKE_PAT",
	})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = q.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name: "testprofile", IsDefault: 1,
		WLastCommit: 0.5, WLastPr: 0.2, WCommitFrequency: 0.2,
		WBranchStaleness: 0.1, WNoReleases: 0.0,
		InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	recent := time.Now().Add(-2 * 24 * time.Hour)
	repoData := map[string][]providers.RepoData{
		"backend": {
			{
				Name:           "api-service",
				LastCommitAt:   &recent,
				CommitCount90d: 30,
			},
		},
	}

	profile := scoring.ScoringProfile{
		Name: "default", Version: 1,
		WLastCommit: 0.5, WLastPR: 0.2, WCommitFrequency: 0.2,
		WBranchStaleness: 0.1, WNoReleases: 0.0,
		InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	}

	cfg := scanner.Config{
		Orgs:    []dbgen.Organization{org},
		Profile: profile,
		Workers: 2,
		TTL:     24,
		Refresh: true,
	}

	rows, inactive, err := scanner.Run(ctx, q, cfg, stubProvider(repoData), nil)
	if err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Repo != "api-service" {
		t.Errorf("want repo api-service, got %s", rows[0].Repo)
	}
	if inactive < 0 {
		t.Error("inactive count should not be negative")
	}
}

func TestRun_EmptyOrg(t *testing.T) {
	q := setupDB(t)
	ctx := context.Background()

	org, _ := q.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "empty", Slug: "empty", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "FAKE",
	})

	cfg := scanner.Config{
		Orgs:    []dbgen.Organization{org},
		Profile: scoring.ScoringProfile{},
		Workers: 1,
		TTL:     24,
	}
	rows, _, err := scanner.Run(ctx, q, cfg, stubProvider(nil), nil)
	if err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows for empty org, got %d", len(rows))
	}
}

func TestDBProfileToScoringProfile(t *testing.T) {
	dbp := dbgen.ScoringProfile{
		Name: "p1", Version: 2,
		WLastCommit: 0.5, WLastPr: 0.2, WCommitFrequency: 0.2,
		WBranchStaleness: 0.1, WNoReleases: 0.0,
		InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	}
	sp := scanner.DBProfileToScoringProfile(dbp)
	if sp.Name != "p1" {
		t.Errorf("want name p1, got %s", sp.Name)
	}
	if sp.Version != 2 {
		t.Errorf("want version 2, got %d", sp.Version)
	}
	if sp.WLastCommit != 0.5 {
		t.Errorf("want WLastCommit 0.5, got %f", sp.WLastCommit)
	}
}

func TestDefaultProviderFor_MissingPAT(t *testing.T) {
	t.Setenv("MISSING_ENV", "")
	org := dbgen.Organization{Provider: "github", PatEnv: "MISSING_ENV"}
	_, err := scanner.DefaultProviderFor(org)
	if err == nil {
		t.Error("expected error when PAT env var is empty")
	}
}

func TestDBOrgToProviderOrg(t *testing.T) {
	dbo := dbgen.Organization{
		ID: 1, Slug: "myorg", Name: "My Org",
		Provider: "github", AccountType: "org",
		BaseUrl: "https://api.github.com", PatEnv: "GH_PAT",
	}
	po := scanner.DBOrgToProviderOrg(dbo)
	if po.Slug != "myorg" {
		t.Errorf("want slug myorg, got %s", po.Slug)
	}
	if po.BaseURL != "https://api.github.com" {
		t.Errorf("want BaseURL, got %s", po.BaseURL)
	}
}
