package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/report"
)

func TestWriteJSON(t *testing.T) {
	reports := []report.RepoReport{
		{
			ProjectName:    "proj",
			RepoName:       "repo",
			ActivityStatus: "ACTIVE",
			LastCommitDefault: &report.CommitInfo{
				CommitID: "abc12345",
				Author:   "Alice",
				Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	outPath := filepath.Join(t.TempDir(), "out.json")
	err := report.WriteJSON(reports, outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("could not read output file: %v", err)
	}

	var parsed []report.RepoReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("expected 1 report, got %d", len(parsed))
	}
	if parsed[0].RepoName != "repo" {
		t.Errorf("unexpected repo name: %s", parsed[0].RepoName)
	}
}
