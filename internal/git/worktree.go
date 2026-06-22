package git

import (
	"bytes"
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
