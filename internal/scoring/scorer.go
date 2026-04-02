package scoring

import (
	"fmt"
	"math"
)

// Score computes an inactivity score from raw metrics and a profile.
// Pure function: no DB access, no side effects.
func Score(metrics RepoMetrics, profile ScoringProfile) ScoringResult {
	if metrics.IsArchived || metrics.IsDisabled {
		return ScoringResult{
			TotalScore: 1.0,
			IsInactive: true,
			Breakdown:  ScoreBreakdown{1, 1, 1, 1, 1},
			Reasons:    buildOverrideReasons(metrics),
		}
	}

	threshold := profile.InactiveDaysThreshold
	lastCommitScore := NormalizeLinear(metrics.DaysSinceLastCommit, threshold)
	lastPRScore := NormalizeLinear(metrics.DaysSinceLastPR, threshold)
	commitFreqScore := NormalizeCommitFrequency(metrics.CommitCount90d, threshold)
	branchScore := NormalizeBranchStaleness(metrics.ActiveBranchCount)
	releaseScore := releaseInactivityScore(metrics.HasRecentRelease)

	total := lastCommitScore*profile.WLastCommit +
		lastPRScore*profile.WLastPR +
		commitFreqScore*profile.WCommitFrequency +
		branchScore*profile.WBranchStaleness +
		releaseScore*profile.WNoReleases
	total = math.Round(total*10000) / 10000

	bd := ScoreBreakdown{
		LastCommitScore:      lastCommitScore,
		LastPRScore:          lastPRScore,
		CommitFrequencyScore: commitFreqScore,
		BranchStalenessScore: branchScore,
		ReleaseScore:         releaseScore,
	}
	return ScoringResult{
		TotalScore: total,
		IsInactive: total >= profile.InactiveScoreThreshold,
		Breakdown:  bd,
		Reasons:    buildReasons(metrics, bd),
	}
}

func releaseInactivityScore(hasRecent bool) float64 {
	if hasRecent {
		return 0.0
	}
	return 1.0
}

func buildReasons(metrics RepoMetrics, b ScoreBreakdown) []string {
	var r []string
	if b.LastCommitScore >= 0.8 {
		r = append(r, fmt.Sprintf("No commits in %.0fd", metrics.DaysSinceLastCommit))
	}
	if b.LastPRScore >= 0.8 {
		r = append(r, fmt.Sprintf("No PR activity in %.0fd", metrics.DaysSinceLastPR))
	}
	if b.CommitFrequencyScore >= 0.8 {
		r = append(r, "Commit frequency near zero (90d)")
	}
	if b.BranchStalenessScore >= 0.8 {
		r = append(r, "No active branches")
	}
	if b.ReleaseScore == 1.0 {
		r = append(r, "No recent releases")
	}
	return r
}

func buildOverrideReasons(metrics RepoMetrics) []string {
	var r []string
	if metrics.IsArchived {
		r = append(r, "Repository is archived")
	}
	if metrics.IsDisabled {
		r = append(r, "Repository is disabled")
	}
	return r
}
