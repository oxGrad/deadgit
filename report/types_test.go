package report_test

import (
	"testing"
	"time"

	"github.com/oxGrad/deadgit/report"
)

func TestRepoReportDefaults(t *testing.T) {
	r := report.RepoReport{
		ProjectName:    "myproject",
		RepoName:       "myrepo",
		RepoID:         "abc123",
		DefaultBranch:  "main",
		WebURL:         "https://dev.azure.com/org/myproject/_git/myrepo",
		IsDisabled:     false,
		TotalBranches:  3,
		OpenPRCount:    1,
		ActivityStatus: "ACTIVE",
	}
	if r.ProjectName != "myproject" {
		t.Errorf("expected myproject, got %s", r.ProjectName)
	}
	if r.ActivityStatus != "ACTIVE" {
		t.Errorf("expected ACTIVE, got %s", r.ActivityStatus)
	}
}

func TestCommitInfo(t *testing.T) {
	now := time.Now()
	c := report.CommitInfo{
		CommitID:   "abcdef12",
		Author:     "Alice",
		Email:      "alice@example.com",
		Date:       now,
		Message:    "fix: something",
		BranchName: "main",
	}
	if len(c.CommitID) != 8 {
		t.Errorf("expected 8 char commit ID, got %d chars", len(c.CommitID))
	}
}
