package discovery_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
)

func TestGatherStackLinearChain(t *testing.T) {
	st := gt.State{
		"master":  {Trunk: true},
		"pay-103": {Parents: []gt.Parent{{Ref: "master"}}},
		"pay-104": {Parents: []gt.Parent{{Ref: "pay-103"}}},
		"pay-105": {Parents: []gt.Parent{{Ref: "pay-104"}}},
	}

	// Gathering from any branch in the chain resolves the same tip + folds.
	for _, from := range []string{"pay-103", "pay-104", "pay-105"} {
		tip, folded, linear, ok := discovery.GatherStack(st, from)
		if !ok {
			t.Fatalf("from %q: ok=false; want a gatherable stack", from)
		}
		if tip != "pay-105" {
			t.Errorf("from %q: tip=%q; want pay-105 (the leaf)", from, tip)
		}
		if !linear {
			t.Errorf("from %q: linear=false; want true for a straight chain", from)
		}
		if got := len(folded); got != 2 {
			t.Errorf("from %q: folded=%v; want the two non-tip branches", from, folded)
		}
		for _, f := range folded {
			if f == "pay-105" {
				t.Errorf("from %q: tip pay-105 must not be folded", from)
			}
		}
	}
}

func TestGatherStackSingleBranchNotGatherable(t *testing.T) {
	st := gt.State{
		"master": {Trunk: true},
		"lone":   {Parents: []gt.Parent{{Ref: "master"}}},
	}
	_, _, _, ok := discovery.GatherStack(st, "lone")
	if ok {
		t.Error("ok=true for a single branch off trunk; nothing to gather")
	}
}

func TestGatherStackForkNotLinear(t *testing.T) {
	// pay-103 has two children (pay-104a, pay-104b): a fork, not a chain.
	st := gt.State{
		"master":   {Trunk: true},
		"pay-103":  {Parents: []gt.Parent{{Ref: "master"}}},
		"pay-104a": {Parents: []gt.Parent{{Ref: "pay-103"}}},
		"pay-104b": {Parents: []gt.Parent{{Ref: "pay-103"}}},
	}
	// Gather from the fork root so both children fall in the lineage.
	_, folded, linear, ok := discovery.GatherStack(st, "pay-103")
	if !ok {
		t.Fatal("ok=false; a fork is still gatherable")
	}
	if linear {
		t.Error("linear=true; want false when the lineage forks")
	}
	if len(folded) == 0 {
		t.Error("folded empty; fork gather should still fold branches")
	}
}
