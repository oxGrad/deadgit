package cmd

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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
	LogLevel        string
	Org             string
	PAT             string
}

// Run is the CLI entry point.
func Run() {
	cfg := parseConfig()

	log := initLogger(cfg.LogLevel)
	defer log.Sync() //nolint:errcheck
	zap.ReplaceGlobals(log)

	if cfg.Org == "" {
		log.Fatal("AZURE_DEVOPS_ORG environment variable is required")
	}
	if cfg.PAT == "" {
		log.Fatal("AZURE_DEVOPS_PAT environment variable is required")
	}

	log.Info("starting deadgit scan",
		zap.String("org", cfg.Org),
		zap.String("output", cfg.Output),
		zap.Int("inactive_days", cfg.InactiveDays),
		zap.Int("workers", cfg.Workers),
		zap.Bool("include_disabled", cfg.IncludeDisabled),
	)

	start := time.Now()
	client := azuredevops.NewClient(cfg.PAT)

	log.Info("listing projects", zap.String("org", cfg.Org))
	projects, err := azuredevops.ListProjects(client, baseURL, cfg.Org)
	if err != nil {
		log.Fatal("failed to list projects", zap.Error(err))
	}
	log.Info("found projects", zap.Int("count", len(projects)))

	// Filter to specific project if requested
	if cfg.Project != "" {
		filtered := projects[:0]
		for _, p := range projects {
			if p.Name == cfg.Project {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
		log.Info("filtered to single project", zap.String("project", cfg.Project))
	}

	// Collect all repos across projects
	type repoWithProject struct {
		repo    azuredevops.Repository
		project string
	}
	var allRepos []repoWithProject
	for _, proj := range projects {
		log.Info("listing repositories", zap.String("project", proj.Name))
		repos, err := azuredevops.ListRepositories(client, baseURL, cfg.Org, proj.Name)
		if err != nil {
			log.Warn("failed to list repos for project", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		active, disabled := 0, 0
		for _, repo := range repos {
			if repo.IsDisabled {
				disabled++
				if !cfg.IncludeDisabled {
					continue
				}
			} else {
				active++
			}
			allRepos = append(allRepos, repoWithProject{repo: repo, project: proj.Name})
		}
		log.Info("repos collected",
			zap.String("project", proj.Name),
			zap.Int("active", active),
			zap.Int("disabled", disabled),
		)
	}

	total := len(allRepos)
	log.Info("starting concurrent scan",
		zap.Int("total_repos", total),
		zap.Int("workers", cfg.Workers),
	)

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
				repoLog := log.With(
					zap.String("project", j.rwp.project),
					zap.String("repo", j.rwp.repo.Name),
					zap.Int("index", j.index+1),
					zap.Int("total", total),
				)
				repoLog.Info("scanning repo")
				r, err := scanRepo(client, cfg, j.rwp.project, j.rwp.repo, repoLog)
				if err != nil {
					errMu.Lock()
					scanErrors = append(scanErrors, fmt.Errorf("%s/%s: %w", j.rwp.project, j.rwp.repo.Name, err))
					errMu.Unlock()
					repoLog.Error("repo scan failed", zap.Error(err))
					continue
				}
				repoLog.Info("repo scan complete",
					zap.String("status", r.ActivityStatus),
					zap.Int("branches", r.TotalBranches),
					zap.Int("open_prs", r.OpenPRCount),
					zap.Int("days_since_any_commit", r.DaysSinceAnyCommit),
					zap.Int("pipelines", len(r.Pipelines)),
				)
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
		log.Warn("scan completed with errors", zap.Int("error_count", len(scanErrors)))
		for _, e := range scanErrors {
			log.Error("scan error", zap.Error(e))
		}
	}

	if err := writeOutput(cfg, reports, log); err != nil {
		log.Fatal("failed to write output", zap.Error(err))
	}

	printSummary(reports, projects, start)
}

func initLogger(level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
	cfg.OutputPaths = []string{"stderr"}
	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return log
}

func parseConfig() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Output, "output", "table", "Output format: table, json, csv")
	flag.StringVar(&cfg.Project, "project", "", "Scan only a specific project (optional)")
	flag.IntVar(&cfg.InactiveDays, "inactive-days", 90, "Days threshold for inactive status")
	flag.IntVar(&cfg.Workers, "workers", 5, "Number of concurrent workers")
	flag.BoolVar(&cfg.IncludeDisabled, "include-disabled", false, "Include disabled repos in output")
	flag.StringVar(&cfg.OutFile, "outfile", "", "Output file path (for json/csv modes)")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level: debug, info, warn, error")
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

func scanRepo(client *azuredevops.Client, cfg Config, projectName string, repo azuredevops.Repository, log *zap.Logger) (report.RepoReport, error) {
	r := report.RepoReport{
		ProjectName:   projectName,
		RepoName:      repo.Name,
		RepoID:        repo.ID,
		DefaultBranch: azuredevops.NormalizeBranch(repo.DefaultBranch),
		WebURL:        repo.WebURL,
		IsDisabled:    repo.IsDisabled,
	}

	if repo.IsDisabled {
		log.Info("repo is disabled, skipping detailed scan")
		r.ActivityStatus = "DISABLED"
		return r, nil
	}

	log.Debug("fetching branches", zap.String("default_branch", r.DefaultBranch))
	branches, err := azuredevops.ListBranches(client, baseURL, cfg.Org, projectName, repo.ID)
	if err != nil {
		log.Warn("failed to list branches", zap.Error(err))
	}
	r.TotalBranches = len(branches)
	log.Debug("branches fetched", zap.Int("count", r.TotalBranches))

	log.Debug("fetching open PRs")
	prCount, err := azuredevops.CountOpenPRs(client, baseURL, cfg.Org, projectName, repo.ID)
	if err != nil {
		log.Warn("failed to count PRs", zap.Error(err))
	}
	r.OpenPRCount = prCount
	log.Debug("open PRs fetched", zap.Int("count", r.OpenPRCount))

	if r.DefaultBranch != "" {
		log.Debug("fetching last commit on default branch", zap.String("branch", r.DefaultBranch))
		commit, err := azuredevops.GetLastCommitOnBranch(client, baseURL, cfg.Org, projectName, repo.ID, r.DefaultBranch)
		if err != nil {
			log.Warn("failed to get default branch commit", zap.Error(err))
		}
		r.LastCommitDefault = commit
		if commit != nil {
			r.DaysSinceDefaultCommit = report.DaysSince(commit.Date)
			log.Debug("default branch commit",
				zap.String("id", commit.CommitID),
				zap.String("author", commit.Author),
				zap.Int("days_ago", r.DaysSinceDefaultCommit),
			)
		}
	}

	if len(branches) > 0 {
		log.Debug("finding most recent commit across all branches", zap.Int("branch_count", len(branches)))
		anyCommit, err := azuredevops.GetLastCommitAnyBranch(client, baseURL, cfg.Org, projectName, repo.ID, branches)
		if err != nil {
			log.Warn("failed to get any branch commit", zap.Error(err))
		}
		r.LastCommitAnyBranch = anyCommit
		if anyCommit != nil {
			r.DaysSinceAnyCommit = report.DaysSince(anyCommit.Date)
			log.Debug("most recent commit across all branches",
				zap.String("branch", anyCommit.BranchName),
				zap.String("id", anyCommit.CommitID),
				zap.Int("days_ago", r.DaysSinceAnyCommit),
			)
		}
	}

	if r.DefaultBranch != "" {
		log.Debug("checking pipeline folder")
		items, err := azuredevops.ListPipelineFolder(client, baseURL, cfg.Org, projectName, repo.ID, r.DefaultBranch)
		if err != nil {
			log.Warn("failed to list pipeline folder", zap.Error(err))
		}
		if len(items) > 0 {
			log.Debug("pipeline files found", zap.Int("count", len(items)))
		}
		for _, item := range items {
			log.Debug("fetching pipeline file", zap.String("path", item.Path))
			content, err := azuredevops.GetFileContent(client, baseURL, cfg.Org, projectName, repo.ID, item.Path, r.DefaultBranch)
			if err != nil {
				log.Warn("failed to fetch pipeline file", zap.String("path", item.Path), zap.Error(err))
				continue
			}
			fileName := pipeline.ExtractFileName(item.Path)
			info, _ := pipeline.ParsePipelineFile(fileName, content)
			log.Debug("parsed pipeline file",
				zap.String("file", fileName),
				zap.String("extends_pipeline", info.ExtendsPipeline),
			)
			r.Pipelines = append(r.Pipelines, info)
		}
	}

	r.ActivityStatus = report.DetermineActivityStatus(&r, cfg.InactiveDays)
	return r, nil
}

func writeOutput(cfg Config, reports []report.RepoReport, log *zap.Logger) error {
	today := time.Now().Format("2006-01-02")
	switch cfg.Output {
	case "json":
		outFile := cfg.OutFile
		if outFile == "" {
			outFile = fmt.Sprintf("azure-repos-report-%s.json", today)
		}
		log.Info("writing JSON report", zap.String("file", outFile), zap.Int("repos", len(reports)))
		return report.WriteJSON(reports, outFile)
	case "csv":
		outFile := cfg.OutFile
		if outFile == "" {
			outFile = fmt.Sprintf("azure-repos-report-%s.csv", today)
		}
		log.Info("writing CSV report", zap.String("file", outFile), zap.Int("repos", len(reports)))
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
