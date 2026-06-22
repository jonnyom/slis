package swap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// RepoState records the before/after state for a single primary checkout
// involved in a slice activation. Enough information to restore the primary
// to its exact pre-activation state (branch, stash, dep install flag).
type RepoState struct {
	Repo        string `json:"repo"`
	Primary     string `json:"primary"`
	PriorBranch string `json:"prior_branch"` // branch the primary was on before activate ("" if it was detached)
	PriorSHA    string `json:"prior_sha"`    // HEAD sha before activate (for detached-prior restore)
	StashRef    string `json:"stash_ref"`    // pinned stash commit sha, "" if nothing stashed
	StashMsg    string `json:"stash_msg"`    // unique stash message used during activation, "" if nothing stashed
	TargetSHA   string `json:"target_sha"`   // the slice branch tip we checked out
	Reconciled  bool   `json:"reconciled"`   // whether a dep install ran during activate
}

// Journal is the activation journal written atomically to disk whenever a
// slice is activated. It carries enough state to deactivate (restore) or
// recover from a crash mid-swap.
type Journal struct {
	Slice string      `json:"slice"`
	Repos []RepoState `json:"repos"`
}

// Save marshals j as indented JSON and writes it to path, creating any
// missing parent directories as needed.
func Save(path string, j *Journal) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads and unmarshals the journal at path. If the file does not exist
// it returns (nil, nil) — meaning no active swap is recorded. Any other read
// or parse error is returned as-is.
func Load(path string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// Clear removes the journal file at path. If the file does not exist the call
// is a no-op (already cleared). Any other removal error is returned.
func Clear(path string) error {
	err := os.Remove(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
