package discovery

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestApplyRegroups(t *testing.T) {
	discovered := []model.Slice{
		{
			Name: "checkout",
			Members: map[string]model.SliceMember{
				"web": {
					Repo:         "web",
					Branch:       "jonny/checkout",
					WorktreePath: "/wt/web",
					TipSHA:       "abc",
				},
			},
		},
		{
			Name: "checkout-api",
			Members: map[string]model.SliceMember{
				"api": {
					Repo:         "api",
					Branch:       "jonny/checkout-api",
					WorktreePath: "/wt/api",
					TipSHA:       "def",
				},
			},
		},
	}

	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	got := Apply(discovered, ov)

	// Expect exactly one slice named "checkout".
	if len(got) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(got))
	}
	if got[0].Name != "checkout" {
		t.Fatalf("expected slice name %q, got %q", "checkout", got[0].Name)
	}

	// Expect both web and api members.
	if len(got[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got[0].Members))
	}

	// Verify web member is intact.
	web, ok := got[0].Members["web"]
	if !ok {
		t.Fatal("expected member 'web' in checkout slice")
	}
	if web.WorktreePath != "/wt/web" || web.TipSHA != "abc" {
		t.Errorf("web member unexpected: %+v", web)
	}

	// Verify api member was moved and kept its original fields.
	api, ok := got[0].Members["api"]
	if !ok {
		t.Fatal("expected member 'api' in checkout slice")
	}
	if api.WorktreePath != "/wt/api" {
		t.Errorf("api WorktreePath: want %q, got %q", "/wt/api", api.WorktreePath)
	}
	if api.TipSHA != "def" {
		t.Errorf("api TipSHA: want %q, got %q", "def", api.TipSHA)
	}
}

func TestSaveLoadOverridesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	if err := SaveOverrides(path, ov); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("LoadOverrides: %v", err)
	}

	if !reflect.DeepEqual(got, ov) {
		t.Errorf("round-trip mismatch:\n  got  %v\n  want %v", got, ov)
	}
}

func TestLoadOverridesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.yaml")

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty Overrides, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty Overrides, got %v", got)
	}
}

func TestResolveKeepsGroupedWorktreeAfterBranchSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	if err := SaveOverrides(path, Overrides{
		"unpaid-leave": {
			"web":  "jonny/unpaid-leave-j-mapping-ui",
			"nory": "jonny/unpaid-leave-f2-endpoint-guards",
		},
	}); err != nil {
		t.Fatal(err)
	}
	slices := []model.Slice{
		{Name: "unpaid-leave-j-mapping-ui", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "jonny/unpaid-leave-j-mapping-ui"},
		}},
		{Name: "unpaid-leave-f2-endpoint-guards", Members: map[string]model.SliceMember{
			"nory": {Repo: "nory", Branch: "jonny/unpaid-leave-e2a-creation-sync"},
		}},
	}

	got := Resolve(slices, path, "jonny/")
	if len(got) != 1 || got[0].Name != "unpaid-leave" || len(got[0].Members) != 2 {
		t.Fatalf("group split after branch switch: %+v", got)
	}
}

func TestApplyFoldsHidesSubsumedBranches(t *testing.T) {
	slices := []model.Slice{
		{Name: "stack", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-105"}, // the tip (representative)
		}},
		{Name: "pay-103", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-103"},
		}},
		{Name: "pay-104", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-104"},
		}},
		{Name: "unrelated", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "other"},
		}},
	}
	folded := Folded{"stack": {"web": {"pay-103", "pay-104"}}}

	got := ApplyFolds(slices, folded)

	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Name)
	}
	want := []string{"stack", "unrelated"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("kept slices = %v; want %v (folded intermediates hidden, tip + unrelated kept)", names, want)
	}
}

func TestApplyFoldsRepoScoped(t *testing.T) {
	// A branch named "pay-103" in repo "api" is NOT folded by a web-repo fold.
	slices := []model.Slice{
		{Name: "api-103", Members: map[string]model.SliceMember{
			"api": {Repo: "api", Branch: "pay-103"},
		}},
	}
	got := ApplyFolds(slices, Folded{"stack": {"web": {"pay-103"}}})
	if len(got) != 1 {
		t.Errorf("api-103 dropped by a web-repo fold; folds must be repo-scoped")
	}
}

func TestSaveOverridesPreservesFolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	if err := SaveConfig(path, Overrides{"stack": {"web": "pay-105"}}, Folded{"stack": {"web": {"pay-103", "pay-104"}}}); err != nil {
		t.Fatal(err)
	}
	// A grouping-only save must not wipe the folded section.
	if err := SaveOverrides(path, Overrides{"stack": {"web": "pay-105"}, "x": {"api": "feat"}}); err != nil {
		t.Fatal(err)
	}
	folded, err := LoadFolded(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(folded, Folded{"stack": {"web": {"pay-103", "pay-104"}}}) {
		t.Errorf("folds lost after SaveOverrides: %v", folded)
	}
}

