package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/testutil"
)

func writeRepoFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitRepo(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitTree(t *testing.T) string {
	t.Helper()
	repo := testutil.NewRepo(t)
	writeRepoFile(t, repo, "README.md", "# hi\n")
	writeRepoFile(t, repo, "src/app.ts", "export const x = 1\n")
	writeRepoFile(t, repo, "src/util/help.ts", "export const help = () => {}\n")
	gitRepo(t, repo, "add", "-A")
	gitRepo(t, repo, "-c", "gc.auto=0", "-c", "maintenance.auto=false", "commit", "-q", "-m", "files")
	return repo
}

func TestLsTreeRootAndOneLevel(t *testing.T) {
	repo := commitTree(t)

	root, err := LsTree(repo, "main", "")
	if err != nil {
		t.Fatalf("LsTree root: %v", err)
	}
	if len(root) != 2 {
		t.Fatalf("root entries = %d, want 2 (src, README.md)", len(root))
	}
	// Trees sort first.
	if root[0].Name != "src" || root[0].Type != "tree" {
		t.Errorf("root[0] = %+v, want src/tree", root[0])
	}
	if root[0].Size != -1 {
		t.Errorf("tree size = %d, want -1", root[0].Size)
	}
	if root[1].Name != "README.md" || root[1].Type != "blob" {
		t.Errorf("root[1] = %+v, want README.md/blob", root[1])
	}
	if root[1].Size != int64(len("# hi\n")) {
		t.Errorf("README size = %d, want %d", root[1].Size, len("# hi\n"))
	}

	sub, err := LsTree(repo, "main", "src")
	if err != nil {
		t.Fatalf("LsTree src: %v", err)
	}
	if len(sub) != 2 {
		t.Fatalf("src entries = %d, want 2 (util, app.ts)", len(sub))
	}
	if sub[0].Name != "util" || sub[0].Type != "tree" {
		t.Errorf("src[0] = %+v, want util/tree", sub[0])
	}
	if sub[1].Name != "app.ts" || sub[1].Type != "blob" {
		t.Errorf("src[1] = %+v, want app.ts/blob", sub[1])
	}
}

func TestLsTreeBadPathErrors(t *testing.T) {
	repo := commitTree(t)
	if _, err := LsTree(repo, "main", "does/not/exist"); err == nil {
		t.Fatal("expected an error listing a non-existent path")
	}
}

func TestObjectHelpersAndShowFile(t *testing.T) {
	repo := commitTree(t)

	typ, err := ObjectType(repo, "main", "src/app.ts")
	if err != nil {
		t.Fatalf("ObjectType: %v", err)
	}
	if typ != "blob" {
		t.Errorf("type = %q, want blob", typ)
	}
	if dt, _ := ObjectType(repo, "main", "src"); dt != "tree" {
		t.Errorf("src type = %q, want tree", dt)
	}

	size, err := ObjectSize(repo, "main", "src/app.ts")
	if err != nil {
		t.Fatalf("ObjectSize: %v", err)
	}
	want := int64(len("export const x = 1\n"))
	if size != want {
		t.Errorf("size = %d, want %d", size, want)
	}

	data, err := ShowFile(repo, "main", "src/app.ts")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if string(data) != "export const x = 1\n" {
		t.Errorf("content = %q", string(data))
	}
}
