package gt

import (
	"encoding/json"
	"strings"

	"github.com/jonnyom/slis/internal/git"
)

// refMeta is the JSON shape stored by Graphite (pre-SQLite, refs backend) in
// each refs/branch-metadata/<branch> blob.
type refMeta struct {
	ParentBranchName     string `json:"parentBranchName"`
	ParentBranchRevision string `json:"parentBranchRevision"`
}

// ReadRefMetadata reads all refs/branch-metadata/<branch> blobs from the repo
// using pure git (zero writes). It returns a map of branchName ->
// parentBranchName. Blobs that cannot be parsed are silently skipped; only
// hard git errors from for-each-ref are propagated.
func ReadRefMetadata(repoDir string) (map[string]string, error) {
	out, err := git.Run(repoDir, "for-each-ref", "--format=%(refname)", "refs/branch-metadata/")
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	if strings.TrimSpace(out) == "" {
		return result, nil
	}

	for _, line := range strings.Split(out, "\n") {
		refname := strings.TrimSpace(line)
		if refname == "" {
			continue
		}

		branch := strings.TrimPrefix(refname, "refs/branch-metadata/")

		blob, err := git.Run(repoDir, "cat-file", "-p", refname)
		if err != nil {
			// Tolerate refs whose blobs can't be read.
			continue
		}

		var m refMeta
		if err := json.Unmarshal([]byte(blob), &m); err != nil {
			// Tolerate unparseable blobs.
			continue
		}

		if m.ParentBranchName == "" {
			continue
		}

		result[branch] = m.ParentBranchName
	}

	return result, nil
}

// StackFromRefMeta builds a State from a branchName -> parentBranchName map
// (the output of ReadRefMetadata). Trunk detection is best-effort: a branch
// that is referenced as a parent but has no metadata entry of its own (i.e. it
// has no parent in the map) is marked as trunk. NeedsRestack is not available
// from refs metadata and is always false.
func StackFromRefMeta(meta map[string]string) State {
	state := make(State, len(meta)+1)

	for branch, parent := range meta {
		bs := state[branch]
		bs.Parents = []Parent{{Ref: parent}}
		state[branch] = bs

		// Ensure parent exists as a key; don't overwrite if already present.
		if _, exists := state[parent]; !exists {
			state[parent] = BranchState{}
		}
	}

	// Trunk detection: a branch that appears only as a parent (never as a key
	// in meta, meaning it has no parent of its own) is the trunk.
	for branch := range state {
		if _, hasMeta := meta[branch]; !hasMeta {
			// This branch has no entry in the refs metadata — it is a root.
			bs := state[branch]
			bs.Trunk = true
			state[branch] = bs
		}
	}

	return state
}

// ReadStack is the unified stack-reading entry point. It first tries
// ReadState (gt state --no-interactive). If that returns a non-empty State it
// is used as-is. Otherwise it falls back to ReadRefMetadata and builds a State
// via StackFromRefMeta.
//
// Note: when using the refs fallback, NeedsRestack is always false (the refs
// metadata does not carry restack status), and trunk detection is best-effort
// (the branch referenced as a parent but absent from the metadata namespace).
func ReadStack(repoDir string) (State, error) {
	st, _ := ReadState(repoDir)
	if len(st) > 0 {
		return st, nil
	}

	meta, err := ReadRefMetadata(repoDir)
	if err != nil {
		return nil, err
	}

	return StackFromRefMeta(meta), nil
}
