package swap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jonnyom/slis/internal/git"
)

func backupWorkingPaths(sourceRoot string, paths []string) (string, []workingPathBackup, error) {
	backupRoot, err := os.MkdirTemp("", "slis-live-rollback-")
	if err != nil {
		return "", nil, err
	}
	backups := make([]workingPathBackup, 0, len(paths))
	for index, path := range paths {
		if !safeRelativePath(path) {
			cleanupErr := os.RemoveAll(backupRoot)
			return "", nil, errors.Join(fmt.Errorf("unsafe mirrored path %q", path), cleanupErr)
		}
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(path))
		info, err := os.Lstat(sourcePath)
		if errors.Is(err, os.ErrNotExist) || err == nil && info.IsDir() {
			backups = append(backups, workingPathBackup{Path: path})
			continue
		}
		if err != nil {
			cleanupErr := os.RemoveAll(backupRoot)
			return "", nil, errors.Join(err, cleanupErr)
		}
		backupPath := filepath.Join(backupRoot, fmt.Sprintf("%d", index))
		if err := replacePathAtomically(sourcePath, backupPath); err != nil {
			cleanupErr := os.RemoveAll(backupRoot)
			return "", nil, errors.Join(err, cleanupErr)
		}
		backups = append(backups, workingPathBackup{Path: path, BackupPath: backupPath, Exists: true})
	}
	return backupRoot, backups, nil
}

func rollbackWorkingSnapshot(repo RepoState, before workingSnapshot, backups []workingPathBackup, cause error) error {
	if err := restoreWorkingSnapshot(repo, before, backups); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback mirror for %q: %w", repo.Repo, err))
	}
	return cause
}

func restoreWorkingSnapshot(repo RepoState, before workingSnapshot, backups []workingPathBackup) error {
	if _, err := git.Run(repo.Primary, "reset", "--mixed", before.Head); err != nil {
		return err
	}
	removals := make([]string, 0, len(backups))
	replacements := make([]workingPathBackup, 0, len(backups))
	for _, backup := range backups {
		if backup.Exists {
			replacements = append(replacements, backup)
		} else {
			removals = append(removals, backup.Path)
		}
	}
	sortPathsDeepestFirst(removals)
	for _, path := range removals {
		if err := removeMirroredPath(repo.Primary, path); err != nil {
			return err
		}
	}
	sort.Slice(replacements, func(left, right int) bool {
		return pathDepth(replacements[left].Path) < pathDepth(replacements[right].Path)
	})
	for _, backup := range replacements {
		targetPath := filepath.Join(repo.Primary, filepath.FromSlash(backup.Path))
		if err := replacePathAtomically(backup.BackupPath, targetPath); err != nil {
			return err
		}
	}
	actual, err := captureWorkingSnapshot(repo.Primary)
	if err != nil {
		return err
	}
	if actual.Fingerprint != before.Fingerprint {
		return errors.New("rolled back primary does not match prior mirror")
	}
	return nil
}

func validatePrimaryTargetPaths(ctx context.Context, repo RepoState, source workingSnapshot) error {
	currentTracked, err := trackedPaths(ctx, repo.Primary, "HEAD")
	if err != nil {
		return err
	}
	sourceTracked, err := trackedPaths(ctx, repo.Worktree, source.Head)
	if err != nil {
		return err
	}
	previousMirrored := make(map[string]struct{})
	if repo.Mirror != nil {
		for _, path := range repo.Mirror.Untracked {
			previousMirrored[path] = struct{}{}
		}
	}
	targetPaths := sourceTracked
	for _, path := range source.Changed {
		targetPaths[path] = struct{}{}
	}
	for path := range targetPaths {
		if _, tracked := currentTracked[path]; tracked {
			continue
		}
		if _, mirrored := previousMirrored[path]; mirrored {
			continue
		}
		if err := refuseExistingPrimaryTarget(repo.Primary, path); err != nil {
			return err
		}
	}
	for _, path := range source.Untracked {
		if _, tracked := currentTracked[path]; tracked {
			continue
		}
		if _, mirrored := previousMirrored[path]; mirrored {
			continue
		}
		if err := refuseExistingPrimaryTarget(repo.Primary, path); err != nil {
			return err
		}
	}
	return nil
}

