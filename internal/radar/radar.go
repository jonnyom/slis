// Package radar is the cross-slice conflict radar: it detects files changed by
// more than one in-flight slice in the same repo, before they collide at merge
// time. It is read-only and the core (Build) is pure — it operates over per-slice
// diff stats already computed elsewhere (the browser cards, or freshly for the
// CLI twin).
//
// Honest framing: this reports FILE OVERLAP — two slices touching the same file.
// That is a high-signal warning, not a proof of a git merge conflict (the edits
// may be in different hunks). The radar informs; it never blocks.
//
//	stats per slice ──▶ Build ──▶ Index{ Overlaps, Incomplete }
//	  (repo→files)                  │
//	                                ├─ HasConflict(slice) / ConflictsFor(slice)
//	                                └─ OverlapsFor(slice)  (for the merge warning)
package radar

import (
	"sort"
	"strings"
	"sync"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// Overlap is a single (repo, path) changed by more than one slice.
type Overlap struct {
	Repo   string   `json:"repo"`
	Path   string   `json:"path"`
	Slices []string `json:"slices"` // sorted, len >= 2
}

// Index is the computed radar state over a set of slices.
type Index struct {
	// Overlaps are the conflicting (repo, path) groups, sorted by repo then path.
	Overlaps []Overlap `json:"overlaps"`
	// Incomplete lists "slice/repo" scopes whose diff could not be computed —
	// blind spots where absence of an overlap does NOT prove the scope is clear.
	Incomplete []string `json:"incomplete"`

	// bySlice maps a slice to the set of other slices it overlaps with.
	bySlice map[string]map[string]struct{}
}

// Build computes the conflict index from per-slice diff stats keyed by slice
// name. The no-silent-failure guard: a RepoDiff with a non-empty Err marks that
// slice/repo "incomplete" (its files are unknown), and a nil/empty stat list
// marks the whole slice incomplete — the radar never reports a scope it could not
// read as conflict-free.
func Build(statsBySlice map[string][]diff.RepoDiff) *Index {
	// index[repo][path] -> set of slice names touching it.
	index := make(map[string]map[string]map[string]struct{})
	var incomplete []string

	// Deterministic iteration so Incomplete (and any tie-breaks) are stable.
	names := make([]string, 0, len(statsBySlice))
	for name := range statsBySlice {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, slice := range names {
		stats := statsBySlice[slice]
		if len(stats) == 0 {
			incomplete = append(incomplete, slice+"/(all)")
			continue
		}
		for _, rd := range stats {
			if rd.Err != "" {
				incomplete = append(incomplete, slice+"/"+rd.Repo)
				continue
			}
			for _, f := range rd.Files {
				for _, p := range diff.ExpandPaths(f.Path) {
					if index[rd.Repo] == nil {
						index[rd.Repo] = make(map[string]map[string]struct{})
					}
					if index[rd.Repo][p] == nil {
						index[rd.Repo][p] = make(map[string]struct{})
					}
					index[rd.Repo][p][slice] = struct{}{}
				}
			}
		}
	}

	idx := &Index{bySlice: make(map[string]map[string]struct{})}

	for repo, paths := range index {
		for path, sliceSet := range paths {
			if len(sliceSet) < 2 {
				continue
			}
			slices := setToSorted(sliceSet)
			idx.Overlaps = append(idx.Overlaps, Overlap{Repo: repo, Path: path, Slices: slices})
			for _, a := range slices {
				if idx.bySlice[a] == nil {
					idx.bySlice[a] = make(map[string]struct{})
				}
				for _, b := range slices {
					if a != b {
						idx.bySlice[a][b] = struct{}{}
					}
				}
			}
		}
	}

	sort.Slice(idx.Overlaps, func(i, j int) bool {
		if idx.Overlaps[i].Repo != idx.Overlaps[j].Repo {
			return idx.Overlaps[i].Repo < idx.Overlaps[j].Repo
		}
		return idx.Overlaps[i].Path < idx.Overlaps[j].Path
	})
	sort.Strings(incomplete)
	idx.Incomplete = incomplete
	return idx
}

// HasConflict reports whether slice overlaps any other slice.
func (idx *Index) HasConflict(slice string) bool {
	return len(idx.bySlice[slice]) > 0
}

// ConflictsFor returns the sorted names of other slices that overlap slice.
func (idx *Index) ConflictsFor(slice string) []string {
	return setToSorted(idx.bySlice[slice])
}

// OverlapsFor returns the overlap groups that involve slice, in Index order.
func (idx *Index) OverlapsFor(slice string) []Overlap {
	var out []Overlap
	for _, o := range idx.Overlaps {
		for _, s := range o.Slices {
			if s == slice {
				out = append(out, o)
				break
			}
		}
	}
	return out
}

// ParentBases returns the per-repo diff base for a slice: each member branch's
// Graphite parent, so the diff measures only that branch's own changes (not the
// whole downstack), falling back to "" (auto-detect trunk) when gt has no data.
// Mirrors the browser card's base computation (slicelist.go) so the radar
// measures the SAME file set the UI shows.
func ParentBases(sl model.Slice) map[string]string {
	bases := make(map[string]string, len(sl.Members))
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		if m.WorktreePath == "" {
			continue
		}
		st, err := gt.ReadStack(m.WorktreePath)
		if err != nil || len(st) == 0 {
			continue
		}
		if bs, ok := st[m.Branch]; ok && len(bs.Parents) > 0 {
			bases[repo] = strings.TrimSpace(bs.Parents[0].Ref)
		}
	}
	return bases
}

// CollectStats computes per-slice changed-file stats (numstat only)
// concurrently, using each slice's Graphite-parent bases. Result is keyed by
// slice name and ready for Build. Used by the CLI twin, which has no TUI card
// cache to reuse.
func CollectStats(slices []model.Slice) map[string][]diff.RepoDiff {
	out := make(map[string][]diff.RepoDiff, len(slices))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, sl := range slices {
		wg.Add(1)
		go func(sl model.Slice) {
			defer wg.Done()
			stats, _ := diff.SliceStatBases(sl, ParentBases(sl))
			mu.Lock()
			out[sl.Name] = stats
			mu.Unlock()
		}(sl)
	}
	wg.Wait()
	return out
}

// setToSorted returns the keys of set as a sorted slice (nil for an empty set).
func setToSorted(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
