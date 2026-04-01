package azuredevops

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// ListRepositories returns all git repositories in a project, handling pagination.
func ListRepositories(client *Client, baseURL, org, project string) ([]Repository, error) {
	var all []Repository
	skip := 0
	page := 1
	const pageSize = 100
	for {
		zap.L().Info("fetching repositories page",
			zap.String("project", project),
			zap.Int("page", page),
			zap.Int("offset", skip),
			zap.Int("collected_so_far", len(all)),
		)
		url := fmt.Sprintf("%s%s/%s/_apis/git/repositories?api-version=7.0&$top=%d&$skip=%d", baseURL, org, project, pageSize, skip)
		var result RepositoryList
		if err := client.Get(url, &result); err != nil {
			return nil, fmt.Errorf("list repos for %s/%s (skip=%d): %w", org, project, skip, err)
		}
		all = append(all, result.Value...)
		zap.L().Info("repositories page done",
			zap.String("project", project),
			zap.Int("page", page),
			zap.Int("page_count", len(result.Value)),
			zap.Int("total_so_far", len(all)),
		)
		if len(result.Value) < pageSize {
			break
		}
		skip += pageSize
		page++
	}
	return all, nil
}

// NormalizeBranch strips the "refs/heads/" prefix from a branch ref.
func NormalizeBranch(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}
