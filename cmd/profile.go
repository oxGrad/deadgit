package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var profileCmd = &cobra.Command{Use: "profile", Short: "Manage scoring profiles"}

var (
	profileDesc     string
	profileWCommit  float64 = -1
	profileWPR      float64 = -1
	profileWFreq    float64 = -1
	profileWBranch  float64 = -1
	profileWRelease float64 = -1
	profileThresh   int     = -1
	profileScoreMin float64 = -1
	profileDefault  bool
)

var profileCreateCmd = &cobra.Command{Use: "create [name]", Short: "Create a scoring profile", Args: cobra.MaximumNArgs(1), RunE: runProfileCreate}
var profileListCmd = &cobra.Command{Use: "list", Short: "List scoring profiles", RunE: runProfileList}
var profileEditCmd = &cobra.Command{Use: "edit [name]", Short: "Edit a profile (increments version)", Args: cobra.MaximumNArgs(1), RunE: runProfileEdit}
var profileSetDefaultCmd = &cobra.Command{Use: "set-default [name]", Short: "Set default profile", Args: cobra.MaximumNArgs(1), RunE: runProfileSetDefault}

func init() {
	for _, c := range []*cobra.Command{profileCreateCmd, profileEditCmd} {
		c.Flags().StringVar(&profileDesc, "description", "", "Profile description")
		c.Flags().Float64Var(&profileWCommit, "w-last-commit", -1, "Weight: last commit (0–1)")
		c.Flags().Float64Var(&profileWPR, "w-last-pr", -1, "Weight: last PR (0–1)")
		c.Flags().Float64Var(&profileWFreq, "w-commit-freq", -1, "Weight: commit frequency (0–1)")
		c.Flags().Float64Var(&profileWBranch, "w-branch-staleness", -1, "Weight: branch staleness (0–1)")
		c.Flags().Float64Var(&profileWRelease, "w-no-releases", -1, "Weight: no releases (0–1)")
		c.Flags().IntVar(&profileThresh, "threshold", -1, "Inactive days threshold")
		c.Flags().Float64Var(&profileScoreMin, "score-min", -1, "Inactive score threshold (0–1)")
	}
	profileCreateCmd.Flags().BoolVar(&profileDefault, "default", false, "Set as default profile")
	profileCmd.AddCommand(profileCreateCmd, profileListCmd, profileEditCmd, profileSetDefaultCmd)
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	wc, wpr, wf, wb, wr := 0.50, 0.20, 0.20, 0.10, 0.00
	thresh, scoreMin := 90, 0.65

	if isInteractive() && name == "" {
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Profile name").Value(&name),
			huh.NewInput().Title("Description (optional)").Value(&profileDesc),
		)).Run(); err != nil {
			return err
		}
	}
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	applyWeightFlags(&wc, &wpr, &wf, &wb, &wr, &thresh, &scoreMin)

	isDefault := int64(0)
	if profileDefault {
		isDefault = 1
	}

	desc := sql.NullString{}
	if profileDesc != "" {
		desc = sql.NullString{String: profileDesc, Valid: true}
	}

	ctx := context.Background()
	p, err := globalQ.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
		Name:                   name,
		Description:            desc,
		IsDefault:              isDefault,
		WLastCommit:            wc,
		WLastPr:                wpr,
		WCommitFrequency:       wf,
		WBranchStaleness:       wb,
		WNoReleases:            wr,
		InactiveDaysThreshold:  int64(thresh),
		InactiveScoreThreshold: scoreMin,
	})
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	if profileDefault {
		if err := globalQ.SetDefaultProfile(ctx, name); err != nil {
			return fmt.Errorf("set default profile: %w", err)
		}
	}
	fmt.Printf("Profile %q created (v%d)\n", p.Name, p.Version)
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	profiles, err := globalQ.ListProfiles(context.Background())
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("No scoring profiles found.")
		return nil
	}
	tbl := tablewriter.NewTable(os.Stdout)
	tbl.Header([]string{"Name", "Ver", "Commit", "PR", "Freq", "Branch", "Release", "Threshold", "Score Min", "Default"}) //nolint:errcheck
	for _, p := range profiles {
		def := ""
		if p.IsDefault == 1 {
			def = "✓"
		}
		tbl.Append([]string{ //nolint:errcheck
			p.Name,
			strconv.FormatInt(p.Version, 10),
			fmt.Sprintf("%.2f", p.WLastCommit),
			fmt.Sprintf("%.2f", p.WLastPr),
			fmt.Sprintf("%.2f", p.WCommitFrequency),
			fmt.Sprintf("%.2f", p.WBranchStaleness),
			fmt.Sprintf("%.2f", p.WNoReleases),
			fmt.Sprintf("%dd", p.InactiveDaysThreshold),
			fmt.Sprintf("%.2f", p.InactiveScoreThreshold),
			def,
		})
	}
	tbl.Render() //nolint:errcheck
	return nil
}

