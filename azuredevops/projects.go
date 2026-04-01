package azuredevops

import (
	"fmt"

	"go.uber.org/zap"
)

// ListProjects returns all projects in the organization, handling pagination.
func ListProjects(client *Client, baseURL, org string) ([]Project, error) {
	var all []Project
	skip := 0
	page := 1
	const pageSize = 100
	for {
		zap.L().Info("fetching projects page",
			zap.String("org", org),
			zap.Int("page", page),
			zap.Int("offset", skip),
			zap.Int("collected_so_far", len(all)),
		)
		url := fmt.Sprintf("%s%s/_apis/projects?api-version=7.0&$top=%d&$skip=%d", baseURL, org, pageSize, skip)
		var result ProjectList
		if err := client.Get(url, &result); err != nil {
			return nil, fmt.Errorf("list projects for org %s (skip=%d): %w", org, skip, err)
		}
		all = append(all, result.Value...)
		zap.L().Info("projects page done",
			zap.String("org", org),
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
