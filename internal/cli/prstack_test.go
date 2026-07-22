package cli

import (
	"testing"

	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/model"
)

func slice3(repos ...string) model.Slice {
	m := map[string]model.SliceMember{}
	for _, r := range repos {
		m[r] = model.SliceMember{Repo: r, Branch: r + "-branch", WorktreePath: "/w/" + r}
	}
	return model.Slice{Name: "s", Members: m}
}

// TestOrderReposByStackTrunkFirst: with stack data, repos are ordered by
// ascending Graphite depth (trunk-first), ties broken by name.
func TestOrderReposByStackTrunkFirst(t *testing.T) {
	sl := slice3("web", "api", "core")
	depths := map[string]int{"web": 3, "api": 1, "core": 1}

	got := orderReposByStack(sl, depths, true)
	want := []string{"api", "core", "web"} // depth 1 (api,core by name) then depth 3 (web)
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v; want %v", got, want)
		}
	}
}

// TestOrderReposByStackAlphabeticalFallback: with no stack data, the plain
// alphabetical order is preserved.
func TestOrderReposByStackAlphabeticalFallback(t *testing.T) {
	sl := slice3("web", "api", "core")
	got := orderReposByStack(sl, map[string]int{}, false)
	want := []string{"api", "core", "web"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v; want alphabetical %v", got, want)
		}
	}
}

func TestShareTargetBranchesUsesGroupedStackTip(t *testing.T) {
	slice := model.Slice{Name: "unpaid-leave", Members: map[string]model.SliceMember{
		"nory": {Repo: "nory", Branch: "jonny/unpaid-leave-e2a-creation-sync", WorktreePath: "/work/nory"},
	}}
	overrides := discovery.Overrides{"unpaid-leave": {
		"nory": "jonny/unpaid-leave-f2-endpoint-guards",
	}}

	got := shareTargetBranches(slice, overrides, func(path, branch string) bool {
		return path == "/work/nory" && branch == "jonny/unpaid-leave-f2-endpoint-guards"
	})
	if got.Members["nory"].Branch != "jonny/unpaid-leave-f2-endpoint-guards" {
		t.Fatalf("branch = %q", got.Members["nory"].Branch)
	}
	if slice.Members["nory"].Branch != "jonny/unpaid-leave-e2a-creation-sync" {
		t.Fatalf("input slice was mutated: %+v", slice)
	}
}
