package github

import "github.com/oxGrad/deadgit/internal/providers"

type ghProvider struct{}

// New creates a GitHub provider stub (implemented in next task).
func New(baseURL, pat, accountType string) providers.Provider {
	return &ghProvider{}
}

func (p *ghProvider) ListProjects(org providers.Organization) ([]providers.Project, error) {
	return nil, nil
}

func (p *ghProvider) FetchRepos(org providers.Organization, project providers.Project) ([]providers.RepoData, error) {
	return nil, nil
}
