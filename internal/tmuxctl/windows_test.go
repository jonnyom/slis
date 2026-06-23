package tmuxctl

import (
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func winNames(ws []window) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.name
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSessionWindows(t *testing.T) {
	members := []model.SliceMember{
		{Repo: "beta", WorktreePath: "/b"},
		{Repo: "alpha", WorktreePath: "/a"},
	}

	cases := []struct {
		name string
		opts SessionOpts
		want []string
	}{
		{"default+root → root only", SessionOpts{Root: "/root"}, []string{"root"}},
		{"default no root → repos (sorted)", SessionOpts{}, []string{"alpha", "beta"}},
		{"both → root first then repos", SessionOpts{Root: "/root", Layout: "both"}, []string{"root", "alpha", "beta"}},
		{"repos explicit", SessionOpts{Root: "/root", Layout: "repos"}, []string{"alpha", "beta"}},
		{"root layout but no root → fallback repos", SessionOpts{Layout: "root"}, []string{"alpha", "beta"}},
	}
	for _, tc := range cases {
		if got := winNames(sessionWindows(members, tc.opts)); !eq(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}

	// The root window's cwd is the configured root.
	w := sessionWindows(members, SessionOpts{Root: "/root"})
	if w[0].cwd != "/root" {
		t.Errorf("root window cwd = %q, want /root", w[0].cwd)
	}
}
