package report_test

import (
	"testing"
	"time"

	"github.com/oxGrad/deadgit/report"
)

func TestDetermineStatus_Active(t *testing.T) {
	now := time.Now()
	r := report.RepoReport{
		IsDisabled:          false,
		OpenPRCount:         0,
		LastCommitAnyBranch: &report.CommitInfo{Date: now.Add(-10 * 24 * time.Hour)},
	}
	status := report.DetermineActivityStatus(&r, 90)
	if status != "ACTIVE" {
		t.Errorf("expected ACTIVE, got %s", status)
	}
}

func TestDetermineStatus_Inactive(t *testing.T) {
	now := time.Now()
	r := report.RepoReport{
		IsDisabled:          false,
		OpenPRCount:         0,
		LastCommitAnyBranch: &report.CommitInfo{Date: now.Add(-100 * 24 * time.Hour)},
	}
	status := report.DetermineActivityStatus(&r, 90)
	if status != "INACTIVE" {
		t.Errorf("expected INACTIVE, got %s", status)
	}
}

func TestDetermineStatus_ActiveDueToOpenPR(t *testing.T) {
	now := time.Now()
	r := report.RepoReport{
		IsDisabled:          false,
		OpenPRCount:         2,
		LastCommitAnyBranch: &report.CommitInfo{Date: now.Add(-200 * 24 * time.Hour)},
	}
	status := report.DetermineActivityStatus(&r, 90)
	if status != "ACTIVE" {
		t.Errorf("expected ACTIVE due to open PRs, got %s", status)
	}
}

func TestDetermineStatus_Dormant(t *testing.T) {
	r := report.RepoReport{
		IsDisabled:          false,
		OpenPRCount:         0,
		LastCommitAnyBranch: nil,
	}
	status := report.DetermineActivityStatus(&r, 90)
	if status != "DORMANT" {
		t.Errorf("expected DORMANT, got %s", status)
	}
}

func TestDetermineStatus_Disabled(t *testing.T) {
	r := report.RepoReport{IsDisabled: true}
	status := report.DetermineActivityStatus(&r, 90)
	if status != "DISABLED" {
		t.Errorf("expected DISABLED, got %s", status)
	}
}

func TestDaysSince(t *testing.T) {
	now := time.Now()
	past := now.Add(-5 * 24 * time.Hour)
	days := report.DaysSince(past)
	if days < 4 || days > 6 {
		t.Errorf("expected ~5 days, got %d", days)
	}
}

func TestDaysSince_Zero(t *testing.T) {
	days := report.DaysSince(time.Time{})
	if days != 0 {
		t.Errorf("expected 0 for zero time, got %d", days)
	}
}
