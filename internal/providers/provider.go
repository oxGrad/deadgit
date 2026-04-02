package providers

import (
	"fmt"

	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
	"github.com/oxGrad/deadgit/internal/providers/types"
)

// Re-export shared types so callers import only this package.
type Organization = types.Organization
type Project = types.Project
type RepoData = types.RepoData
type Provider = types.Provider

// ProviderFor returns the correct Provider for the given org.
func ProviderFor(org Organization, pat string) (Provider, error) {
	switch org.Provider {
	case "azure":
		return azure.New(org.BaseURL, pat), nil
	case "github":
		return github.New(org.BaseURL, pat, org.AccountType), nil
	default:
		return nil, fmt.Errorf("unknown provider %q for org %q", org.Provider, org.Slug)
	}
}
