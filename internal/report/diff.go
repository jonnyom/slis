package report

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

func DiffFingerprint(sl model.Slice, scope string) (string, error) {
	bases := scopeBases(sl, scope)
	fingerprint := sha256.New()
	writePart := func(value []byte) {
		_, _ = fmt.Fprintf(fingerprint, "%d:", len(value))
		_, _ = fingerprint.Write(value)
	}

	for _, repo := range sl.Repos() {
		member := sl.Members[repo]
		base := bases[repo]
		baseSHA, err := git.RevParse(member.WorktreePath, base)
		if err != nil {
			return "", err
		}
		trackedPaths, err := git.RunRaw(
			member.WorktreePath,
			"diff",
			"--name-only",
			"-z",
			"--merge-base",
			"--end-of-options",
			base,
		)
		if err != nil {
			return "", err
		}
		untrackedPaths, err := git.RunRaw(
			member.WorktreePath,
			"ls-files",
			"--others",
			"--exclude-standard",
			"-z",
		)
		if err != nil {
			return "", err
		}

		writePart([]byte(repo))
		writePart([]byte(member.Branch))
		writePart([]byte(base))
		writePart([]byte(baseSHA))
		writePart(trackedPaths)
		writePart(untrackedPaths)

		paths := append(append([]byte(nil), trackedPaths...), untrackedPaths...)
		for _, relativePath := range bytes.Split(paths, []byte{0}) {
			if len(relativePath) == 0 {
				continue
			}
			if err := writeDiffPathState(fingerprint, member.WorktreePath, string(relativePath)); err != nil {
				return "", err
			}
		}
	}

	return fmt.Sprintf("%x", fingerprint.Sum(nil)), nil
}

func writeDiffPathState(destination io.Writer, worktree, relativePath string) error {
	path := filepath.Join(worktree, relativePath)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		_, err = io.WriteString(destination, "missing\x00")
		return err
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(destination, "%s\x00", info.Mode()); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		_, err = io.WriteString(destination, target)
		return err
	}
	if info.IsDir() {
		sha, err := git.RevParse(path, "HEAD")
		if err != nil {
			return err
		}
		_, err = io.WriteString(destination, sha)
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = destination.Write(data)
	return err
}

// DiffFileStat is a JSON-friendly per-file line-change summary. Added/Deleted
// are -1 for binary files (git reports no line counts for them).
type DiffFileStat struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// DiffStatDTO is the numstat rollup for one repo: the per-file changes plus the
// summed additions and deletions across non-binary files.
type DiffStatDTO struct {
	Files   []DiffFileStat `json:"files"`
	Added   int            `json:"added"`
	Deleted int            `json:"deleted"`
}

// DiffRepoResult is one repo's diff within a slice. Stat is present unless the
// caller asked for patch-only; Patch is present unless the caller asked for
// stat-only. Err is set (and the others empty) when this repo's diff failed.
type DiffRepoResult struct {
	Repo   string       `json:"repo"`
	Branch string       `json:"branch"`
	Stat   *DiffStatDTO `json:"stat,omitempty"`
	Patch  *string      `json:"patch,omitempty"`
	Err    string       `json:"err,omitempty"`
}

// DiffResult is the whole-slice diff: one entry per member repo.
type DiffResult struct {
	Repos []DiffRepoResult `json:"repos"`
}

// format selects what each repo entry carries: "stat" (numstat only, cheapest),
// "patch" (the unified diff text only), or "both".
func SliceDiffScoped(sl model.Slice, scope, format string) (DiffResult, error) {
	statOnly := format == "stat"

	var repoDiffs []diff.RepoDiff
	var err error
	switch scope {
	case "working":
		bases := scopeBases(sl, "parent")
		if statOnly {
			repoDiffs, err = diff.SliceStatBases(sl, bases)
		} else {
			repoDiffs, err = diff.SliceDiffBases(sl, bases)
		}
	default:
		bases := scopeBases(sl, scope)
		if statOnly {
			repoDiffs, err = diff.SliceStatBases(sl, bases)
		} else {
			repoDiffs, err = diff.SliceDiffBases(sl, bases)
		}
	}
	if err != nil {
		return DiffResult{}, err
	}

	branchByRepo := make(map[string]string, len(sl.Members))
	for repo, mem := range sl.Members {
		branchByRepo[repo] = mem.Branch
	}

	repos := make([]DiffRepoResult, 0, len(repoDiffs))
	for _, rd := range repoDiffs {
		entry := DiffRepoResult{Repo: rd.Repo, Branch: branchByRepo[rd.Repo], Err: rd.Err}
		if rd.Err == "" {
			if format != "patch" {
				entry.Stat = statDTO(rd)
			}
			if format != "stat" {
				patch := rd.Patch
				entry.Patch = &patch
			}
		}
		repos = append(repos, entry)
	}
	return DiffResult{Repos: repos}, nil
}

