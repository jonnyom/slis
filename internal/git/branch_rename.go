package git

import (
	"context"
	"fmt"
	"strings"
)

// BranchRenamedFrom reports whether newBranch was produced by renaming oldBranch
// in place (`git branch -m`). git records that in the new branch's reflog as
//
//	Branch: renamed refs/heads/<old> to refs/heads/<new>
//
// which is the only positive proof available once the old ref is gone. Creating a
// branch, or checking a different one out into the same worktree, leaves no such
// entry — so callers can tell "the user renamed their branch" apart from "this
// worktree now holds somebody else's work".
//
// A missing ref, an expired reflog or any git failure reports false: unprovable
// is treated as not-a-rename.
func BranchRenamedFrom(dir, newBranch, oldBranch string) bool {
	return BranchRenamedFromCtx(context.Background(), dir, newBranch, oldBranch)
}

// BranchRenamedFromCtx is BranchRenamedFrom with a caller-supplied context.
func BranchRenamedFromCtx(ctx context.Context, dir, newBranch, oldBranch string) bool {
	if dir == "" || newBranch == "" || oldBranch == "" || newBranch == oldBranch {
		return false
	}
	out, err := RunCtx(ctx, dir, "reflog", "show", "--format=%gs", "--end-of-options", "refs/heads/"+newBranch)
	if err != nil {
		return false
	}
	want := fmt.Sprintf("Branch: renamed refs/heads/%s to refs/heads/%s", oldBranch, newBranch)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}
