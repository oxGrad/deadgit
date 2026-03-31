package cmd

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/oxGrad/deadgit/azuredevops"
	"github.com/oxGrad/deadgit/pipeline"
	"github.com/oxGrad/deadgit/report"
)

const baseURL = "https://dev.azure.com/"

// Config holds all runtime configuration parsed from flags + env vars.
type Config struct {
	Output          string
	Project         string
	InactiveDays    int
	Workers         int
	IncludeDisabled bool
	OutFile         string
	Org             string
	PAT             string
}

// Run is the CLI entry point.
func Run() {
	cfg := parseConfig()

	if cfg.Org == "" {
		slog.Error("AZURE_DEVOPS_ORG environment variable is required")
		os.Exit(1)
	}
	if cfg.PAT == "" {
		slog.Error("AZURE_DEVOPS_PAT environment variable is required")
		os.Exit(1)
	}

	start := time.Now()
	client := azuredevops.NewClient(cfg.PAT)

	projects, err := azuredevops.ListProjects(client, baseURL, cfg.Org)
	if err != nil {
		slog.Error("failed to list projects", "error", err)
		os.Exit(1)
	}

	// Filter to specific project if requested
	if cfg.Project != "" {
		filtered := projects[:0]
		for _, p := range projects {
			if p.Name == cfg.Project {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	// Collect all repos across projects
	type repoWithProject struct {
		repo    azuredevops.Repository
		project string
	}
	var allRepos []repoWithProject
	for _, proj := range projects {
		repos, err := azuredevops.ListRepositories(client, baseURL, cfg.Org, proj.Name)
		if err != nil {
			slog.Warn("failed to list repos for project", "project", proj.Name, "error", err)
			continue
		}
		for _, repo := range repos {
			if repo.IsDisabled && !cfg.IncludeDisabled {
				continue
			}
			allRepos = append(allRepos, repoWithProject{repo: repo, project: proj.Name})
		}
	}

	total := len(allRepos)
	fmt.Fprintf(os.Stderr, "Found %d repos across %d projects. Scanning with %d workers...\n",
		total, len(projects), cfg.Workers)

	// Worker pool
	type job struct {
		index int
		rwp   repoWithProject
	}
	jobs := make(chan job, total)
	results := make(chan report.RepoReport, total)
	var errMu sync.Mutex
	var scanErrors []error

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				r, err := scanRepo(client, cfg, j.rwp.project, j.rwp.repo)
				if err != nil {
					errMu.Lock()
					scanErrors = append(scanErrors, fmt.Errorf("%s/%s: %w", j.rwp.project, j.rwp.repo.Name, err))
					errMu.Unlock()
					continue
				}
				fmt.Fprintf(os.Stderr, "Scanning repo %d of %d: %s/%s\n",
					j.index+1, total, j.rwp.project, j.rwp.repo.Name)
				results <- r
			}
		}()
	}

	for i, rwp := range allRepos {
		jobs <- job{index: i, rwp: rwp}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var reports []report.RepoReport
	for r := range results {
		reports = append(reports, r)
	}

	if len(scanErrors) > 0 {
		fmt.Fprintf(os.Stderr, "\n--- Scan Errors (%d) ---\n", len(scanErrors))
		for _, e := range scanErrors {
			slog.Error("scan error", "error", e)
		}
	}

	if err := writeOutput(cfg, reports); err != nil {
		slog.Error("failed to write output", "error", err)
		os.Exit(1)
	}

	printSummary(reports, projects, start)
}

func parseConfig() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Output, "output", "table", "Output format: table, json, csv")
	flag.StringVar(&cfg.Project, "project", "", "Scan only a specific project (optional)")
	flag.IntVar(&cfg.InactiveDays, "inactive-days", 90, "Days threshold for inactive status")
	flag.IntVar(&cfg.Workers, "workers", 5, "Number of concurrent workers")
	flag.BoolVar(&cfg.IncludeDisabled, "include-disabled", false, "Include disabled repos in output")
	flag.StringVar(&cfg.OutFile, "outfile", "", "Output file path (for json/csv modes)")
	flag.Parse()

	cfg.Org = os.Getenv("AZURE_DEVOPS_ORG")
	cfg.PAT = os.Getenv("AZURE_DEVOPS_PAT")

	if v := os.Getenv("INACTIVE_DAYS_THRESHOLD"); v != "" {
		fmt.Sscan(v, &cfg.InactiveDays)
	}
	if v := os.Getenv("WORKER_COUNT"); v != "" {
		fmt.Sscan(v, &cfg.Workers)
	}
	return cfg
}

