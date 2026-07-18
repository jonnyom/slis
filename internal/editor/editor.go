// Package editor resolves and launches a code editor for slis worktrees and
// slices. For VS Code-family editors it generates a multi-root .code-workspace
// file so a whole slice (several repo worktrees) opens in a single window with
// each repo as its own root (and its own git). Other editors fall back to
// opening multiple directory arguments or the slice's common parent directory.
package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Mode describes how an editor opens several folders at once.
type Mode int

const (
	// ModeWorkspace: VS Code-family — opens a generated .code-workspace file
	// listing every worktree as a root folder.
	ModeWorkspace Mode = iota
	// ModeMultiDir: editor opens several directory args in one window (e.g. zed).
	ModeMultiDir
	// ModeSingleDir: editor opens a single directory; a whole slice falls back to
	// the worktrees' common parent directory.
	ModeSingleDir
)

// Editor is a resolved editor: a user-facing name, the binary to exec, and how
// it handles opening several folders.
type Editor struct {
	Name string
	Bin  string
	Mode Mode
}

// known lists the editors slis recognises, in detection-preference order.
var known = []Editor{
	{Name: "Cursor", Bin: "cursor", Mode: ModeWorkspace},
	{Name: "VS Code", Bin: "code", Mode: ModeWorkspace},
	{Name: "VS Code Insiders", Bin: "code-insiders", Mode: ModeWorkspace},
	{Name: "VSCodium", Bin: "codium", Mode: ModeWorkspace},
	{Name: "Windsurf", Bin: "windsurf", Mode: ModeWorkspace},
	{Name: "Zed", Bin: "zed", Mode: ModeMultiDir},
}

// lookPath is indirected so tests can stub PATH resolution.
var lookPath = exec.LookPath

