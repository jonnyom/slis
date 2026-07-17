package cli

import (
	"testing"

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
