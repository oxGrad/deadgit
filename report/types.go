package report

import "time"

type PipelineInfo struct {
	FileName        string
	ExtendsPipeline string
}

type CommitInfo struct {
	CommitID   string
	Author     string
	Email      string
	Date       time.Time
	Message    string
	BranchName string // only for "last commit on any branch"
}

type RepoReport struct {
	ProjectName            string
	RepoName               string
	RepoID                 string
	DefaultBranch          string
	WebURL                 string
	IsDisabled             bool
	TotalBranches          int
	OpenPRCount            int
	Pipelines              []PipelineInfo
	LastCommitDefault      *CommitInfo
	LastCommitAnyBranch    *CommitInfo
	DaysSinceDefaultCommit int
	DaysSinceAnyCommit     int
	ActivityStatus         string // ACTIVE, INACTIVE, DORMANT, DISABLED
}
