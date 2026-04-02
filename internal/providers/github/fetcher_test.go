package github_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/github"
)

func TestListProjects_ReturnsStub(t *testing.T) {
	p := github.New("http://unused", "token", "org")
	org := providers.Organization{Slug: "myorg"}
	projects, err := p.ListProjects(org)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "myorg" {
		t.Errorf("expected stub project 'myorg', got %+v", projects)
	}
}

func TestFetchRepos_OrgAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/orgs/myorg/repos" {
			json.NewEncoder(w).Encode([]map[string]any{{
				"id": 1, "name": "my-repo", "clone_url": "https://github.com/myorg/my-repo.git",
				"default_branch": "main", "archived": false, "disabled": false, "pushed_at": "2024-01-01T00:00:00Z",
			}})
		} else {
			json.NewEncoder(w).Encode([]any{})
		}
	}))
	defer srv.Close()

	p := github.New(srv.URL, "token", "org")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL, AccountType: "org"}
	repos, err := p.FetchRepos(org, providers.Project{Name: "myorg"})
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "my-repo" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}

func TestFetchRepos_PersonalAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/user/repos" {
			json.NewEncoder(w).Encode([]map[string]any{{
				"id": 2, "name": "personal-repo", "clone_url": "https://github.com/user/personal-repo.git",
				"default_branch": "main", "archived": false, "disabled": false, "pushed_at": "2024-06-01T00:00:00Z",
			}})
		} else {
			json.NewEncoder(w).Encode([]any{})
		}
	}))
	defer srv.Close()

	p := github.New(srv.URL, "token", "personal")
	org := providers.Organization{Slug: "myuser", BaseURL: srv.URL, AccountType: "personal"}
	repos, err := p.FetchRepos(org, providers.Project{Name: "myuser"})
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "personal-repo" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}
