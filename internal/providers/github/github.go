package github

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/oxGrad/deadgit/internal/providers"
)

type ghProvider struct {
	client      *client
	accountType string
}

// New creates a GitHub provider. accountType is "org" or "personal".
func New(baseURL, pat, accountType string) providers.Provider {
	return &ghProvider{client: newClient(baseURL, pat), accountType: accountType}
}

type ghRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	CloneURL      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
	Archived      bool      `json:"archived"`
	Disabled      bool      `json:"disabled"`
	PushedAt      time.Time `json:"pushed_at"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type ghBranch struct{ Name string `json:"name"` }
type ghPR struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProjects returns a single stub project — GitHub has no project layer.
func (p *ghProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	return []providers.Project{{Name: org.Slug, ExternalID: ""}}, nil
}

func (p *ghProvider) FetchRepos(org providers.Organization, project providers.Project) ([]providers.RepoData, error) {
	repos, err := p.listRepos(org)
	if err != nil {
		return nil, err
	}
	const maxConcurrency = 5
	result := make([]providers.RepoData, len(repos))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r ghRepo) {
			defer wg.Done()
			defer func() { <-sem }()
			result[i] = p.fetchRepoData(org, r)
		}(i, repo)
	}
	wg.Wait()
	return result, nil
}

func (p *ghProvider) listRepos(org providers.Organization) ([]ghRepo, error) {
	var all []ghRepo
	for page := 1; ; page++ {
		var url string
		if p.accountType == "personal" {
			url = fmt.Sprintf("%s/user/repos?per_page=100&page=%d&type=owner", org.BaseURL, page)
		} else {
			url = fmt.Sprintf("%s/orgs/%s/repos?per_page=100&page=%d&type=all", org.BaseURL, org.Slug, page)
		}
		var repos []ghRepo
		if err := p.client.get(url, &repos); err != nil {
			return nil, fmt.Errorf("list repos page %d: %w", page, err)
		}
		all = append(all, repos...)
		if len(repos) < 100 {
			break
		}
	}
	return all, nil
}

func (p *ghProvider) fetchRepoData(org providers.Organization, repo ghRepo) providers.RepoData {
	var lastCommitAt *time.Time
	if repo.DefaultBranch != "" {
		url := fmt.Sprintf("%s/repos/%s/%s/commits?sha=%s&per_page=1",
			org.BaseURL, org.Slug, repo.Name, repo.DefaultBranch)
		var commits []ghCommit
		if err := p.client.get(url, &commits); err == nil && len(commits) > 0 {
			t := commits[0].Commit.Author.Date
			lastCommitAt = &t
		}
	}

	var branches []ghBranch
	p.client.get(fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", org.BaseURL, org.Slug, repo.Name), &branches) //nolint:errcheck

	var recentCommits []ghCommit
	since := time.Now().AddDate(0, 0, -90).Format(time.RFC3339)
	p.client.get(fmt.Sprintf("%s/repos/%s/%s/commits?since=%s&per_page=100", org.BaseURL, org.Slug, repo.Name, since), &recentCommits) //nolint:errcheck

	var prs []ghPR
	p.client.get(fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", org.BaseURL, org.Slug, repo.Name), &prs) //nolint:errcheck

	var lastPRCreatedAt, lastPushAt *time.Time
	if len(prs) > 0 {
		t := prs[0].CreatedAt
		lastPRCreatedAt = &t
	}
	if !repo.PushedAt.IsZero() {
		t := repo.PushedAt
		lastPushAt = &t
	}

	blob, _ := json.Marshal(map[string]any{"repo": repo, "commits": recentCommits})
	return providers.RepoData{
		Name: repo.Name, RemoteURL: repo.CloneURL,
		ExternalID: fmt.Sprintf("%d", repo.ID), DefaultBranch: repo.DefaultBranch,
		IsArchived: repo.Archived, IsDisabled: repo.Disabled,
		LastCommitAt: lastCommitAt, LastPushAt: lastPushAt, LastPRCreatedAt: lastPRCreatedAt,
		CommitCount90d: len(recentCommits), ActiveBranchCount: len(branches),
		RawAPIBlob: string(blob),
	}
}
