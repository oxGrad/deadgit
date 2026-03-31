package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestListRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "_apis/git/repositories") {
			http.NotFound(w, r)
			return
		}
		resp := azuredevops.RepositoryList{
			Count: 2,
			Value: []azuredevops.Repository{
				{ID: "r1", Name: "Repo1", DefaultBranch: "refs/heads/main", IsDisabled: false},
				{ID: "r2", Name: "Repo2", DefaultBranch: "refs/heads/master", IsDisabled: true},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	repos, err := azuredevops.ListRepositories(client, srv.URL+"/", "myorg", "MyProject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}
	if repos[1].IsDisabled != true {
		t.Error("expected repo2 to be disabled")
	}
}

func TestNormalizeBranch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"refs/heads/main", "main"},
		{"refs/heads/master", "master"},
		{"main", "main"},
		{"", ""},
	}
	for _, tt := range tests {
		got := azuredevops.NormalizeBranch(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeBranch(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
