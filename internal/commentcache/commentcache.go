// Package commentcache persists the PR comments slis has seen per slice, so they
// survive a slice being cleared (its branch/worktree gone means `gh` can no
// longer re-fetch them). It is a plain JSON file in the state dir, updated each
// time comments are fetched and read by the TUI overlay and `slis comments`.
package commentcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Comment is one cached PR comment.
type Comment struct {
	Author string `json:"author"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// RepoComments holds one repo's PR comments within a slice.
type RepoComments struct {
	PR       int       `json:"pr"`
	URL      string    `json:"url"`
	Comments []Comment `json:"comments"`
}

// Store maps slice name → repo name → that repo's cached PR comments.
type Store map[string]map[string]RepoComments

// Load reads the cache at path. A missing file yields an empty (non-nil) Store.
func Load(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("commentcache: read %q: %w", path, err)
	}
	s := Store{}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("commentcache: parse %q: %w", path, err)
	}
	if s == nil {
		s = Store{}
	}
	return s, nil
}

// Save writes the cache to path (creating parent dirs).
func (s Store) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("commentcache: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("commentcache: marshal: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("commentcache: write %q: %w", path, err)
	}
	return nil
}

// Put records a repo's comments for a slice. To avoid clobbering cached comments
// when a later fetch can't reach the PR (e.g. the branch was deleted), it is a
// no-op when cs is empty — call it only with freshly-fetched, non-empty results.
func (s Store) Put(slice, repo string, pr int, url string, cs []Comment) {
	if len(cs) == 0 {
		return
	}
	if s[slice] == nil {
		s[slice] = map[string]RepoComments{}
	}
	s[slice][repo] = RepoComments{PR: pr, URL: url, Comments: cs}
}

// Slices returns the cached slice names, sorted.
func (s Store) Slices() []string {
	names := make([]string, 0, len(s))
	for k := range s {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
