package azuredevops

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// ListRepositories returns all git repositories in a project.
// The Azure DevOps repositories API returns the full list in a single response;
// $top/$skip are not supported and cause an infinite loop when ignored server-side.
func ListRepositories(client *Client, baseURL, org, project string) ([]Repository, error) {
	url := fmt.Sprintf("%s%s/%s/_apis/git/repositories?api-version=7.0", baseURL, org, project)
	var result RepositoryList
	if err := client.Get(url, &result); err != nil {
		return nil, fmt.Errorf("list repos for %s/%s: %w", org, project, err)
	}

	total := result.Count
	zap.L().Info("fetched all repositories",
		zap.String("project", project),
		zap.Int("total", total),
	)

	for i, repo := range result.Value {
		zap.L().Debug("repository",
			zap.String("project", project),
			zap.Int("index", i+1),
			zap.Int("total", total),
			zap.String("name", repo.Name),
			zap.Bool("disabled", repo.IsDisabled),
		)
	}

	return result.Value, nil
}

// NormalizeBranch strips the "refs/heads/" prefix from a branch ref.
func NormalizeBranch(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}
