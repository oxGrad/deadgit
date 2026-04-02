package scoring_test

import (
	"testing"

	"github.com/oxGrad/deadgit/internal/scoring"
)

func TestNormalizeLinear(t *testing.T) {
	tests := []struct {
		days      float64
		threshold int
		want      float64
	}{
		{0, 90, 0.0},
		{45, 90, 0.5},
		{90, 90, 1.0},
		{200, 90, 1.0}, // capped at 1.0
		{0, 0, 0.0},    // zero threshold safe
	}
	for _, tc := range tests {
		got := scoring.NormalizeLinear(tc.days, tc.threshold)
		if got != tc.want {
			t.Errorf("NormalizeLinear(%v, %v) = %v, want %v", tc.days, tc.threshold, got, tc.want)
		}
	}
}

func TestNormalizeCommitFrequency(t *testing.T) {
	tests := []struct {
		commits   int
		threshold int
		wantHigh  bool
	}{
		{0, 90, true},    // zero commits → fully inactive
		{100, 90, false}, // many commits → active
		{1, 90, true},    // very few commits → inactive
	}
	for _, tc := range tests {
		got := scoring.NormalizeCommitFrequency(tc.commits, tc.threshold)
		if got < 0 || got > 1 {
			t.Errorf("NormalizeCommitFrequency(%d, %d) = %v out of range", tc.commits, tc.threshold, got)
		}
		if (got >= 0.8) != tc.wantHigh {
			t.Errorf("NormalizeCommitFrequency(%d, %d) = %v, wantHigh=%v", tc.commits, tc.threshold, got, tc.wantHigh)
		}
	}
}

func TestNormalizeBranchStaleness(t *testing.T) {
	tests := []struct {
		branches int
		want     float64
	}{
		{0, 1.0},
		{3, 0.0},
		{10, 0.0},
	}
	for _, tc := range tests {
		got := scoring.NormalizeBranchStaleness(tc.branches)
		if got < 0 || got > 1 {
			t.Errorf("NormalizeBranchStaleness(%d) = %v out of range", tc.branches, got)
		}
		if got != tc.want {
			t.Errorf("NormalizeBranchStaleness(%d) = %v, want %v", tc.branches, got, tc.want)
		}
	}
}
