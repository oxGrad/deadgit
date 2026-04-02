package scoring

import "math"

// NormalizeLinear maps days to 0.0–1.0. 0 days = 0.0 (active), >= threshold = 1.0 (inactive).
func NormalizeLinear(days float64, threshold int) float64 {
	if threshold <= 0 {
		return 0
	}
	return math.Min(days/float64(threshold), 1.0)
}

// NormalizeCommitFrequency returns a 0–1 inactivity score. More commits = lower score.
func NormalizeCommitFrequency(commitCount90d int, threshold int) float64 {
	if commitCount90d <= 0 {
		return 1.0
	}
	baseline := float64(threshold) / 30.0 * 4.0 // ~4 commits/month as "active"
	return math.Max(1.0-math.Min(float64(commitCount90d)/baseline, 1.0), 0.0)
}

// NormalizeBranchStaleness returns 0–1. 0 branches = 1.0 (stale), >= 3 branches = 0.0.
func NormalizeBranchStaleness(activeBranches int) float64 {
	if activeBranches <= 0 {
		return 1.0
	}
	return math.Max(1.0-float64(activeBranches)/3.0, 0.0)
}
