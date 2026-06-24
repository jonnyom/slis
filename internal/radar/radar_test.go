package radar_test

import (
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/radar"
)

// rd builds a RepoDiff for a repo with the given (optional) error and file paths.
func rd(repo, err string, paths ...string) diff.RepoDiff {
	d := diff.RepoDiff{Repo: repo, Err: err}
	for _, p := range paths {
		d.Files = append(d.Files, diff.FileStat{Path: p})
	}
	return d
}

func TestBuild_DetectsOverlap(t *testing.T) {
	idx := radar.Build(map[string][]diff.RepoDiff{
		"alpha": {rd("web", "", "src/app.go", "src/only-a.go")},
		"beta":  {rd("web", "", "src/app.go", "src/only-b.go")},
	})
	if len(idx.Overlaps) != 1 {
		t.Fatalf("want 1 overlap, got %d: %+v", len(idx.Overlaps), idx.Overlaps)
	}
	o := idx.Overlaps[0]
	if o.Repo != "web" || o.Path != "src/app.go" {
		t.Fatalf("unexpected overlap %+v", o)
	}
	if !reflect.DeepEqual(o.Slices, []string{"alpha", "beta"}) {
		t.Fatalf("want slices [alpha beta], got %v", o.Slices)
	}
	if !idx.HasConflict("alpha") || !idx.HasConflict("beta") {
		t.Fatal("both slices should report a conflict")
	}
	if got := idx.ConflictsFor("alpha"); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("alpha conflicts: want [beta], got %v", got)
	}
	if got := idx.OverlapsFor("beta"); len(got) != 1 || got[0].Path != "src/app.go" {
		t.Fatalf("OverlapsFor(beta) = %+v", got)
	}
}

func TestBuild_NoOverlap(t *testing.T) {
	idx := radar.Build(map[string][]diff.RepoDiff{
		"alpha": {rd("web", "", "a.go")},
		"beta":  {rd("web", "", "b.go")},
		"gamma": {rd("api", "", "a.go")}, // same path, different repo → not an overlap
	})
	if len(idx.Overlaps) != 0 {
		t.Fatalf("want 0 overlaps, got %+v", idx.Overlaps)
	}
	if idx.HasConflict("alpha") || idx.ConflictsFor("gamma") != nil {
		t.Fatal("no slice should report a conflict")
	}
}

func TestBuild_ThreeWay(t *testing.T) {
	idx := radar.Build(map[string][]diff.RepoDiff{
		"a": {rd("web", "", "shared.go")},
		"b": {rd("web", "", "shared.go")},
		"c": {rd("web", "", "shared.go")},
	})
	if len(idx.Overlaps) != 1 || len(idx.Overlaps[0].Slices) != 3 {
		t.Fatalf("want 1 overlap of 3 slices, got %+v", idx.Overlaps)
	}
	if got := idx.ConflictsFor("a"); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("a conflicts: want [b c], got %v", got)
	}
}

func TestBuild_SelfNotCounted(t *testing.T) {
	// A single slice listing the same path twice (e.g. via rename expansion)
	// must never conflict with itself.
	idx := radar.Build(map[string][]diff.RepoDiff{
		"solo": {rd("web", "", "x.go", "x.go")},
	})
	if len(idx.Overlaps) != 0 {
		t.Fatalf("a slice cannot conflict with itself, got %+v", idx.Overlaps)
	}
	if idx.HasConflict("solo") {
		t.Fatal("solo should not report a conflict")
	}
}

func TestBuild_IncompleteRepoExcludedAndMarked(t *testing.T) {
	idx := radar.Build(map[string][]diff.RepoDiff{
		"alpha": {rd("web", "boom: git failed", "would-overlap.go")}, // errored repo
		"beta":  {rd("web", "", "would-overlap.go")},
	})
	// alpha's web files are unknown → no false overlap, but flagged as a blind spot.
	if len(idx.Overlaps) != 0 {
		t.Fatalf("errored repo files must not be indexed, got %+v", idx.Overlaps)
	}
	if want := []string{"alpha/web"}; !reflect.DeepEqual(idx.Incomplete, want) {
		t.Fatalf("incomplete: want %v, got %v", want, idx.Incomplete)
	}
}

func TestBuild_NilStatsWholeSliceIncomplete(t *testing.T) {
	idx := radar.Build(map[string][]diff.RepoDiff{"empty": nil})
	if want := []string{"empty/(all)"}; !reflect.DeepEqual(idx.Incomplete, want) {
		t.Fatalf("want %v, got %v", want, idx.Incomplete)
	}
}

func TestBuild_RenameMatchesOldAndNew(t *testing.T) {
	// alpha renames foo.go → bar.go; beta edits foo.go (the old path). Collision.
	idx := radar.Build(map[string][]diff.RepoDiff{
		"alpha": {rd("web", "", "foo.go => bar.go")},
		"beta":  {rd("web", "", "foo.go")},
	})
	if len(idx.Overlaps) != 1 || idx.Overlaps[0].Path != "foo.go" {
		t.Fatalf("rename old-path collision not detected: %+v", idx.Overlaps)
	}
}

func TestBuild_BinaryCollision(t *testing.T) {
	bin := func(repo, path string) diff.RepoDiff {
		return diff.RepoDiff{Repo: repo, Files: []diff.FileStat{{Path: path, Added: -1, Deleted: -1}}}
	}
	idx := radar.Build(map[string][]diff.RepoDiff{
		"alpha": {bin("web", "logo.png")},
		"beta":  {bin("web", "logo.png")},
	})
	if len(idx.Overlaps) != 1 || idx.Overlaps[0].Path != "logo.png" {
		t.Fatalf("binary collision not detected: %+v", idx.Overlaps)
	}
}
