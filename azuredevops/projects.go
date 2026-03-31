package azuredevops

import "fmt"

// ListProjects returns all projects in the organization, handling pagination.
func ListProjects(client *Client, baseURL, org string) ([]Project, error) {
	var all []Project
	skip := 0
	const pageSize = 100
	for {
		url := fmt.Sprintf("%s%s/_apis/projects?api-version=7.0&$top=%d&$skip=%d", baseURL, org, pageSize, skip)
		var result ProjectList
		if err := client.Get(url, &result); err != nil {
			return nil, fmt.Errorf("list projects for org %s (skip=%d): %w", org, skip, err)
		}
		all = append(all, result.Value...)
		if len(result.Value) < pageSize {
			break
		}
		skip += pageSize
	}
	return all, nil
}
