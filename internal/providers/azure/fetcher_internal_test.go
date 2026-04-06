package azure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/internal/providers"
)

func TestNormalizeBranch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"refs/heads/main", "main"},
		{"refs/heads/feature/foo", "feature/foo"},
		{"main", "main"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeBranch(c.in)
		if got != c.want {
			t.Errorf("normalizeBranch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFetchRepoData_PopulatesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case strings.Contains(r.URL.Path, "/refs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"name": "refs/heads/main"},
					{"name": "refs/heads/dev"},
				},
			})
		case strings.Contains(r.URL.Path, "/commits") && q.Get("searchCriteria.itemVersion.version") != "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"commitId": "abc", "author": map[string]any{"date": "2024-03-01T12:00:00Z"}},
				},
			})
		case strings.Contains(r.URL.Path, "/commits"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{{"commitId": "abc"}, {"commitId": "def"}},
			})
		case strings.Contains(r.URL.Path, "/pullrequests") && q.Get("searchCriteria.status") == "completed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{{"creationDate": "2024-02-20T08:00:00Z", "status": "completed"}},
			})
		case strings.Contains(r.URL.Path, "/pullrequests"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{{"creationDate": "2024-02-25T09:00:00Z", "status": "active"}},
			})
		}
	}))
	defer srv.Close()

	p := &azureProvider{
		baseURL: srv.URL,
		client:  newClient("token"),
	}
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj"}
	repo := azRepo{
		ID:            "repo1",
		Name:          "my-repo",
		RemoteURL:     "https://dev.azure.com/myorg/proj/_git/my-repo",
		DefaultBranch: "refs/heads/main",
	}

	data := p.fetchRepoData(org, proj, repo)

	if data.Name != "my-repo" {
		t.Errorf("Name: want my-repo got %q", data.Name)
	}
	if data.DefaultBranch != "main" {
		t.Errorf("DefaultBranch: want main got %q", data.DefaultBranch)
	}
	if data.ActiveBranchCount != 2 {
		t.Errorf("ActiveBranchCount: want 2 got %d", data.ActiveBranchCount)
	}
	if data.CommitCount90d != 2 {
		t.Errorf("CommitCount90d: want 2 got %d", data.CommitCount90d)
	}
	if data.LastCommitAt == nil {
		t.Error("LastCommitAt: expected non-nil")
	}
	if data.LastPRCreatedAt == nil {
		t.Error("LastPRCreatedAt: expected non-nil")
	}
	if data.LastPRMergedAt == nil {
		t.Error("LastPRMergedAt: expected non-nil")
	}
	if data.RawAPIBlob == "" {
		t.Error("RawAPIBlob: expected non-empty")
	}
}

func TestFetchRepoData_NoDefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{}})
	}))
	defer srv.Close()

	p := &azureProvider{baseURL: srv.URL, client: newClient("token")}
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj"}
	repo := azRepo{ID: "r1", Name: "empty-repo", DefaultBranch: ""}

	data := p.fetchRepoData(org, proj, repo)

	if data.Name != "empty-repo" {
		t.Errorf("Name: want empty-repo got %q", data.Name)
	}
	if data.LastCommitAt != nil {
		t.Error("LastCommitAt: expected nil for repo with no default branch")
	}
	if data.ActiveBranchCount != 0 {
		t.Errorf("ActiveBranchCount: want 0 got %d", data.ActiveBranchCount)
	}
}

func TestFetchRepoData_PRDateParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			// Return a PR with a bad date string — should be gracefully ignored
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{{"creationDate": "not-a-date"}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{}})
		}
	}))
	defer srv.Close()

	p := &azureProvider{baseURL: srv.URL, client: newClient("token")}
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj"}
	repo := azRepo{ID: "r1", Name: "repo", DefaultBranch: ""}

	data := p.fetchRepoData(org, proj, repo)
	// Bad dates should result in nil, not a panic
	if data.LastPRCreatedAt != nil {
		t.Error("expected nil LastPRCreatedAt for bad date")
	}
}
