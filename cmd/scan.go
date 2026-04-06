package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/oxGrad/deadgit/internal/cache"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/output"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
	"github.com/oxGrad/deadgit/internal/scoring"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan repositories for inactivity",
	RunE:  runScan,
}

var (
	scanOrgs      []string
	scanAllOrgs   bool
	scanProfile   string
	scanRefresh   bool
	scanTTL       int
	scanOutFile   string
	scanWorkers   int
	scanWCommit   float64 = -1
	scanWPR       float64 = -1
	scanWFreq     float64 = -1
	scanWBranch   float64 = -1
	scanWRelease  float64 = -1
	scanThreshold int     = -1
	scanScoreMin  float64 = -1
)

func init() {
	scanCmd.Flags().StringSliceVar(&scanOrgs, "org", nil, "Org slugs to scan (repeatable)")
	scanCmd.Flags().BoolVar(&scanAllOrgs, "all-orgs", false, "Scan all active orgs")
	scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scoring profile (default if omitted)")
	scanCmd.Flags().BoolVar(&scanRefresh, "refresh", false, "Force re-fetch ignoring cache")
	scanCmd.Flags().IntVar(&scanTTL, "ttl", cache.DefaultTTLHours, "Cache TTL in hours")
	scanCmd.Flags().StringVar(&scanOutFile, "outfile", "", "Output file (json/csv modes)")
	scanCmd.Flags().IntVar(&scanWorkers, "workers", 5, "Concurrent workers")
	scanCmd.Flags().Float64Var(&scanWCommit, "w-last-commit", -1, "Override: last commit weight")
	scanCmd.Flags().Float64Var(&scanWPR, "w-last-pr", -1, "Override: last PR weight")
	scanCmd.Flags().Float64Var(&scanWFreq, "w-commit-freq", -1, "Override: commit freq weight")
	scanCmd.Flags().Float64Var(&scanWBranch, "w-branch-staleness", -1, "Override: branch staleness weight")
	scanCmd.Flags().Float64Var(&scanWRelease, "w-no-releases", -1, "Override: no releases weight")
	scanCmd.Flags().IntVar(&scanThreshold, "threshold", -1, "Override: inactive days threshold")
	scanCmd.Flags().Float64Var(&scanScoreMin, "score-min", -1, "Override: inactive score threshold")
}

