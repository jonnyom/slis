package report

import (
	"bytes"
	"fmt"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/safeterm"
)

// DefaultFileCap bounds FileAtRevision's content when the caller sets no cap:
// enough for any source file, small enough that a huge generated file or
// accidental binary can't flood a JSON transport.
const DefaultFileCap = 256 * 1024

// FileContent is a file's content at a branch's revision, marshal-ready. Content
// is omitted (and Binary true) for a binary file; text content is control-byte
// stripped (safeterm) so it is safe to render.
type FileContent struct {
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Binary  bool   `json:"binary"`
	Content string `json:"content,omitempty"`
}

// FileError carries a machine-readable kind for a FileAtRevision failure, so the
// RPC layer maps it to an error kind and the CLI prints the message. Kind is one
// of "path-not-found", "not-a-file", "file-too-large", or "" for an unexpected
// git failure.
type FileError struct {
	Kind string
	Err  error
}

func (e *FileError) Error() string { return e.Err.Error() }

// FileAtRevision reads one file's content at branch's revision from repoDir (the
// repo's primary checkout). It refuses non-blobs (a directory path), caps the
// size (maxBytes, or DefaultFileCap when ≤ 0), and flags binary content (with
// content omitted). Text content is safeterm-stripped.
func FileAtRevision(repoDir, repo, branch, path string, maxBytes int) (FileContent, *FileError) {
	if maxBytes <= 0 {
		maxBytes = DefaultFileCap
	}

	typ, err := git.ObjectType(repoDir, branch, path)
	if err != nil {
		return FileContent{}, &FileError{Kind: "path-not-found", Err: fmt.Errorf("path %q not found in %s", path, branch)}
	}
	if typ != "blob" {
		return FileContent{}, &FileError{Kind: "not-a-file", Err: fmt.Errorf("path %q is a %s, not a file", path, typ)}
	}

	size, err := git.ObjectSize(repoDir, branch, path)
	if err != nil {
		return FileContent{}, &FileError{Err: err}
	}
	if size > int64(maxBytes) {
		return FileContent{}, &FileError{Kind: "file-too-large", Err: fmt.Errorf("file is %d bytes, over the %d-byte cap", size, maxBytes)}
	}

	data, err := git.ShowFile(repoDir, branch, path)
	if err != nil {
		return FileContent{}, &FileError{Err: err}
	}

	fc := FileContent{Repo: repo, Branch: branch, Path: path, Size: size}
	if bytes.IndexByte(data, 0) >= 0 {
		fc.Binary = true
		return fc, nil
	}
	fc.Content = safeterm.Strip(string(data))
	return fc, nil
}
