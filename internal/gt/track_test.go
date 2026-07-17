package gt_test

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/testutil"
)

// TestTrackNotInstalled: with gt absent, Track returns ErrNotInstalled so
// best-effort callers can warn and carry on.
func TestTrackNotInstalled(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt installed; this test asserts the gt-absent branch")
	}
	repo := testutil.NewRepo(t)
	if _, err := gt.Track(repo, "feat", "main"); !errors.Is(err, gt.ErrNotInstalled) {
		t.Errorf("Track err = %v; want ErrNotInstalled", err)
	}
}

// TestTrackTracksBranch: with gt installed, tracking a slis-born branch makes it
// appear in the repo's stack (read via ReadStack) with the given parent.
func TestTrackTracksBranch(t *testing.T) {
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed")
	}
	repo := testutil.NewRepo(t)

	trunk, err := git.CurrentBranch(repo)
	if err != nil || trunk == "" {
		t.Fatalf("current branch: %v", err)
	}
	if _, err := git.Run(repo, "switch", "-c", "feat"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	if _, err := gt.Track(repo, "feat", trunk); err != nil {
		t.Fatalf("Track: %v", err)
	}

	st, err := gt.ReadStack(repo)
	if err != nil {
		t.Fatalf("ReadStack: %v", err)
	}
	bs, ok := st["feat"]
	if !ok {
		t.Fatalf("feat not tracked; state = %v", st)
	}
	if len(bs.Parents) == 0 || bs.Parents[0].Ref != trunk {
		t.Errorf("feat parent = %v; want %q", bs.Parents, trunk)
	}
}
