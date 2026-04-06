package output

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WriteCSV writes rows as CSV to path. Profile name+version appear as columns on every row.
func WriteCSV(path string, rows []RepoRow, profileName string, profileVersion int) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"org", "project", "repository", "score", "is_inactive", "status", "reasons", "profile", "profile_version"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		status := "active"
		if r.IsInactive {
			status = "INACTIVE"
		}
		record := []string{
			r.OrgSlug, r.Project, r.Repo,
			fmt.Sprintf("%.4f", r.Score),
			strconv.FormatBool(r.IsInactive),
			status,
			strings.Join(r.Reasons, "|"),
			profileName,
			strconv.Itoa(profileVersion),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.Error()
}
