package gt_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/gt"
)

// buildState constructs a State: trunk "main"; "a" off main; "b" off "a";
// "other" off main (an unrelated stack that Lineage must exclude).
func buildState() gt.State {
	return gt.State{
		"main":  {Trunk: true},
		"a":     {Parents: []gt.Parent{{Ref: "main"}}},
		"b":     {Parents: []gt.Parent{{Ref: "a"}}, NeedsRestack: true},
		"other": {Parents: []gt.Parent{{Ref: "main"}}},
	}
}

func names(bs []gt.OrderedBranch) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Name
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestLineageScopesToStack verifies Lineage returns the trunk, the branch's
// ancestors, and its descendants — but not unrelated sibling stacks.
func TestLineageScopesToStack(t *testing.T) {
	s := buildState()

	got := names(s.Lineage("a"))
	// Expect main (trunk ancestor), a (self), b (descendant); never "other".
	for _, want := range []string{"main", "a", "b"} {
		if !contains(got, want) {
			t.Errorf("Lineage(a) = %v, missing %q", got, want)
		}
	}
	if contains(got, "other") {
		t.Errorf("Lineage(a) = %v, should not contain unrelated stack 'other'", got)
	}
	if len(got) != 3 {
		t.Errorf("Lineage(a) = %v, want exactly 3 branches", got)
	}

	// Trunk-first ordering.
	if got[0] != "main" {
		t.Errorf("Lineage(a)[0] = %q, want trunk 'main' first", got[0])
	}
}

// TestLineageFromLeaf verifies walking up from a deeper branch still includes
// the full ancestor chain to trunk.
func TestLineageFromLeaf(t *testing.T) {
	s := buildState()
	got := names(s.Lineage("b"))
	for _, want := range []string{"main", "a", "b"} {
		if !contains(got, want) {
			t.Errorf("Lineage(b) = %v, missing %q", got, want)
		}
	}
	if contains(got, "other") {
		t.Errorf("Lineage(b) = %v, should not contain 'other'", got)
	}
}

// TestLineageAbsentBranch verifies a branch not in the state yields nil so the
// caller can fall back to showing the bare branch name.
func TestLineageAbsentBranch(t *testing.T) {
	s := buildState()
	if got := s.Lineage("ghost"); got != nil {
		t.Errorf("Lineage(ghost) = %v, want nil", got)
	}
}
