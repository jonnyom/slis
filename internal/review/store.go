// Package review holds the per-slice pending inline-review-comment store and the
// machinery that turns a batch of comments into an agent-ready instruction and
// delivers it to the slice's running session.
//
// A review comment is GitHub-review-style: while reading a slice's diff you
// comment on a file:line (optionally quoting the hunk) with an instruction. The
// comments accumulate in a single JSON file in the slis state dir; submitting a
// batch composes a structured prompt (ComposePrompt) and injects it into the
// slice's tmux session (Send). Mutation lives in the CLI (`slis review …`); the
// read-only RPC sidecar only lists pending comments.
package review

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Comment is one pending inline-review comment awaiting delivery to a slice's
// agent. Line is a 1-based line number in the new (post-change) file; Hunk is an
// optional excerpt of the diff giving the agent surrounding context.
type Comment struct {
	ID        string    `json:"id"`
	Slice     string    `json:"slice"`
	Repo      string    `json:"repo"`
	Branch    string    `json:"branch,omitempty"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	Hunk      string    `json:"hunk,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrCommentNotFound is returned by Remove when no pending comment has the given
// id.
var ErrCommentNotFound = errors.New("review comment not found")

// Store is the pending-review-comment store: a single JSON file holding every
// slice's pending comments. Reads and writes are serialised by a mutex; each
// mutating op is a read-modify-write with an atomic (temp+rename) save, matching
// the swap journal's durability pattern.
type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	newID func() string
}

// Open returns a Store backed by the file at path. The file is created lazily on
// the first write; a missing file reads as an empty store.
func Open(path string) *Store {
	return &Store{
		path:  path,
		now:   func() time.Time { return time.Now().UTC() },
		newID: randomID,
	}
}

// Add stores a new comment and returns it as persisted. A blank ID is filled
// with a fresh random id and a zero CreatedAt is stamped with the current time,
// so callers (and tests) may supply either explicitly for determinism.
func (s *Store) Add(c Comment) (Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.ID == "" {
		c.ID = s.newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = s.now()
	}

	all, err := s.load()
	if err != nil {
		return Comment{}, err
	}
	all = append(all, c)
	if err := s.save(all); err != nil {
		return Comment{}, err
	}
	return c, nil
}

// List returns the pending comments for one slice in deterministic order
// (repo, file, line, id). An empty result is a nil slice, never an error.
func (s *Store) List(slice string) ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(all))
	for _, c := range all {
		if c.Slice == slice {
			out = append(out, c)
		}
	}
	sortComments(out)
	return out, nil
}

// ListAll returns every pending comment across all slices in deterministic order
// (slice, repo, file, line, id).
func (s *Store) ListAll() ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return nil, err
	}
	if all == nil {
		all = []Comment{}
	}
	sortComments(all)
	return all, nil
}

// Remove deletes the pending comment with the given id. It returns
// ErrCommentNotFound when no comment matches, so the caller can report a miss.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	kept := make([]Comment, 0, len(all))
	found := false
	for _, c := range all {
		if c.ID == id {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		return ErrCommentNotFound
	}
	return s.save(kept)
}

// Clear drops every pending comment for one slice (used after a successful
// send). Clearing a slice with no pending comments is a no-op, not an error.
func (s *Store) Clear(slice string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	kept := make([]Comment, 0, len(all))
	for _, c := range all {
		if c.Slice != slice {
			kept = append(kept, c)
		}
	}
	return s.save(kept)
}

// load reads and parses the store file. A missing file yields an empty slice.
func (s *Store) load() ([]Comment, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var all []Comment
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// save writes all comments to the store file atomically (temp file + rename), so
// a crash or full disk mid-write can never leave a truncated store.
func (s *Store) save(all []Comment) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// sortComments orders comments deterministically by slice, repo, file, line then
// id, so both List/ListAll and the composed prompt are stable.
func sortComments(cs []Comment) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Slice != b.Slice {
			return a.Slice < b.Slice
		}
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.ID < b.ID
	})
}

// randomID returns a short random hex id for a new comment.
func randomID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is catastrophic and unrecoverable; fall back to a
		// time-based id so Add never silently drops a comment.
		return "c" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
