package scanner

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oxGrad/deadgit/internal/cache"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/output"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
	"github.com/oxGrad/deadgit/internal/scoring"
)

// ProviderFunc creates a provider for the given organization.
type ProviderFunc func(org dbgen.Organization) (providers.Provider, error)

// ProgressFunc is called after each repo is fetched.
// done and total refer to the count of stale repos being refreshed.
type ProgressFunc func(done, total int, repoName string)

// Config holds parameters for a scan run.
type Config struct {
	Orgs    []dbgen.Organization
	Profile scoring.ScoringProfile
	Workers int
	TTL     int
	Refresh bool
}

// Run executes a full scan: initial fetch if needed, stale-repo refresh,
// scoring. Returns scored rows and inactive count. If ctx is cancelled,
// returns whatever rows were scored before cancellation with ctx.Err().
func Run(ctx context.Context, q *dbgen.Queries, cfg Config, provFn ProviderFunc, onProgress ProgressFunc) ([]output.RepoRow, int, error) {
	type repoEntry struct {
		org   dbgen.Organization
		repo  dbgen.ListRepositoriesByOrgRow
		stale bool
	}

	var allEntries []repoEntry
	for _, org := range cfg.Orgs {
		repos, err := q.ListRepositoriesByOrg(ctx, org.Slug)
		if err != nil || len(repos) == 0 {
			if ferr := fetchAndStoreAllRepos(ctx, q, org, provFn); ferr != nil {
				continue
			}
			repos, _ = q.ListRepositoriesByOrg(ctx, org.Slug)
		}
		for _, r := range repos {
			stale := cfg.Refresh || cache.IsStale(r.LastFetched, cfg.TTL)
			allEntries = append(allEntries, repoEntry{org: org, repo: r, stale: stale})
		}
	}

	type result struct {
		entry   repoEntry
		fetched bool
	}

	staleTotal := 0
	for _, e := range allEntries {
		if e.stale {
			staleTotal++
		}
	}

	resultCh := make(chan result, len(allEntries)+1)
	jobCh := make(chan repoEntry, len(allEntries)+1)

	var fetchStarted atomic.Int32
	var wg sync.WaitGroup
	workers := max(cfg.Workers, 1)
	for range workers {
		wg.Go(func() {
			for entry := range jobCh {
				select {
				case <-ctx.Done():
					resultCh <- result{entry: entry, fetched: false}
					continue
				default:
				}
				if entry.stale {
					n := int(fetchStarted.Add(1))
					if onProgress != nil {
						onProgress(n, staleTotal, entry.repo.Name)
					}
					// provFn is called per repo refresh; callers should ensure it is cheap (no I/O per call).
					if rerr := refreshSingleRepo(ctx, q, entry.org, entry.repo, provFn); rerr != nil {
						_ = rerr // refresh errors are silently discarded; affected rows retain stale data
					}
					resultCh <- result{entry: entry, fetched: true}
				} else {
					resultCh <- result{entry: entry, fetched: false}
				}
			}
		})
	}
	for _, e := range allEntries {
		jobCh <- e
	}
	close(jobCh)
	go func() { wg.Wait(); close(resultCh) }()

	var rows []output.RepoRow
	for res := range resultCh {
		r := res.entry.repo
		metrics := repoRowToMetrics(r)
		sr := scoring.Score(metrics, cfg.Profile)
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

	inactiveCount := 0
	for _, r := range rows {
		if r.IsInactive {
			inactiveCount++
		}
	}
	return rows, inactiveCount, ctx.Err()
}

// DefaultProviderFor creates the correct provider for an org using PAT from env.
func DefaultProviderFor(org dbgen.Organization) (providers.Provider, error) {
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

// DBOrgToProviderOrg converts a DB organization to a providers.Organization.
func DBOrgToProviderOrg(o dbgen.Organization) providers.Organization {
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

// DBProfileToScoringProfile converts a DB profile to a scoring.ScoringProfile.
func DBProfileToScoringProfile(p dbgen.ScoringProfile) scoring.ScoringProfile {
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

func fetchAndStoreAllRepos(ctx context.Context, q *dbgen.Queries, org dbgen.Organization, provFn ProviderFunc) error {
	prov, err := provFn(org)
	if err != nil {
		return err
	}
	provOrg := DBOrgToProviderOrg(org)
	projects, err := prov.ListProjects(provOrg)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	for _, proj := range projects {
		extID := proj.ExternalID
		dbProj, err := q.UpsertProject(ctx, dbgen.UpsertProjectParams{
			OrgID:      org.ID,
			Name:       proj.Name,
			ExternalID: sql.NullString{String: extID, Valid: extID != ""},
		})
		if err != nil {
			continue
		}
		repos, err := prov.FetchRepos(provOrg, proj)
		if err != nil {
			continue
		}
		for _, r := range repos {
			upsertRepo(ctx, q, dbProj.ID, r)
		}
	}
	_ = q.UpdateOrganizationLastSynced(ctx, org.ID)
	return nil
}

func refreshSingleRepo(ctx context.Context, q *dbgen.Queries, org dbgen.Organization, repo dbgen.ListRepositoriesByOrgRow, provFn ProviderFunc) error {
	// Note: FetchRepos fetches all repos for the project to find this one repo;
	// this is a known limitation inherited from the original implementation.
	prov, err := provFn(org)
	if err != nil {
		return err
	}
	provOrg := DBOrgToProviderOrg(org)
	proj := providers.Project{Name: repo.ProjectName}
	repos, err := prov.FetchRepos(provOrg, proj)
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.Name == repo.Name {
			upsertRepo(ctx, q, repo.ProjectID, r)
			break
		}
	}
	return nil
}

func upsertRepo(ctx context.Context, q *dbgen.Queries, projectID int64, r providers.RepoData) {
	toNullTime := func(t *time.Time) sql.NullTime {
		if t == nil {
			return sql.NullTime{}
		}
		return sql.NullTime{Valid: true, Time: *t}
	}
	toNullInt64 := func(v int) sql.NullInt64 {
		return sql.NullInt64{Valid: true, Int64: int64(v)}
	}
	boolToInt64 := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	_, _ = q.UpsertRepository(ctx, dbgen.UpsertRepositoryParams{
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
