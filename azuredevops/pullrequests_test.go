package azuredevops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxGrad/deadgit/azuredevops"
)

func TestCountOpenPRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("searchCriteria.status") != "active" {
			http.Error(w, "bad request", 400)
			return
		}
		resp := azuredevops.PullRequestList{
			Count: 3,
			Value: []azuredevops.PullRequest{
				{PullRequestID: 1},
				{PullRequestID: 2},
				{PullRequestID: 3},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := azuredevops.NewClient("tok")
	count, err := azuredevops.CountOpenPRs(client, srv.URL+"/", "myorg", "MyProject", "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 open PRs, got %d", count)
	}
}
