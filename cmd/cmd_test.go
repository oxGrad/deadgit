package cmd

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/spf13/cobra"
)

// setupTestDB initialises globalDB and globalQ with a temp SQLite database.
func setupTestDB(t *testing.T) {
	t.Helper()
	sqlDB, err := deaddb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	globalDB = sqlDB
	globalQ = dbgen.New(sqlDB)
	t.Cleanup(func() {
		sqlDB.Close() //nolint:errcheck
		globalDB = nil
		globalQ = nil
	})
}

// --- applyWeightFlags ---

func TestApplyWeightFlags_NoOverrides(t *testing.T) {
	// All profile* vars default to -1 (no override)
	profileWCommit, profileWPR, profileWFreq, profileWBranch, profileWRelease = -1, -1, -1, -1, -1
	profileThresh, profileScoreMin = -1, -1

	wc, wpr, wf, wb, wr := 0.5, 0.2, 0.2, 0.1, 0.0
	thresh, scoreMin := 90, 0.65
	applyWeightFlags(&wc, &wpr, &wf, &wb, &wr, &thresh, &scoreMin)

	if wc != 0.5 || thresh != 90 {
		t.Error("values should be unchanged when flags are all -1")
	}
}

func TestApplyWeightFlags_WithOverrides(t *testing.T) {
	profileWCommit = 0.6
	profileWPR = 0.15
	profileWFreq = 0.1
	profileWBranch = 0.1
	profileWRelease = 0.05
	profileThresh = 30
	profileScoreMin = 0.8
	t.Cleanup(func() {
		profileWCommit, profileWPR, profileWFreq, profileWBranch, profileWRelease = -1, -1, -1, -1, -1
		profileThresh, profileScoreMin = -1, -1
	})

	wc, wpr, wf, wb, wr := 0.5, 0.2, 0.2, 0.1, 0.0
	thresh, scoreMin := 90, 0.65
	applyWeightFlags(&wc, &wpr, &wf, &wb, &wr, &thresh, &scoreMin)

	if wc != 0.6 {
		t.Errorf("wc: want 0.6 got %f", wc)
	}
	if thresh != 30 {
		t.Errorf("thresh: want 30 got %d", thresh)
	}
	if scoreMin != 0.8 {
		t.Errorf("scoreMin: want 0.8 got %f", scoreMin)
	}
}

// --- providerFor ---

func TestProviderFor_MissingPAT(t *testing.T) {
	t.Setenv("MISSING_PAT_ENV", "")
	org := dbgen.Organization{Provider: "github", PatEnv: "MISSING_PAT_ENV"}
	_, err := providerFor(org)
	if err == nil {
		t.Error("expected error when PAT env var is empty")
	}
}

