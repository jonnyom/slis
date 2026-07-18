package git

import (
	"strconv"
	"strings"
)

// ObjectType returns the git object type at rev:path ("blob", "tree", or
// "commit"). It errors when the path does not resolve in the revision.
func ObjectType(dir, rev, path string) (string, error) {
	out, err := Run(dir, "cat-file", "-t", rev+":"+path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ObjectSize returns the byte size of the blob at rev:path without reading its
// content, so a caller can enforce a cap before loading a huge file.
func ObjectSize(dir, rev, path string) (int64, error) {
	out, err := Run(dir, "cat-file", "-s", rev+":"+path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

// ShowFile returns the raw bytes of the file at rev:path via `git show`. dir
// MUST be the repo's primary checkout; the read is ref-scoped and never touches
// a working tree.
func ShowFile(dir, rev, path string) ([]byte, error) {
	return RunRaw(dir, "show", rev+":"+path)
}
