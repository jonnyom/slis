package forge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestPRsForBranchesCtxIncludesInlineReviewComments(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = \"pr\" ]; then cat \"$PR_JSON\"; else cat \"$INLINE_JSON\"; fi\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prJSON, err := filepath.Abs("testdata/pr.json")
	if err != nil {
		t.Fatal(err)
	}
	inlineJSON, err := filepath.Abs("testdata/comments_inline.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PR_JSON", prJSON)
	t.Setenv("INLINE_JSON", inlineJSON)

	prs, err := PRsForBranchesCtx(context.Background(), directory, []string{"demo/pr-features"})
	if err != nil {
		t.Fatal(err)
	}
	pr := prs["demo/pr-features"]
	if pr == nil {
		t.Fatal("PR not found")
	}
	inlineCount := 0
	for _, comment := range pr.Comments {
		if comment.Kind == CommentInline {
			inlineCount++
		}
	}
	if inlineCount != 2 {
		t.Fatalf("inline comment count = %d, want 2", inlineCount)
	}
}
