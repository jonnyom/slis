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
