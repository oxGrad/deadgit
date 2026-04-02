package scoring_test

import (
	"testing"

	"github.com/oxGrad/deadgit/internal/scoring"
)

var defaultProfile = scoring.ScoringProfile{
	Name:                   "default",
	Version:                1,
	WLastCommit:            0.40,
	WLastPR:                0.20,
	WCommitFrequency:       0.20,
	WBranchStaleness:       0.10,
	WNoReleases:            0.10,
	InactiveDaysThreshold:  90,
	InactiveScoreThreshold: 0.65,
}

func TestScore_ArchivedAlwaysInactive(t *testing.T) {
	result := scoring.Score(scoring.RepoMetrics{IsArchived: true}, defaultProfile)
	if result.TotalScore != 1.0 {
		t.Errorf("archived: TotalScore = %v, want 1.0", result.TotalScore)
	}
	if !result.IsInactive {
		t.Error("archived: IsInactive should be true")
	}
	if len(result.Reasons) == 0 {
		t.Error("archived: expected at least one reason")
	}
}

func TestScore_DisabledAlwaysInactive(t *testing.T) {
	result := scoring.Score(scoring.RepoMetrics{IsDisabled: true}, defaultProfile)
	if !result.IsInactive {
		t.Error("disabled: IsInactive should be true")
	}
}

func TestScore_ActiveRepo(t *testing.T) {
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 5,
		DaysSinceLastPR:     10,
		CommitCount90d:      30,
		ActiveBranchCount:   3,
		HasRecentRelease:    true,
	}
	result := scoring.Score(metrics, defaultProfile)
	if result.IsInactive {
		t.Errorf("active repo scored inactive: score=%v", result.TotalScore)
	}
}

func TestScore_InactiveRepo(t *testing.T) {
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 300,
		DaysSinceLastPR:     300,
		CommitCount90d:      0,
		ActiveBranchCount:   0,
		HasRecentRelease:    false,
	}
	result := scoring.Score(metrics, defaultProfile)
	if !result.IsInactive {
		t.Errorf("inactive repo not flagged: score=%v", result.TotalScore)
	}
	if len(result.Reasons) == 0 {
		t.Error("inactive repo: expected reasons")
	}
}

func TestScore_MaxScore(t *testing.T) {
	metrics := scoring.RepoMetrics{
		DaysSinceLastCommit: 9999,
		DaysSinceLastPR:     9999,
		CommitCount90d:      0,
		ActiveBranchCount:   0,
		HasRecentRelease:    false,
	}
	result := scoring.Score(metrics, defaultProfile)
	if result.TotalScore != 1.0 {
		t.Errorf("max score = %v, want 1.0", result.TotalScore)
	}
}