func scanRepo(client *azuredevops.Client, cfg Config, projectName string, repo azuredevops.Repository) (report.RepoReport, error) {
	r := report.RepoReport{
		ProjectName:   projectName,
		RepoName:      repo.Name,
		RepoID:        repo.ID,
		DefaultBranch: azuredevops.NormalizeBranch(repo.DefaultBranch),
		WebURL:        repo.WebURL,
		IsDisabled:    repo.IsDisabled,
	}

	if repo.IsDisabled {
		r.ActivityStatus = "DISABLED"
		return r, nil
	}

	branches, err := azuredevops.ListBranches(client, baseURL, cfg.Org, projectName, repo.ID)
	if err != nil {
		slog.Warn("failed to list branches", "repo", repo.Name, "error", err)
	}
	r.TotalBranches = len(branches)

	prCount, err := azuredevops.CountOpenPRs(client, baseURL, cfg.Org, projectName, repo.ID)
	if err != nil {
		slog.Warn("failed to count PRs", "repo", repo.Name, "error", err)
	}
	r.OpenPRCount = prCount

	if r.DefaultBranch != "" {
		commit, err := azuredevops.GetLastCommitOnBranch(client, baseURL, cfg.Org, projectName, repo.ID, r.DefaultBranch)
		if err != nil {
			slog.Warn("failed to get default branch commit", "repo", repo.Name, "error", err)
		}
		r.LastCommitDefault = commit
		if commit != nil {
			r.DaysSinceDefaultCommit = report.DaysSince(commit.Date)
		}
	}

	if len(branches) > 0 {
		anyCommit, err := azuredevops.GetLastCommitAnyBranch(client, baseURL, cfg.Org, projectName, repo.ID, branches)
		if err != nil {
			slog.Warn("failed to get any branch commit", "repo", repo.Name, "error", err)
		}
		r.LastCommitAnyBranch = anyCommit
		if anyCommit != nil {
			r.DaysSinceAnyCommit = report.DaysSince(anyCommit.Date)
		}
	}

	if r.DefaultBranch != "" {
		items, err := azuredevops.ListPipelineFolder(client, baseURL, cfg.Org, projectName, repo.ID, r.DefaultBranch)
		if err != nil {
			slog.Warn("failed to list pipeline folder", "repo", repo.Name, "error", err)
		}
		for _, item := range items {
			content, err := azuredevops.GetFileContent(client, baseURL, cfg.Org, projectName, repo.ID, item.Path, r.DefaultBranch)
			if err != nil {
				slog.Warn("failed to fetch pipeline file", "repo", repo.Name, "path", item.Path, "error", err)
				continue
			}
			fileName := pipeline.ExtractFileName(item.Path)
			info, _ := pipeline.ParsePipelineFile(fileName, content)
			r.Pipelines = append(r.Pipelines, info)
		}
	}

	r.ActivityStatus = report.DetermineActivityStatus(&r, cfg.InactiveDays)
	return r, nil
}

func writeOutput(cfg Config, reports []report.RepoReport) error {
	today := time.Now().Format("2006-01-02")
	switch cfg.Output {
	case "json":
		outFile := cfg.OutFile
		if outFile == "" {
			outFile = fmt.Sprintf("azure-repos-report-%s.json", today)
		}
		fmt.Fprintf(os.Stderr, "Writing JSON to %s\n", outFile)
		return report.WriteJSON(reports, outFile)
	case "csv":
		outFile := cfg.OutFile
		if outFile == "" {
			outFile = fmt.Sprintf("azure-repos-report-%s.csv", today)
		}
		fmt.Fprintf(os.Stderr, "Writing CSV to %s\n", outFile)
		return report.WriteCSV(reports, outFile)
	default:
		report.PrintTable(reports, os.Stdout)
		return nil
	}
}

func printSummary(reports []report.RepoReport, projects []azuredevops.Project, start time.Time) {
	counts := map[string]int{}
	for _, r := range reports {
		counts[r.ActivityStatus]++
	}
	fmt.Printf("\n--- Summary ---\n")
	fmt.Printf("Total projects scanned : %d\n", len(projects))
	fmt.Printf("Total repos scanned    : %d\n", len(reports))
	fmt.Printf("  ACTIVE               : %d\n", counts["ACTIVE"])
	fmt.Printf("  INACTIVE             : %d\n", counts["INACTIVE"])
	fmt.Printf("  DORMANT              : %d\n", counts["DORMANT"])
	fmt.Printf("  DISABLED             : %d\n", counts["DISABLED"])
	fmt.Printf("Total scan duration    : %s\n", time.Since(start).Round(time.Millisecond))
}