func runProfileEdit(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	ctx := context.Background()

	if isInteractive() && name == "" {
		profiles, err := globalQ.ListProfiles(ctx)
		if err != nil {
			return err
		}
		opts := make([]huh.Option[string], len(profiles))
		for i, p := range profiles {
			opts[i] = huh.NewOption(fmt.Sprintf("%s v%d", p.Name, p.Version), p.Name)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Select profile to edit").Options(opts...).Value(&name),
		)).Run(); err != nil {
			return err
		}
	}
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	existing, err := globalQ.GetProfileByName(ctx, name)
	if err != nil {
		return fmt.Errorf("profile %q not found: %w", name, err)
	}

	wc := existing.WLastCommit
	wpr := existing.WLastPr
	wf := existing.WCommitFrequency
	wb := existing.WBranchStaleness
	wr := existing.WNoReleases
	thresh := int(existing.InactiveDaysThreshold)
	scoreMin := existing.InactiveScoreThreshold

	applyWeightFlags(&wc, &wpr, &wf, &wb, &wr, &thresh, &scoreMin)

	oldJSON, _ := json.Marshal(existing)

	// Build description: flag value wins; fall back to existing
	desc := sql.NullString{}
	if profileDesc != "" {
		desc = sql.NullString{String: profileDesc, Valid: true}
	} else if existing.Description.Valid {
		desc = existing.Description
	}

	updated, err := globalQ.UpdateProfile(ctx, dbgen.UpdateProfileParams{
		Description:            desc,
		WLastCommit:            wc,
		WLastPr:                wpr,
		WCommitFrequency:       wf,
		WBranchStaleness:       wb,
		WNoReleases:            wr,
		InactiveDaysThreshold:  int64(thresh),
		InactiveScoreThreshold: scoreMin,
		Name:                   name,
	})
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}

	newJSON, _ := json.Marshal(updated)
	_ = globalQ.InsertProfileHistory(ctx, dbgen.InsertProfileHistoryParams{
		ProfileID: updated.ID,
		Version:   updated.Version,
		OldValues: string(oldJSON),
		NewValues: string(newJSON),
		ChangedBy: "cli",
	})

	fmt.Printf("Profile %q updated to v%d\n", updated.Name, updated.Version)
	return nil
}

func runProfileSetDefault(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	ctx := context.Background()

	if isInteractive() && name == "" {
		profiles, err := globalQ.ListProfiles(ctx)
		if err != nil {
			return err
		}
		opts := make([]huh.Option[string], len(profiles))
		for i, p := range profiles {
			opts[i] = huh.NewOption(fmt.Sprintf("%s v%d", p.Name, p.Version), p.Name)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Select default profile").Options(opts...).Value(&name),
		)).Run(); err != nil {
			return err
		}
	}
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	if err := globalQ.SetDefaultProfile(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Default profile set to %q\n", name)
	return nil
}

func applyWeightFlags(wc, wpr, wf, wb, wr *float64, thresh *int, scoreMin *float64) {
	if profileWCommit >= 0 {
		*wc = profileWCommit
	}
	if profileWPR >= 0 {
		*wpr = profileWPR
	}
	if profileWFreq >= 0 {
		*wf = profileWFreq
	}
	if profileWBranch >= 0 {
		*wb = profileWBranch
	}
	if profileWRelease >= 0 {
		*wr = profileWRelease
	}
	if profileThresh >= 0 {
		*thresh = profileThresh
	}
	if profileScoreMin >= 0 {
		*scoreMin = profileScoreMin
	}
}
