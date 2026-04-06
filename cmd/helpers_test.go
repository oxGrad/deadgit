package cmd

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/scoring"
)

// --- boolToInt64 ---

func TestBoolToInt64(t *testing.T) {
	if got := boolToInt64(true); got != 1 {
		t.Errorf("boolToInt64(true) = %d, want 1", got)
	}
	if got := boolToInt64(false); got != 0 {
		t.Errorf("boolToInt64(false) = %d, want 0", got)
	}
}

// --- baseURLForProvider ---

func TestBaseURLForProvider(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"azure", "https://dev.azure.com"},
		{"AZURE", "https://dev.azure.com"},
		{"github", "https://api.github.com"},
		{"GitHub", "https://api.github.com"},
		{"unknown", "https://api.github.com"},
	}
	for _, c := range cases {
		got := baseURLForProvider(c.provider)
		if got != c.want {
			t.Errorf("baseURLForProvider(%q) = %q, want %q", c.provider, got, c.want)
		}
	}
}

// --- mustHomeDir ---

func TestMustHomeDir_ReturnsNonEmpty(t *testing.T) {
	got := mustHomeDir()
	if got == "" {
		t.Error("mustHomeDir returned empty string")
	}
}

// --- newLogger ---

func TestNewLogger_DefaultIsInfoLevel(t *testing.T) {
	os.Unsetenv("DG_DEBUG") //nolint:errcheck
	log := newLogger()
	if log == nil {
		t.Fatal("newLogger returned nil")
	}
	if log.Core().Enabled(zapcore.DebugLevel) {
		t.Error("expected debug level disabled by default")
	}
	if !log.Core().Enabled(zapcore.InfoLevel) {
		t.Error("expected info level enabled by default")
	}
}

func TestNewLogger_DebugEnabledByEnv(t *testing.T) {
	t.Setenv("DG_DEBUG", "true")
	log := newLogger()
	if !log.Core().Enabled(zapcore.DebugLevel) {
		t.Error("expected debug level enabled when DG_DEBUG=true")
	}
}

func TestNewLogger_IgnoresNonTrueValues(t *testing.T) {
	for _, val := range []string{"1", "yes", "TRUE", "false"} {
		t.Setenv("DG_DEBUG", val)
		log := newLogger()
		if log.Core().Enabled(zapcore.DebugLevel) {
			t.Errorf("DG_DEBUG=%q should not enable debug level", val)
		}
	}
}

// --- dbOrgToProviderOrg ---

func TestDbOrgToProviderOrg(t *testing.T) {
	org := dbgen.Organization{
		ID:          42,
		Slug:        "myorg",
		Name:        "My Org",
		Provider:    "azure",
		AccountType: "org",
		BaseUrl:     "https://dev.azure.com",
		PatEnv:      "AZURE_PAT",
	}
	got := dbOrgToProviderOrg(org)
	if got.ID != 42 {
		t.Errorf("ID: want 42 got %d", got.ID)
	}
	if got.Slug != "myorg" {
		t.Errorf("Slug: want myorg got %q", got.Slug)
	}
	if got.Provider != "azure" {
		t.Errorf("Provider: want azure got %q", got.Provider)
	}
	if got.BaseURL != "https://dev.azure.com" {
		t.Errorf("BaseURL: want https://dev.azure.com got %q", got.BaseURL)
	}
	if got.PatEnv != "AZURE_PAT" {
		t.Errorf("PatEnv: want AZURE_PAT got %q", got.PatEnv)
	}
}

// --- dbProfileToScoringProfile ---

func TestDbProfileToScoringProfile(t *testing.T) {
	p := dbgen.ScoringProfile{
		Name:                   "strict",
		Version:                3,
		WLastCommit:            0.5,
		WLastPr:                0.2,
		WCommitFrequency:       0.15,
		WBranchStaleness:       0.1,
		WNoReleases:            0.05,
		InactiveDaysThreshold:  60,
		InactiveScoreThreshold: 0.7,
	}
	got := dbProfileToScoringProfile(p)
	if got.Name != "strict" {
		t.Errorf("Name: want strict got %q", got.Name)
	}
	if got.Version != 3 {
		t.Errorf("Version: want 3 got %d", got.Version)
	}
	if got.WLastCommit != 0.5 {
		t.Errorf("WLastCommit: want 0.5 got %f", got.WLastCommit)
	}
	if got.InactiveDaysThreshold != 60 {
		t.Errorf("InactiveDaysThreshold: want 60 got %d", got.InactiveDaysThreshold)
	}
	if got.InactiveScoreThreshold != 0.7 {
		t.Errorf("InactiveScoreThreshold: want 0.7 got %f", got.InactiveScoreThreshold)
	}
}

// --- repoRowToMetrics ---

