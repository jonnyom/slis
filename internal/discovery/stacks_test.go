package discovery_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// webStack is the Graphite state shared by the "web" repo's worktrees:
// master(trunk) ← pay-103 ← pay-104 ← pay-105, plus an unrelated lone branch.
func webStack() gt.State {
	return gt.State{
		"master":  {Trunk: true},
		"pay-103": {Parents: []gt.Parent{{Ref: "master"}}},
		"pay-104": {Parents: []gt.Parent{{Ref: "pay-103"}}},
		"pay-105": {Parents: []gt.Parent{{Ref: "pay-104"}}},
		"lone":    {Parents: []gt.Parent{{Ref: "master"}}},
	}
}

func sliceWith(name, repo, branch, path string) model.Slice {
	return model.Slice{
		Name: name,
		Members: map[string]model.SliceMember{
			repo: {Repo: repo, Branch: branch, WorktreePath: path},
		},
	}
}

// fakeReader returns webStack for any web-repo worktree path and an empty state
// otherwise.
func fakeReader(byPath map[string]gt.State) discovery.StackReader {
	return func(path string) (gt.State, error) {
		if st, ok := byPath[path]; ok {
			return st, nil
		}
		return gt.State{}, nil
	}
}

func TestAnnotateStacksSharesStackID(t *testing.T) {
	st := webStack()
	byPath := map[string]gt.State{
		"/w/103": st, "/w/104": st, "/w/105": st, "/w/lone": st,
	}
	slices := []model.Slice{
		sliceWith("103", "web", "pay-103", "/w/103"),
		sliceWith("104", "web", "pay-104", "/w/104"),
		sliceWith("105", "web", "pay-105", "/w/105"),
		sliceWith("lone", "web", "lone", "/w/lone"),
	}

	slices = discovery.AnnotateStacks(slices, fakeReader(byPath))

	id103, id104, id105 := slices[0].StackID, slices[1].StackID, slices[2].StackID
	if id103 == "" {
		t.Fatal("pay-103 got empty StackID")
	}
	if id103 != id104 || id104 != id105 {
		t.Errorf("stacked slices have differing StackIDs: %q %q %q", id103, id104, id105)
	}
	if slices[0].StackOrder != 0 || slices[1].StackOrder != 1 || slices[2].StackOrder != 2 {
		t.Errorf("StackOrder = %d/%d/%d; want 0/1/2",
			slices[0].StackOrder, slices[1].StackOrder, slices[2].StackOrder)
	}
	// The lone branch (directly off trunk, no descendants) forms its own stack.
	if slices[3].StackID == id103 {
		t.Errorf("lone slice shares stack with pay chain: %q", slices[3].StackID)
	}
}

func TestAnnotateStacksNoDataLeavesEmpty(t *testing.T) {
	slices := []model.Slice{sliceWith("x", "web", "feat", "/w/x")}
	// Reader returns empty state for every path.
	slices = discovery.AnnotateStacks(slices, fakeReader(nil))
	if slices[0].StackID != "" {
		t.Errorf("StackID = %q; want empty when no stack data", slices[0].StackID)
	}
}

// TestAnnotateStacksDistinctReposDoNotShare verifies the StackID is repo-scoped:
// the same root branch name in two different repos does not merge two slices.
func TestAnnotateStacksDistinctReposDoNotShare(t *testing.T) {
	st := gt.State{"master": {Trunk: true}, "feat": {Parents: []gt.Parent{{Ref: "master"}}}}
	byPath := map[string]gt.State{"/a": st, "/b": st}
	slices := []model.Slice{
		sliceWith("one", "web", "feat", "/a"),
		sliceWith("two", "api", "feat", "/b"),
	}
	slices = discovery.AnnotateStacks(slices, fakeReader(byPath))
	if slices[0].StackID == slices[1].StackID {
		t.Errorf("slices in different repos share StackID %q; must be repo-scoped", slices[0].StackID)
	}
}

// TestAnnotateStacksCachesPerRepo verifies the reader is invoked once per
// distinct repo, not once per member: gt state is repo-global (identical across
// all of a repo's worktrees), so re-reading it per worktree is wasted subprocess
// spawns. Four worktrees of one repo must cost exactly one read.
func TestAnnotateStacksCachesPerRepo(t *testing.T) {
	st := webStack()
	calls := 0
	reader := func(path string) (gt.State, error) {
		calls++
		return st, nil
	}
	slices := []model.Slice{
		sliceWith("103", "web", "pay-103", "/w/103"),
		sliceWith("104", "web", "pay-104", "/w/104"),
		sliceWith("105", "web", "pay-105", "/w/105"),
		sliceWith("lone", "web", "lone", "/w/lone"),
	}

	discovery.AnnotateStacks(slices, reader)

	if calls != 1 {
		t.Errorf("reader called %d times across 4 worktrees of one repo; want 1 (per-repo cache)", calls)
	}
}
