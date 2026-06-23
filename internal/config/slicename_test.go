package config

import "testing"

func TestSliceNameFromBranch(t *testing.T) {
	cases := []struct {
		name   string
		branch string
		prefix string
		want   string
	}{
		{"slash prefix stripped", "jonny/wfm-1", "jonny/", "wfm-1"},
		// slashless prefix (the config that caused jonnyjonny/): leftover leading
		// slash is also trimmed so the slice name is clean.
		{"slashless prefix trims leading slash", "jonny/wfm-1", "jonny", "wfm-1"},
		{"no prefix match left unchanged", "feature/x", "jonny/", "feature/x"},
		{"empty prefix is identity", "jonny/wfm-1", "", "jonny/wfm-1"},
		{"nested name kept", "jonny/wfm-1/sub", "jonny/", "wfm-1/sub"},
		{"non-slash prefix", "feature-x", "feature-", "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SliceNameFromBranch(c.branch, c.prefix); got != c.want {
				t.Errorf("SliceNameFromBranch(%q, %q) = %q, want %q", c.branch, c.prefix, got, c.want)
			}
		})
	}
}
