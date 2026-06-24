package commentcache

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("missing file: want empty store, got %v", s)
	}
}

func TestPutSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments.json")
	s := Store{}
	s.Put("wfm-1", "web", 42, "http://pr/42", []Comment{{Author: "a", Body: "hi", URL: "u"}})
	// Empty comments are a no-op (don't clobber on a failed re-fetch).
	s.Put("wfm-1", "api", 7, "http://pr/7", nil)

	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rc, ok := got["wfm-1"]["web"]
	if !ok {
		t.Fatalf("web entry missing: %+v", got)
	}
	if rc.PR != 42 || len(rc.Comments) != 1 || rc.Comments[0].Body != "hi" {
		t.Errorf("round-trip mismatch: %+v", rc)
	}
	if _, ok := got["wfm-1"]["api"]; ok {
		t.Error("empty-comment Put should not have stored an api entry")
	}
}

func TestPutPreservesOnEmptyRefetch(t *testing.T) {
	s := Store{}
	s.Put("s", "web", 1, "u", []Comment{{Author: "a", Body: "keep"}})
	// A later fetch that finds nothing (branch gone) must not wipe the cache.
	s.Put("s", "web", 0, "", nil)
	if len(s["s"]["web"].Comments) != 1 {
		t.Errorf("cached comment was clobbered by empty re-fetch: %+v", s["s"]["web"])
	}
}
