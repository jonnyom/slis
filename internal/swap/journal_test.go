package swap

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")

	want := &Journal{
		Slice: "feature/my-slice",
		Repos: []RepoState{
			{
				Repo:        "api",
				Primary:     "/home/user/checkouts/api",
				PriorBranch: "main",
				PriorSHA:    "abc123def456",
				StashRef:    "stash-sha-xyz",
				TargetSHA:   "deadbeef1234",
				Reconciled:  true,
			},
			{
				Repo:        "web",
				Primary:     "/home/user/checkouts/web",
				PriorBranch: "",
				PriorSHA:    "111aaa222bbb",
				StashRef:    "",
				TargetSHA:   "cafebabe5678",
				Reconciled:  false,
			},
		},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestLoadMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "none.json")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load of missing file should return nil error, got: %v", err)
	}
	if got != nil {
		t.Errorf("Load of missing file should return nil journal, got: %+v", got)
	}
}

func TestClearRemoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")

	j := &Journal{
		Slice: "test-slice",
		Repos: []RepoState{},
	}

	if err := Save(path, j); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Clear(path); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Clear should return nil error, got: %v", err)
	}
	if got != nil {
		t.Errorf("Load after Clear should return nil journal, got: %+v", got)
	}

	// Clear on missing file should be idempotent (no error)
	if err := Clear(path); err != nil {
		t.Fatalf("Clear of already-missing file should return nil, got: %v", err)
	}
}
