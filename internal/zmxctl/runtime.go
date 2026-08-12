package zmxctl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func EnsureRuntimeDirectory(scope string) (string, error) {
	directory := os.Getenv("SLIS_SESSION_RUNTIME_DIR")
	if directory == "" {
		name := fmt.Sprintf("slis-session-%d", os.Getuid())
		if scope != "" {
			digest := sha256.Sum256([]byte(scope))
			name += "-" + hex.EncodeToString(digest[:4])
		}
		directory = filepath.Join("/tmp", name)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}
