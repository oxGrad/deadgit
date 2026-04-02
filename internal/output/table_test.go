package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oxGrad/deadgit/internal/output"
)

func TestPrintTable_ProfileVersion(t *testing.T) {
	var buf bytes.Buffer
	rows := []output.RepoRow{
		{OrgSlug: "myorg", Project: "proj", Repo: "legacy-api", Score: 0.8821, IsInactive: true, Reasons: []string{"No commits in 210d"}},
		{OrgSlug: "myorg", Project: "proj", Repo: "active-svc", Score: 0.1240, IsInactive: false},
	}
	opts := output.TableOptions{
		ProfileName: "default", ProfileVersion: 2, OrgSlugs: []string{"myorg"},
		TotalRepos: 2, InactiveCount: 1, CachedCount: 1, FetchedCount: 1, DurationSec: 0.5,
	}
	output.PrintTable(&buf, rows, opts)
	out := buf.String()
	if !strings.Contains(out, "default v2") {
		t.Errorf("expected 'default v2' in output:\n%s", out)
	}
	if !strings.Contains(out, "INACTIVE") {
		t.Errorf("expected 'INACTIVE' in output")
	}
	if !strings.Contains(out, "No commits in 210d") {
		t.Errorf("expected reason in output")
	}
}

func TestPrintTable_OverridesLabel(t *testing.T) {
	var buf bytes.Buffer
	opts := output.TableOptions{
		ProfileName: "default", ProfileVersion: 1, HasOverrides: true, OrgSlugs: []string{"myorg"},
	}
	output.PrintTable(&buf, nil, opts)
	if !strings.Contains(buf.String(), "(overrides active)") {
		t.Errorf("expected overrides label:\n%s", buf.String())
	}
}