func runScan(cmd *cobra.Command, args []string) error {
	start := time.Now()
	ctx := context.Background()

	log := globalLog

	// 1. Load scoring profile — interactive selector if not specified
	if isInteractive() && scanProfile == "" {
		profiles, perr := globalQ.ListProfiles(ctx)
		if perr == nil && len(profiles) > 0 {
			opts := make([]huh.Option[string], len(profiles))
			for i, p := range profiles {
				label := fmt.Sprintf("%s v%d", p.Name, p.Version)
				if p.IsDefault == 1 {
					label += " [default]"
				}
				opts[i] = huh.NewOption(label, p.Name)
			}
			var chosenProfile string
			_ = huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().Title("Select scoring profile").Options(opts...).Value(&chosenProfile),
			)).Run()
			if chosenProfile != "" {
				scanProfile = chosenProfile
			}
		}
	}

	// Interactive refresh confirm if not already set
	if isInteractive() && !scanRefresh {
		var doRefresh bool
		_ = huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Force-refresh data from API? (bypasses cache)").
				Value(&doRefresh),
		)).Run()
		if doRefresh {
			scanRefresh = true
		}
	}

	var dbProfile dbgen.ScoringProfile
	var err error
	if scanProfile != "" {
		dbProfile, err = globalQ.GetProfileByName(ctx, scanProfile)
	} else {
		dbProfile, err = globalQ.GetDefaultProfile(ctx)
	}
	if err != nil {
		return fmt.Errorf("load scoring profile: %w", err)
	}
	if !isInteractive() && scanProfile == "" {
		fmt.Fprintf(os.Stderr, "Using default profile %q (use --profile <name> to specify, or run interactively to select)\n", dbProfile.Name)
	}
	log.Info("loaded scoring profile", zap.String("name", dbProfile.Name), zap.Int64("version", dbProfile.Version))

	profile := dbProfileToScoringProfile(dbProfile)

	// 2. Apply inline overrides (in-memory only, never saved)
	hasOverrides := applyScanOverrides(cmd, &profile)

	// 3. Warn if weights don't sum to ~1.0
	wSum := profile.WLastCommit + profile.WLastPR + profile.WCommitFrequency +
		profile.WBranchStaleness + profile.WNoReleases
	if math.Abs(wSum-1.0) > 0.01 {
		fmt.Fprintf(os.Stderr, "warning: weights sum to %.4f (expected ~1.0)\n", wSum)
	}

	// 4. Resolve orgs to scan
	orgsToScan, err := resolveOrgsToScan(ctx, cmd)
	if err != nil {
		return err
	}
	if len(orgsToScan) == 0 {
		return fmt.Errorf("no orgs to scan — use --org <slug>, --all-orgs, or run interactively")
	}

	// 5. For each org, ensure data exists and refresh stale repos
	type repoEntry struct {
		org   dbgen.Organization
		repo  dbgen.ListRepositoriesByOrgRow
		stale bool
	}

	log.Info("scanning orgs", zap.Int("count", len(orgsToScan)))
	var allEntries []repoEntry
	for _, org := range orgsToScan {
		// Initial fetch if org has never been scanned
		repos, err := globalQ.ListRepositoriesByOrg(ctx, org.Slug)
		if err != nil || len(repos) == 0 {
			log.Info("initial fetch", zap.String("org", org.Slug))
			if ferr := fetchAndStoreAllRepos(ctx, org, log); ferr != nil {
				log.Error("initial fetch failed", zap.String("org", org.Slug), zap.Error(ferr))
				continue
			}
			repos, _ = globalQ.ListRepositoriesByOrg(ctx, org.Slug)
		}
		staleCount := 0
		for _, r := range repos {
			stale := scanRefresh || cache.IsStale(r.LastFetched, scanTTL)
			if stale {
				staleCount++
			}
			allEntries = append(allEntries, repoEntry{org: org, repo: r, stale: stale})
		}
		log.Info("repos loaded", zap.String("org", org.Slug), zap.Int("total", len(repos)), zap.Int("stale", staleCount))
	}

	// 6. Refresh stale repos concurrently
	type result struct {
		entry   repoEntry
		fetched bool
	}
	resultCh := make(chan result, len(allEntries)+1)
	jobCh := make(chan repoEntry, len(allEntries)+1)

	staleTotal := 0
	for _, e := range allEntries {
		if e.stale {
			staleTotal++
		}
	}

	var fetchStarted atomic.Int32

	var wg sync.WaitGroup
	workers := scanWorkers
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobCh {
				if entry.stale {
					n := int(fetchStarted.Add(1))
					log.Info("fetching repo", zap.String("repo", entry.repo.Name), zap.String("org", entry.org.Slug), zap.Int("progress", n), zap.Int("total", staleTotal))
					if isInteractive() {
						printProgress(n, staleTotal, entry.repo.Name)
					}
					if rerr := refreshSingleRepo(ctx, entry.org, entry.repo, log); rerr != nil {
						log.Warn("refresh failed", zap.String("repo", entry.repo.Name), zap.Error(rerr))
					}
					resultCh <- result{entry: entry, fetched: true}
				} else {
					log.Debug("repo cached, skipping fetch", zap.String("repo", entry.repo.Name))
					resultCh <- result{entry: entry, fetched: false}
				}
			}
		}()
	}
	for _, e := range allEntries {
		jobCh <- e
	}
	close(jobCh)
	go func() { wg.Wait(); close(resultCh) }()

	// 7. Collect and score
	var rows []output.RepoRow
	cached, fetched := 0, 0
	for res := range resultCh {
		if res.fetched {
			fetched++
		} else {
			cached++
		}
		r := res.entry.repo
		metrics := repoRowToMetrics(r)
		sr := scoring.Score(metrics, profile)
		rows = append(rows, output.RepoRow{
			OrgSlug:    res.entry.org.Slug,
			Project:    r.ProjectName,
			Repo:       r.Name,
			Score:      sr.TotalScore,
			IsInactive: sr.IsInactive,
			Reasons:    sr.Reasons,
			Cached:     !res.fetched,
		})
	}
	if isInteractive() && staleTotal > 0 {
		fmt.Fprint(os.Stderr, "\r\033[K") // clear progress line
	}

	inactiveCount := 0
	for _, r := range rows {
		if r.IsInactive {
			inactiveCount++
		}
	}
	log.Info("scoring complete", zap.Int("repos", len(rows)), zap.Int("inactive", inactiveCount), zap.Int("fetched", fetched), zap.Int("cached", cached))

	orgSlugs := make([]string, len(orgsToScan))
	for i, o := range orgsToScan {
		orgSlugs[i] = o.Slug
	}

	// 8. Record scan run for each org
	for _, org := range orgsToScan {
		recordScanRun(ctx, org.ID, dbProfile.ID, profile, len(rows), inactiveCount)
	}

	// 9. Render
	today := time.Now().Format("2006-01-02")
	switch outputFmt {
	case "json":
		path := scanOutFile
		if path == "" {
			path = fmt.Sprintf("deadgit-report-%s.json", today)
		}
		return output.WriteJSON(path, rows, profile.Name, profile.Version)
	case "csv":
		path := scanOutFile
		if path == "" {
			path = fmt.Sprintf("deadgit-report-%s.csv", today)
		}
		return output.WriteCSV(path, rows, profile.Name, profile.Version)
	default:
		output.PrintTable(os.Stdout, rows, output.TableOptions{
			ProfileName:    profile.Name,
			ProfileVersion: profile.Version,
			OrgSlugs:       orgSlugs,
			HasOverrides:   hasOverrides,
			TotalRepos:     len(rows),
			InactiveCount:  inactiveCount,
			CachedCount:    cached,
			FetchedCount:   fetched,
			DurationSec:    time.Since(start).Seconds(),
		})
	}
	return nil
}

