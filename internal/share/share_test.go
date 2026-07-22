package share

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/report"
)

func TestMarkdownIncludesEveryRepoStackPRAndPatch(t *testing.T) {
	slice := model.Slice{Name: "unpaid-leave", Members: map[string]model.SliceMember{
		"nory": {Repo: "nory", Branch: "feature-b", WorktreePath: "/work/nory"},
		"web":  {Repo: "web", Branch: "web-feature", WorktreePath: "/work/web"},
	}}
	readStack := func(path string) (gt.State, error) {
		if path == "/work/nory" {
			return gt.State{
				"main":      {Trunk: true},
				"feature-a": {Parents: []gt.Parent{{Ref: "main"}}},
				"feature-b": {Parents: []gt.Parent{{Ref: "feature-a"}}},
			}, nil
		}
		return nil, nil
	}
	prForBranch := func(_ string, branch string) (*forge.PR, error) {
		return &forge.PR{Branch: branch, Number: len(branch), Title: "PR " + branch, URL: "https://example.com/" + branch, State: "OPEN"}, nil
	}
	branchDiff := func(_, repo, branch, _ string) (report.BranchDiffResult, error) {
		added := len(branch)
		deleted := len(repo)
		return report.BranchDiffResult{Repo: repo, Branch: branch, Stat: &report.DiffStatDTO{Added: added, Deleted: deleted}}, nil
	}

	markdown, err := Markdown(slice, readStack, prForBranch, branchDiff)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"**nory**", "PR feature-a", "PR feature-b", "**web**", "PR web-feature", "https://example.com/feature-a", "`+9`", "`-4`"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if strings.Index(markdown, "feature-a") > strings.Index(markdown, "feature-b") {
		t.Fatalf("stack is not trunk-first:\n%s", markdown)
	}
	if strings.Contains(markdown, "```diff") || strings.Contains(markdown, "No pull request found") {
		t.Fatalf("markdown contains verbose patch output:\n%s", markdown)
	}
}

func TestMarkdownFailsWhenADiffCannotBeProduced(t *testing.T) {
	slice := model.Slice{Name: "slice", Members: map[string]model.SliceMember{
		"web": {Repo: "web", Branch: "feature", WorktreePath: "/work/web"},
	}}
	_, err := Markdown(
		slice,
		func(string) (gt.State, error) { return nil, nil },
		func(_, branch string) (*forge.PR, error) { return &forge.PR{Branch: branch}, nil },
		func(string, string, string, string) (report.BranchDiffResult, error) {
			return report.BranchDiffResult{Err: "missing ref"}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "missing ref") {
		t.Fatalf("err = %v", err)
	}
}

func TestMarkdownOmitsBranchesWithoutPullRequests(t *testing.T) {
	slice := model.Slice{Name: "slice", Members: map[string]model.SliceMember{
		"web": {Repo: "web", Branch: "feature", WorktreePath: "/work/web"},
	}}
	markdown, err := Markdown(
		slice,
		func(string) (gt.State, error) { return nil, nil },
		func(string, string) (*forge.PR, error) { return nil, nil },
		func(string, string, string, string) (report.BranchDiffResult, error) {
			t.Fatal("diff should not run without a PR")
			return report.BranchDiffResult{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "" {
		t.Fatalf("markdown = %q", markdown)
	}
}
