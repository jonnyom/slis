package zmxctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrRuntimeMissing = errors.New("Slis session runtime is missing")

func ResolveBinary(slisBinary, developmentOverride string) (string, error) {
	candidates := []string{
		filepath.Join(filepath.Dir(slisBinary), "zmx"),
		filepath.Clean(filepath.Join(filepath.Dir(slisBinary), "..", "libexec", "zmx")),
	}
	if resolved, err := filepath.EvalSymlinks(slisBinary); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(resolved), "zmx"))
	}
	for _, candidate := range candidates {
		available, err := executableFile(candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}
	if developmentOverride != "" {
		available, err := executableFile(developmentOverride)
		if err != nil {
			return "", err
		}
		if available {
			return developmentOverride, nil
		}
	}
	return "", fmt.Errorf("%w next to %s", ErrRuntimeMissing, slisBinary)
}

func executableFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0, nil
}