// resolveOrgsToScan returns orgs from --org flags, --all-orgs, or interactive picker.
func resolveOrgsToScan(ctx context.Context, cmd *cobra.Command) ([]dbgen.Organization, error) {
	if scanAllOrgs {
		return globalQ.ListOrganizations(ctx)
	}
	if len(scanOrgs) > 0 {
		var result []dbgen.Organization
		for _, slug := range scanOrgs {
			o, err := globalQ.GetOrganizationBySlug(ctx, slug)
			if err != nil {
				return nil, fmt.Errorf("org %q not found: %w", slug, err)
			}
			result = append(result, o)
		}
		return result, nil
	}
	if isInteractive() {
		all, err := globalQ.ListOrganizations(ctx)
		if err != nil || len(all) == 0 {
			return nil, fmt.Errorf("no orgs registered — run: deadgit org add")
		}
		opts := make([]huh.Option[string], len(all))
		for i, o := range all {
			opts[i] = huh.NewOption(fmt.Sprintf("%s (%s)", o.Slug, o.Provider), o.Slug)
		}
		var selected []string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().Title("Select orgs to scan").Options(opts...).Value(&selected),
		)).Run(); err != nil {
			return nil, err
		}
		scanOrgs = selected
		return resolveOrgsToScan(ctx, cmd)
	}
	return nil, nil
}

var providerFor = func(org dbgen.Organization) (providers.Provider, error) {
	pat := os.Getenv(org.PatEnv)
	if pat == "" {
		return nil, fmt.Errorf("PAT env var %q is not set for org %q", org.PatEnv, org.Slug)
	}
	switch org.Provider {
	case "azure":
		return azure.New(org.BaseUrl, pat), nil
	case "github":
		return github.New(org.BaseUrl, pat, org.AccountType), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", org.Provider)
	}
}

