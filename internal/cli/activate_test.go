package cli

import (
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
)

// TestBuildActivations verifies that buildActivations correctly maps workspace
// config and model.Slice into []swap.RepoActivation entries.
func TestBuildActivations(t *testing.T) {
	ws := config.Workspace{
		Root: "/repos",
		Repos: map[string]config.Repo{
			"web": {Primary: "/repos/web", DefaultBranch: "main"},
			"api": {Primary: "/repos/api", DefaultBranch: "main"},
		},
		Swap: config.Swap{
			DepReconcile: map[string]config.DepReconcile{
				"web": {
					Lockfiles: []string{"pnpm-lock.yaml"},
					Install:   "pnpm install",
				},
				// "api" has no DepReconcile entry — Lockfiles should be nil/empty.
			},
		},
	}

	sl := model.Slice{
		Name: "checkout",
		Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "jonny/checkout", WorktreePath: "/repos/.slis/worktrees/checkout/web"},
			"api": {Repo: "api", Branch: "jonny/checkout", WorktreePath: "/repos/.slis/worktrees/checkout/api"},
		},
	}

	got := buildActivations(ws, sl)

	if len(got) != 2 {
		t.Fatalf("want 2 activations, got %d", len(got))
	}

	// Build a map for order-independent lookup.
	byRepo := make(map[string]swap.RepoActivation, len(got))
	for _, ra := range got {
		byRepo[ra.Repo] = ra
	}

	// web: should have Primary and Lockfiles from DepReconcile.
	webRA, ok := byRepo["web"]
	if !ok {
		t.Fatal("missing activation for repo 'web'")
	}
	if webRA.Primary != "/repos/web" {
		t.Errorf("web Primary = %q, want /repos/web", webRA.Primary)
	}
	if webRA.Branch != "jonny/checkout" {
		t.Errorf("web Branch = %q, want jonny/checkout", webRA.Branch)
	}
	if len(webRA.Lockfiles) != 1 || webRA.Lockfiles[0] != "pnpm-lock.yaml" {
		t.Errorf("web Lockfiles = %v, want [pnpm-lock.yaml]", webRA.Lockfiles)
	}

	// api: should have Primary from ws but empty Lockfiles.
	apiRA, ok := byRepo["api"]
	if !ok {
		t.Fatal("missing activation for repo 'api'")
	}
	if apiRA.Primary != "/repos/api" {
		t.Errorf("api Primary = %q, want /repos/api", apiRA.Primary)
	}
	if apiRA.Branch != "jonny/checkout" {
		t.Errorf("api Branch = %q, want jonny/checkout", apiRA.Branch)
	}
	if len(apiRA.Lockfiles) != 0 {
		t.Errorf("api Lockfiles = %v, want empty", apiRA.Lockfiles)
	}
}

// TestWorktreePlan verifies that worktreePlan derives branch names and paths
// correctly from workspace config without touching git.
func TestWorktreePlan(t *testing.T) {
	ws := config.Workspace{
		Root: "/x",
		Repos: map[string]config.Repo{
			"a": {Primary: "/x/a", DefaultBranch: "main"},
			"b": {Primary: "/x/b", DefaultBranch: "main"},
		},
		Grouping: config.Grouping{
			StripPrefix: "jonny/",
		},
	}

	plans := worktreePlan(ws, "checkout", "jonny/checkout")

	if len(plans) != 2 {
		t.Fatalf("want 2 plans, got %d", len(plans))
	}

	// Build a map for order-independent lookup.
	type planEntry = struct{ Repo, Primary, Branch, Path, StartPoint string }
	byRepo := make(map[string]planEntry, len(plans))
	for _, p := range plans {
		byRepo[p.Repo] = p
	}

	for _, repoName := range []string{"a", "b"} {
		p, ok := byRepo[repoName]
		if !ok {
			t.Fatalf("missing plan for repo %q", repoName)
		}
		wantBranch := "jonny/checkout"
		if p.Branch != wantBranch {
			t.Errorf("repo %s Branch = %q, want %q", repoName, p.Branch, wantBranch)
		}
		wantPath := filepath.Join("/x", ".slis", "worktrees", "checkout", repoName)
		if p.Path != wantPath {
			t.Errorf("repo %s Path = %q, want %q", repoName, p.Path, wantPath)
		}
		wantPrimary := "/x/" + repoName
		if p.Primary != wantPrimary {
			t.Errorf("repo %s Primary = %q, want %q", repoName, p.Primary, wantPrimary)
		}
		// New slices fork from the repo's trunk, not the primary's current HEAD.
		if p.StartPoint != "main" {
			t.Errorf("repo %s StartPoint = %q, want %q", repoName, p.StartPoint, "main")
		}
	}
}
