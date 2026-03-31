package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestListProjects_All(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "_apis/projects") {
			http.NotFound(w, r)
			return
		}
		resp := azuredevops.ProjectList{
			Count: 2,
			Value: []azuredevops.Project{
				{ID: "p1", Name: "Alpha", State: "wellFormed"},
				{ID: "p2", Name: "Beta", State: "wellFormed"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	projects, err := azuredevops.ListProjects(client, srv.URL+"/", "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "Alpha" {
		t.Errorf("unexpected first project: %s", projects[0].Name)
	}
}