func TestProviderFor_AzureProvider(t *testing.T) {
	t.Setenv("AZURE_TEST_PAT", "test-token")
	org := dbgen.Organization{Provider: "azure", PatEnv: "AZURE_TEST_PAT", BaseUrl: "https://dev.azure.com"}
	p, err := providerFor(org)
	if err != nil {
		t.Fatalf("providerFor azure: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestProviderFor_GithubProvider(t *testing.T) {
	t.Setenv("GH_TEST_PAT", "test-token")
	org := dbgen.Organization{Provider: "github", PatEnv: "GH_TEST_PAT", BaseUrl: "https://api.github.com", AccountType: "org"}
	p, err := providerFor(org)
	if err != nil {
		t.Fatalf("providerFor github: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestProviderFor_UnknownProvider(t *testing.T) {
	t.Setenv("SOME_PAT", "token")
	org := dbgen.Organization{Provider: "bitbucket", PatEnv: "SOME_PAT"}
	_, err := providerFor(org)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

// --- upsertRepo ---

func TestUpsertRepo_CreatesEntry(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	org, _ := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "upsert-org", Slug: "upsert-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	proj, _ := globalQ.UpsertProject(ctx, dbgen.UpsertProjectParams{OrgID: org.ID, Name: "proj"})

	now := time.Now()
	upsertRepo(ctx, proj.ID, providers.RepoData{
		Name:           "test-repo",
		RemoteURL:      "https://github.com/org/test-repo.git",
		DefaultBranch:  "main",
		LastCommitAt:   &now,
		CommitCount90d: 10,
	}, globalLog)

	repos, err := globalQ.ListRepositoriesByOrg(ctx, "upsert-org")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "test-repo" {
		t.Errorf("expected test-repo, got %v", repos)
	}
}

// --- recordScanRun ---

func TestRecordScanRun_Inserts(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	org, _ := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "run-org", Slug: "run-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	profile, _ := globalQ.GetDefaultProfile(ctx)

	recordScanRun(ctx, org.ID, profile.ID, dbProfileToScoringProfile(profile), 5, 2)

	runs, err := globalQ.ListScanRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 scan run, got %d", len(runs))
	}
	if runs[0].TotalRepos != 5 || runs[0].InactiveCount != 2 {
		t.Errorf("unexpected run: %+v", runs[0])
	}
}

// --- runProfileList / runProfileCreate ---

func TestRunProfileList(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runProfileList(cmd, nil); err != nil {
		t.Fatalf("runProfileList: %v", err)
	}
}

func TestRunProfileCreate_FromFlags(t *testing.T) {
	setupTestDB(t)

	profileWCommit = 0.5
	profileWPR = 0.2
	profileWFreq = 0.15
	profileWBranch = 0.1
	profileWRelease = 0.05
	profileThresh = 60
	profileScoreMin = 0.65
	profileDefault = false
	profileDesc = "test profile"
	t.Cleanup(func() {
		profileWCommit, profileWPR, profileWFreq, profileWBranch, profileWRelease = -1, -1, -1, -1, -1
		profileThresh, profileScoreMin = -1, -1
		profileDesc = ""
	})

	cmd := &cobra.Command{}
	if err := runProfileCreate(cmd, []string{"test-profile"}); err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	p, err := globalQ.GetProfileByName(context.Background(), "test-profile")
	if err != nil {
		t.Fatalf("profile not created: %v", err)
	}
	if p.WLastCommit != 0.5 {
		t.Errorf("WLastCommit: want 0.5 got %f", p.WLastCommit)
	}
}

func TestRunProfileCreate_EmptyName(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runProfileCreate(cmd, nil); err == nil {
		t.Error("expected error for missing profile name")
	}
}

func TestRunProfileSetDefault(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	profileWCommit, profileWPR, profileWFreq, profileWBranch, profileWRelease = -1, -1, -1, -1, -1
	profileThresh, profileScoreMin = -1, -1

	_, err := globalQ.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name: "new-default", WLastCommit: 0.5, InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	if err := runProfileSetDefault(cmd, []string{"new-default"}); err != nil {
		t.Fatalf("runProfileSetDefault: %v", err)
	}

	p, err := globalQ.GetDefaultProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "new-default" {
		t.Errorf("default: want new-default got %q", p.Name)
	}
}

// --- runOrgList / runOrgRemove ---

func TestRunOrgList_Empty(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runOrgList(cmd, nil); err != nil {
		t.Fatalf("runOrgList: %v", err)
	}
}

func TestRunOrgList_WithOrgs(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	_, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "list-org", Slug: "list-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	if err := runOrgList(cmd, nil); err != nil {
		t.Fatalf("runOrgList: %v", err)
	}
}

func TestRunOrgRemove(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	_, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "to-remove", Slug: "to-remove", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	if err := runOrgRemove(cmd, []string{"to-remove"}); err != nil {
		t.Fatalf("runOrgRemove: %v", err)
	}

	orgs, _ := globalQ.ListOrganizations(ctx)
	for _, o := range orgs {
		if o.Slug == "to-remove" {
			t.Error("org should be deactivated")
		}
	}
}

// --- resolveOrgsToScan ---

func TestResolveOrgsToScan_BySlug(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	_, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "scan-org", Slug: "scan-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reset and set scanOrgs
	old := scanOrgs
	scanOrgs = []string{"scan-org"}
	t.Cleanup(func() { scanOrgs = old })

	cmd := &cobra.Command{}
	orgs, err := resolveOrgsToScan(ctx, cmd)
	if err != nil {
		t.Fatalf("resolveOrgsToScan: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Slug != "scan-org" {
		t.Errorf("unexpected orgs: %v", orgs)
	}
}

func TestResolveOrgsToScan_AllOrgs(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	for _, slug := range []string{"org-a", "org-b"} {
		_, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
			Name: slug, Slug: slug, Provider: "github",
			AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	old := scanAllOrgs
	scanAllOrgs = true
	t.Cleanup(func() { scanAllOrgs = old })

	cmd := &cobra.Command{}
	orgs, err := resolveOrgsToScan(ctx, cmd)
	if err != nil {
		t.Fatalf("resolveOrgsToScan: %v", err)
	}
	if len(orgs) < 2 {
		t.Errorf("expected at least 2 orgs, got %d", len(orgs))
	}
}

func TestResolveOrgsToScan_UnknownSlug(t *testing.T) {
	setupTestDB(t)

	old := scanOrgs
	scanOrgs = []string{"does-not-exist"}
	t.Cleanup(func() { scanOrgs = old })

	cmd := &cobra.Command{}
	_, err := resolveOrgsToScan(context.Background(), cmd)
	if err == nil {
		t.Error("expected error for unknown org slug")
	}
}

// --- dbgen.New / WithTx (ensure they're exercised) ---

func TestQueriesNew(t *testing.T) {
	setupTestDB(t)
	if globalQ == nil {
		t.Error("globalQ should be non-nil after setup")
	}
}

// Ensure scanOrgs has a zero value after test cleanup — use sql.NullString
var _ = sql.NullString{}
