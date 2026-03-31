package report

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WriteCSV writes reports to a CSV file at outPath.
func WriteCSV(reports []RepoReport, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create CSV file %s: %w", outPath, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"PROJECT", "REPO", "REPO_ID", "DEFAULT_BRANCH", "WEB_URL",
		"IS_DISABLED", "TOTAL_BRANCHES", "OPEN_PRS", "ACTIVITY_STATUS",
		"DAYS_SINCE_DEFAULT_COMMIT", "DAYS_SINCE_ANY_COMMIT",
		"LAST_COMMIT_DEFAULT_ID", "LAST_COMMIT_DEFAULT_AUTHOR", "LAST_COMMIT_DEFAULT_EMAIL",
		"LAST_COMMIT_DEFAULT_DATE", "LAST_COMMIT_DEFAULT_MESSAGE",
		"LAST_COMMIT_ANY_ID", "LAST_COMMIT_ANY_AUTHOR", "LAST_COMMIT_ANY_EMAIL",
		"LAST_COMMIT_ANY_DATE", "LAST_COMMIT_ANY_MESSAGE", "LAST_COMMIT_ANY_BRANCH",
		"PIPELINE_TEMPLATES",
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	for _, r := range reports {
		pipelines := make([]string, 0, len(r.Pipelines))
		for _, p := range r.Pipelines {
			if p.ExtendsPipeline != "" {
				pipelines = append(pipelines, p.ExtendsPipeline)
			}
		}

		row := []string{
			r.ProjectName,
			r.RepoName,
			r.RepoID,
			r.DefaultBranch,
			r.WebURL,
			strconv.FormatBool(r.IsDisabled),
			strconv.Itoa(r.TotalBranches),
			strconv.Itoa(r.OpenPRCount),
			r.ActivityStatus,
			strconv.Itoa(r.DaysSinceDefaultCommit),
			strconv.Itoa(r.DaysSinceAnyCommit),
			commitField(r.LastCommitDefault, "id"),
			commitField(r.LastCommitDefault, "author"),
			commitField(r.LastCommitDefault, "email"),
			commitField(r.LastCommitDefault, "date"),
			commitField(r.LastCommitDefault, "message"),
			commitField(r.LastCommitAnyBranch, "id"),
			commitField(r.LastCommitAnyBranch, "author"),
			commitField(r.LastCommitAnyBranch, "email"),
			commitField(r.LastCommitAnyBranch, "date"),
			commitField(r.LastCommitAnyBranch, "message"),
			commitField(r.LastCommitAnyBranch, "branch"),
			strings.Join(pipelines, "; "),
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write CSV row for %s/%s: %w", r.ProjectName, r.RepoName, err)
		}
	}
	return nil
}

func commitField(c *CommitInfo, field string) string {
	if c == nil {
		return ""
	}
	switch field {
	case "id":
		return c.CommitID
	case "author":
		return c.Author
	case "email":
		return c.Email
	case "date":
		return c.Date.Format("2006-01-02 15:04:05")
	case "message":
		return c.Message
	case "branch":
		return c.BranchName
	}
	return ""
}
