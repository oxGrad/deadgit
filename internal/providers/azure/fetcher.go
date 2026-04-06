package azure

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oxGrad/deadgit/internal/providers"
)

const apiVersion = "api-version=7.0"

type azureProvider struct {
	baseURL string
	client  *client
}

// New creates an Azure DevOps provider.
func New(baseURL, pat string) providers.Provider {
	return &azureProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  newClient(pat),
	}
}

// --- Azure API response types ---

type azProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type azProjectList struct {
	Value []azProject `json:"value"`
	Count int         `json:"count"`
}

type azRepo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RemoteURL     string `json:"remoteUrl"`
	DefaultBranch string `json:"defaultBranch"`
	IsDisabled    bool   `json:"isDisabled"`
}

type azRepoList struct {
	Value []azRepo `json:"value"`
}

type azCommit struct {
	CommitID string `json:"commitId"`
	Author   struct {
		Date time.Time `json:"date"`
	} `json:"author"`
}

type azCommitList struct {
	Value []azCommit `json:"value"`
}

type azRef struct {
	Name string `json:"name"`
}

type azRefList struct {
	Value []azRef `json:"value"`
}

type azPRList struct {
	Value []struct {
		CreationDate string `json:"creationDate"`
		Status       string `json:"status"`
	} `json:"value"`
}

// --- Provider implementation ---

func (p *azureProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	var result []providers.Project
	skip, top := 0, 100
	for {
		url := fmt.Sprintf("%s/%s/_apis/projects?$top=%d&$skip=%d&%s",
			p.baseURL, org.Slug, top, skip, apiVersion)
		var list azProjectList
		if err := p.client.get(url, &list); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		for _, proj := range list.Value {
			result = append(result, providers.Project{Name: proj.Name, ExternalID: proj.ID})
		}
		if len(list.Value) < top {
			break
		}
		skip += top
	}
	return result, nil
}

func (p *azureProvider) FetchRepos(org providers.Organization, project providers.Project) ([]providers.RepoData, error) {
	url := fmt.Sprintf("%s/%s/%s/_apis/git/repositories?%s",
		p.baseURL, org.Slug, project.Name, apiVersion)
	var list azRepoList
	if err := p.client.get(url, &list); err != nil {
		return nil, fmt.Errorf("list repos for %s/%s: %w", org.Slug, project.Name, err)
	}

	const maxConcurrency = 5
	result := make([]providers.RepoData, len(list.Value))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i, repo := range list.Value {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r azRepo) {
			defer wg.Done()
			defer func() { <-sem }()
			result[i] = p.fetchRepoData(org, project, r)
		}(i, repo)
	}
	wg.Wait()
	return result, nil
}

func (p *azureProvider) fetchRepoData(org providers.Organization, project providers.Project, repo azRepo) providers.RepoData {
	defaultBranch := normalizeBranch(repo.DefaultBranch)

	// Fetch branches
	branchURL := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?filter=heads/&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, apiVersion)
	var refs azRefList
	p.client.get(branchURL, &refs) // best effort

	// Fetch last commit on default branch
	var lastCommitAt *time.Time
	if defaultBranch != "" {
		commitURL := fmt.Sprintf(
			"%s/%s/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=1&%s",
			p.baseURL, org.Slug, project.Name, repo.ID, defaultBranch, apiVersion)
		var commits azCommitList
		if err := p.client.get(commitURL, &commits); err == nil && len(commits.Value) > 0 {
			t := commits.Value[0].Author.Date
			lastCommitAt = &t
		}
	}

	// Fetch 90-day commit count (capped at 1000)
	since := time.Now().AddDate(0, 0, -90).Format(time.RFC3339)
	countURL := fmt.Sprintf(
		"%s/%s/%s/_apis/git/repositories/%s/commits?searchCriteria.fromDate=%s&searchCriteria.$top=1000&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, since, apiVersion)
	var recent azCommitList
	p.client.get(countURL, &recent) // best effort

	// Fetch most recently created PR (any status) to populate LastPRCreatedAt
	prURL := fmt.Sprintf(
		"%s/%s/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=all&$top=1&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, apiVersion)
	var prs azPRList
	p.client.get(prURL, &prs) // best effort

	var lastPRCreatedAt *time.Time
	if len(prs.Value) > 0 && prs.Value[0].CreationDate != "" {
		if t, err := time.Parse(time.RFC3339, prs.Value[0].CreationDate); err == nil {
			lastPRCreatedAt = &t
		}
	}

	// Fetch most recently merged PR to populate LastPRMergedAt
	completedPRURL := fmt.Sprintf(
		"%s/%s/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=completed&$top=1&%s",
		p.baseURL, org.Slug, project.Name, repo.ID, apiVersion)
	var completedPRs azPRList
	p.client.get(completedPRURL, &completedPRs) // best effort

	var lastPRMergedAt *time.Time
	if len(completedPRs.Value) > 0 && completedPRs.Value[0].CreationDate != "" {
		if t, err := time.Parse(time.RFC3339, completedPRs.Value[0].CreationDate); err == nil {
			lastPRMergedAt = &t
		}
	}

	// Build raw blob
	blob, _ := json.Marshal(map[string]interface{}{
		"repo":    repo,
		"commits": recent,
		"refs":    refs,
	})

	return providers.RepoData{
		Name:              repo.Name,
		RemoteURL:         repo.RemoteURL,
		ExternalID:        repo.ID,
		DefaultBranch:     defaultBranch,
		IsDisabled:        repo.IsDisabled,
		LastCommitAt:      lastCommitAt,
		LastPRCreatedAt:   lastPRCreatedAt,
		LastPRMergedAt:    lastPRMergedAt,
		CommitCount90d:    len(recent.Value),
		ActiveBranchCount: len(refs.Value),
		RawAPIBlob:        string(blob),
	}
}

func normalizeBranch(branch string) string {
	return strings.TrimPrefix(branch, "refs/heads/")
}
