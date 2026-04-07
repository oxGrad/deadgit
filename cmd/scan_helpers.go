package cmd

// scan_helpers.go contains helper functions used by cmd/scan.go and its tests.
// These are thin wrappers/adapters kept in the cmd package so existing tests
// can reference them directly.

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/scoring"
)

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

func fetchAndStoreAllRepos(ctx context.Context, org dbgen.Organization, log *zap.Logger) error {
	prov, err := providerFor(org)
	if err != nil {
		return err
	}
	provOrg := dbOrgToProviderOrg(org)
	log.Debug("listing projects", zap.String("org", org.Slug))
	projects, err := prov.ListProjects(provOrg)
	if err != nil {
		return err
	}
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
		repos, err := prov.FetchRepos(provOrg, proj)
		if err != nil {
			log.Warn("fetch repos failed", zap.String("project", proj.Name), zap.Error(err))
			continue
		}
		for _, r := range repos {
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
	repos, err := prov.FetchRepos(provOrg, proj)
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.Name == repo.Name {
			upsertRepo(ctx, repo.ProjectID, r, log)
			break
		}
	}
	return nil
}

