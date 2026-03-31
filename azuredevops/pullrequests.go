package azuredevops

import "fmt"

// CountOpenPRs returns the number of active pull requests for a repository.
func CountOpenPRs(client *Client, baseURL, org, project, repoID string) (int, error) {
	url := fmt.Sprintf(
		"%s%s/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=active&api-version=7.0",
		baseURL, org, project, repoID,
	)
	var result PullRequestList
	if err := client.Get(url, &result); err != nil {
		return 0, fmt.Errorf("count PRs for repo %s: %w", repoID, err)
	}
	return result.Count, nil
}
