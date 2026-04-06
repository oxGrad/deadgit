package azure_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
)

func TestListProjects_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"value": []map[string]string{{"id": "p1", "name": "Project1"}},
			"count": 1,
		})
	}))
	defer srv.Close()

	p := azure.New(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", Provider: "azure", BaseURL: srv.URL}
	projects, err := p.ListProjects(org)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "Project1" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestFetchRepos_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"value": []interface{}{}})
	}))
	defer srv.Close()

	p := azure.New(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj", ExternalID: "pid"}
	repos, err := p.FetchRepos(org, proj)
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestListProjects_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("$skip") == "0" {
			// Page 1: 100 items
			items := make([]map[string]string, 100)
			for i := 0; i < 100; i++ {
				items[i] = map[string]string{"id": fmt.Sprintf("p%d", i), "name": fmt.Sprintf("P%d", i)}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"value": items,
				"count": 100,
			})
		} else {
			// Page 2: 1 item
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]string{{"id": "p101", "name": "P101"}},
				"count": 1,
			})
		}
	}))
	defer srv.Close()

	p := azure.New(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	projects, err := p.ListProjects(org)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	if len(projects) != 101 {
		t.Errorf("expected 101 projects, got %d", len(projects))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestFetchRepos_Detailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "repositories"):
			if strings.Contains(path, "commits") {
				// Last commit or recent commits
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"commitId": "c123",
							"author":   map[string]string{"date": "2024-01-01T12:00:00Z"},
						},
					},
				})
			} else if strings.Contains(path, "refs") {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"value": []map[string]string{{"name": "refs/heads/main"}},
				})
			} else if strings.Contains(path, "pullrequests") {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"value": []map[string]string{{"creationDate": "2024-01-02T12:00:00Z", "status": "active"}},
				})
			} else {
				// List repositories
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":            "r1",
							"name":          "Repo1",
							"remoteUrl":     "https://dev.azure.com/org/proj/_git/Repo1",
							"defaultBranch": "refs/heads/main",
						},
					},
				})
			}
		}
	}))
	defer srv.Close()

	p := azure.New(srv.URL, "token")
	org := providers.Organization{Slug: "myorg", BaseURL: srv.URL}
	proj := providers.Project{Name: "proj"}
	repos, err := p.FetchRepos(org, proj)
	if err != nil {
		t.Fatalf("FetchRepos: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	r := repos[0]
	if r.Name != "Repo1" {
		t.Errorf("Name: %s", r.Name)
	}
	if r.LastCommitAt == nil || r.LastCommitAt.Format(time.RFC3339) != "2024-01-01T12:00:00Z" {
		t.Errorf("LastCommitAt: %v", r.LastCommitAt)
	}
	if r.CommitCount90d != 1 {
		t.Errorf("CommitCount90d: %d", r.CommitCount90d)
	}
}
