package swap

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonnyom/slis/internal/git"
)

func captureWorkingSnapshot(directory string) (workingSnapshot, error) {
	return captureWorkingSnapshotContext(context.Background(), directory)
}

func captureWorkingSnapshotContext(ctx context.Context, directory string) (workingSnapshot, error) {
	head, err := git.RevParseCtx(ctx, directory, "HEAD")
	if err != nil {
		return workingSnapshot{}, err
	}
	branch, err := currentBranchContext(ctx, directory)
	if err != nil {
		return workingSnapshot{}, err
	}
	patch, err := git.RunRawCtx(ctx, directory, "diff", "--binary", "--full-index", "HEAD", "--")
	if err != nil {
		return workingSnapshot{}, err
	}
	changedRaw, err := git.RunRawCtx(ctx, directory, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return workingSnapshot{}, err
	}
	changed := splitNullTerminated(changedRaw)
	sort.Strings(changed)
	untrackedRaw, err := git.RunRawCtx(ctx, directory, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return workingSnapshot{}, err
	}
	untracked := mirrorableUntrackedPaths(splitNullTerminated(untrackedRaw))
	sort.Strings(untracked)

	hash := sha256.New()
	writeHashField(hash, []byte(head))
	writeHashField(hash, patch)
	for _, path := range untracked {
		if err := ctx.Err(); err != nil {
			return workingSnapshot{}, err
		}
		if !safeRelativePath(path) {
			return workingSnapshot{}, fmt.Errorf("unsafe untracked path %q", path)
		}
		writeHashField(hash, []byte(path))
		info, err := os.Lstat(filepath.Join(directory, filepath.FromSlash(path)))
		if err != nil {
			return workingSnapshot{}, err
		}
		writeHashField(hash, []byte(info.Mode().String()))
		writeHashField(hash, []byte(fmt.Sprintf("%d", info.Size())))
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(directory, filepath.FromSlash(path)))
			if err != nil {
				return workingSnapshot{}, err
			}
			writeHashField(hash, []byte(target))
			continue
		}
		file, err := os.Open(filepath.Join(directory, filepath.FromSlash(path)))
		if err != nil {
			return workingSnapshot{}, err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return workingSnapshot{}, copyErr
		}
		if closeErr != nil {
			return workingSnapshot{}, closeErr
		}
	}
	return workingSnapshot{
		Head:        head,
		Branch:      branch,
		Patch:       patch,
		Changed:     changed,
		Untracked:   untracked,
		Fingerprint: fmt.Sprintf("%x", hash.Sum(nil)),
	}, nil
}

func mirrorableUntrackedPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if !strings.Contains(path, "/") && strings.HasPrefix(path, ".watchman-cookie-") {
			continue
		}
		result = append(result, path)
	}
	return result
}

func currentBranchContext(ctx context.Context, directory string) (string, error) {
	branch, err := git.RunCtx(ctx, directory, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", nil
	}
	return branch, nil
}

func writeHashField(writer io.Writer, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(value)
}

func splitNullTerminated(value []byte) []string {
	parts := strings.Split(string(value), "\x00")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func safeRelativePath(path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	return path != "" && !filepath.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func removeMirroredPath(root, path string) error {
	if !safeRelativePath(path) {
		return fmt.Errorf("unsafe mirrored path %q", path)
	}
	err := os.Remove(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
