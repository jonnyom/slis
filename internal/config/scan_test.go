package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// mustInitRepo creates dir and runs `git init -q` inside it.
// The test is skipped if git is not on PATH.
func mustInitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mustInitRepo: MkdirAll %q: %v", dir, err)
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mustInitRepo: git init in %q: %v\n%s", dir, err, out)
	}
}

func TestScanReposFindsOnlyGitDirs(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "a"))
	mustInitRepo(t, filepath.Join(root, "b"))
	os.MkdirAll(filepath.Join(root, "c"), 0o755) // not a repo
	// also a loose file to confirm it is not returned
	os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x"), 0o644)

	got, err := ScanRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, r := range got {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Fatalf("got %v, want [a b]", names)
	}
}

func TestScanReposPathAndDefaultBranch(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "myrepo"))

	got, err := ScanRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(got))
	}
	c := got[0]
	if c.Name != "myrepo" {
		t.Errorf("Name = %q, want myrepo", c.Name)
	}
	if c.Path != filepath.Join(root, "myrepo") {
		t.Errorf("Path = %q, want %q", c.Path, filepath.Join(root, "myrepo"))
	}
	// fresh local repo has no remote, so DefaultBranch falls back to current HEAD branch or "main"
	if c.DefaultBranch == "" {
		t.Error("DefaultBranch is empty")
	}
}

func TestBuildWorkspaceDefaults(t *testing.T) {
	root := t.TempDir()
	candidates := []Candidate{
		{Name: "alpha", Path: filepath.Join(root, "alpha"), DefaultBranch: "main"},
	}

	ws := BuildWorkspace(root, candidates, "jonny/")

	if ws.Root != root {
		t.Errorf("Root = %q, want %q", ws.Root, root)
	}
	if len(ws.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(ws.Repos))
	}
	if ws.Grouping.Strategy != "branch-name" {
		t.Errorf("Grouping.Strategy = %q, want %q", ws.Grouping.Strategy, "branch-name")
	}
	if ws.Grouping.StripPrefix != "jonny/" {
		t.Errorf("Grouping.StripPrefix = %q, want %q", ws.Grouping.StripPrefix, "jonny/")
	}
	if ws.Processes.CPUWarnPct != 150 {
		t.Errorf("Processes.CPUWarnPct = %d, want 150", ws.Processes.CPUWarnPct)
	}

	// Verify empty strip prefix is preserved.
	ws2 := BuildWorkspace(root, candidates, "")
	if ws2.Grouping.StripPrefix != "" {
		t.Errorf("Grouping.StripPrefix = %q, want empty", ws2.Grouping.StripPrefix)
	}
}

func TestScanReposSorted(t *testing.T) {
	root := t.TempDir()
	// Create in reverse alphabetical order to confirm sorting
	mustInitRepo(t, filepath.Join(root, "zeta"))
	mustInitRepo(t, filepath.Join(root, "alpha"))
	mustInitRepo(t, filepath.Join(root, "mu"))

	got, err := ScanRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(got))
	for i, c := range got {
		names[i] = c.Name
	}
	want := []string{"alpha", "mu", "zeta"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}
