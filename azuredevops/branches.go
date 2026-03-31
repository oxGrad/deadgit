package azuredevops

import "fmt"

// ListBranches returns all branch refs for a repository.
func ListBranches(client *Client, baseURL, org, project, repoID string) ([]Ref, error) {
	url := fmt.Sprintf(
		"%s%s/%s/_apis/git/repositories/%s/refs?filter=heads/&api-version=7.0",
		baseURL, org, project, repoID,
	)
	var result RefList
	if err := client.Get(url, &result); err != nil {
		return nil, fmt.Errorf("list branches for repo %s: %w", repoID, err)
	}
	return result.Value, nil
}
