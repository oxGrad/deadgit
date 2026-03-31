package report_test

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/report"
)

func TestWriteCSV(t *testing.T) {
	reports := []report.RepoReport{
		{
			ProjectName:    "proj",
			RepoName:       "repo",
			RepoID:         "r1",
			DefaultBranch:  "main",
			WebURL:         "https://example.com",
			IsDisabled:     false,
			TotalBranches:  3,
			OpenPRCount:    1,
			ActivityStatus: "ACTIVE",
			DaysSinceDefaultCommit: 5,
			DaysSinceAnyCommit:     2,
			LastCommitDefault: &report.CommitInfo{
				CommitID: "abc12345",
				Author:   "Alice",
				Email:    "alice@x.com",
				Date:     time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Message:  "fix: something",
			},
			Pipelines: []report.PipelineInfo{
				{FileName: "build.pipeline.yaml", ExtendsPipeline: "shared/base.yml"},
			},
		},
	}

	outPath := filepath.Join(t.TempDir(), "out.csv")
	err := report.WriteCSV(reports, outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("could not open output file: %v", err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(records) < 2 {
		t.Errorf("expected header + 1 data row, got %d rows", len(records))
	}
	if records[0][0] != "PROJECT" {
		t.Errorf("expected PROJECT header, got %s", records[0][0])
	}
}
