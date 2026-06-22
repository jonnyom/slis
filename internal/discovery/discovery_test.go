package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/testutil"
)

func findSlice(t *testing.T, slices []model.Slice, name string) model.Slice {
	t.Helper()
	for _, s := range slices {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("slice %q not found in %v", name, sliceNames(slices))
	return model.Slice{}
}

func sliceNames(slices []model.Slice) []string {
	names := make([]string, len(slices))
	for i, s := range slices {
		names[i] = s.Name
	}
	return names
}

func TestDiscover_BranchNameGrouping(t *testing.T) {
	// Build three temp repos.
	webRepo := testutil.NewRepo(t)
	apiRepo := testutil.NewRepo(t)
	opsRepo := testutil.NewRepo(t)

	// Add worktrees: web + api get jonny/checkout, ops gets jonny/other.
	webWTPath := filepath.Join(t.TempDir(), "web-checkout")
	apiWTPath := filepath.Join(t.TempDir(), "api-checkout")
	opsWTPath := filepath.Join(t.TempDir(), "ops-other")

	testutil.AddWorktree(t, webRepo, "jonny/checkout", webWTPath)
	testutil.AddWorktree(t, apiRepo, "jonny/checkout", apiWTPath)
	testutil.AddWorktree(t, opsRepo, "jonny/other", opsWTPath)

	ws := config.Workspace{
		Repos: map[string]config.Repo{
			"web": {Primary: webRepo, DefaultBranch: "main"},
			"api": {Primary: apiRepo, DefaultBranch: "main"},
			"ops": {Primary: opsRepo, DefaultBranch: "main"},
		},
		Grouping: config.Grouping{
			Strategy:    "branch-name",
			StripPrefix: "jonny/",
		},
	}

	slices, err := discovery.Discover(ws)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Expect exactly 2 slices.
	if len(slices) != 2 {
		t.Fatalf("expected 2 slices, got %d: %v", len(slices), sliceNames(slices))
	}

	// Slices must be sorted by name: "checkout" < "other".
	if slices[0].Name != "checkout" || slices[1].Name != "other" {
		t.Errorf("expected sorted [checkout other], got %v", sliceNames(slices))
	}

	// --- slice "checkout" must have web and api, not ops ---
	checkout := findSlice(t, slices, "checkout")
	if len(checkout.Members) != 2 {
		t.Errorf("checkout: expected 2 members, got %d: %v", len(checkout.Members), checkout.Repos())
	}
	for _, repoName := range []string{"web", "api"} {
		m, ok := checkout.Members[repoName]
		if !ok {
			t.Errorf("checkout: missing member %q", repoName)
			continue
		}
		if m.WorktreePath == "" {
			t.Errorf("checkout[%s]: WorktreePath is empty", repoName)
		}
		if len(m.TipSHA) != 40 {
			t.Errorf("checkout[%s]: TipSHA %q has length %d, want 40", repoName, m.TipSHA, len(m.TipSHA))
		}
		if m.Branch != "jonny/checkout" {
			t.Errorf("checkout[%s]: Branch = %q, want jonny/checkout", repoName, m.Branch)
		}
		if m.Repo != repoName {
			t.Errorf("checkout[%s]: Repo = %q, want %q", repoName, m.Repo, repoName)
		}
	}
	if _, hasOps := checkout.Members["ops"]; hasOps {
		t.Error("checkout: unexpected member \"ops\"")
	}

	// --- slice "other" must have only ops ---
	other := findSlice(t, slices, "other")
	if len(other.Members) != 1 {
		t.Errorf("other: expected 1 member, got %d: %v", len(other.Members), other.Repos())
	}
	m, ok := other.Members["ops"]
	if !ok {
		t.Error("other: missing member \"ops\"")
	} else {
		if m.WorktreePath == "" {
			t.Error("other[ops]: WorktreePath is empty")
		}
		if len(m.TipSHA) != 40 {
			t.Errorf("other[ops]: TipSHA %q has length %d, want 40", m.TipSHA, len(m.TipSHA))
		}
		if m.Branch != "jonny/other" {
			t.Errorf("other[ops]: Branch = %q, want jonny/other", m.Branch)
		}
	}

	// Base should be populated from the member repo's DefaultBranch ("main" for
	// all repos in this test). Active should be false (not set by Discover).
	for _, s := range slices {
		if s.Base != "main" {
			t.Errorf("slice %q: Base = %q, want \"main\"", s.Name, s.Base)
		}
		if s.Active {
			t.Errorf("slice %q: Active = true, want false", s.Name)
		}
	}
}
