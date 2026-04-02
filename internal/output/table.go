package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// RepoRow is one fully-scored repository row ready for display.
type RepoRow struct {
	OrgSlug    string
	Project    string
	Repo       string
	Score      float64
	IsInactive bool
	Reasons    []string
	Cached     bool
}

// TableOptions controls the header line and footer summary.
type TableOptions struct {
	ProfileName    string
	ProfileVersion int
	OrgSlugs       []string
	HasOverrides   bool
	TotalRepos     int
	InactiveCount  int
	CachedCount    int
	FetchedCount   int
	DurationSec    float64
}

// PrintTable renders rows to w as a formatted table.
func PrintTable(w io.Writer, rows []RepoRow, opts TableOptions) {
	profileLabel := fmt.Sprintf("%s v%d", opts.ProfileName, opts.ProfileVersion)
	if opts.HasOverrides {
		profileLabel += " (overrides active)"
	}
	fmt.Fprintf(w, "\nScan Results  •  Profile: %s  •  Orgs: %s\n",
		profileLabel, strings.Join(opts.OrgSlugs, ", "))

	tbl := tablewriter.NewTable(w)
	tbl.Header([]string{"Org", "Project", "Repository", "Score", "Status", "Reasons"})

	inactive := color.New(color.FgYellow).SprintFunc()
	active := color.New(color.FgGreen).SprintFunc()

	for _, r := range rows {
		status := active("✓ active")
		if r.IsInactive {
			status = inactive("⚠ INACTIVE")
		}
		tbl.Append([]string{
			r.OrgSlug,
			r.Project,
			r.Repo,
			fmt.Sprintf("%.4f", r.Score),
			status,
			strings.Join(r.Reasons, "; "),
		})
	}
	tbl.Render()

	fmt.Fprintf(w, "Total: %d repos  •  %d inactive  •  Cached: %d  •  Fetched: %d  •  Duration: %.2fs\n",
		opts.TotalRepos, opts.InactiveCount, opts.CachedCount, opts.FetchedCount, opts.DurationSec)
}
