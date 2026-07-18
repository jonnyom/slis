package report

import (
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

func TestBuildDetailAddsGitTrunkForUntrackedGraphiteBranch(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, err := git.Run(repo, "switch", "-c", "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}

	detail := BuildDetail(SliceDTO{
		Name: "feature",
		Members: []MemberDTO{{
			Repo:         "web",
			Branch:       "feature",
			WorktreePath: repo,
		}},
	})

	if len(detail.Members) != 1 {
		t.Fatalf("members = %d, want 1", len(detail.Members))
	}
	stack := detail.Members[0].Stack
	if len(stack) != 2 {
		t.Fatalf("stack = %+v, want detected trunk + current branch", stack)
	}
	if stack[0].Name != "main" || !stack[0].Trunk || stack[0].Depth != 0 {
		t.Fatalf("first row = %+v, want main trunk at depth 0", stack[0])
	}
	if stack[1].Name != "feature" || stack[1].Trunk || stack[1].Depth != 1 {
		t.Fatalf("second row = %+v, want feature at depth 1", stack[1])
	}
}

func TestBuildDetailDoesNotDuplicateCurrentTrunk(t *testing.T) {
	repo := testutil.NewRepo(t)
	detail := BuildDetail(SliceDTO{
		Name: "main",
		Members: []MemberDTO{{
			Repo:         "web",
			Branch:       "main",
			WorktreePath: repo,
		}},
	})

	stack := detail.Members[0].Stack
	if len(stack) != 1 || stack[0].Name != "main" || !stack[0].Trunk {
		t.Fatalf("stack = %+v, want one main trunk row", stack)
	}
}
