package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

func openTestDB(t *testing.T) (*sql.DB, *dbgen.Queries) {
	t.Helper()
	sqlDB, err := deaddb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() }) //nolint:errcheck
	return sqlDB, dbgen.New(sqlDB)
}

func createTestOrg(t *testing.T, q *dbgen.Queries, slug string) dbgen.Organization {
	t.Helper()
	org, err := q.CreateOrganization(context.Background(), dbgen.CreateOrganizationParams{
		Name:        slug + "-name",
		Slug:        slug,
		Provider:    "github",
		AccountType: "org",
		BaseUrl:     "https://api.github.com",
		PatEnv:      "GITHUB_PAT",
	})
	if err != nil {
		t.Fatalf("CreateOrganization(%s): %v", slug, err)
	}
	return org
}

// --- Organizations ---

func TestCreateAndGetOrganization(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "testorg")
	if org.Slug != "testorg" {
		t.Errorf("Slug: want testorg got %q", org.Slug)
	}
	if org.Provider != "github" {
		t.Errorf("Provider: want github got %q", org.Provider)
	}
	if org.IsActive != 1 {
		t.Errorf("IsActive: want 1 got %d", org.IsActive)
	}

	got, err := q.GetOrganizationBySlug(ctx, "testorg")
	if err != nil {
		t.Fatalf("GetOrganizationBySlug: %v", err)
	}
	if got.ID != org.ID {
		t.Errorf("ID mismatch: want %d got %d", org.ID, got.ID)
	}
}

func TestListOrganizations_OnlyActive(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	createTestOrg(t, q, "active-org")
	inactive := createTestOrg(t, q, "inactive-org")
	if err := q.DeactivateOrganization(ctx, inactive.Slug); err != nil {
		t.Fatalf("DeactivateOrganization: %v", err)
	}

	orgs, err := q.ListOrganizations(ctx)
	if err != nil {
		t.Fatalf("ListOrganizations: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Slug != "active-org" {
		t.Errorf("expected 1 active org, got %v", orgs)
	}
}

func TestListAllOrganizations_IncludesInactive(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	createTestOrg(t, q, "org1")
	org2 := createTestOrg(t, q, "org2")
	if err := q.DeactivateOrganization(ctx, org2.Slug); err != nil {
		t.Fatal(err)
	}

	all, err := q.ListAllOrganizations(ctx)
	if err != nil {
		t.Fatalf("ListAllOrganizations: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 orgs, got %d", len(all))
	}
}

func TestUpdateOrganizationLastSynced(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "sync-org")
	if err := q.UpdateOrganizationLastSynced(ctx, org.ID); err != nil {
		t.Fatalf("UpdateOrganizationLastSynced: %v", err)
	}

	got, err := q.GetOrganizationBySlug(ctx, "sync-org")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastSynced.Valid {
		t.Error("LastSynced should be set after UpdateOrganizationLastSynced")
	}
}

func TestGetOrganizationBySlug_NotFound(t *testing.T) {
	_, q := openTestDB(t)
	_, err := q.GetOrganizationBySlug(context.Background(), "does-not-exist")
	if err == nil {
		t.Error("expected error for missing org")
	}
}

// --- Profiles ---

func TestCreateAndListProfiles(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	p, err := q.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name: "custom", WLastCommit: 0.4, WLastPr: 0.2, WCommitFrequency: 0.2,
		WBranchStaleness: 0.1, WNoReleases: 0.1, InactiveDaysThreshold: 60, InactiveScoreThreshold: 0.6,
	})
	if err != nil {
		t.Fatalf("CreateScoringProfile: %v", err)
	}
	if p.Name != "custom" {
		t.Errorf("Name: want custom got %q", p.Name)
	}
	if p.Version != 1 {
		t.Errorf("Version: want 1 got %d", p.Version)
	}

	profiles, err := q.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	// default profile + custom
	found := false
	for _, pr := range profiles {
		if pr.Name == "custom" {
			found = true
		}
	}
	if !found {
		t.Error("custom profile not found in ListProfiles")
	}
}

func TestGetProfileByName(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	p, err := q.GetProfileByName(ctx, "default")
	if err != nil {
		t.Fatalf("GetProfileByName: %v", err)
	}
	if p.Name != "default" {
		t.Errorf("Name: want default got %q", p.Name)
	}
}

