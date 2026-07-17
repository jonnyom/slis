package gt

import "os/exec"

// Native reports whether the repo at repoDir is a first-class Graphite repo:
// the gt CLI is on PATH AND the repo carries Graphite metadata (either `gt
// state` parses a non-empty stack, or pure-git refs/branch-metadata refs are
// present). It is the gate slis uses before running any gt mutation — auto
// tracking a slis-born branch only makes sense in a repo Graphite already
// manages.
//
// It performs the same reads as ReadStack, so callers that already hold a State
// should prefer checking that directly rather than paying for a second read.
func Native(repoDir string) bool {
	if _, err := exec.LookPath("gt"); err != nil {
		return false
	}
	if st, _ := ReadState(repoDir); len(st) > 0 {
		return true
	}
	meta, err := ReadRefMetadata(repoDir)
	if err != nil {
		return false
	}
	return len(meta) > 0
}
