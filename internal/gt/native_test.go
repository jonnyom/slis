package gt_test

import (
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/testutil"
)

// TestNativeWithoutGt: with gt absent, Native is always false regardless of the
// repo's metadata (the CLI is a hard precondition).
func TestNativeWithoutGt(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt installed; this test asserts the gt-absent branch")
	}
	repo := testutil.NewRepo(t)
	if gt.Native(repo) {
		t.Error("Native = true with gt absent; want false")
	}
}

// TestNativeWithGt: with gt installed, a repo gt can read (gt auto-initialises
// every repo it touches) is reported native.
func TestNativeWithGt(t *testing.T) {
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed")
	}
	repo := testutil.NewRepo(t)
	if !gt.Native(repo) {
		t.Error("Native = false with gt installed on a real repo; want true")
	}
}
