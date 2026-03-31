package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestListBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") != "heads/" {
			http.NotFound(w, r)
			return
		}
		resp := azuredevops.RefList{
			Count: 2,
			Value: []azuredevops.Ref{
				{Name: "refs/heads/main", ObjectID: "abc"},
				{Name: "refs/heads/feature/x", ObjectID: "def"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	branches, err := azuredevops.ListBranches(client, srv.URL+"/", "myorg", "MyProject", "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(branches))
	}
}
