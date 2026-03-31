package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestListPipelineFolder_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scopePath") != "/pipeline" {
			http.NotFound(w, r)
			return
		}
		resp := azuredevops.ItemList{
			Count: 3,
			Value: []azuredevops.Item{
				{Path: "/pipeline", GitObjectType: "tree"},
				{Path: "/pipeline/build.pipeline.yaml", GitObjectType: "blob"},
				{Path: "/pipeline/deploy.pipeline.yml", GitObjectType: "blob"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	items, err := azuredevops.ListPipelineFolder(client, srv.URL+"/", "org", "project", "repo1", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 pipeline files, got %d", len(items))
	}
}

func TestListPipelineFolder_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	items, err := azuredevops.ListPipelineFolder(client, srv.URL+"/", "org", "project", "repo1", "main")
	if err != nil {
		t.Fatalf("expected nil error on 404, got: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on 404, got %d", len(items))
	}
}

func TestGetFileContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "scopePath=%2Fpipeline%2Fbuild.pipeline.yaml") {
			w.Write([]byte("extends:\n  pipeline: shared/base.yml\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	content, err := azuredevops.GetFileContent(client, srv.URL+"/", "org", "project", "repo1", "/pipeline/build.pipeline.yaml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(content), "shared/base.yml") {
		t.Errorf("unexpected content: %s", content)
	}
}
