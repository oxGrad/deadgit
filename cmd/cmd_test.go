package cmd

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/scoring"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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
	globalLog = zap.NewNop()
	t.Cleanup(func() {
		sqlDB.Close() //nolint:errcheck
		globalDB = nil
		globalQ = nil
		globalLog = nil
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

	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "upsert-org", Slug: "upsert-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
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

	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "run-org", Slug: "run-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
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

func TestRunProfileList_NonEmpty(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	_, _ = globalQ.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name: "p1", IsDefault: 1, WLastCommit: 0.5, InactiveDaysThreshold: 90, InactiveScoreThreshold: 0.65,
	})

	cmd := &cobra.Command{}
	if err := runProfileList(cmd, nil); err != nil {
		t.Fatalf("runProfileList: %v", err)
	}
}

func TestRunProfileCreate_WithDefault(t *testing.T) {
	setupTestDB(t)

	profileDefault = true
	t.Cleanup(func() { profileDefault = false })

	cmd := &cobra.Command{}
	if err := runProfileCreate(cmd, []string{"default-profile"}); err != nil {
		t.Fatalf("runProfileCreate: %v", err)
	}

	p, err := globalQ.GetDefaultProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "default-profile" {
		t.Errorf("default: want default-profile got %q", p.Name)
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

func TestRunProfileSetDefault_NotFound(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runProfileSetDefault(cmd, []string{"no-such-profile"}); err == nil {
		t.Error("expected error for non-existent profile")
	}
}

func TestRunProfileEdit_FromFlags(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	profileDesc = "initial"
	cmd := &cobra.Command{}
	if err := runProfileCreate(cmd, []string{"edit-me"}); err != nil {
		t.Fatal(err)
	}

	profileDesc = "updated"
	profileWCommit = 0.9
	t.Cleanup(func() {
		profileDesc = ""
		profileWCommit = -1
	})

	if err := runProfileEdit(cmd, []string{"edit-me"}); err != nil {
		t.Fatalf("runProfileEdit: %v", err)
	}

	p, _ := globalQ.GetProfileByName(ctx, "edit-me")
	if p.Description.String != "updated" {
		t.Errorf("Description: want updated got %q", p.Description.String)
	}
	if p.WLastCommit != 0.9 {
		t.Errorf("WLastCommit: want 0.9 got %f", p.WLastCommit)
	}
	if p.Version != 2 {
		t.Errorf("Version: want 2 got %d", p.Version)
	}

	history, _ := globalQ.ListProfileHistory(ctx, p.ID)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestRunProfileEdit_MaintainExistingDescription(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	profileDesc = "initial"
	if err := runProfileCreate(&cobra.Command{}, []string{"keep-desc"}); err != nil {
		t.Fatal(err)
	}

	profileDesc = ""
	profileWCommit = 0.8
	t.Cleanup(func() {
		profileDesc = ""
		profileWCommit = -1
	})

	if err := runProfileEdit(&cobra.Command{}, []string{"keep-desc"}); err != nil {
		t.Fatal(err)
	}

	p, _ := globalQ.GetProfileByName(ctx, "keep-desc")
	if p.Description.String != "initial" {
		t.Errorf("Description: want initial got %q", p.Description.String)
	}
	if p.WLastCommit != 0.8 {
		t.Errorf("WLastCommit: want 0.8 got %f", p.WLastCommit)
	}
}

func TestRunProfileEdit_NotFound(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runProfileEdit(cmd, []string{"missing"}); err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestRunProfileEdit_EmptyArgs(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	// Non-interactive will fail with empty args
	if err := runProfileEdit(cmd, nil); err == nil {
		t.Error("expected error for empty args (non-interactive)")
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

// --- applyScanOverrides ---

func TestApplyScanOverrides(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64("w-last-commit", -1, "")
	cmd.Flags().Float64("w-last-pr", -1, "")
	cmd.Flags().Float64("w-commit-freq", -1, "")
	cmd.Flags().Float64("w-branch-staleness", -1, "")
	cmd.Flags().Float64("w-no-releases", -1, "")
	cmd.Flags().Int("threshold", -1, "")
	cmd.Flags().Float64("score-min", -1, "")

	_ = cmd.Flags().Set("w-last-commit", "0.99")
	_ = cmd.Flags().Set("threshold", "123")
	
	scanWCommit = 0.99
	scanThreshold = 123
	t.Cleanup(func() {
		scanWCommit = -1
		scanThreshold = -1
	})

	p := scoring.ScoringProfile{WLastCommit: 0.1, InactiveDaysThreshold: 30}
	changed := applyScanOverrides(cmd, &p)

	if !changed {
		t.Error("expected changed=true")
	}
	if p.WLastCommit != 0.99 {
		t.Errorf("WLastCommit: %f", p.WLastCommit)
	}
	if p.InactiveDaysThreshold != 123 {
		t.Errorf("threshold: %d", p.InactiveDaysThreshold)
	}
}

// --- runOrgAdd ---

func TestRunOrgAdd_MissingArgs(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runOrgAdd(cmd, nil); err == nil {
		t.Error("expected error for missing slug (non-interactive)")
	}
}

func TestRunOrgAdd_MissingPatEnv(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	if err := runOrgAdd(cmd, []string{"myorg"}); err == nil {
		t.Error("expected error for missing pat-env")
	}
}

func TestRunOrgAdd_PatNotSet(t *testing.T) {
	setupTestDB(t)
	cmd := &cobra.Command{}
	orgAddPatEnv = "NONEXISTENT_PAT"
	t.Cleanup(func() { orgAddPatEnv = "" })
	if err := runOrgAdd(cmd, []string{"myorg"}); err == nil {
		t.Error("expected error when PAT env var is not set")
	}
}

func TestRunOrgAdd_UnknownProvider(t *testing.T) {
	setupTestDB(t)
	t.Setenv("TEST_PAT", "token")
	cmd := &cobra.Command{}
	orgAddPatEnv = "TEST_PAT"
	orgAddProvider = "unknown"
	t.Cleanup(func() { orgAddPatEnv = ""; orgAddProvider = "github" })
	if err := runOrgAdd(cmd, []string{"myorg"}); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestResolveOrgsToScan_InteractiveNoOrgs(t *testing.T) {
	setupTestDB(t)
	old := isInteractive
	isInteractive = func() bool { return true }
	t.Cleanup(func() { isInteractive = old })

	// DB is empty, should return error
	_, err := resolveOrgsToScan(context.Background(), &cobra.Command{})
	if err == nil {
		t.Error("expected error when no orgs are registered in interactive mode")
	}
}

func TestRunProfileSetDefault_InteractiveError(t *testing.T) {
	setupTestDB(t)
	old := isInteractive
	isInteractive = func() bool { return true }
	t.Cleanup(func() { isInteractive = old })

	// This will call ListProfiles, then try huh.Run() and fail
	err := runProfileSetDefault(&cobra.Command{}, nil)
	if err == nil {
		t.Error("expected error from huh in non-TTY")
	}
}

func TestRunOrgAdd_InteractiveError(t *testing.T) {
	setupTestDB(t)
	old := isInteractive
	isInteractive = func() bool { return true }
	t.Cleanup(func() { isInteractive = old })

	err := runOrgAdd(&cobra.Command{}, nil)
	if err == nil {
		t.Error("expected error from huh in non-TTY")
	}
}

func TestRunScan_InteractiveProfile(t *testing.T) {
	setupTestDB(t)
	old := isInteractive
	isInteractive = func() bool { return true }
	t.Cleanup(func() { isInteractive = old })

	scanProfile = ""
	// This will list profiles, then try huh.Run() and fail
	err := runScan(&cobra.Command{}, nil)
	if err == nil {
		t.Error("expected error from huh in non-TTY")
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

type mockProvider struct {
	projects []providers.Project
	repos    map[string][]providers.RepoData
}

func (m *mockProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	return m.projects, nil
}
func (m *mockProvider) FetchRepos(org providers.Organization, proj providers.Project) ([]providers.RepoData, error) {
	return m.repos[proj.Name], nil
}

func TestRunScan_Basic(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// 1. Setup org
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name: "scan-org", Slug: "scan-org", Provider: "github",
		AccountType: "org", BaseUrl: "https://api.github.com", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Setup mock provider
	now := time.Now()
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "proj1"}},
		repos: map[string][]providers.RepoData{
			"proj1": {
				{Name: "repo1", RemoteURL: "url1", LastCommitAt: &now, CommitCount90d: 5},
			},
		},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	// 3. Setup flags
	scanOrgs = []string{"scan-org"}
	scanRefresh = true
	t.Cleanup(func() { scanOrgs = nil; scanRefresh = false })

	cmd := &cobra.Command{}
	if err := runScan(cmd, nil); err != nil {
		t.Fatalf("runScan: %v", err)
	}

	// Verify scan run recorded
	runs, _ := globalQ.ListScanRuns(ctx)
	if len(runs) != 1 {
		t.Errorf("expected 1 scan run, got %d", len(runs))
	}
	if runs[0].OrgID.Int64 != org.ID {
		t.Errorf("OrgID: want %d got %d", org.ID, runs[0].OrgID.Int64)
	}
}

func TestRunScan_NoOrgs(t *testing.T) {
	setupTestDB(t)
	scanOrgs = nil
	scanAllOrgs = false
	cmd := &cobra.Command{}
	if err := runScan(cmd, nil); err == nil {
		t.Error("expected error for no orgs")
	}
}

func TestRunScan_OutputJSON(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "json-org", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanOrgs = []string{"json-org"}
	outputFmt = "json"
	tmpFile := filepath.Join(t.TempDir(), "out.json")
	scanOutFile = tmpFile
	t.Cleanup(func() { scanOrgs = nil; outputFmt = "table"; scanOutFile = "" })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmpFile); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestRunScan_OutputCSV(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "csv-org", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanOrgs = []string{"csv-org"}
	outputFmt = "csv"
	tmpFile := filepath.Join(t.TempDir(), "out.csv")
	scanOutFile = tmpFile
	t.Cleanup(func() { scanOrgs = nil; outputFmt = "table"; scanOutFile = "" })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmpFile); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestRunScan_InitialFetch(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "init-org", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	
	// No repos in DB initially for this org
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "proj"}},
		repos: map[string][]providers.RepoData{
			"proj": {{Name: "repo1"}},
		},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanOrgs = []string{"init-org"}
	t.Cleanup(func() { scanOrgs = nil })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	
	repos, _ := globalQ.ListRepositoriesByOrg(ctx, "init-org")
	if len(repos) != 1 {
		t.Errorf("expected 1 repo after initial fetch, got %d", len(repos))
	}
}

func TestRunScan_UnknownProfile(t *testing.T) {
	setupTestDB(t)
	scanProfile = "missing-profile"
	t.Cleanup(func() { scanProfile = "" })
	cmd := &cobra.Command{}
	if err := runScan(cmd, nil); err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestRunScan_RefreshCached(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "cached-org", Provider: "github", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
	proj, _ := globalQ.UpsertProject(ctx, dbgen.UpsertProjectParams{OrgID: org.ID, Name: "p1"})
	
	// Create repo that is NOT stale
	_, _ = globalQ.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{
		ProjectID: proj.ID, Name: "r1", RemoteUrl: "url1",
	})

	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanOrgs = []string{"cached-org"}
	scanRefresh = false // Should use cache
	t.Cleanup(func() { scanOrgs = nil; scanRefresh = false })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunScan_AllOrgs(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "org-a", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "org-b", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p"}},
		repos:    map[string][]providers.RepoData{"p": {{Name: "r"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanAllOrgs = true
	t.Cleanup(func() { scanAllOrgs = false })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOrgsToScan_Empty(t *testing.T) {
	setupTestDB(t)
	scanOrgs = nil
	scanAllOrgs = false
	// Non-interactive
	orgs, err := resolveOrgsToScan(context.Background(), &cobra.Command{})
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 0 {
		t.Errorf("expected 0 orgs, got %d", len(orgs))
	}
}

func TestRunScan_ProfileError(t *testing.T) {
	setupTestDB(t)
	// No profiles in DB initially (except default might be created by Open)
	// But GetProfileByName with non-existent name should fail
	scanProfile = "non-existent"
	t.Cleanup(func() { scanProfile = "" })
	if err := runScan(&cobra.Command{}, nil); err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestRunScan_WeightWarning(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	if _, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "warn-org", Provider: "github", PatEnv: "PAT",
	}); err != nil {
		t.Fatal(err)
	}
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	scanOrgs = []string{"warn-org"}
	// Weights sum to 2.0
	scanWCommit = 1.0
	scanWPR = 1.0
	t.Cleanup(func() { scanOrgs = nil; scanWCommit = -1; scanWPR = -1 })

	if err := runScan(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunOrgAdd_Success(t *testing.T) {
	setupTestDB(t)
	t.Setenv("TEST_PAT", "token")
	
	mockP := &mockProvider{projects: []providers.Project{{Name: "p1"}}}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	orgAddPatEnv = "TEST_PAT"
	orgAddProvider = "github"
	t.Cleanup(func() { orgAddPatEnv = ""; orgAddProvider = "github" })

	if err := runOrgAdd(&cobra.Command{}, []string{"new-org"}); err != nil {
		t.Fatalf("runOrgAdd: %v", err)
	}

	org, err := globalQ.GetOrganizationBySlug(context.Background(), "new-org")
	if err != nil {
		t.Fatal(err)
	}
	if org.Slug != "new-org" {
		t.Errorf("Slug: %s", org.Slug)
	}
}

func TestFetchAndStoreAllRepos(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "fetch-org", Provider: "github", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	if err := fetchAndStoreAllRepos(ctx, org, globalLog); err != nil {
		t.Fatal(err)
	}

	repos, _ := globalQ.ListRepositoriesByOrg(ctx, "fetch-org")
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestRefreshSingleRepo(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Slug: "refresh-org", Provider: "github", PatEnv: "PAT",
	})
	if err != nil {
		t.Fatal(err)
	}
	proj, _ := globalQ.UpsertProject(ctx, dbgen.UpsertProjectParams{OrgID: org.ID, Name: "p1"})
	_, _ = globalQ.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{ProjectID: proj.ID, Name: "r1"})

	mockP := &mockProvider{
		projects: []providers.Project{{Name: "p1"}},
		repos:    map[string][]providers.RepoData{"p1": {{Name: "r1", RemoteURL: "updated-url"}}},
	}
	oldProv := providerFor
	providerFor = func(o dbgen.Organization) (providers.Provider, error) { return mockP, nil }
	t.Cleanup(func() { providerFor = oldProv })

	row := dbgen.ListRepositoriesByOrgRow{
		ProjectID:   proj.ID,
		ProjectName: "p1",
		Name:        "r1",
	}

	if err := refreshSingleRepo(ctx, org, row, globalLog); err != nil {
		t.Fatal(err)
	}

	repos, _ := globalQ.ListRepositoriesByOrg(ctx, "refresh-org")
	if repos[0].RemoteUrl != "updated-url" {
		t.Errorf("RemoteUrl not updated: %s", repos[0].RemoteUrl)
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
