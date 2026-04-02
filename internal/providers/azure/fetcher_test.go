package azure_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
)

func TestListProjects_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
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
		json.NewEncoder(w).Encode(map[string]interface{}{"value": []interface{}{}})
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
