package azuredevops

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/oxGrad/deadgit/pipeline"
)

// ListPipelineFolder fetches items under /pipeline at OneLevel depth.
// Returns only blob items matching *.pipeline.yaml or *.pipeline.yml.
// Returns empty slice (no error) if the pipeline folder doesn't exist (404).
func ListPipelineFolder(client *Client, baseURL, org, project, repoID, branch string) ([]Item, error) {
	apiURL := fmt.Sprintf(
		"%s%s/%s/_apis/git/repositories/%s/items?scopePath=/pipeline&recursionLevel=OneLevel&versionDescriptor.version=%s&api-version=7.0",
		baseURL, org, project, repoID, branch,
	)
	var result ItemList
	err := client.Get(apiURL, &result)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, nil
		}
		return nil, fmt.Errorf("list pipeline folder for repo %s: %w", repoID, err)
	}

	var matched []Item
	for _, item := range result.Value {
		if item.GitObjectType == "blob" && pipeline.MatchesPipelineGlob(item.Path) {
			matched = append(matched, item)
		}
	}
	return matched, nil
}

// GetFileContent fetches the raw content of a file at the given path and branch.
func GetFileContent(client *Client, baseURL, org, project, repoID, filePath, branch string) ([]byte, error) {
	encodedPath := url.QueryEscape(filePath)
	apiURL := fmt.Sprintf(
		"%s%s/%s/_apis/git/repositories/%s/items?scopePath=%s&versionDescriptor.version=%s&api-version=7.0",
		baseURL, org, project, repoID, encodedPath, branch,
	)
	content, err := client.GetRaw(apiURL)
	if err != nil {
		return nil, fmt.Errorf("get file %s from repo %s: %w", filePath, repoID, err)
	}
	return content, nil
}