func TestRepoRowToMetrics_WithDates(t *testing.T) {
	now := time.Now()
	row := dbgen.ListRepositoriesByOrgRow{
		LastCommitAt:      sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -10)},
		LastPrCreatedAt:   sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -5)},
		CommitCount90d:    sql.NullInt64{Valid: true, Int64: 20},
		ActiveBranchCount: sql.NullInt64{Valid: true, Int64: 3},
		IsArchived:        0,
		IsDisabled:        1,
	}
	m := repoRowToMetrics(row)
	if m.CommitCount90d != 20 {
		t.Errorf("CommitCount90d: want 20 got %d", m.CommitCount90d)
	}
	if m.ActiveBranchCount != 3 {
		t.Errorf("ActiveBranchCount: want 3 got %d", m.ActiveBranchCount)
	}
	if m.IsArchived {
		t.Error("IsArchived: want false")
	}
	if !m.IsDisabled {
		t.Error("IsDisabled: want true")
	}
	// 10 days ago — DaysSinceLastCommit should be ~10
	if m.DaysSinceLastCommit < 9 || m.DaysSinceLastCommit > 11 {
		t.Errorf("DaysSinceLastCommit: want ~10, got %.1f", m.DaysSinceLastCommit)
	}
}

func TestRepoRowToMetrics_NullDates(t *testing.T) {
	row := dbgen.ListRepositoriesByOrgRow{
		LastCommitAt:    sql.NullTime{Valid: false},
		LastPrCreatedAt: sql.NullTime{Valid: false},
	}
	m := repoRowToMetrics(row)
	if m.DaysSinceLastCommit != 9999 {
		t.Errorf("DaysSinceLastCommit: want 9999 got %.1f", m.DaysSinceLastCommit)
	}
	if m.DaysSinceLastPR != 9999 {
		t.Errorf("DaysSinceLastPR: want 9999 got %.1f", m.DaysSinceLastPR)
	}
}

// --- applyScanOverrides ---

func newScanCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Float64Var(&scanWCommit, "w-last-commit", -1, "")
	cmd.Flags().Float64Var(&scanWPR, "w-last-pr", -1, "")
	cmd.Flags().Float64Var(&scanWFreq, "w-commit-freq", -1, "")
	cmd.Flags().Float64Var(&scanWBranch, "w-branch-staleness", -1, "")
	cmd.Flags().Float64Var(&scanWRelease, "w-no-releases", -1, "")
	cmd.Flags().IntVar(&scanThreshold, "threshold", -1, "")
	cmd.Flags().Float64Var(&scanScoreMin, "score-min", -1, "")
	return cmd
}

func TestApplyScanOverrides_NoFlags(t *testing.T) {
	cmd := newScanCmd(t)
	profile := scoring.ScoringProfile{WLastCommit: 0.5}
	changed := applyScanOverrides(cmd, &profile)
	if changed {
		t.Error("expected changed=false when no flags set")
	}
	if profile.WLastCommit != 0.5 {
		t.Errorf("profile should be unchanged, WLastCommit = %f", profile.WLastCommit)
	}
}

func TestApplyScanOverrides_SetWeights(t *testing.T) {
	cmd := newScanCmd(t)
	if err := cmd.Flags().Set("w-last-commit", "0.3"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("threshold", "45"); err != nil {
		t.Fatal(err)
	}
	profile := scoring.ScoringProfile{WLastCommit: 0.5, InactiveDaysThreshold: 90}
	changed := applyScanOverrides(cmd, &profile)
	if !changed {
		t.Error("expected changed=true")
	}
	if profile.WLastCommit != 0.3 {
		t.Errorf("WLastCommit: want 0.3 got %f", profile.WLastCommit)
	}
	if profile.InactiveDaysThreshold != 45 {
		t.Errorf("InactiveDaysThreshold: want 45 got %d", profile.InactiveDaysThreshold)
	}
	// Other fields should be unchanged
	if profile.WLastPR != 0 {
		t.Errorf("WLastPR should be unchanged (0), got %f", profile.WLastPR)
	}
}

func TestApplyScanOverrides_AllFlags(t *testing.T) {
	cmd := newScanCmd(t)
	flags := map[string]string{
		"w-last-commit":      "0.4",
		"w-last-pr":          "0.2",
		"w-commit-freq":      "0.15",
		"w-branch-staleness": "0.1",
		"w-no-releases":      "0.05",
		"threshold":          "30",
		"score-min":          "0.8",
	}
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	profile := scoring.ScoringProfile{}
	changed := applyScanOverrides(cmd, &profile)
	if !changed {
		t.Error("expected changed=true")
	}
	if profile.WLastCommit != 0.4 {
		t.Errorf("WLastCommit: want 0.4 got %f", profile.WLastCommit)
	}
	if profile.InactiveDaysThreshold != 30 {
		t.Errorf("InactiveDaysThreshold: want 30 got %d", profile.InactiveDaysThreshold)
	}
	if profile.InactiveScoreThreshold != 0.8 {
		t.Errorf("InactiveScoreThreshold: want 0.8 got %f", profile.InactiveScoreThreshold)
	}
}

// --- printProgress ---

func TestPrintProgress_DoesNotPanic(t *testing.T) {
	// production always guards total > 0 before calling printProgress
	printProgress(1, 10, "my-repo")
	printProgress(10, 10, "last-repo")
	printProgress(1, 1, "a-very-long-repo-name-that-exceeds-forty-characters-definitely")
}
