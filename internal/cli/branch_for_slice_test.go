package cli

import "testing"

func TestBranchForSlice(t *testing.T) {
	cases := []struct {
		name       string
		prefix     string
		slice      string
		wantBranch string
	}{
		{"adds prefix to bare name", "jonny/", "wfm-123", "jonny/wfm-123"},
		{"no double when name already prefixed", "jonny/", "jonny/wfm-123", "jonny/wfm-123"},
		// Regression: strip_prefix without a trailing slash + a fully-qualified
		// name used to yield "jonnyjonny/wfm-123".
		{"no double with slashless prefix", "jonny", "jonny/wfm-123", "jonny/wfm-123"},
		{"empty prefix is identity", "", "wfm-123", "wfm-123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := branchForSlice(c.prefix, c.slice); got != c.wantBranch {
				t.Errorf("branchForSlice(%q, %q) = %q, want %q", c.prefix, c.slice, got, c.wantBranch)
			}
		})
	}
}
