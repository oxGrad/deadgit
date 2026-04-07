package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/output"
	"github.com/oxGrad/deadgit/internal/scanner"
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

// providerFor is a package-level var so tests can swap it out.
var providerFor scanner.ProviderFunc = scanner.DefaultProviderFor

func init() {
	scanCmd.Flags().StringSliceVar(&scanOrgs, "org", nil, "Org slugs to scan (repeatable)")
	scanCmd.Flags().BoolVar(&scanAllOrgs, "all-orgs", false, "Scan all active orgs")
	scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scoring profile (default if omitted)")
	scanCmd.Flags().BoolVar(&scanRefresh, "refresh", false, "Force re-fetch ignoring cache")
	scanCmd.Flags().IntVar(&scanTTL, "ttl", 24, "Cache TTL in hours")
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

	// 1. Interactive profile selection
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
			var chosen string
			_ = huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().Title("Select scoring profile").Options(opts...).Value(&chosen),
			)).Run()
			if chosen != "" {
				scanProfile = chosen
			}
		}
	}

	// 2. Interactive refresh confirm
	if isInteractive() && !scanRefresh {
		var doRefresh bool
		_ = huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Force-refresh data from API? (bypasses cache)").Value(&doRefresh),
		)).Run()
		if doRefresh {
			scanRefresh = true
		}
	}

	// 3. Load scoring profile
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
		fmt.Fprintf(os.Stderr, "Using default profile %q\n", dbProfile.Name)
	}
	log.Info("loaded scoring profile", zap.String("name", dbProfile.Name))

	profile := scanner.DBProfileToScoringProfile(dbProfile)
	hasOverrides := applyScanOverrides(cmd, &profile)

	// 4. Weight sum warning
	wSum := profile.WLastCommit + profile.WLastPR + profile.WCommitFrequency +
		profile.WBranchStaleness + profile.WNoReleases
	if math.Abs(wSum-1.0) > 0.01 {
		fmt.Fprintf(os.Stderr, "warning: weights sum to %.4f (expected ~1.0)\n", wSum)
	}

	// 5. Resolve orgs
	orgsToScan, err := resolveOrgsToScan(ctx, cmd)
	if err != nil {
		return err
	}
	if len(orgsToScan) == 0 {
		return fmt.Errorf("no orgs to scan — use --org <slug>, --all-orgs, or run interactively")
	}

	// 6. Run scan
	cfg := scanner.Config{
		Orgs:    orgsToScan,
		Profile: profile,
		Workers: scanWorkers,
		TTL:     scanTTL,
		Refresh: scanRefresh,
	}

	var progressFn scanner.ProgressFunc
	if isInteractive() {
		progressFn = printProgress
	}

	rows, inactiveCount, scanErr := scanner.Run(ctx, globalQ, cfg, providerFor, progressFn)
	if isInteractive() {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if scanErr != nil && scanErr != context.Canceled {
		return scanErr
	}

	log.Info("scoring complete",
		zap.Int("repos", len(rows)),
		zap.Int("inactive", inactiveCount),
	)

	orgSlugs := make([]string, len(orgsToScan))
	for i, o := range orgsToScan {
		orgSlugs[i] = o.Slug
	}

	// 7. Record scan run
	for _, org := range orgsToScan {
		recordScanRun(ctx, org.ID, dbProfile.ID, profile, len(rows), inactiveCount)
	}

	// 8. Render
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
			DurationSec:    time.Since(start).Seconds(),
		})
	}
	return nil
}

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