// BranchDiffResult is one branch's diff against its stack parent, marshal-ready.
// It mirrors DiffRepoResult (stat/patch/err by format) and adds Parent — the ref
// the branch was diffed against — so a caller can title the view `repo › branch
// › vs <parent>`.
type BranchDiffResult struct {
	Repo   string       `json:"repo"`
	Branch string       `json:"branch"`
	Parent string       `json:"parent"`
	Stat   *DiffStatDTO `json:"stat,omitempty"`
	Patch  *string      `json:"patch,omitempty"`
	Err    string       `json:"err,omitempty"`
}

// BranchDiff computes one branch's committed diff against its Graphite stack
// parent, falling back to the repo trunk when the branch has no recorded parent
// (or the parent ref is missing). repoDir MUST be the repo's primary checkout.
// format selects the payload: "stat" (numstat only), "patch" (patch only), or
// "both".
func BranchDiff(repoDir, repo, branch, format string) (BranchDiffResult, error) {
	parent := gtParent(repoDir, branch)
	if parent == "" || !git.RefExists(repoDir, parent) {
		parent = git.DetectBase(repoDir)
	}

	rd, err := diff.BranchAgainstParent(repoDir, parent, branch, format != "stat")
	if err != nil {
		return BranchDiffResult{}, err
	}

	res := BranchDiffResult{Repo: repo, Branch: branch, Parent: parent, Err: rd.Err}
	if rd.Err == "" {
		if format != "patch" {
			res.Stat = statDTO(rd)
		}
		if format != "stat" {
			patch := rd.Patch
			res.Patch = &patch
		}
	}
	return res, nil
}

// statDTO converts a diff.RepoDiff's numstat into a DiffStatDTO.
func statDTO(rd diff.RepoDiff) *DiffStatDTO {
	files := make([]DiffFileStat, 0, len(rd.Files))
	for _, f := range rd.Files {
		files = append(files, DiffFileStat{Path: f.Path, Added: f.Added, Deleted: f.Deleted})
	}
	return &DiffStatDTO{Files: files, Added: rd.TotalAdded(), Deleted: rd.TotalDeleted()}
}

// gtParent returns the Graphite parent branch of branch in dir's repo, or "".
func gtParent(dir, branch string) string {
	st, err := gt.ReadStack(dir)
	if err != nil {
		return ""
	}
	bs, ok := st[branch]
	if !ok || len(bs.Parents) == 0 {
		return ""
	}
	parent := bs.Parents[0].Ref
	trunk := git.DetectBase(dir)
	if sameBranchRef(parent, trunk) {
		return trunk
	}
	return parent
}

func sameBranchRef(left, right string) bool {
	normalize := func(ref string) string {
		ref = strings.TrimPrefix(ref, "refs/heads/")
		return strings.TrimPrefix(ref, "origin/")
	}
	return normalize(left) == normalize(right)
}

// scopeBases computes the per-repo diff base for a non-working scope, mirroring
// the cockpit diff pane: an explicit slice Base override wins; "parent" uses the
// branch's Graphite parent (falling back to the detected trunk when the branch
// is not stacked); "trunk" (and anything else) uses the detected trunk.
func scopeBases(sl model.Slice, scope string) map[string]string {
	bases := make(map[string]string, len(sl.Members))
	for repo, mem := range sl.Members {
		switch {
		case sl.Base != "":
			bases[repo] = sl.Base
		case scope == "parent":
			if p := gtParent(mem.WorktreePath, mem.Branch); p != "" {
				bases[repo] = p
			} else {
				bases[repo] = git.DetectBase(mem.WorktreePath)
			}
		default:
			bases[repo] = git.DetectBase(mem.WorktreePath)
		}
	}
	return bases
}
