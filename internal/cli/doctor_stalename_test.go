package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
)

func staleNameWorkspace() config.Workspace {
	return config.Workspace{
		Root:     "/ws",
		Grouping: config.Grouping{StripPrefix: "jonny/"},
	}
}

// The live incident: a slice still named after `jonny/unpaid-leave-f2-endpoint-guards`
// (merged as #8456, then deleted) while its worktree — an agent scratch checkout
// slis ignores by default — holds unrelated wage-proration work.
func TestStaleNameFindings_ReportsAndOffersFixForIgnoredPath(t *testing.T) {
	dtos := []SliceDTO{{
		Name: "unpaid-leave-f2-endpoint-guards",
		Members: []MemberDTO{{
			Repo:         "nory",
			Branch:       "claude/wage-changes-proration-bffowg",
			WorktreePath: "/ws/nory/.claude/worktrees/unpaid-nory",
		}},
	}}

	findings := staleNameFindings(staleNameWorkspace(), dtos, filepath.Join(t.TempDir(), "registry.yaml"))

	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %+v", findings)
	}
	f := findings[0]
	if f.Level != lvlWarn {
		t.Errorf("stale name should warn, got %v", f.Level)
	}
	if !strings.Contains(f.Title, "unpaid-leave-f2-endpoint-guards") {
		t.Errorf("finding should name the slice: %q", f.Title)
	}
	if !strings.Contains(f.Detail, "claude/wage-changes-proration-bffowg") {
		t.Errorf("finding should say what it actually holds: %q", f.Detail)
	}
	if f.fix == nil {
		t.Error("a slice on a default-ignored path should be fixable by forgetting it")
	}
}

// Real work that merely drifted is reported, never auto-changed: dropping the
// registry entry would un-manage a worktree the user still cares about.
func TestStaleNameFindings_RealWorktreeIsReportOnly(t *testing.T) {
	dtos := []SliceDTO{{
		Name: "PAY-207",
		Members: []MemberDTO{{
			Repo:         "nory",
			Branch:       "jonny/pay-208-start-date-guard",
			WorktreePath: "/ws/.slis/worktrees/PAY-207/nory",
		}},
	}}

	findings := staleNameFindings(staleNameWorkspace(), dtos, filepath.Join(t.TempDir(), "registry.yaml"))

	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %+v", findings)
	}
	if findings[0].fix != nil {
		t.Error("a managed worktree must not be forgotten automatically")
	}
}

// Healthy slices stay silent: an auto-grouped name, and a stacked switch inside
// the same feature.
func TestStaleNameFindings_SilentWhenNamesStillDescribeTheWork(t *testing.T) {
	dtos := []SliceDTO{
		{Name: "sick-pay-not-syncing", Members: []MemberDTO{
			{Repo: "nory", Branch: "jonny/sick-pay-not-syncing", WorktreePath: "/ws/.slis/worktrees/sick-pay/nory"},
		}},
		{Name: "unpaid-leave", Members: []MemberDTO{
			{Repo: "web", Branch: "jonny/unpaid-leave-j-mapping-ui", WorktreePath: "/ws/.slis/worktrees/unpaid/web"},
		}},
	}

	if findings := staleNameFindings(staleNameWorkspace(), dtos, "/dev/null"); len(findings) != 0 {
		t.Fatalf("healthy slices must not warn, got %+v", findings)
	}
}
