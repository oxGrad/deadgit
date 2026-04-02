package scoring

// RepoMetrics holds normalized inputs derived from stored raw timestamps.
// Computed at runtime — never stored in DB.
type RepoMetrics struct {
	DaysSinceLastCommit float64
	DaysSinceLastPR     float64
	CommitCount90d      int
	ActiveBranchCount   int
	HasRecentRelease    bool
	IsArchived          bool
	IsDisabled          bool
}

// ScoringProfile is a pure value object derived from the DB row.
type ScoringProfile struct {
	Name                   string
	Version                int
	WLastCommit            float64
	WLastPR                float64
	WCommitFrequency       float64
	WBranchStaleness       float64
	WNoReleases            float64
	InactiveDaysThreshold  int
	InactiveScoreThreshold float64
}

// ScoreBreakdown holds per-component scores for transparency.
type ScoreBreakdown struct {
	LastCommitScore      float64 `json:"last_commit_score"`
	LastPRScore          float64 `json:"last_pr_score"`
	CommitFrequencyScore float64 `json:"commit_frequency_score"`
	BranchStalenessScore float64 `json:"branch_staleness_score"`
	ReleaseScore         float64 `json:"release_score"`
}

// ScoringResult is the final computed output. Never persisted.
type ScoringResult struct {
	TotalScore float64        `json:"total_score"`
	IsInactive bool           `json:"is_inactive"`
	Breakdown  ScoreBreakdown `json:"breakdown"`
	Reasons    []string       `json:"reasons"`
}
