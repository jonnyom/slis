package report

import (
	"context"
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// TestSetPRPopulatesCIRollup: SetPR copies identity/review fields and derives the
// CI rollup (state word + per-state counts) from the PR's checks.
func TestSetPRPopulatesCIRollup(t *testing.T) {
	pr := &forge.PR{
		Number:         8107,
		URL:            "https://github.com/acme/web/pull/8107",
		State:          "OPEN",
		Title:          "Checkout revamp",
		ReviewDecision: "APPROVED",
		Checks: []forge.Check{
			{Name: "build", State: forge.CheckPass},
			{Name: "lint", State: forge.CheckPass},
			{Name: "test", State: forge.CheckFail},
		},
		Comments: []forge.Comment{{
			Author: "reviewer", Body: "Fix this", Kind: forge.CommentInline,
			Path: "src/cart.ts", Line: 42, Side: "RIGHT", DiffHunk: "+broken()",
		}},
	}
	var row PRStackRowDTO
	row.SetPR(pr)

	if row.Number != 8107 || row.State != "OPEN" || row.ReviewDecision != "APPROVED" {
		t.Fatalf("identity fields not copied: %+v", row)
	}
	if row.CI != "fail" {
		t.Errorf("ci = %q, want fail (a failing check dominates the rollup)", row.CI)
	}
	if row.CIPass != 2 || row.CIFail != 1 || row.CIPending != 0 {
		t.Errorf("counts = pass %d fail %d pending %d, want 2/1/0", row.CIPass, row.CIFail, row.CIPending)
	}
	if len(row.Comments) != 1 || row.Comments[0].Path != "src/cart.ts" || row.Comments[0].Line != 42 {
		t.Errorf("comments not copied: %+v", row.Comments)
	}
}

// TestSetPRNilLeavesBareRow: a nil PR leaves the row as a repo/branch stub with
// no CI fields, so callers can invoke it unconditionally.
func TestSetPRNilLeavesBareRow(t *testing.T) {
	row := PRStackRowDTO{Repo: "web", Branch: "jonny/checkout"}
	row.SetPR(nil)
	if row.Number != 0 || row.CI != "" || row.CIFail != 0 {
		t.Errorf("nil PR should leave a bare row, got %+v", row)
	}
}

func TestBuildPRStackRowsIncludesFullGraphiteStack(t *testing.T) {
	sl := model.Slice{
		Members: map[string]model.SliceMember{
			"nory": {
				Repo:         "nory",
				Branch:       "stack-2",
				WorktreePath: "/worktree/nory",
			},
		},
	}
	state := gt.State{
		"main":    {Trunk: true},
		"stack-1": {Parents: []gt.Parent{{Ref: "main"}}},
		"stack-2": {Parents: []gt.Parent{{Ref: "stack-1"}}},
		"stack-3": {Parents: []gt.Parent{{Ref: "stack-2"}}},
		"other":   {Parents: []gt.Parent{{Ref: "main"}}},
	}
	prNumbers := map[string]int{"stack-1": 101, "stack-2": 102, "stack-3": 103}
	prLookupCalls := 0

	rows := buildPRStackRowsCtx(
		context.Background(),
		sl,
		func(context.Context, string) (gt.State, error) { return state, nil },
		func(_ context.Context, _ string, branches []string) (map[string]*forge.PR, error) {
			prLookupCalls++
			prs := make(map[string]*forge.PR, len(branches))
			for _, branch := range branches {
				prs[branch] = &forge.PR{Branch: branch, Number: prNumbers[branch], State: "OPEN"}
			}
			return prs, nil
		},
	)

	branches := make([]string, 0, len(rows))
	numbers := make([]int, 0, len(rows))
	for _, row := range rows {
		branches = append(branches, row.Branch)
		numbers = append(numbers, row.Number)
	}
	if !reflect.DeepEqual(branches, []string{"stack-1", "stack-2", "stack-3"}) {
		t.Errorf("branches = %v, want the complete stack", branches)
	}
	if !reflect.DeepEqual(numbers, []int{101, 102, 103}) {
		t.Errorf("PR numbers = %v, want every stack PR", numbers)
	}
	if prLookupCalls != 1 {
		t.Errorf("PR lookup calls = %d, want one batched lookup per repo", prLookupCalls)
	}
}