// Available returns the known editors found on PATH, in preference order.
func Available() []Editor {
	var out []Editor
	for _, e := range known {
		if _, err := lookPath(e.Bin); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// knownByBin returns the known editor whose Bin matches (basename-insensitive),
// or (zero, false).
func knownByBin(bin string) (Editor, bool) {
	base := filepath.Base(bin)
	for _, e := range known {
		if e.Bin == base {
			return e, true
		}
	}
	return Editor{}, false
}

// Resolve picks the editor to use. A non-empty configured binary wins when it is
// on PATH (its Mode comes from the known list, or ModeSingleDir if unrecognised).
// Otherwise the first Available editor is used. Returns an error when nothing is
// usable, so callers can prompt the user to install or configure one.
func Resolve(configured string) (Editor, error) {
	if configured != "" {
		if _, err := lookPath(configured); err != nil {
			return Editor{}, fmt.Errorf("configured editor %q not found on PATH", configured)
		}
		if e, ok := knownByBin(configured); ok {
			return e, nil
		}
		return Editor{Name: configured, Bin: configured, Mode: ModeSingleDir}, nil
	}
	avail := Available()
	if len(avail) == 0 {
		return Editor{}, fmt.Errorf("no supported editor found on PATH (tried cursor, code, codium, windsurf, zed)")
	}
	return avail[0], nil
}

// codeWorkspace is the minimal .code-workspace JSON shape VS Code-family editors
// read: a list of root folders, each opened as its own workspace root.
type codeWorkspace struct {
	Folders []wsFolder `json:"folders"`
}

type wsFolder struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

// WriteWorkspaceFile writes a <slice>.code-workspace file into dir listing each
// worktree as a root folder (absolute path, labelled by its directory name), and
// returns the file's path. dir is created if missing.
func WriteWorkspaceFile(dir, slice string, worktrees []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("editor: mkdir %q: %w", dir, err)
	}
	ws := codeWorkspace{Folders: make([]wsFolder, 0, len(worktrees))}
	for _, wt := range worktrees {
		if wt == "" {
			continue
		}
		abs, err := filepath.Abs(wt)
		if err != nil {
			abs = wt
		}
		ws.Folders = append(ws.Folders, wsFolder{Name: filepath.Base(abs), Path: abs})
	}
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return "", fmt.Errorf("editor: marshal workspace: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, sanitizeFilename(slice)+".code-workspace")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("editor: write %q: %w", path, err)
	}
	return path, nil
}

// sanitizeFilename makes slice a safe single path segment (slices may nest, e.g.
// "feat/sub"): path separators become '-'.
func sanitizeFilename(slice string) string {
	return strings.NewReplacer("/", "-", string(os.PathSeparator), "-").Replace(slice)
}

// commonParent returns the shared parent directory of paths, or "" when they
// don't all live under one parent (or the list is empty).
func commonParent(paths []string) string {
	parent := ""
	for _, p := range paths {
		if p == "" {
			return ""
		}
		d := filepath.Dir(p)
		if parent == "" {
			parent = d
		} else if d != parent {
			return ""
		}
	}
	return parent
}

// SliceArgs prepares the positional path arguments to open a whole slice, given
// its member worktree paths. workspaceDir is where a generated .code-workspace
// is written (used only in ModeWorkspace). It returns the argv path(s) to pass
// to the editor binary. ModeWorkspace writes and returns the workspace file;
// ModeMultiDir returns the worktrees; ModeSingleDir returns their common parent
// (falling back to the first worktree).
func SliceArgs(ed Editor, slice string, worktrees []string, workspaceDir string) ([]string, error) {
	if len(worktrees) == 0 {
		return nil, fmt.Errorf("editor: slice %q has no worktrees to open", slice)
	}
	switch ed.Mode {
	case ModeWorkspace:
		f, err := WriteWorkspaceFile(workspaceDir, slice, worktrees)
		if err != nil {
			return nil, err
		}
		return []string{f}, nil
	case ModeMultiDir:
		return append([]string(nil), worktrees...), nil
	default: // ModeSingleDir
		if p := commonParent(worktrees); p != "" {
			return []string{p}, nil
		}
		return []string{worktrees[0]}, nil
	}
}

// run is indirected so tests can stub process launches.
var run = func(bin string, args ...string) error {
	c := exec.Command(bin, args...) //nolint:gosec // bin is a resolved editor, args are paths
	return c.Start()                // GUI editors fork; launch detached, don't block the TUI
}

// OpenSlice opens every worktree of a slice in one editor window.
func OpenSlice(ed Editor, slice string, worktrees []string, workspaceDir string) error {
	args, err := SliceArgs(ed, slice, worktrees, workspaceDir)
	if err != nil {
		return err
	}
	if err := run(ed.Bin, args...); err != nil {
		return fmt.Errorf("editor: launch %s: %w", ed.Bin, err)
	}
	return nil
}

// OpenDir opens a single directory in the editor.
func OpenDir(ed Editor, dir string) error {
	if dir == "" {
		return fmt.Errorf("editor: empty directory")
	}
	if err := run(ed.Bin, dir); err != nil {
		return fmt.Errorf("editor: launch %s: %w", ed.Bin, err)
	}
	return nil
}

// FileArgs returns the editor-specific arguments for opening a file, optionally
// at a one-based line. VS Code-family editors use --goto; Zed accepts the same
// path:line form directly. Unknown configured editors receive the plain path so
// file opening remains portable even when precise line addressing is unknown.
func FileArgs(ed Editor, path string, line int) []string {
	target := path
	if line > 0 && (ed.Mode == ModeWorkspace || filepath.Base(ed.Bin) == "zed") {
		target = fmt.Sprintf("%s:%d", path, line)
	}
	if ed.Mode == ModeWorkspace {
		return []string{"--goto", target}
	}
	return []string{target}
}

// OpenFile opens a single file in the editor, optionally at a one-based line.
func OpenFile(ed Editor, path string, line int) error {
	if path == "" {
		return fmt.Errorf("editor: empty file path")
	}
	if err := run(ed.Bin, FileArgs(ed, path, line)...); err != nil {
		return fmt.Errorf("editor: launch %s: %w", ed.Bin, err)
	}
	return nil
}
