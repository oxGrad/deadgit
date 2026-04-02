package github

import "github.com/oxGrad/deadgit/internal/providers/types"

type ghProvider struct{}

// New creates a GitHub provider stub (implemented in next task).
func New(baseURL, pat, accountType string) types.Provider {
	return &ghProvider{}
}

func (p *ghProvider) ListProjects(org types.Organization) ([]types.Project, error) {
	return nil, nil
}

func (p *ghProvider) FetchRepos(org types.Organization, project types.Project) ([]types.RepoData, error) {
	return nil, nil
}
