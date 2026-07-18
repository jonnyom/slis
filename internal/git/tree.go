package git

import (
	"sort"
	"strconv"
	"strings"
)

// TreeEntry is one entry in a git tree listing (`git ls-tree`). Name is the
// leaf name relative to the listed directory (not the full path), so the caller
// reconstructs a child's path as parent+"/"+Name for lazy expansion. Size is the
// blob byte size; it is -1 for trees and submodules (commits), which have none.
type TreeEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "blob", "tree", or "commit" (submodule)
	Size int64  `json:"size"`
}

// LsTree lists a single directory level of a branch's tree at the given path,
// via `git ls-tree -l`. An empty path lists the tree root. Names come back
// relative to the listed directory (basenames), so the caller expands lazily
// one level per call. Entries are sorted trees-first then by name. dir MUST be
// the repo's primary checkout — this is a pure, ref-scoped read that never
// touches a working tree.
func LsTree(dir, rev, path string) ([]TreeEntry, error) {
	treeish := rev
	if path != "" {
		treeish = rev + ":" + path
	}
	out, err := Run(dir, "ls-tree", "-l", "-z", treeish)
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(rec[:tab])
		if len(meta) < 4 {
			continue
		}
		e := TreeEntry{Name: rec[tab+1:], Type: meta[1], Size: -1}
		if meta[1] == "blob" {
			if n, perr := strconv.ParseInt(meta[3], 10, 64); perr == nil {
				e.Size = n
			}
		}
		entries = append(entries, e)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		ti, tj := entries[i].Type == "tree", entries[j].Type == "tree"
		if ti != tj {
			return ti
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