// A grouping override names a branch. When that branch disappears, slis assumes
// the branch was renamed in place and re-points the override at whatever the same
// worktree now holds, so a rename does not break your grouping.
//
// The failure this guards: a MERGED branch gets deleted by `gt sync`, its worktree
// is then reused by something else (observed live: a Claude Code session checked
// `claude/wage-changes-proration-bffowg` out into the worktree that used to hold
// `jonny/unpaid-leave-f2-endpoint-guards`), and the override silently adopted a
// different feature's branch into the slice. A rename is provable — `git branch -m`
// records it in the new branch's reflog — so anything unprovable is left alone.
func TestRetargetMovedOverrideMembers(t *testing.T) {
	// The worktree keeps its durable slice identity (named after the branch the
	// override still points at) while its branch has moved on — that pairing is
	// what the retarget looks up.
	renamedInPlace := func() []model.Slice {
		return []model.Slice{{
			Name: "feature-one",
			Members: map[string]model.SliceMember{
				"api": {Repo: "api", Branch: "jonny/feature-two", WorktreePath: "/wt/api"},
			},
		}}
	}

	t.Run("retargets a proven rename", func(t *testing.T) {
		slices := renamedInPlace()
		ov := Overrides{"group": {"api": "jonny/feature-one"}}
		renamed := func(dir, newBranch, oldBranch string) bool {
			return dir == "/wt/api" && newBranch == "jonny/feature-two" && oldBranch == "jonny/feature-one"
		}

		retargetMovedOverrideMembers(slices, ov, "jonny/", renamed)

		if got := ov["group"]["api"]; got != "jonny/feature-two" {
			t.Fatalf("override should follow the rename, got %q", got)
		}
	})

	t.Run("leaves the override alone when no rename is recorded", func(t *testing.T) {
		slices := []model.Slice{{
			Name: "feature-one",
			Members: map[string]model.SliceMember{
				// Same worktree, unrelated branch: a takeover, not a rename.
				"api": {Repo: "api", Branch: "claude/other-work", WorktreePath: "/wt/api"},
			},
		}}
		ov := Overrides{"group": {"api": "jonny/feature-one"}}

		retargetMovedOverrideMembers(slices, ov, "jonny/", func(string, string, string) bool { return false })

		if got := ov["group"]["api"]; got != "jonny/feature-one" {
			t.Fatalf("override must not adopt an unrelated branch, got %q", got)
		}
	})

	t.Run("a takeover leaves the member out of the group entirely", func(t *testing.T) {
		slices := []model.Slice{{
			Name: "feature-one",
			Members: map[string]model.SliceMember{
				"api": {Repo: "api", Branch: "claude/other-work", WorktreePath: "/wt/api"},
				"web": {Repo: "web", Branch: "jonny/feature-one", WorktreePath: "/wt/web"},
			},
		}}
		ov := Overrides{"group": {"api": "jonny/feature-one", "web": "jonny/feature-one"}}

		retargetMovedOverrideMembers(slices, ov, "jonny/", func(string, string, string) bool { return false })
		out := Apply(slices, ov)

		group := findSliceByName(t, out, "group")
		if _, claimed := group.Members["api"]; claimed {
			t.Fatalf("the taken-over worktree must not be grouped: %+v", group.Members)
		}
		if group.Members["web"].Branch != "jonny/feature-one" {
			t.Fatalf("the untouched member should still be grouped, got %+v", group.Members)
		}
	})

	t.Run("does nothing while the override's branch still exists", func(t *testing.T) {
		slices := []model.Slice{{
			Name: "feature-one",
			Members: map[string]model.SliceMember{
				"api": {Repo: "api", Branch: "jonny/feature-one", WorktreePath: "/wt/api"},
			},
		}}
		ov := Overrides{"group": {"api": "jonny/feature-one"}}
		renamed := func(string, string, string) bool {
			t.Fatal("must not consult git while the branch is present")
			return false
		}

		retargetMovedOverrideMembers(slices, ov, "jonny/", renamed)

		if got := ov["group"]["api"]; got != "jonny/feature-one" {
			t.Fatalf("override changed unexpectedly: %q", got)
		}
	})
}

