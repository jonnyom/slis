package zmxctl

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureRuntimeDirectory() (string, error) {
	directory := os.Getenv("SLIS_SESSION_RUNTIME_DIR")
	if directory == "" {
		directory = filepath.Join("/tmp", fmt.Sprintf("slis-session-%d", os.Getuid()))
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}
