package forge

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestPRsForBranchesCtxUsesBoundedConcurrency(t *testing.T) {
	branches := make([]string, 12)
	for index := range branches {
		branches[index] = fmt.Sprintf("stack-%d", index)
	}
	var active atomic.Int32
	var maximum atomic.Int32
	lookup := func(_ context.Context, _ string, branch string) (*PR, error) {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return &PR{Branch: branch, Number: 1, State: "OPEN"}, nil
	}

	prs, err := prsForBranchesCtx(context.Background(), "/repo", branches, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != len(branches) {
		t.Fatalf("PR count = %d, want %d", len(prs), len(branches))
	}
	if maximum.Load() != 4 {
		t.Fatalf("maximum concurrent lookups = %d, want 4", maximum.Load())
	}
}
