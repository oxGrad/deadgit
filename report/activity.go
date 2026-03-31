package report

import (
	"math"
	"time"
)

// DetermineActivityStatus returns ACTIVE, INACTIVE, DORMANT, or DISABLED.
// A repo is INACTIVE only if ALL of: last commit older than threshold, 0 open PRs, not disabled.
func DetermineActivityStatus(r *RepoReport, inactiveDays int) string {
	if r.IsDisabled {
		return "DISABLED"
	}
	if r.LastCommitAnyBranch == nil {
		return "DORMANT"
	}
	days := DaysSince(r.LastCommitAnyBranch.Date)
	if days <= inactiveDays || r.OpenPRCount > 0 {
		return "ACTIVE"
	}
	return "INACTIVE"
}

// DaysSince returns the number of days between t and now.
// Returns 0 for zero-value time.
func DaysSince(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	return int(math.Round(time.Since(t).Hours() / 24))
}
