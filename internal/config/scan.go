package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Candidate is a git repository discovered under a project root.
type Candidate struct {
	Name          string
	Path          string
	DefaultBranch string
}

// ScanRepos walks the immediate children of root, returning one Candidate
// for each subdirectory that contains a .git entry (directory or file).
// Results are sorted by Name. The function does not recurse into nested
// directories — only top-level children of root are inspected.
func ScanRepos(root string) ([]Candidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var candidates []Candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			// .git does not exist — not a git repo
			continue
		}
		candidates = append(candidates, Candidate{
			Name:          e.Name(),
			Path:          dir,
			DefaultBranch: detectDefaultBranch(dir),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	return candidates, nil
}

// detectDefaultBranch attempts to determine the default branch for the repo
// at dir using the following priority order:
//  1. git symbolic-ref --quiet --short refs/remotes/origin/HEAD  (strip "origin/" prefix)
//  2. git symbolic-ref --quiet --short HEAD  (current branch)
//  3. falls back to "main"
//
// Any error in git invocations is silently ignored — the function always
// returns a non-empty string.
func detectDefaultBranch(dir string) string {
	// Try origin/HEAD first (set when remote is configured)
	if out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" {
			// Strip "origin/" prefix if present
			branch = strings.TrimPrefix(branch, "origin/")
			if branch != "" {
				return branch
			}
		}
	}

	// Fall back to current HEAD branch
	if out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--quiet", "--short", "HEAD").Output(); err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" {
			return branch
		}
	}

	return "main"
}

// BuildWorkspace assembles a Workspace from a project root, the set of
// selected Candidates, and a stripPrefix string. The returned Workspace has
// Root and Repos populated from the arguments, plus sensible defaults for
// Grouping (strategy "branch-name", strip_prefix set to stripPrefix) and
// Processes (cpu_warn_pct 150) so the persisted file is clean and complete.
func BuildWorkspace(root string, selected []Candidate, stripPrefix string) Workspace {
	repos := make(map[string]Repo, len(selected))
	for _, c := range selected {
		repos[c.Name] = Repo{
			Primary:       c.Path,
			DefaultBranch: c.DefaultBranch,
		}
	}
	return Workspace{
		Root:  root,
		Repos: repos,
		Grouping: Grouping{
			Strategy:    "branch-name",
			StripPrefix: stripPrefix,
		},
		Processes: Processes{
			CPUWarnPct: 150,
		},
	}
}