func workingTransitionPaths(ctx context.Context, repo RepoState, before, source workingSnapshot) ([]string, error) {
	committedRaw, err := git.RunRawCtx(ctx, repo.Primary, "diff", "--name-only", "-z", before.Head, source.Head, "--")
	if err != nil {
		return nil, err
	}
	pathSet := make(map[string]struct{})
	pathGroups := [][]string{
		splitNullTerminated(committedRaw),
		before.Changed,
		source.Changed,
		before.Untracked,
		source.Untracked,
	}
	for _, paths := range pathGroups {
		for _, path := range paths {
			if !safeRelativePath(path) {
				return nil, fmt.Errorf("unsafe mirrored path %q", path)
			}
			pathSet[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func trackedPaths(ctx context.Context, directory, revision string) (map[string]struct{}, error) {
	raw, err := git.RunRawCtx(ctx, directory, "ls-tree", "-r", "--name-only", "-z", revision)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]struct{})
	for _, path := range splitNullTerminated(raw) {
		paths[path] = struct{}{}
	}
	return paths, nil
}

func refuseExistingPrimaryTarget(root, path string) error {
	if !safeRelativePath(path) {
		return fmt.Errorf("unsafe target path %q", path)
	}
	target := filepath.Join(root, filepath.FromSlash(path))
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%w in %q: target already exists: %s", ErrUnknownPrimaryChanges, root, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validatePrimaryMirror(repo RepoState) error {
	return validatePrimaryMirrorContext(context.Background(), repo)
}

func validatePrimaryMirrorContext(ctx context.Context, repo RepoState) error {
	branch, err := currentBranchContext(ctx, repo.Primary)
	if err != nil {
		return fmt.Errorf("read primary branch for %q: %w", repo.Repo, err)
	}
	if branch != repo.TempBranch {
		return fmt.Errorf("%w in %q: expected branch %q, found %q", ErrUnknownPrimaryChanges, repo.Primary, repo.TempBranch, branch)
	}
	snapshot, err := captureWorkingSnapshotContext(ctx, repo.Primary)
	if err != nil {
		return fmt.Errorf("capture primary %q: %w", repo.Primary, err)
	}
	if snapshot.Head != repo.TargetSHA {
		return fmt.Errorf("%w in %q: expected tip %s, found %s", ErrUnknownPrimaryChanges, repo.Primary, shortSHA(repo.TargetSHA), shortSHA(snapshot.Head))
	}
	if repo.Mirror == nil {
		if len(snapshot.Patch) == 0 && len(snapshot.Untracked) == 0 {
			return nil
		}
		return fmt.Errorf("%w in %q: active journal has no mirror record", ErrUnknownPrimaryChanges, repo.Primary)
	}
	if snapshot.Fingerprint != repo.Mirror.Fingerprint {
		return fmt.Errorf("%w in %q: primary no longer matches the Slis mirror", ErrUnknownPrimaryChanges, repo.Primary)
	}
	return nil
}

func applyWorkingSnapshotContext(ctx context.Context, repo RepoState, source workingSnapshot, transitionPaths []string) error {
	if _, err := git.RunCtx(ctx, repo.Primary, "reset", "--mixed", source.Head); err != nil {
		return err
	}
	sourceTracked, err := trackedPaths(ctx, repo.Worktree, source.Head)
	if err != nil {
		return err
	}
	sourceVisible := sourceTracked
	for _, path := range source.Untracked {
		sourceVisible[path] = struct{}{}
	}
	removals := make([]string, 0, len(transitionPaths))
	replacements := make([]string, 0, len(transitionPaths))
	for _, path := range transitionPaths {
		if _, visible := sourceVisible[path]; !visible {
			removals = append(removals, path)
			continue
		}
		sourcePath := filepath.Join(repo.Worktree, filepath.FromSlash(path))
		info, err := os.Lstat(sourcePath)
		if errors.Is(err, os.ErrNotExist) {
			removals = append(removals, path)
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("mirrored path is a directory: %s", sourcePath)
		}
		replacements = append(replacements, path)
	}
	sortPathsDeepestFirst(removals)
	for _, path := range removals {
		if err := removeMirroredPath(repo.Primary, path); err != nil {
			return err
		}
	}
	sort.Slice(replacements, func(left, right int) bool {
		return pathDepth(replacements[left]) < pathDepth(replacements[right])
	})
	for _, path := range replacements {
		sourcePath := filepath.Join(repo.Worktree, filepath.FromSlash(path))
		targetPath := filepath.Join(repo.Primary, filepath.FromSlash(path))
		if err := replacePathAtomically(sourcePath, targetPath); err != nil {
			return err
		}
	}
	actual, err := captureWorkingSnapshotContext(ctx, repo.Primary)
	if err != nil {
		return err
	}
	if actual.Fingerprint != source.Fingerprint {
		latestSource, latestSourceErr := captureWorkingSnapshotContext(ctx, repo.Worktree)
		if latestSourceErr != nil {
			return errors.Join(
				fmt.Errorf("mirrored primary does not match source worktree"),
				fmt.Errorf("capture source after mirror mismatch: %w", latestSourceErr),
			)
		}
		return newMirrorMismatchError(source, actual, latestSource)
	}
	return nil
}

func clearWorkingMirror(repo RepoState) error {
	if err := validatePrimaryMirror(repo); err != nil {
		return err
	}
	if repo.Mirror != nil {
		for _, path := range repo.Mirror.Untracked {
			if err := removeMirroredPath(repo.Primary, path); err != nil {
				return err
			}
		}
	}
	if _, err := git.Run(repo.Primary, "reset", "--hard", repo.TargetSHA); err != nil {
		return err
	}
	return nil
}
