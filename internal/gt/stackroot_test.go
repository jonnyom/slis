package gt_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/gt"
)

// stackState builds a State for the chain main(trunk) ← feat ← feat2.
func stackState() gt.State {
	return gt.State{
		"main":  {Trunk: true},
		"feat":  {Parents: []gt.Parent{{Ref: "main"}}},
		"feat2": {Parents: []gt.Parent{{Ref: "feat"}}},
	}
}

func TestStackRootChain(t *testing.T) {
	st := stackState()

	root, depth, ok := st.StackRoot("feat2")
	if !ok {
		t.Fatal("StackRoot(feat2): ok = false; want true")
	}
	if root != "feat" {
		t.Errorf("StackRoot(feat2) root = %q; want %q", root, "feat")
	}
	if depth != 1 {
		t.Errorf("StackRoot(feat2) depth = %d; want 1", depth)
	}

	root, depth, ok = st.StackRoot("feat")
	if !ok || root != "feat" || depth != 0 {
		t.Errorf("StackRoot(feat) = (%q, %d, %v); want (feat, 0, true)", root, depth, ok)
	}
}

func TestStackRootTrunkAndAbsent(t *testing.T) {
	st := stackState()

	if _, _, ok := st.StackRoot("main"); ok {
		t.Error("StackRoot(main): ok = true; a trunk is not part of a stack")
	}
	if _, _, ok := st.StackRoot("nope"); ok {
		t.Error("StackRoot(nope): ok = true; absent branch should be false")
	}
}

// TestStackRootSiblingsShareRoot verifies two branches stacked above the same
// base report the same root (so they cluster into one stack).
func TestStackRootSiblingsShareRoot(t *testing.T) {
	st := gt.State{
		"main": {Trunk: true},
		"base": {Parents: []gt.Parent{{Ref: "main"}}},
		"a":    {Parents: []gt.Parent{{Ref: "base"}}},
		"b":    {Parents: []gt.Parent{{Ref: "base"}}},
	}
	ra, _, _ := st.StackRoot("a")
	rb, _, _ := st.StackRoot("b")
	if ra != rb || ra != "base" {
		t.Errorf("siblings roots = (%q, %q); want both %q", ra, rb, "base")
	}
}
