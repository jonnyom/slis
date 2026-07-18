package git

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Worktree represents a single git worktree as parsed from
// `git worktree list --porcelain -z` output.
type Worktree struct {
	Path    string
	HeadSHA string
	Branch  string

	Detached bool
	Bare     bool
	Locked   bool
	Prunable bool
}

// ParseWorktreeList parses the raw output of `git worktree list --porcelain -z`.
//
// The --porcelain -z format uses NUL terminators: each attribute line is
// terminated by a single \0, and each worktree record is terminated by an
// extra \0 (records are therefore separated by \0\0).
func ParseWorktreeList(data []byte) []Worktree {
	// Trim any trailing NULs so that a final \0\0 from git does not produce a
	// phantom empty record.
	data = bytes.TrimRight(data, "\x00")

	records := bytes.Split(data, []byte("\x00\x00"))

	var result []Worktree
	for _, rec := range records {
		if len(bytes.TrimSpace(rec)) == 0 {
			continue
		}

		attrs := bytes.Split(rec, []byte("\x00"))

		var wt Worktree
		hasPath := false

		for _, attr := range attrs {
			s := string(attr)
			if s == "" {
				continue
			}

			// Split on the first space into key + optional value.
			key, value, _ := strings.Cut(s, " ")

			switch key {
			case "worktree":
				wt.Path = value
				hasPath = true
			case "HEAD":
				wt.HeadSHA = value
			case "branch":
				wt.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "detached":
				wt.Detached = true
			case "bare":
				wt.Bare = true
			case "locked":
				wt.Locked = true
			case "prunable":
				wt.Prunable = true
			}
		}

		// Skip records that had no `worktree` attribute (e.g. empty trailing records).
		if !hasPath {
			continue
		}

		result = append(result, wt)
	}

	return result
}

// ListWorktrees runs `git worktree list --porcelain -z` in dir and returns
// the parsed list of worktrees.
func ListWorktrees(dir string) ([]Worktree, error) {
	out, err := Run(dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktreeList([]byte(out)), nil
}

// ForgetMissingWorktree removes the administrative record for one exact
// worktree whose checkout directory is already gone. Unlike repo-wide
// `git worktree prune`, it cannot affect another missing worktree (for example
// one on a temporarily unavailable external volume).
func ForgetMissingWorktree(dir, worktreePath string) error {
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("%s still exists; refusing to forget a live worktree", worktreePath)
		}
		return fmt.Errorf("inspect worktree %s: %w", worktreePath, err)
	}
	_, err := Run(dir, "worktree", "remove", "--force", "--", worktreePath)
	return err
}

func sameWorktreePath(a, b string) bool {
	resolve := func(path string) string {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			return real
		}
		clean := filepath.Clean(path)
		cur := clean
		var suffix []string
		for {
			if real, err := filepath.EvalSymlinks(cur); err == nil {
				return filepath.Join(append([]string{real}, suffix...)...)
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				return clean
			}
			suffix = append([]string{filepath.Base(cur)}, suffix...)
			cur = parent
		}
	}
	return resolve(a) == resolve(b)
}

// RemoveWorktree runs `git worktree remove [--force] <worktreePath>` against the
// primary repo at dir. Without force, git refuses when the worktree has
// uncommitted changes, untracked files, or is locked — the safe default.
func RemoveWorktree(dir, worktreePath string, force bool) error {
	wts, err := ListWorktrees(dir)
	if err != nil {
		return err
	}
	var tracked *Worktree
	for i := range wts {
		if sameWorktreePath(wts[i].Path, worktreePath) {
			tracked = &wts[i]
			break
		}
	}
	if tracked == nil {
		if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
			return nil // already fully removed: cleanup is intentionally idempotent
		}
		return fmt.Errorf("%s exists but git does not track it as a worktree; refusing to delete it", worktreePath)
	}
	if _, statErr := os.Stat(worktreePath); tracked.Prunable || os.IsNotExist(statErr) {
		if err := ForgetMissingWorktree(dir, worktreePath); err != nil {
			return fmt.Errorf("forget stale worktree metadata: %w", err)
		}
		remaining, err := ListWorktrees(dir)
		if err != nil {
			return err
		}
		for _, wt := range remaining {
			if sameWorktreePath(wt.Path, worktreePath) {
				return fmt.Errorf("stale worktree metadata for %s is locked or could not be pruned", worktreePath)
			}
		}
		return nil
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
	_, err = Run(dir, args...)
	return err
}

// DeleteBranch runs `git branch -d|-D <branch>` against the repo at dir. With
// force=false (-d) git refuses to delete a branch not fully merged into its
// upstream/HEAD; force=true (-D) deletes regardless. The branch must not be
// checked out in any worktree (remove the worktree first).
func DeleteBranch(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := Run(dir, "branch", flag, branch)
	return err
}
