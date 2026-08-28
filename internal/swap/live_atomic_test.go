package swap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/git"
)

func TestSyncWorkingTreeNeverRemovesModifiedTrackedFile(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	path := filepath.Join(worktree, "f.txt")
	primaryPath := filepath.Join(primary, "f.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 32<<20)), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	if _, err := git.Run(worktree, "add", "f.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-m", "large tracked file"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("initial SyncWorkingTree: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("b", 32<<20)), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}

	ready := make(chan struct{})
	stop := make(chan struct{})
	done := make(chan struct{})
	missing := make(chan struct{}, 1)
	go func() {
		defer close(done)
		close(ready)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
				missing <- struct{}{}
				return
			}
		}
	}()
	<-ready
	_, syncErr := SyncWorkingTree(journalPath)
	close(stop)
	<-done
	if syncErr != nil {
		t.Fatalf("SyncWorkingTree: %v", syncErr)
	}
	select {
	case <-missing:
		t.Fatal("modified tracked file was temporarily absent")
	default:
	}
}
