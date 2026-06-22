package gt_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/testutil"
)

// TestReadRefMetadata creates a refs/branch-metadata/feat ref pointing at a
// blob whose content is the Graphite per-branch JSON, then asserts
// ReadRefMetadata returns the correct parent mapping.
func TestReadRefMetadata(t *testing.T) {
	repo := testutil.NewRepo(t)

	// Get HEAD sha to embed in metadata JSON (mirrors real Graphite format).
	sha, err := git.Run(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	// Build the metadata JSON.
	metaJSON := fmt.Sprintf(`{"parentBranchName":"main","parentBranchRevision":%q}`, sha)

	// Write JSON to a temp file so git hash-object can read it.
	tmp := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(tmp, []byte(metaJSON), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Hash the blob and write it into the object store.
	blobSha, err := git.Run(repo, "hash-object", "-w", tmp)
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}

	// Point refs/branch-metadata/feat at the blob.
	if _, err := git.Run(repo, "update-ref", "refs/branch-metadata/feat", blobSha); err != nil {
		t.Fatalf("update-ref: %v", err)
	}

	meta, err := gt.ReadRefMetadata(repo)
	if err != nil {
		t.Fatalf("ReadRefMetadata: %v", err)
	}

	got, ok := meta["feat"]
	if !ok {
		t.Fatalf("ReadRefMetadata: key %q missing; map = %v", "feat", meta)
	}
	if got != "main" {
		t.Errorf("meta[%q] = %q; want %q", "feat", got, "main")
	}
}

// TestReadRefMetadataEmptyRepo asserts that a repo with no refs/branch-metadata
// refs returns an empty map (not an error).
func TestReadRefMetadataEmptyRepo(t *testing.T) {
	repo := testutil.NewRepo(t)

	meta, err := gt.ReadRefMetadata(repo)
	if err != nil {
		t.Fatalf("ReadRefMetadata on empty repo: %v", err)
	}
	if len(meta) != 0 {
		t.Errorf("expected empty map, got %v", meta)
	}
}

// TestStackFromRefMeta unit-tests the pure State-building logic without any gt
// or repo dependency. Input: feat -> main. Expected: feat.Parents[0].Ref ==
// "main" and main.Trunk == true.
func TestStackFromRefMeta(t *testing.T) {
	meta := map[string]string{"feat": "main"}
	state := gt.StackFromRefMeta(meta)

	feat, ok := state["feat"]
	if !ok {
		t.Fatalf("state missing key %q", "feat")
	}
	if len(feat.Parents) != 1 {
		t.Fatalf("feat.Parents len = %d; want 1", len(feat.Parents))
	}
	if feat.Parents[0].Ref != "main" {
		t.Errorf("feat.Parents[0].Ref = %q; want %q", feat.Parents[0].Ref, "main")
	}
	if feat.Trunk {
		t.Errorf("feat.Trunk = true; want false")
	}

	main, ok := state["main"]
	if !ok {
		t.Fatalf("state missing key %q", "main")
	}
	if !main.Trunk {
		t.Errorf("main.Trunk = false; want true")
	}
}

// TestStackFromRefMetaMultiBranch tests a two-level chain: feat2 -> feat -> main.
func TestStackFromRefMetaMultiBranch(t *testing.T) {
	meta := map[string]string{
		"feat":  "main",
		"feat2": "feat",
	}
	state := gt.StackFromRefMeta(meta)

	if len(state) != 3 {
		t.Fatalf("state len = %d; want 3", len(state))
	}

	main := state["main"]
	if !main.Trunk {
		t.Errorf("main.Trunk = false; want true")
	}

	feat := state["feat"]
	if feat.Trunk {
		t.Errorf("feat.Trunk = true; want false")
	}
	if len(feat.Parents) != 1 || feat.Parents[0].Ref != "main" {
		t.Errorf("feat.Parents unexpected: %v", feat.Parents)
	}

	feat2 := state["feat2"]
	if feat2.Trunk {
		t.Errorf("feat2.Trunk = true; want false")
	}
	if len(feat2.Parents) != 1 || feat2.Parents[0].Ref != "feat" {
		t.Errorf("feat2.Parents unexpected: %v", feat2.Parents)
	}
}

// TestReadStackFallback verifies that ReadStack falls back to refs-metadata
// when gt is absent. When gt IS installed it auto-initialises every repo it
// touches (returning {"main":{trunk:true}}), so the fallback path can only be
// exercised on machines without gt. The deterministic, gt-presence-agnostic
// coverage of the fallback builder is in TestStackFromRefMeta above; here we
// guard with a skip so the test never produces a false failure.
func TestReadStackFallback(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt is installed; gt auto-initialises repos so the refs fallback path is unreachable — fallback builder is exercised by TestStackFromRefMeta instead")
	}

	repo := testutil.NewRepo(t)

	sha, err := git.Run(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	metaJSON := fmt.Sprintf(`{"parentBranchName":"main","parentBranchRevision":%q}`, sha)
	tmp := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(tmp, []byte(metaJSON), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	blobSha, err := git.Run(repo, "hash-object", "-w", tmp)
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}
	if _, err := git.Run(repo, "update-ref", "refs/branch-metadata/feat", blobSha); err != nil {
		t.Fatalf("update-ref: %v", err)
	}

	// With gt absent, ReadStack must use the refs fallback.
	state, err := gt.ReadStack(repo)
	if err != nil {
		t.Fatalf("ReadStack: %v", err)
	}

	feat, ok := state["feat"]
	if !ok {
		t.Fatalf("state missing key %q; state = %v", "feat", state)
	}
	if len(feat.Parents) != 1 || feat.Parents[0].Ref != "main" {
		t.Errorf("feat.Parents unexpected: %v", feat.Parents)
	}

	main, ok := state["main"]
	if !ok {
		t.Fatalf("state missing key %q; state = %v", "main", state)
	}
	if !main.Trunk {
		t.Errorf("main.Trunk = false; want true")
	}
}
