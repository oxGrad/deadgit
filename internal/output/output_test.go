package output_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oxGrad/deadgit/internal/output"
)

func makeRows() []output.RepoRow {
	return []output.RepoRow{
		{
			OrgSlug: "myorg", Project: "proj1", Repo: "repo-a",
			Score: 0.75, IsInactive: true, Reasons: []string{"stale", "no-pr"}, Cached: false,
		},
		{
			OrgSlug: "myorg", Project: "proj1", Repo: "repo-b",
			Score: 0.20, IsInactive: false, Reasons: nil, Cached: true,
		},
	}
}

// --- WriteCSV ---

func TestWriteCSV_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := output.WriteCSV(path, makeRows(), "default", 1); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestWriteCSV_HeaderAndRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := output.WriteCSV(path, makeRows(), "default", 2); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("csv read: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("expected 3 records (header+2), got %d", len(records))
	}

	header := records[0]
	wantHeader := []string{"org", "project", "repository", "score", "is_inactive", "status", "reasons", "profile", "profile_version"}
	for i, h := range wantHeader {
		if header[i] != h {
			t.Errorf("header[%d]: want %q got %q", i, h, header[i])
		}
	}

	row := records[1]
	if row[0] != "myorg" {
		t.Errorf("org: want myorg got %q", row[0])
	}
	if row[4] != "true" {
		t.Errorf("is_inactive: want true got %q", row[4])
	}
	if row[5] != "INACTIVE" {
		t.Errorf("status: want INACTIVE got %q", row[5])
	}
	if row[6] != "stale|no-pr" {
		t.Errorf("reasons: want stale|no-pr got %q", row[6])
	}
	if row[7] != "default" {
		t.Errorf("profile: want default got %q", row[7])
	}
	if row[8] != "2" {
		t.Errorf("profile_version: want 2 got %q", row[8])
	}

	activeRow := records[2]
	if activeRow[5] != "active" {
		t.Errorf("active row status: want active got %q", activeRow[5])
	}
}

func TestWriteCSV_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := output.WriteCSV(path, nil, "default", 1); err != nil {
		t.Fatalf("WriteCSV empty: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	records, _ := csv.NewReader(f).ReadAll()
	if len(records) != 1 {
		t.Errorf("expected only header row, got %d records", len(records))
	}
}

func TestWriteCSV_BadPath(t *testing.T) {
	if err := output.WriteCSV("/no/such/dir/out.csv", makeRows(), "default", 1); err == nil {
		t.Error("expected error for bad path")
	}
}

// --- WriteJSON ---

func TestWriteJSON_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := output.WriteJSON(path, makeRows(), "default", 1); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestWriteJSON_Envelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := output.WriteJSON(path, makeRows(), "myprofile", 3); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	var report output.JSONReport
	if err := json.NewDecoder(f).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Profile != "myprofile" {
		t.Errorf("profile: want myprofile got %q", report.Profile)
	}
	if report.ProfileVersion != 3 {
		t.Errorf("profile_version: want 3 got %d", report.ProfileVersion)
	}
	if len(report.Repos) != 2 {
		t.Errorf("repos: want 2 got %d", len(report.Repos))
	}
	if report.Repos[0].Repo != "repo-a" {
		t.Errorf("first repo: want repo-a got %q", report.Repos[0].Repo)
	}
}

func TestWriteJSON_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := output.WriteJSON(path, nil, "default", 1); err != nil {
		t.Fatalf("WriteJSON empty: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	var report output.JSONReport
	if err := json.NewDecoder(f).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(report.Repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(report.Repos))
	}
}

func TestWriteJSON_BadPath(t *testing.T) {
	if err := output.WriteJSON("/no/such/dir/out.json", makeRows(), "default", 1); err == nil {
		t.Error("expected error for bad path")
	}
}
