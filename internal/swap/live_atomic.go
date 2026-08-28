package swap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func pathDepth(path string) int {
	return strings.Count(filepath.ToSlash(path), "/")
}

func sortPathsDeepestFirst(paths []string) {
	sort.Slice(paths, func(left, right int) bool {
		leftDepth := pathDepth(paths[left])
		rightDepth := pathDepth(paths[right])
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return paths[left] < paths[right]
	})
}

func replacePathAtomically(sourcePath, targetPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("mirrored path is a directory: %s", sourcePath)
	}
	targetDirectory := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDirectory, 0o755); err != nil {
		return err
	}
	temporaryPath, err := createTemporaryReplacement(sourcePath, targetDirectory, info)
	if err != nil {
		return err
	}
	if targetInfo, err := os.Lstat(targetPath); err == nil && targetInfo.IsDir() {
		if err := os.Remove(targetPath); err != nil {
			cleanupErr := os.Remove(temporaryPath)
			return errors.Join(err, cleanupErr)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		cleanupErr := os.Remove(temporaryPath)
		return errors.Join(err, cleanupErr)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		cleanupErr := os.Remove(temporaryPath)
		return errors.Join(err, cleanupErr)
	}
	return nil
}

func createTemporaryReplacement(sourcePath, targetDirectory string, info os.FileInfo) (string, error) {
	temporaryFile, err := os.CreateTemp(targetDirectory, ".slis-live-")
	if err != nil {
		return "", err
	}
	temporaryPath := temporaryFile.Name()
	if info.Mode()&os.ModeSymlink != 0 {
		closeErr := temporaryFile.Close()
		removeErr := os.Remove(temporaryPath)
		if err := errors.Join(closeErr, removeErr); err != nil {
			return "", err
		}
		destination, err := os.Readlink(sourcePath)
		if err != nil {
			return "", err
		}
		if err := os.Symlink(destination, temporaryPath); err != nil {
			return "", err
		}
		return temporaryPath, nil
	}
	if !info.Mode().IsRegular() {
		closeErr := temporaryFile.Close()
		removeErr := os.Remove(temporaryPath)
		return "", errors.Join(fmt.Errorf("unsupported mirrored path mode %s", info.Mode()), closeErr, removeErr)
	}
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		closeErr := temporaryFile.Close()
		removeErr := os.Remove(temporaryPath)
		return "", errors.Join(err, closeErr, removeErr)
	}
	_, copyErr := io.Copy(temporaryFile, sourceFile)
	chmodErr := temporaryFile.Chmod(info.Mode().Perm())
	sourceCloseErr := sourceFile.Close()
	temporaryCloseErr := temporaryFile.Close()
	if err := errors.Join(copyErr, chmodErr, sourceCloseErr, temporaryCloseErr); err != nil {
		removeErr := os.Remove(temporaryPath)
		return "", errors.Join(err, removeErr)
	}
	return temporaryPath, nil
}
