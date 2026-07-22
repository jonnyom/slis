package review

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := Open(filepath.Join(t.TempDir(), "reviews.json"))
	// Deterministic ids/timestamps: sequential ids and a fixed clock.
	n := 0
	s.newID = func() string {
		n++
		return "id" + string(rune('0'+n))
	}
	s.now = func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
	return s
}

func TestAddGeneratesIDAndTimestamp(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Add(Comment{Slice: "checkout", Repo: "web", File: "a.go", Line: 10, Body: "fix"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got.ID == "" {
		t.Error("Add did not assign an ID")
	}
	if got.CreatedAt.IsZero() {
		t.Error("Add did not stamp CreatedAt")
	}
}

func TestAddPreservesExplicitIDAndTime(t *testing.T) {
	s := newTestStore(t)
	ts := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := s.Add(Comment{ID: "explicit", CreatedAt: ts, Slice: "x", Repo: "web", File: "a.go", Line: 1, Body: "b"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got.ID != "explicit" || !got.CreatedAt.Equal(ts) {
		t.Errorf("Add overrode explicit fields: %+v", got)
	}
}

func TestAddRejectsConflictingExplicitID(t *testing.T) {
	s := newTestStore(t)
	first := Comment{ID: "stable", Slice: "x", Repo: "web", File: "a.go", Line: 1, Body: "first"}
	if _, err := s.Add(first); err != nil {
		t.Fatal(err)
	}
	first.Body = "different"
	if _, err := s.Add(first); !errors.Is(err, errCommentIDConflict) {
		t.Fatalf("Add conflicting ID = %v, want errCommentIDConflict", err)
	}
}

func TestListFiltersBySliceAndOrders(t *testing.T) {
	s := newTestStore(t)
	// Insert out of order; expect (repo, file, line) ordering on read.
	mustAdd(t, s, Comment{Slice: "checkout", Repo: "web", File: "b.go", Line: 5, Body: "1"})
	mustAdd(t, s, Comment{Slice: "checkout", Repo: "api", File: "z.go", Line: 2, Body: "2"})
	mustAdd(t, s, Comment{Slice: "checkout", Repo: "web", File: "b.go", Line: 1, Body: "3"})
	mustAdd(t, s, Comment{Slice: "other", Repo: "web", File: "a.go", Line: 1, Body: "4"})

	got, err := s.List("checkout")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List returned %d, want 3 (filtered by slice)", len(got))
	}
	wantOrder := []struct {
		repo string
		file string
		line int
	}{
		{"api", "z.go", 2},
		{"web", "b.go", 1},
		{"web", "b.go", 5},
	}
	for i, w := range wantOrder {
		if got[i].Repo != w.repo || got[i].File != w.file || got[i].Line != w.line {
			t.Errorf("List[%d] = %s %s:%d, want %s %s:%d", i, got[i].Repo, got[i].File, got[i].Line, w.repo, w.file, w.line)
		}
	}
}

func TestListEmptyWhenNoFile(t *testing.T) {
	s := newTestStore(t)
	got, err := s.List("nope")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %v, want empty", got)
	}
}

func TestRemove(t *testing.T) {
	s := newTestStore(t)
	c := mustAdd(t, s, Comment{Slice: "x", Repo: "web", File: "a.go", Line: 1, Body: "b"})
	mustAdd(t, s, Comment{Slice: "x", Repo: "web", File: "a.go", Line: 2, Body: "c"})

	if err := s.Remove(c.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, _ := s.List("x")
	if len(got) != 1 {
		t.Fatalf("after Remove List = %d, want 1", len(got))
	}
	if err := s.Remove("does-not-exist"); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("Remove(missing) = %v, want ErrCommentNotFound", err)
	}
}

func TestClear(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Comment{Slice: "x", Repo: "web", File: "a.go", Line: 1, Body: "b"})
	mustAdd(t, s, Comment{Slice: "x", Repo: "api", File: "a.go", Line: 1, Body: "c"})
	mustAdd(t, s, Comment{Slice: "y", Repo: "web", File: "a.go", Line: 1, Body: "d"})

	if err := s.Clear("x"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got, _ := s.List("x"); len(got) != 0 {
		t.Errorf("after Clear List(x) = %d, want 0", len(got))
	}
	if got, _ := s.List("y"); len(got) != 1 {
		t.Errorf("Clear(x) touched slice y: List(y) = %d, want 1", len(got))
	}
	// Clearing an already-empty slice is a no-op.
	if err := s.Clear("x"); err != nil {
		t.Errorf("Clear(empty) = %v, want nil", err)
	}
}

func TestListAllAcrossSlices(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Comment{Slice: "b", Repo: "web", File: "a.go", Line: 1, Body: "1"})
	mustAdd(t, s, Comment{Slice: "a", Repo: "web", File: "a.go", Line: 1, Body: "2"})

	got, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAll = %d, want 2", len(got))
	}
	if got[0].Slice != "a" || got[1].Slice != "b" {
		t.Errorf("ListAll order = %s,%s, want a,b", got[0].Slice, got[1].Slice)
	}
}

func TestPersistsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.json")
	s := Open(path)
	if _, err := s.Add(Comment{ID: "x", Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "b"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("store file not written: %v", err)
	}
	// A fresh Store over the same path reads the persisted comment.
	got, err := Open(path).List("s")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Errorf("reopened store = %+v, want the one comment", got)
	}
	// No leftover temp file.
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file left behind: %v", err)
	}
}

func mustAdd(t *testing.T, s *Store, c Comment) Comment {
	t.Helper()
	got, err := s.Add(c)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return got
}
