package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestGetLastCommitOnBranch_Found(t *testing.T) {
	commitTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := azuredevops.CommitList{
			Count: 1,
			Value: []azuredevops.Commit{
				{
					CommitID: "abcdef1234567890",
					Author: azuredevops.GitPerson{
						Name:  "Alice",
						Email: "alice@example.com",
						Date:  commitTime,
					},
					Comment: "fix: something important\n\nmore details",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	commit, err := azuredevops.GetLastCommitOnBranch(client, srv.URL+"/", "org", "project", "repo1", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit == nil {
		t.Fatal("expected commit, got nil")
	}
	if commit.CommitID != "abcdef12" {
		t.Errorf("expected short ID abcdef12, got %s", commit.CommitID)
	}
	if commit.Author != "Alice" {
		t.Errorf("expected Alice, got %s", commit.Author)
	}
	if commit.Message != "fix: something important" {
		t.Errorf("unexpected message: %s", commit.Message)
	}
	if !commit.Date.Equal(commitTime) {
		t.Errorf("unexpected date: %v", commit.Date)
	}
}

func TestGetLastCommitOnBranch_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(azuredevops.CommitList{Count: 0, Value: []azuredevops.Commit{}})
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	commit, err := azuredevops.GetLastCommitOnBranch(client, srv.URL+"/", "org", "project", "repo1", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit != nil {
		t.Error("expected nil commit for empty list")
	}
}

func TestGetLastCommitAnyBranch(t *testing.T) {
	t1 := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC) // most recent
	t3 := time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var commits []azuredevops.Commit
		switch callCount {
		case 1:
			commits = []azuredevops.Commit{{CommitID: "aaa0000000000000", Author: azuredevops.GitPerson{Name: "A", Email: "a@x.com", Date: t1}, Comment: "msg1"}}
		case 2:
			commits = []azuredevops.Commit{{CommitID: "bbb0000000000000", Author: azuredevops.GitPerson{Name: "B", Email: "b@x.com", Date: t2}, Comment: "msg2"}}
		case 3:
			commits = []azuredevops.Commit{{CommitID: "ccc0000000000000", Author: azuredevops.GitPerson{Name: "C", Email: "c@x.com", Date: t3}, Comment: "msg3"}}
		}
		json.NewEncoder(w).Encode(azuredevops.CommitList{Count: len(commits), Value: commits})
	}))
	defer srv.Close()

	branches := []azuredevops.Ref{
		{Name: "refs/heads/main"},
		{Name: "refs/heads/feature"},
		{Name: "refs/heads/old"},
	}

	client := azuredevops.NewClient("tok")
	commit, err := azuredevops.GetLastCommitAnyBranch(client, srv.URL+"/", "org", "project", "repo1", branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit == nil {
		t.Fatal("expected commit, got nil")
	}
	if !commit.Date.Equal(t2) {
		t.Errorf("expected most recent commit date %v, got %v", t2, commit.Date)
	}
	if commit.BranchName != "feature" {
		t.Errorf("expected branch feature, got %s", commit.BranchName)
	}
}

// Verify NormalizeBranch is available (from repositories.go)
func TestNormalizeBranchUsedInCommits(t *testing.T) {
	got := azuredevops.NormalizeBranch("refs/heads/main")
	if got != "main" {
		t.Errorf("expected main, got %s", got)
	}
}

var _ = strings.Contains // ensure strings is used (suppress unused import)
