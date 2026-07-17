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
	// Created-slice case: every member's worktree shares one immediate parent
	// (ws.Root/.slis/worktrees/<slice>/).
	shareParent := []model.SliceMember{
		{Repo: "beta", WorktreePath: "/ws/.slis/worktrees/feat/beta"},
		{Repo: "alpha", WorktreePath: "/ws/.slis/worktrees/feat/alpha"},
	}
	// Adopted/discovered slice: worktrees live under arbitrary, unrelated parents.
	scattered := []model.SliceMember{
		{Repo: "beta", WorktreePath: "/somewhere/beta-wt"},
		{Repo: "alpha", WorktreePath: "/elsewhere/alpha-wt"},
	}
	single := []model.SliceMember{
		{Repo: "alpha", WorktreePath: "/ws/.slis/worktrees/feat/alpha"},
	}

	cases := []struct {
		name    string
		members []model.SliceMember
		opts    SessionOpts
		want    []string
	}{
		{"default+root, common parent → root only", shareParent, SessionOpts{Root: "/ws"}, []string{"root"}},
		{"default no root → repos (sorted)", shareParent, SessionOpts{}, []string{"alpha", "beta"}},
		{"both, common parent → root then repos", shareParent, SessionOpts{Root: "/ws", Layout: "both"}, []string{"root", "alpha", "beta"}},
		{"repos explicit unchanged", shareParent, SessionOpts{Root: "/ws", Layout: "repos"}, []string{"alpha", "beta"}},
		{"root layout but no root → fallback repos", shareParent, SessionOpts{Layout: "root"}, []string{"alpha", "beta"}},
		{"root, no common parent → fallback repos", scattered, SessionOpts{Root: "/ws"}, []string{"alpha", "beta"}},
		{"both, no common parent → fallback repos", scattered, SessionOpts{Root: "/ws", Layout: "both"}, []string{"alpha", "beta"}},
		{"single member → root only", single, SessionOpts{Root: "/ws"}, []string{"root"}},
	}
	for _, tc := range cases {
		if got := winNames(sessionWindows(tc.members, tc.opts)); !eq(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}

	// Common-parent case: the root window cd's into the members' shared parent,
	// NOT the workspace root (which holds the primary checkouts).
	w := sessionWindows(shareParent, SessionOpts{Root: "/ws"})
	if w[0].cwd != "/ws/.slis/worktrees/feat" {
		t.Errorf("root window cwd = %q, want /ws/.slis/worktrees/feat", w[0].cwd)
	}

	// "both" root window also targets the shared parent, then per-repo windows
	// keep their own worktree cwds.
	wb := sessionWindows(shareParent, SessionOpts{Root: "/ws", Layout: "both"})
	if wb[0].cwd != "/ws/.slis/worktrees/feat" {
		t.Errorf("both root window cwd = %q, want /ws/.slis/worktrees/feat", wb[0].cwd)
	}
	if wb[1].cwd != "/ws/.slis/worktrees/feat/alpha" || wb[2].cwd != "/ws/.slis/worktrees/feat/beta" {
		t.Errorf("both repo cwds = %q,%q; want alpha then beta worktrees", wb[1].cwd, wb[2].cwd)
	}

	// Single member: the root window cd's straight into that member's worktree.
	ws := sessionWindows(single, SessionOpts{Root: "/ws"})
	if ws[0].cwd != "/ws/.slis/worktrees/feat/alpha" {
		t.Errorf("single-member root window cwd = %q, want the member worktree", ws[0].cwd)
	}

	// No common parent: fallback per-repo windows keep each worktree cwd.
	ws2 := sessionWindows(scattered, SessionOpts{Root: "/ws"})
	if ws2[0].cwd != "/elsewhere/alpha-wt" || ws2[1].cwd != "/somewhere/beta-wt" {
		t.Errorf("fallback repo cwds = %q,%q; want each worktree path", ws2[0].cwd, ws2[1].cwd)
	}
}
