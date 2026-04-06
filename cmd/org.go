package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
)

var isInteractive = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func baseURLForProvider(provider string) string {
	if strings.ToLower(provider) == "azure" {
		return "https://dev.azure.com"
	}
	return "https://api.github.com"
}

var orgCmd = &cobra.Command{Use: "org", Short: "Manage organizations"}

var (
	orgAddName     string
	orgAddProvider string
	orgAddType     string
	orgAddPatEnv   string
)

var orgAddCmd = &cobra.Command{
	Use:   "add [slug]",
	Short: "Register an organization",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOrgAdd,
}

var orgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered organizations",
	RunE:  runOrgList,
}

var orgRemoveCmd = &cobra.Command{
	Use:   "remove <slug>",
	Short: "Deactivate an organization",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgRemove,
}

func init() {
	orgAddCmd.Flags().StringVar(&orgAddName, "name", "", "Display name")
	orgAddCmd.Flags().StringVar(&orgAddProvider, "provider", "github", "Provider: github | azure")
	orgAddCmd.Flags().StringVar(&orgAddType, "type", "org", "Account type: org | personal")
	orgAddCmd.Flags().StringVar(&orgAddPatEnv, "pat-env", "", "Env var name holding the PAT")
	orgCmd.AddCommand(orgAddCmd, orgListCmd, orgRemoveCmd)
}

func runOrgAdd(cmd *cobra.Command, args []string) error {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	if isInteractive() && (slug == "" || orgAddPatEnv == "") {
		if err := orgAddInteractive(&slug); err != nil {
			return err
		}
	}

	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	if orgAddPatEnv == "" {
		return fmt.Errorf("--pat-env is required")
	}
	if orgAddName == "" {
		orgAddName = slug
	}

	pat := os.Getenv(orgAddPatEnv)
	if pat == "" {
		return fmt.Errorf("PAT env var %q is not set", orgAddPatEnv)
	}

	// Validate connectivity before saving to DB
	baseURL := baseURLForProvider(orgAddProvider)
	newOrg := providers.Organization{
		Slug:        slug,
		Name:        orgAddName,
		Provider:    orgAddProvider,
		AccountType: orgAddType,
		BaseURL:     baseURL,
		PatEnv:      orgAddPatEnv,
	}
	var prov providers.Provider
	switch strings.ToLower(orgAddProvider) {
	case "azure":
		prov = azure.New(baseURL, pat)
	case "github":
		prov = github.New(baseURL, pat, orgAddType)
	default:
		return fmt.Errorf("unknown provider %q", orgAddProvider)
	}
	fmt.Printf("Validating connectivity to %s/%s...\n", baseURL, slug)
	if _, err := prov.ListProjects(newOrg); err != nil {
		return fmt.Errorf("connectivity check failed for %q: %w", slug, err)
	}

	ctx := context.Background()
	org, err := globalQ.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
		Name:        orgAddName,
		Slug:        slug,
		Provider:    orgAddProvider,
		AccountType: orgAddType,
		BaseUrl:     baseURL,
		PatEnv:      orgAddPatEnv,
	})
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	fmt.Printf("Organization %q added (id=%d, provider=%s)\n", org.Slug, org.ID, org.Provider)
	return nil
}

func orgAddInteractive(slug *string) error {
	name := orgAddName
	provider := orgAddProvider
	accountType := orgAddType
	patEnv := orgAddPatEnv

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Slug").Value(slug),
		huh.NewInput().Title("Display name (leave blank to use slug)").Value(&name),
		huh.NewSelect[string]().Title("Provider").
			Options(huh.NewOption("GitHub", "github"), huh.NewOption("Azure DevOps", "azure")).
			Value(&provider),
		huh.NewSelect[string]().Title("Account type").
			Options(huh.NewOption("Organization / Team", "org"), huh.NewOption("Personal account", "personal")).
			Value(&accountType),
		huh.NewInput().Title("PAT env var name (e.g. GITHUB_PAT)").Value(&patEnv),
	)).Run(); err != nil {
		return err
	}

	if name != "" {
		orgAddName = name
	}
	orgAddProvider = provider
	orgAddType = accountType
	orgAddPatEnv = patEnv
	return nil
}

func runOrgList(cmd *cobra.Command, args []string) error {
	orgs, err := globalQ.ListAllOrganizations(context.Background())
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		fmt.Println("No organizations registered. Run: deadgit org add")
		return nil
	}
	for _, o := range orgs {
		status := "active"
		if o.IsActive == 0 {
			status = "inactive"
		}
		fmt.Printf("  %-20s  %-8s  %-10s  %-8s  pat-env=%-15s  [%s]\n",
			o.Slug, o.Provider, o.AccountType, status, o.PatEnv, o.BaseUrl)
	}
	return nil
}

func runOrgRemove(cmd *cobra.Command, args []string) error {
	if err := globalQ.DeactivateOrganization(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Printf("Organization %q deactivated.\n", args[0])
	return nil
}
