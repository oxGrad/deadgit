package report

import (
	"fmt"
	"io"
	"strconv"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// PrintTable writes a formatted table of RepoReports to the given writer.
func PrintTable(reports []RepoReport, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.Header(
		"PROJECT",
		"REPO",
		"STATUS",
		"DEFAULT BRANCH",
		"LAST COMMIT (DEFAULT)",
		"LAST COMMIT (ANY)",
		"DAYS INACTIVE",
		"BRANCHES",
		"OPEN PRs",
		"PIPELINE TEMPLATE",
	)

	for _, r := range reports {
		status := colorizeStatus(r.ActivityStatus)

		defaultCommit := "N/A"
		if r.LastCommitDefault != nil {
			defaultCommit = r.LastCommitDefault.Date.Format("2006-01-02 15:04:05")
		}

		anyCommit := "N/A"
		if r.LastCommitAnyBranch != nil {
			anyCommit = r.LastCommitAnyBranch.Date.Format("2006-01-02 15:04:05")
		}

		daysInactive := strconv.Itoa(r.DaysSinceAnyCommit)

		pipelineTemplate := "N/A"
		if len(r.Pipelines) > 0 && r.Pipelines[0].ExtendsPipeline != "" {
			pipelineTemplate = r.Pipelines[0].ExtendsPipeline
		}

		if err := table.Append(
			r.ProjectName,
			r.RepoName,
			status,
			r.DefaultBranch,
			defaultCommit,
			anyCommit,
			daysInactive,
			fmt.Sprintf("%d", r.TotalBranches),
			fmt.Sprintf("%d", r.OpenPRCount),
			pipelineTemplate,
		); err != nil {
			// best-effort: skip row on error
			continue
		}
	}

	if err := table.Render(); err != nil {
		fmt.Fprintf(w, "table render error: %v\n", err)
	}
}

// colorizeStatus returns the status string wrapped in ANSI color codes.
// ACTIVE=Green, INACTIVE=Yellow, DORMANT=Red, DISABLED=HiBlack (Gray)
func colorizeStatus(status string) string {
	// Disable color output to non-terminal writers by using Sprint which respects NO_COLOR
	// but we want colors to appear in the string regardless of terminal detection,
	// so we use color.New(...).SprintFunc() with NoColor disabled temporarily.
	c := color.New()
	switch status {
	case "ACTIVE":
		c = color.New(color.FgGreen, color.Bold)
	case "INACTIVE":
		c = color.New(color.FgYellow, color.Bold)
	case "DORMANT":
		c = color.New(color.FgRed, color.Bold)
	case "DISABLED":
		c = color.New(color.FgHiBlack, color.Bold)
	default:
		return status
	}
	return c.Sprint(status)
}
