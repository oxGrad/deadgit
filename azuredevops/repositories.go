package azuredevops

import (
	"fmt"
	"strings"
)

// ListRepositories returns all git repositories in a project, handling pagination.
func ListRepositories(client *Client, baseURL, org, project string) ([]Repository, error) {
	var all []Repository
	skip := 0
	const pageSize = 100
	for {
		url := fmt.Sprintf("%s%s/%s/_apis/git/repositories?api-version=7.0&$top=%d&$skip=%d", baseURL, org, project, pageSize, skip)
		var result RepositoryList
		if err := client.Get(url, &result); err != nil {
			return nil, fmt.Errorf("list repos for %s/%s (skip=%d): %w", org, project, skip, err)
		}
		all = append(all, result.Value...)
		if len(result.Value) < pageSize {
			break
		}
		skip += pageSize
	}
	return all, nil
}

// NormalizeBranch strips the "refs/heads/" prefix from a branch ref.
func NormalizeBranch(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}