func fetchAndStoreAllRepos(ctx context.Context, org dbgen.Organization, log *zap.Logger) error {
	prov, err := providerFor(org)
	if err != nil {
		return err
	}
	provOrg := dbOrgToProviderOrg(org)
	log.Debug("listing projects", zap.String("org", org.Slug))
	projects, err := prov.ListProjects(provOrg)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	log.Debug("projects found", zap.String("org", org.Slug), zap.Int("count", len(projects)))
	for _, proj := range projects {
		extID := proj.ExternalID
		dbProj, err := globalQ.UpsertProject(ctx, dbgen.UpsertProjectParams{
			OrgID:      org.ID,
			Name:       proj.Name,
			ExternalID: sql.NullString{String: extID, Valid: extID != ""},
		})
		if err != nil {
			log.Warn("upsert project failed", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		log.Debug("fetching repos", zap.String("org", org.Slug), zap.String("project", proj.Name))
		repos, err := prov.FetchRepos(provOrg, proj)
		if err != nil {
			log.Warn("fetch repos failed", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		log.Debug("repos fetched", zap.String("project", proj.Name), zap.Int("count", len(repos)))
		for _, r := range repos {
			log.Debug("upserting repo", zap.String("repo", r.Name))
			upsertRepo(ctx, dbProj.ID, r, log)
		}
	}
	_ = globalQ.UpdateOrganizationLastSynced(ctx, org.ID)
	return nil
}

func refreshSingleRepo(ctx context.Context, org dbgen.Organization, repo dbgen.ListRepositoriesByOrgRow, log *zap.Logger) error {
	prov, err := providerFor(org)
	if err != nil {
		return err
	}
	provOrg := dbOrgToProviderOrg(org)
	proj := providers.Project{Name: repo.ProjectName}
	log.Debug("fetching project repos for refresh", zap.String("project", repo.ProjectName))
	repos, err := prov.FetchRepos(provOrg, proj)
	if err != nil {
		return err
	}
	log.Debug("project repos fetched", zap.String("project", repo.ProjectName), zap.Int("count", len(repos)))
	for _, r := range repos {
		if r.Name == repo.Name {
			log.Debug("found repo, upserting", zap.String("repo", r.Name))
			upsertRepo(ctx, repo.ProjectID, r, log)
			break
		}
	}
	return nil
}

func upsertRepo(ctx context.Context, projectID int64, r providers.RepoData, log *zap.Logger) {
	toNullTime := func(t *time.Time) sql.NullTime {
		if t == nil {
			return sql.NullTime{}
		}
		return sql.NullTime{Valid: true, Time: *t}
	}
	toNullInt64 := func(v int) sql.NullInt64 {
		return sql.NullInt64{Valid: true, Int64: int64(v)}
	}
	_, err := globalQ.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{
		ProjectID:         projectID,
		Name:              r.Name,
		RemoteUrl:         r.RemoteURL,
		ExternalID:        sql.NullString{String: r.ExternalID, Valid: r.ExternalID != ""},
		DefaultBranch:     sql.NullString{String: r.DefaultBranch, Valid: r.DefaultBranch != ""},
		IsArchived:        boolToInt64(r.IsArchived),
		IsDisabled:        boolToInt64(r.IsDisabled),
		LastCommitAt:      toNullTime(r.LastCommitAt),
		LastPushAt:        toNullTime(r.LastPushAt),
		LastPrMergedAt:    toNullTime(r.LastPRMergedAt),
		LastPrCreatedAt:   toNullTime(r.LastPRCreatedAt),
		CommitCount90d:    toNullInt64(r.CommitCount90d),
		ActiveBranchCount: toNullInt64(r.ActiveBranchCount),
		ContributorCount:  toNullInt64(r.ContributorCount),
		RawApiBlob:        sql.NullString{String: r.RawAPIBlob, Valid: r.RawAPIBlob != ""},
	})
	if err != nil {
		log.Warn("upsert repo failed", zap.String("repo", r.Name), zap.Error(err))
	}
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func dbOrgToProviderOrg(o dbgen.Organization) providers.Organization {
	return providers.Organization{
		ID:          o.ID,
		Slug:        o.Slug,
		Name:        o.Name,
		Provider:    o.Provider,
		AccountType: o.AccountType,
		BaseURL:     o.BaseUrl,
		PatEnv:      o.PatEnv,
	}
}

func repoRowToMetrics(r dbgen.ListRepositoriesByOrgRow) scoring.RepoMetrics {
	daysSince := func(t sql.NullTime) float64 {
		if !t.Valid {
			return 9999
		}
		return time.Since(t.Time).Hours() / 24
	}
	commitCount := int64(0)
	if r.CommitCount90d.Valid {
		commitCount = r.CommitCount90d.Int64
	}
	branchCount := int64(0)
	if r.ActiveBranchCount.Valid {
		branchCount = r.ActiveBranchCount.Int64
	}
	return scoring.RepoMetrics{
		DaysSinceLastCommit: daysSince(r.LastCommitAt),
		DaysSinceLastPR:     daysSince(r.LastPrCreatedAt),
		CommitCount90d:      int(commitCount),
		ActiveBranchCount:   int(branchCount),
		HasRecentRelease:    false,
		IsArchived:          r.IsArchived == 1,
		IsDisabled:          r.IsDisabled == 1,
	}
}

func dbProfileToScoringProfile(p dbgen.ScoringProfile) scoring.ScoringProfile {
	return scoring.ScoringProfile{
		Name:                   p.Name,
		Version:                int(p.Version),
		WLastCommit:            p.WLastCommit,
		WLastPR:                p.WLastPr,
		WCommitFrequency:       p.WCommitFrequency,
		WBranchStaleness:       p.WBranchStaleness,
		WNoReleases:            p.WNoReleases,
		InactiveDaysThreshold:  int(p.InactiveDaysThreshold),
		InactiveScoreThreshold: p.InactiveScoreThreshold,
	}
}

func applyScanOverrides(cmd *cobra.Command, p *scoring.ScoringProfile) bool {
	changed := false
	if cmd.Flags().Changed("w-last-commit") {
		p.WLastCommit = scanWCommit
		changed = true
	}
	if cmd.Flags().Changed("w-last-pr") {
		p.WLastPR = scanWPR
		changed = true
	}
	if cmd.Flags().Changed("w-commit-freq") {
		p.WCommitFrequency = scanWFreq
		changed = true
	}
	if cmd.Flags().Changed("w-branch-staleness") {
		p.WBranchStaleness = scanWBranch
		changed = true
	}
	if cmd.Flags().Changed("w-no-releases") {
		p.WNoReleases = scanWRelease
		changed = true
	}
	if cmd.Flags().Changed("threshold") {
		p.InactiveDaysThreshold = scanThreshold
		changed = true
	}
	if cmd.Flags().Changed("score-min") {
		p.InactiveScoreThreshold = scanScoreMin
		changed = true
	}
	return changed
}

func printProgress(done, total int, repoName string) {
	const barWidth = 20
	filled := done * barWidth / total
	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
	if len(repoName) > 40 {
		repoName = repoName[:37] + "..."
	}
	fmt.Fprintf(os.Stderr, "\r  %s  %d/%d  %-40s", bar, done, total, repoName)
}

func recordScanRun(ctx context.Context, orgID int64, profileID int64, profile scoring.ScoringProfile, total, inactive int) {
	snapshot, _ := json.Marshal(profile)
	_ = globalQ.InsertScanRun(ctx, dbgen.InsertScanRunParams{
		OrgID:           sql.NullInt64{Valid: true, Int64: orgID},
		ProfileID:       sql.NullInt64{Valid: true, Int64: profileID},
		ProfileName:     profile.Name,
		ProfileVersion:  int64(profile.Version),
		ProfileSnapshot: string(snapshot),
		TotalRepos:      int64(total),
		InactiveCount:   int64(inactive),
	})
}
