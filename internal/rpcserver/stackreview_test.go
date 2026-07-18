package rpcserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/testutil"
)

// makeStackWorkspace builds a single-repo workspace (web on jonny/checkout) —
// the minimum the stack-review reads need. It is lighter than makeWorkspace's
// three repos, so the per-test temp-dir churn (and the macOS t.TempDir cleanup
// race with git worktrees) stays small. Returns the workspace and its root.
func makeStackWorkspace(t *testing.T) config.Workspace {
	t.Helper()
	web := testutil.NewRepo(t)
	base := t.TempDir()
	testutil.AddWorktree(t, web, "jonny/checkout", filepath.Join(base, "web-checkout"))
	return config.Workspace{
		Root: base,
		Repos: map[string]config.Repo{
			"web": {Primary: web, DefaultBranch: "main"},
		},
		Grouping: config.Grouping{Strategy: "branch-name", StripPrefix: "jonny/"},
	}
}

// gitWT runs a git command inside a worktree (or primary) dir and fails the test
// on error. The throwaway repos set a local identity (testutil.NewRepo), shared
// with their worktrees, so commits work with no global config.
func gitWT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// commitFile writes a file under the web-checkout worktree and commits it onto
// the jonny/checkout branch, so the branch advances past its trunk. The branch
// ref is shared with the web primary, so a subsequent ref-scoped read there sees
// the new commit.
func commitFile(t *testing.T, root, rel, content string) {
	t.Helper()
	wt := filepath.Join(root, "web-checkout")
	full := filepath.Join(wt, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	gitWT(t, wt, "add", "-A")
	// Disable auto gc + maintenance: either can spawn a background git process
	// that writes into .git after `commit` returns, racing t.TempDir's cleanup
	// ("directory not empty" on .git/objects).
	gitWT(t, wt, "-c", "gc.auto=0", "-c", "maintenance.auto=false", "commit", "-q", "-m", "add "+rel)
}

func TestBranchDiff(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "src/app.ts", "export const x = 1\n")
	h := newHarness(t, ws)

	resp := h.call(1, "branchDiff", `{"slice":"checkout","repo":"web","branch":"jonny/checkout"}`)
	var got report.BranchDiffResult
	decodeResult(t, resp, &got)

	if got.Repo != "web" || got.Branch != "jonny/checkout" {
		t.Errorf("branchDiff repo/branch = %s/%s", got.Repo, got.Branch)
	}
	if got.Parent == "" {
		t.Error("branchDiff should report the parent it diffed against")
	}
	if got.Stat == nil || got.Patch == nil {
		t.Fatalf("default format should carry both stat and patch: %+v", got)
	}
	found := false
	for _, f := range got.Stat.Files {
		if f.Path == "src/app.ts" {
			found = true
		}
	}
	if !found {
		t.Errorf("branchDiff files = %+v, want src/app.ts", got.Stat.Files)
	}
}

func TestBranchDiffNonMemberRepo(t *testing.T) {
	h := newHarness(t, makeStackWorkspace(t))
	resp := h.call(1, "branchDiff", `{"slice":"checkout","repo":"ops","branch":"jonny/checkout"}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params for non-member repo, got %+v", resp.Error)
	}
}

func TestBranchDiffMissingBranch(t *testing.T) {
	h := newHarness(t, makeStackWorkspace(t))
	resp := h.call(1, "branchDiff", `{"slice":"checkout","repo":"web","branch":"jonny/nope"}`)
	if resp.Error == nil || resp.Error.Data == nil || resp.Error.Data.Kind != "branch-not-found" {
		t.Fatalf("expected branch-not-found, got %+v", resp.Error)
	}
}

func TestTree(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "src/app.ts", "export const x = 1\n")
	commitFile(t, ws.Root, "src/util/help.ts", "export const help = () => {}\n")
	h := newHarness(t, ws)

	// Root listing: src (tree). The initial commit is empty, so src is the only
	// entry at root.
	resp := h.call(1, "tree", `{"slice":"checkout","repo":"web","branch":"jonny/checkout"}`)
	var root treeResult
	decodeResult(t, resp, &root)
	if len(root.Entries) != 1 || root.Entries[0].Name != "src" || root.Entries[0].Type != "tree" {
		t.Fatalf("root entries = %+v, want [src/tree]", root.Entries)
	}

	// One level under src: util (tree) then app.ts (blob).
	resp = h.call(2, "tree", `{"slice":"checkout","repo":"web","branch":"jonny/checkout","path":"src"}`)
	var sub treeResult
	decodeResult(t, resp, &sub)
	if len(sub.Entries) != 2 {
		t.Fatalf("src entries = %+v, want 2", sub.Entries)
	}
	if sub.Entries[0].Name != "util" || sub.Entries[1].Name != "app.ts" {
		t.Errorf("src entries order = %+v, want util then app.ts", sub.Entries)
	}
	if sub.Entries[1].Size != int64(len("export const x = 1\n")) {
		t.Errorf("app.ts size = %d", sub.Entries[1].Size)
	}
}

func TestFile(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "src/app.ts", "export const x = 1\n")
	h := newHarness(t, ws)

	resp := h.call(1, "file", `{"slice":"checkout","repo":"web","branch":"jonny/checkout","path":"src/app.ts"}`)
	var got report.FileContent
	decodeResult(t, resp, &got)
	if got.Binary {
		t.Error("text file flagged binary")
	}
	if got.Content != "export const x = 1\n" {
		t.Errorf("content = %q", got.Content)
	}
	if got.Size != int64(len("export const x = 1\n")) {
		t.Errorf("size = %d", got.Size)
	}
}

func TestFileBinary(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "logo.bin", "PNG\x00\x01\x02data")
	h := newHarness(t, ws)

	resp := h.call(1, "file", `{"slice":"checkout","repo":"web","branch":"jonny/checkout","path":"logo.bin"}`)
	var got report.FileContent
	decodeResult(t, resp, &got)
	if !got.Binary {
		t.Error("binary file not flagged")
	}
	if got.Content != "" {
		t.Errorf("binary content should be omitted, got %q", got.Content)
	}
}

func TestFileTooLarge(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "big.txt", "0123456789")
	h := newHarness(t, ws)

	resp := h.call(1, "file", `{"slice":"checkout","repo":"web","branch":"jonny/checkout","path":"big.txt","maxBytes":4}`)
	if resp.Error == nil || resp.Error.Data == nil || resp.Error.Data.Kind != "file-too-large" {
		t.Fatalf("expected file-too-large, got %+v", resp.Error)
	}
}

func TestFileNotAFile(t *testing.T) {
	ws := makeStackWorkspace(t)
	commitFile(t, ws.Root, "src/app.ts", "export const x = 1\n")
	h := newHarness(t, ws)

	resp := h.call(1, "file", `{"slice":"checkout","repo":"web","branch":"jonny/checkout","path":"src"}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params for a directory path, got %+v", resp.Error)
	}
}

func TestStackReviewRequiresParams(t *testing.T) {
	h := newHarness(t, makeStackWorkspace(t))
	for _, m := range []string{"branchDiff", "tree", "file"} {
		resp := h.call(1, m, `{"slice":"checkout"}`)
		if resp.Error == nil || resp.Error.Code != codeInvalidParams {
			t.Errorf("%s with missing repo/branch: expected invalid-params, got %+v", m, resp.Error)
		}
	}
}
