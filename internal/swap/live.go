package swap

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonnyom/slis/internal/git"
)

var ErrUnknownPrimaryChanges = errors.New("unknown primary changes")

func RunLiveSync(ctx context.Context, journalPath string, interval time.Duration) error {
	journal, err := syncWorkingTree(ctx, journalPath)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if journal == nil {
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	watchedDirectories := make(map[string]struct{})
	if err := refreshWatchDirectories(watcher, watchedDirectories, journal); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	scheduleSync := func() {
		if debounce == nil {
			debounce = time.NewTimer(75 * time.Millisecond)
		} else {
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(75 * time.Millisecond)
		}
		debounceChannel = debounce.C
	}
	sync := func() error {
		updated, err := syncWorkingTree(ctx, journalPath)
		if err != nil {
			return err
		}
		journal = updated
		return refreshWatchDirectories(watcher, watchedDirectories, journal)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-watcher.Events:
			if !ok {
				return errors.New("live mirror watcher closed")
			}
			scheduleSync()
		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.New("live mirror watcher closed")
			}
			return err
		case <-debounceChannel:
			debounceChannel = nil
			if err := sync(); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		case <-ticker.C:
			if err := sync(); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func refreshWatchDirectories(watcher *fsnotify.Watcher, watched map[string]struct{}, journal *Journal) error {
	if journal == nil {
		return nil
	}
	for _, repo := range journal.Repos {
		paths, err := git.RunRaw(repo.Worktree, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
		directories := map[string]struct{}{repo.Worktree: {}}
		for _, path := range splitNullTerminated(paths) {
			directory := filepath.Dir(filepath.Join(repo.Worktree, filepath.FromSlash(path)))
			for strings.HasPrefix(directory, repo.Worktree) {
				directories[directory] = struct{}{}
				if directory == repo.Worktree {
					break
				}
				directory = filepath.Dir(directory)
			}
		}
		for directory := range directories {
			if _, exists := watched[directory]; exists {
				continue
			}
			info, err := os.Stat(directory)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
			if !info.IsDir() {
				continue
			}
			if err := watcher.Add(directory); err != nil {
				return err
			}
			watched[directory] = struct{}{}
		}
	}
	return nil
}

type workingSnapshot struct {
	Head        string
	Branch      string
	Patch       []byte
	Changed     []string
	Untracked   []string
	Fingerprint string
}

type mirrorMismatchError struct {
	capturedSource workingSnapshot
	actualPrimary  workingSnapshot
	latestSource   workingSnapshot
}

func newMirrorMismatchError(capturedSource, actualPrimary, latestSource workingSnapshot) *mirrorMismatchError {
	return &mirrorMismatchError{capturedSource: capturedSource, actualPrimary: actualPrimary, latestSource: latestSource}
}

func (failure *mirrorMismatchError) Error() string {
	if failure.latestSource.Fingerprint != failure.capturedSource.Fingerprint {
		return "source worktree changed during mirror"
	}
	return "mirrored primary differs from stable source worktree"
}

func (failure *mirrorMismatchError) DiagnosticFields() map[string]any {
	return map[string]any{
		"source_changed_during_mirror": failure.latestSource.Fingerprint != failure.capturedSource.Fingerprint,
		"captured_source":              snapshotDiagnosticFields(failure.capturedSource),
		"actual_primary":               snapshotDiagnosticFields(failure.actualPrimary),
		"latest_source":                snapshotDiagnosticFields(failure.latestSource),
	}
}

func snapshotDiagnosticFields(snapshot workingSnapshot) map[string]any {
	patchHash := sha256.Sum256(snapshot.Patch)
	return map[string]any{
		"head":         snapshot.Head,
		"branch":       snapshot.Branch,
		"fingerprint":  snapshot.Fingerprint,
		"patch_sha256": fmt.Sprintf("%x", patchHash[:]),
		"changed":      append([]string(nil), snapshot.Changed...),
		"untracked":    append([]string(nil), snapshot.Untracked...),
	}
}

type workingPathBackup struct {
	Path       string
	BackupPath string
	Exists     bool
}

func (snapshot workingSnapshot) mirrorState() *MirrorState {
	return &MirrorState{
		Fingerprint: snapshot.Fingerprint,
		Untracked:   append([]string(nil), snapshot.Untracked...),
	}
}

func SyncWorkingTree(journalPath string) (*Journal, error) {
	return syncWorkingTree(context.Background(), journalPath)
}

func syncWorkingTree(ctx context.Context, journalPath string) (*Journal, error) {
	journal, err := Load(journalPath)
	if err != nil {
		return nil, err
	}
	if journal == nil {
		return nil, nil
	}

	sources := make([]workingSnapshot, len(journal.Repos))
	for index := range journal.Repos {
		repo := &journal.Repos[index]
		if repo.Worktree == "" {
			return nil, fmt.Errorf("live mirror unavailable for %q: worktree path is missing; reset the active slice and activate it again", repo.Repo)
		}
		if err := validatePrimaryMirrorContext(ctx, *repo); err != nil {
			return nil, err
		}
		source, err := captureWorkingSnapshotContext(ctx, repo.Worktree)
		if err != nil {
			return nil, fmt.Errorf("capture worktree %q: %w", repo.Worktree, err)
		}
		if err := validatePrimaryTargetPaths(ctx, *repo, source); err != nil {
			return nil, err
		}
		sources[index] = source
	}

	for index := range journal.Repos {
		repo := &journal.Repos[index]
		source := sources[index]
		if repo.TargetSHA == source.Head && repo.Branch == source.Branch && repo.Mirror != nil && repo.Mirror.Fingerprint == source.Fingerprint {
			continue
		}
		before, err := captureWorkingSnapshotContext(ctx, repo.Primary)
		if err != nil {
			return nil, fmt.Errorf("capture primary before mirror %q: %w", repo.Repo, err)
		}
		transitionPaths, err := workingTransitionPaths(ctx, *repo, before, source)
		if err != nil {
			return nil, fmt.Errorf("find mirror transition paths for %q: %w", repo.Repo, err)
		}
		backupRoot, backups, err := backupWorkingPaths(repo.Primary, transitionPaths)
		if err != nil {
			return nil, fmt.Errorf("backup primary before mirror %q: %w", repo.Repo, err)
		}
		if err := applyWorkingSnapshotContext(ctx, *repo, source, transitionPaths); err != nil {
			rollbackErr := rollbackWorkingSnapshot(*repo, before, backups, fmt.Errorf("mirror %q: %w", repo.Repo, err))
			cleanupErr := os.RemoveAll(backupRoot)
			return nil, errors.Join(rollbackErr, cleanupErr)
		}
		previousRepo := *repo
		repo.TargetSHA = source.Head
		if source.Branch != "" {
			repo.Branch = source.Branch
		}
		repo.Mirror = source.mirrorState()
		if err := Save(journalPath, journal); err != nil {
			*repo = previousRepo
			rollbackErr := rollbackWorkingSnapshot(*repo, before, backups, fmt.Errorf("save mirror state for %q: %w", repo.Repo, err))
			cleanupErr := os.RemoveAll(backupRoot)
			return nil, errors.Join(rollbackErr, cleanupErr)
		}
		if err := os.RemoveAll(backupRoot); err != nil {
			return nil, err
		}
	}
	return journal, nil
}
