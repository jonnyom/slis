package discovery

import "testing"

func TestSliceNameIsStale(t *testing.T) {
	cases := []struct {
		name     string
		branches []string
		want     bool
	}{
		// The live incident: the branch the name came from merged and was deleted,
		// and an agent checked unrelated work out into the worktree.
		{"unpaid-leave-f2-endpoint-guards", []string{"claude/wage-changes-proration-bffowg"}, true},
		// Healthy: auto-grouped slices are named from their own branch.
		{"sick-pay-not-syncing", []string{"jonny/sick-pay-not-syncing"}, false},
		// Healthy: a stacked switch inside the same feature.
		{"unpaid-leave", []string{"jonny/unpaid-leave-e2a-creation-sync"}, false},
		// Healthy: one repo drifted but another still carries the name.
		{"PAY-207", []string{"jonny/pay-208-start-date-guard", "jonny/PAY-207"}, false},
		// Every member drifted → the label describes nothing present.
		{"PAY-207", []string{"jonny/pay-208-start-date-guard"}, true},
		// Nothing to judge.
		{"anything", nil, false},
		{"", []string{"jonny/x"}, false},
	}
	for _, tc := range cases {
		if got := SliceNameIsStale(tc.name, tc.branches, "jonny/"); got != tc.want {
			t.Errorf("SliceNameIsStale(%q, %v) = %v, want %v", tc.name, tc.branches, got, tc.want)
		}
	}
}
