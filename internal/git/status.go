package git

import "strings"

// IsDirty reports whether the working tree at dir has any uncommitted changes
// (staged, unstaged, or untracked). It uses `git status --porcelain -z` which
// produces NUL-delimited output: an empty result means a clean tree.
func IsDirty(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// RevParse runs `git rev-parse <rev>` and returns the resolved SHA (trimmed).
// For "HEAD" this is the 40-character commit hash of the current commit.
func RevParse(dir, rev string) (string, error) {
	return Run(dir, "rev-parse", rev)
}

// IsMergedInto reports whether branch has been merged into trunk in dir's repo
// — i.e. branch's tip is an ancestor of trunk. It runs
// `git merge-base --is-ancestor <branch> <trunk>`, which exits 0 when branch is
// an ancestor and 1 when it is not. A non-zero exit (including a missing ref) is
// reported as "not merged" rather than a hard error, so a locally-merged branch
// with no PR can still be flagged ready-to-clear without shelling out to gh.
// A branch identical to trunk (no divergence) is trivially an ancestor → true.
func IsMergedInto(dir, branch, trunk string) bool {
	if branch == "" || trunk == "" {
		return false
	}
	_, err := Run(dir, "merge-base", "--is-ancestor", "--end-of-options", branch, trunk)
	return err == nil
}

// IsAncestor reports whether commit `ancestor` is an ancestor of commit
// `descendant` in dir's repo — i.e. `descendant` contains `ancestor` in its
// history. It runs `git merge-base --is-ancestor <ancestor> <descendant>`,
// which exits 0 when true and non-zero otherwise. A missing ref or any other
// non-zero exit is reported as false (not an ancestor) rather than a hard
// error. Two identical commits are trivially ancestor-of each other → true.
func IsAncestor(dir, ancestor, descendant string) bool {
	if ancestor == "" || descendant == "" {
		return false
	}
	_, err := Run(dir, "merge-base", "--is-ancestor", "--end-of-options", ancestor, descendant)
	return err == nil
}

// CurrentBranch returns the short branch name for the worktree at dir, or ""
// if HEAD is detached. It uses `git symbolic-ref --quiet --short HEAD`:
// the command exits non-zero with empty output when HEAD is not a symbolic
// ref (i.e. detached), so any error is treated as "detached" rather than a
// hard failure.
func CurrentBranch(dir string) (string, error) {
	out, err := Run(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// symbolic-ref exits non-zero exactly when HEAD is detached;
		// return empty string with no error to signal detached state.
		return "", nil
	}
	return out, nil
}
