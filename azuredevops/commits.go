package azuredevops

import (
	"fmt"
	"strings"

	"github.com/oxGrad/deadgit/report"
)

// GetLastCommitOnBranch fetches the latest commit on a specific branch.
// Returns nil if no commits are found.
func GetLastCommitOnBranch(client *Client, baseURL, org, project, repoID, branch string) (*report.CommitInfo, error) {
	url := fmt.Sprintf(
		"%s%s/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=1&api-version=7.0",
		baseURL, org, project, repoID, branch,
	)
	var result CommitList
	if err := client.Get(url, &result); err != nil {
		return nil, fmt.Errorf("get commit on %s for repo %s: %w", branch, repoID, err)
	}
	if len(result.Value) == 0 {
		return nil, nil
	}
	return toCommitInfo(result.Value[0], ""), nil
}

// GetLastCommitAnyBranch finds the most recent commit across all given branches.
// Returns nil if no commits are found on any branch.
func GetLastCommitAnyBranch(client *Client, baseURL, org, project, repoID string, branches []Ref) (*report.CommitInfo, error) {
	var latest *report.CommitInfo
	for _, branch := range branches {
		branchName := NormalizeBranch(branch.Name)
		url := fmt.Sprintf(
			"%s%s/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=1&api-version=7.0",
			baseURL, org, project, repoID, branchName,
		)
		var result CommitList
		if err := client.Get(url, &result); err != nil {
			// skip branches that fail — don't abort entire repo
			continue
		}
		if len(result.Value) == 0 {
			continue
		}
		info := toCommitInfo(result.Value[0], branchName)
		if latest == nil || info.Date.After(latest.Date) {
			latest = info
		}
	}
	return latest, nil
}

func toCommitInfo(c Commit, branchName string) *report.CommitInfo {
	commitID := c.CommitID
	if len(commitID) > 8 {
		commitID = commitID[:8]
	}
	message := c.Comment
	if idx := strings.Index(message, "\n"); idx >= 0 {
		message = message[:idx]
	}
	return &report.CommitInfo{
		CommitID:   commitID,
		Author:     c.Author.Name,
		Email:      c.Author.Email,
		Date:       c.Author.Date,
		Message:    message,
		BranchName: branchName,
	}
}