func findSliceByName(t *testing.T, slices []model.Slice, name string) model.Slice {
	t.Helper()
	for _, s := range slices {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("slice %q not found in %+v", name, slices)
	return model.Slice{}
}

// The two live scenarios, side by side. Same worktree, same group, same mechanism —
// the only difference is whether the branch that replaced the override's branch is
// still that feature's work.
func TestRetargetDistinguishesStackSwitchFromTakeover(t *testing.T) {
	group := Overrides{"unpaid-leave": {"nory": "jonny/unpaid-leave-f2-endpoint-guards"}}
	worktreeNowOn := func(branch string) []model.Slice {
		return []model.Slice{{
			// Durable identity: still named after the branch the override points at.
			Name: "unpaid-leave-f2-endpoint-guards",
			Members: map[string]model.SliceMember{
				"nory": {Repo: "nory", Branch: branch, WorktreePath: "/wt/nory"},
			},
		}}
	}
	noRename := func(string, string, string) bool { return false }

	t.Run("stacked switch inside the feature is followed", func(t *testing.T) {
		ov := Overrides{"unpaid-leave": {"nory": group["unpaid-leave"]["nory"]}}
		retargetMovedOverrideMembers(worktreeNowOn("jonny/unpaid-leave-e2a-creation-sync"), ov, "jonny/", noRename)
		if got := ov["unpaid-leave"]["nory"]; got != "jonny/unpaid-leave-e2a-creation-sync" {
			t.Fatalf("a stacked switch should keep the group together, got %q", got)
		}
	})

	t.Run("an unrelated branch taking over the worktree is not adopted", func(t *testing.T) {
		ov := Overrides{"unpaid-leave": {"nory": group["unpaid-leave"]["nory"]}}
		retargetMovedOverrideMembers(worktreeNowOn("claude/wage-changes-proration-bffowg"), ov, "jonny/", noRename)
		if got := ov["unpaid-leave"]["nory"]; got != "jonny/unpaid-leave-f2-endpoint-guards" {
			t.Fatalf("a foreign branch must not be adopted into the group, got %q", got)
		}
	})
}

func TestBranchBelongsToGroup(t *testing.T) {
	cases := []struct {
		slice, branch string
		want          bool
	}{
		{"unpaid-leave", "jonny/unpaid-leave", true},
		{"unpaid-leave", "jonny/unpaid-leave-e2a-creation-sync", true},
		{"unpaid-leave-e2a", "jonny/unpaid-leave", true}, // group narrower than the branch
		{"unpaid-leave", "claude/wage-changes-proration-bffowg", false},
		{"unpaid-leave", "jonny/unpaid-leaver-typo", false}, // prefix must end at a dash
		// A single-token label is too generic to infer identity from: `api` must not
		// swallow whatever `api-*` branch lands in its worktree.
		{"api", "jonny/api", true},
		{"api", "jonny/api-infra", false},
		{"api", "jonny/api-v2", false},
		{"unpaid-leave", "", false},
		{"", "jonny/unpaid-leave", false},
	}
	for _, tc := range cases {
		if got := branchBelongsToGroup(tc.slice, tc.branch, "jonny/"); got != tc.want {
			t.Errorf("branchBelongsToGroup(%q, %q) = %v, want %v", tc.slice, tc.branch, got, tc.want)
		}
	}
}

// An auto-grouped worktree has no durable slice name: it is named after whatever
// branch it currently holds. So after a rename the identity lookup misses — the
// slice is called `feature-two`, while the override still says `jonny/feature-one`.
// git is then the only thing that can connect them.
func TestRetargetFollowsRenameWithoutDurableSliceName(t *testing.T) {
	slices := []model.Slice{{
		Name: "feature-two", // renamed: no trace of feature-one in the name
		Members: map[string]model.SliceMember{
			"api": {Repo: "api", Branch: "jonny/feature-two", WorktreePath: "/wt/api"},
		},
	}}
	ov := Overrides{"group": {"api": "jonny/feature-one"}}
	renamed := func(dir, newBranch, oldBranch string) bool {
		return dir == "/wt/api" && newBranch == "jonny/feature-two" && oldBranch == "jonny/feature-one"
	}

	retargetMovedOverrideMembers(slices, ov, "jonny/", renamed)

	if got := ov["group"]["api"]; got != "jonny/feature-two" {
		t.Fatalf("a git-proven rename should be followed even with no durable name, got %q", got)
	}
}

// The same shape, but nothing was renamed — the worktree simply holds unrelated
// work now. The git fallback must not turn into "adopt any branch in this repo".
func TestRetargetFallbackDoesNotAdoptUnprovenBranches(t *testing.T) {
	slices := []model.Slice{
		{Name: "claude/other-work", Members: map[string]model.SliceMember{
			"api": {Repo: "api", Branch: "claude/other-work", WorktreePath: "/wt/api"},
		}},
		{Name: "unrelated", Members: map[string]model.SliceMember{
			"api": {Repo: "api", Branch: "jonny/unrelated", WorktreePath: "/wt/other"},
		}},
	}
	ov := Overrides{"group": {"api": "jonny/feature-one"}}

	retargetMovedOverrideMembers(slices, ov, "jonny/", func(string, string, string) bool { return false })

	if got := ov["group"]["api"]; got != "jonny/feature-one" {
		t.Fatalf("override must stay put when no rename is provable, got %q", got)
	}
}