func TestGetProfileByName_NotFound(t *testing.T) {
	_, q := openTestDB(t)
	_, err := q.GetProfileByName(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestSetDefaultProfile(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	_, err := q.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name: "new-default", WLastCommit: 0.5, InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := q.SetDefaultProfile(ctx, "new-default"); err != nil {
		t.Fatalf("SetDefaultProfile: %v", err)
	}

	p, err := q.GetDefaultProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "new-default" {
		t.Errorf("default profile: want new-default got %q", p.Name)
	}
}

func TestUpdateProfile(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	updated, err := q.UpdateProfile(ctx, dbgen.UpdateProfileParams{
		Name:                   "default",
		WLastCommit:            0.6,
		WLastPr:                0.1,
		WCommitFrequency:       0.1,
		WBranchStaleness:       0.1,
		WNoReleases:            0.1,
		InactiveDaysThreshold:  120,
		InactiveScoreThreshold: 0.7,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("Version: want 2 got %d", updated.Version)
	}
	if updated.WLastCommit != 0.6 {
		t.Errorf("WLastCommit: want 0.6 got %f", updated.WLastCommit)
	}
}

func TestInsertProfileHistory(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	p, _ := q.GetDefaultProfile(ctx)
	if err := q.InsertProfileHistory(ctx, dbgen.InsertProfileHistoryParams{
		ProfileID: p.ID,
		Version:   p.Version,
		OldValues: `{"name":"default"}`,
		NewValues: `{"name":"default","updated":true}`,
		ChangedBy: "test",
	}); err != nil {
		t.Fatalf("InsertProfileHistory: %v", err)
	}

	history, err := q.ListProfileHistory(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListProfileHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
	if history[0].ChangedBy != "test" {
		t.Errorf("ChangedBy: want test got %q", history[0].ChangedBy)
	}
}

// --- Projects and Repos ---

func TestUpsertProjectAndListByOrg(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "proj-org")

	proj, err := q.UpsertProject(ctx, dbgen.UpsertProjectParams{
		OrgID:      org.ID,
		Name:       "my-project",
		ExternalID: sql.NullString{String: "ext-123", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if proj.Name != "my-project" {
		t.Errorf("Name: want my-project got %q", proj.Name)
	}

	projects, err := q.ListProjectsByOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListProjectsByOrg: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "my-project" {
		t.Errorf("unexpected projects: %v", projects)
	}

	got, err := q.GetProjectByName(ctx, dbgen.GetProjectByNameParams{OrgID: org.ID, Name: "my-project"})
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	if got.ID != proj.ID {
		t.Errorf("ID mismatch: want %d got %d", proj.ID, got.ID)
	}
}

func TestUpsertAndListRepositories(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "repo-org")
	proj, _ := q.UpsertProject(ctx, dbgen.UpsertProjectParams{OrgID: org.ID, Name: "proj"})

	now := time.Now()
	_, err := q.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{
		ProjectID:         proj.ID,
		Name:              "my-repo",
		RemoteUrl:         "https://github.com/repo-org/my-repo.git",
		DefaultBranch:     sql.NullString{String: "main", Valid: true},
		IsArchived:        0,
		IsDisabled:        0,
		LastCommitAt:      sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -5)},
		CommitCount90d:    sql.NullInt64{Valid: true, Int64: 15},
		ActiveBranchCount: sql.NullInt64{Valid: true, Int64: 2},
	})
	if err != nil {
		t.Fatalf("UpsertRepository: %v", err)
	}

	repos, err := q.ListRepositoriesByOrg(ctx, org.Slug)
	if err != nil {
		t.Fatalf("ListRepositoriesByOrg: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "my-repo" {
		t.Errorf("unexpected repos: %v", repos)
	}
	if !repos[0].LastCommitAt.Valid {
		t.Error("LastCommitAt should be set")
	}

	allRepos, err := q.ListAllRepositories(ctx)
	if err != nil {
		t.Fatalf("ListAllRepositories: %v", err)
	}
	if len(allRepos) != 1 {
		t.Errorf("ListAllRepositories: want 1 got %d", len(allRepos))
	}

	byProj, err := q.ListRepositoriesByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListRepositoriesByProject: %v", err)
	}
	if len(byProj) != 1 {
		t.Errorf("ListRepositoriesByProject: want 1 got %d", len(byProj))
	}
}

func TestListStaleRepositories(t *testing.T) {
	sqlDB, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "stale-org")
	proj, _ := q.UpsertProject(ctx, dbgen.UpsertProjectParams{OrgID: org.ID, Name: "proj"})

	if _, err := q.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{
		ProjectID: proj.ID,
		Name:      "stale-repo",
		RemoteUrl: "https://github.com/stale-org/stale-repo.git",
	}); err != nil {
		t.Fatalf("UpsertRepository: %v", err)
	}

	// Force last_fetched to an old date so the repo appears stale for any TTL.
	if _, err := sqlDB.ExecContext(ctx, `UPDATE repositories SET last_fetched = '2000-01-01 00:00:00' WHERE name = 'stale-repo'`); err != nil {
		t.Fatalf("update last_fetched: %v", err)
	}

	stale, err := q.ListStaleRepositories(ctx, sql.NullString{String: "1", Valid: true})
	if err != nil {
		t.Fatalf("ListStaleRepositories: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale repo, got %d", len(stale))
	}
}

// --- Scan Runs ---

func TestInsertAndListScanRuns(t *testing.T) {
	_, q := openTestDB(t)
	ctx := context.Background()

	org := createTestOrg(t, q, "run-org")
	profile, _ := q.GetDefaultProfile(ctx)

	if err := q.InsertScanRun(ctx, dbgen.InsertScanRunParams{
		OrgID:           sql.NullInt64{Valid: true, Int64: org.ID},
		ProfileID:       sql.NullInt64{Valid: true, Int64: profile.ID},
		ProfileName:     profile.Name,
		ProfileVersion:  profile.Version,
		ProfileSnapshot: `{"name":"default"}`,
		TotalRepos:      10,
		InactiveCount:   3,
	}); err != nil {
		t.Fatalf("InsertScanRun: %v", err)
	}

	runs, err := q.ListScanRuns(ctx)
	if err != nil {
		t.Fatalf("ListScanRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].TotalRepos != 10 {
		t.Errorf("TotalRepos: want 10 got %d", runs[0].TotalRepos)
	}
	if runs[0].InactiveCount != 3 {
		t.Errorf("InactiveCount: want 3 got %d", runs[0].InactiveCount)
	}
}

// --- WithTx ---

func TestWithTx_RollbackOnError(t *testing.T) {
	sqlDB, q := openTestDB(t)
	ctx := context.Background()

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	qtx := q.WithTx(tx)
	_, err = qtx.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "tx-org", Slug: "tx-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// After rollback, org should not exist
	_, err = q.GetOrganizationBySlug(ctx, "tx-org")
	if err == nil {
		t.Error("expected org to be absent after rollback")
	}
}
