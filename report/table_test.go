package report_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/report"
)

func TestPrintTable_Smoke(t *testing.T) {
	reports := []report.RepoReport{
		{
			ProjectName:    "proj",
			RepoName:       "myrepo",
			DefaultBranch:  "main",
			ActivityStatus: "ACTIVE",
			TotalBranches:  2,
			OpenPRCount:    1,
			DaysSinceAnyCommit: 5,
			LastCommitDefault: &report.CommitInfo{
				CommitID: "abc12345",
				Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			},
			LastCommitAnyBranch: &report.CommitInfo{
				Date: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
			},
			Pipelines: []report.PipelineInfo{
				{ExtendsPipeline: "shared/base.yml"},
			},
		},
		{
			ProjectName:    "proj",
			RepoName:       "dormantrepo",
			ActivityStatus: "DORMANT",
		},
	}

	var buf bytes.Buffer
	report.PrintTable(reports, &buf)

	output := buf.String()
	if !strings.Contains(output, "myrepo") {
		t.Errorf("expected myrepo in output, got:\n%s", output)
	}
	if !strings.Contains(output, "DORMANT") {
		t.Errorf("expected DORMANT in output, got:\n%s", output)
	}
}
