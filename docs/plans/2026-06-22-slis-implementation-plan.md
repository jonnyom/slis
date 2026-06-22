# slis Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** Build `slis` — a lazygit-style TUI + scriptable CLI that treats a *slice* (a feature's git worktrees across three repos) as the first-class unit: discover slices, swap one into the primary trees so running servers rebuild, review the whole-slice diff, read the Graphite stack, manage tmux Claude sessions (attach/detach + CPU view + needs-input notifications).

**Architecture:** Single Go binary, three surfaces over one core (see design §4). Core = git plumbing + slice model + swap engine + gt reader + tmux/process/event services. Surfaces = Bubble Tea TUI, cobra CLI twin (`--json`), and a `slis hook` handler that Claude Code hooks feed. All slow work shells out via an injection-proof argv builder; `CGO_ENABLED=0` keeps the binary fully static.

**Tech Stack:** Go · Bubble Tea/Bubbles/Lipgloss (TUI) · huh (init repo picker) · cobra (CLI) · chroma (diff highlight) · glamour (markdown) · gopsutil (process info) · fsnotify (watch) · `os/exec` to `git`/`gt`/`tmux`/`claude`. GoReleaser + Homebrew tap for distribution.

**Source design:** `docs/plans/2026-06-22-slis-multi-repo-worktree-cockpit-design.md`

---

## Conventions (read once)

- **Module path:** `github.com/jonnyomahony/slis` — *set the real GitHub path in Task 0 before committing `go.mod`.*
- **Layout:**
  ```
  cmd/slis/main.go               # entrypoint → internal/cli
  internal/config/               # workspace.yaml + XDG state paths
  internal/git/                  # argv builder, exec, porcelain parsers
  internal/model/                # Slice, SliceMember, status enums
  internal/discovery/            # worktree → slice grouping
  internal/swap/                 # engine, journal, dep-reconcile
  internal/gt/                   # graphite stack reader (read-only)
  internal/tmuxctl/              # tmux session/window control
  internal/proc/                 # process sampler + kill
  internal/hooks/                # claude hook handler + init-hooks
  internal/notify/               # event store, desktop notify
  internal/summary/              # commit + claude -p summaries
  internal/tui/                  # bubbletea app + panes
  internal/cli/                  # cobra commands
  testutil/                      # temp-repo helpers (shared test scaffolding)
  ```
- **TDD everywhere.** Each task: failing test → run (fail) → minimal impl → run (pass) → commit. Commit messages end with the Co-Authored-By + Claude-Session trailers (see existing commit `c3a53a6`).
- **Test scaffolding for git:** `testutil.NewRepo(t)` creates a temp git repo with an initial commit; `testutil.AddWorktree(t, repo, branch)` creates a worktree on a new branch. Built in Task 3; reused throughout. Tests that need `git`/`tmux`/`gt` skip with `t.Skip` if the binary is absent (`exec.LookPath`).
- **No network in tests.** All git/gt fixtures are local temp repos or captured output strings.
- **Library API caveat:** Bubble Tea had a v2 type rename (`tea.KeyPressMsg`, `View() tea.View`). **Before writing any `internal/tui` task, pin the version in `go.mod` and verify the current API** via context7 (`mcp__plugin_compound-engineering_context7__query-docs` for charmbracelet/bubbletea) or the pinned version's godoc. Same for gopsutil signatures. Phases 0–4 below carry full code; phases 5+ give exact specs + representative code and require this API check at execution.
- **Safety rule (swap):** never `--force`, never blind `git stash pop`, never mutate a worktree, never write gt metadata. Assert these in tests.

---

## Phase 0 — Skeleton & CI

### Task 0: Initialise module, layout, Makefile, CI

**Files:**
- Create: `go.mod`, `cmd/slis/main.go`, `Makefile`, `.github/workflows/ci.yml`, `.golangci.yml`

**Step 1 — module + entrypoint**
```bash
cd /Users/jonathanomahony/personal/slis
go mod init github.com/jonnyomahony/slis   # <-- replace with real path
```
`cmd/slis/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "slis:", err)
		os.Exit(1)
	}
}

// run is replaced by cli.Execute in Task 17; stub keeps the binary buildable now.
func run(_ []string) error { return nil }
```

**Step 2 — Makefile**
```makefile
.PHONY: build test lint
build:
	CGO_ENABLED=0 go build -o slis ./cmd/slis
test:
	go test ./...
lint:
	golangci-lint run
```

**Step 3 — `.golangci.yml`** (minimal: govet, staticcheck, errcheck, ineffassign, gofmt). **`.github/workflows/ci.yml`**: matrix `{macos-latest, ubuntu-latest}`, steps = `actions/setup-go` (pin Go 1.26), `go test ./...`, `golangci-lint`, `CGO_ENABLED=0 go build`. Install `tmux` on the linux runner (`sudo apt-get install -y tmux`) so tmux tests run there.

**Step 4 — verify**
```bash
make build && ./slis && echo OK
```
Expected: builds, prints nothing, exits 0, prints `OK`.

**Step 5 — commit**
```bash
git add -A && git commit -m "chore: go module skeleton, Makefile, CI"
```

---

### Task 1: Config types + workspace loader

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

**Step 1 — failing test** (`config_test.go`):
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkspace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
  api: { primary: ~/code/api, default_branch: main }
grouping:
  strategy: branch-name
  strip_prefix: "jonny/"
sessions:
  autostart_claude: false
processes:
  cpu_warn_pct: 150
`), 0o644)

	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(ws.Repos))
	}
	if ws.Grouping.StripPrefix != "jonny/" {
		t.Errorf("strip_prefix = %q", ws.Grouping.StripPrefix)
	}
	if ws.Processes.CPUWarnPct != 150 {
		t.Errorf("cpu_warn_pct = %v", ws.Processes.CPUWarnPct)
	}
	// "~" must expand
	if got := ws.Repos["web"].Primary; got == "~/code/web" {
		t.Errorf("primary not expanded: %q", got)
	}
}
```

**Step 2 — run, expect FAIL** (`go test ./internal/config/` → undefined: LoadWorkspace).

**Step 3 — implement** (`config.go`): use `gopkg.in/yaml.v3`. Types `Workspace{Root string; Repos map[string]Repo; Grouping; Swap; Sessions; Notify; Processes}`, `Repo{Primary string; DefaultBranch string}`. `LoadWorkspace(path)`: read file, `yaml.Unmarshal`, expand `~` in `Root` and every `Primary` via `os.UserHomeDir`, default `cpu_warn_pct` to 150 and `grouping.strategy` to `branch-name` when zero. Return typed error if a repo `primary` is empty. Add `SaveWorkspace(path, ws)` (used by `slis init`, Task 2b) — marshal back to YAML, `MkdirAll` the config dir.
```bash
go get gopkg.in/yaml.v3
```

**Step 4 — run, expect PASS.**

**Step 5 — commit:** `feat(config): workspace.yaml loader with ~ expansion + defaults`

---

### Task 2: XDG state paths

**Files:** Create `internal/config/paths.go`, `internal/config/paths_test.go`

**Step 1 — test:** `StatePaths()` returns a struct with `Overrides`, `ActiveJournal`, `EventsDir` rooted at `$XDG_STATE_HOME/slis` (fallback `~/.local/state/slis`); honours `XDG_STATE_HOME` when set (set it to `t.TempDir()` and assert paths are under it). `EnsureDirs()` creates the dirs (assert they exist after).

**Step 2 — run, FAIL.**

**Step 3 — implement** using `os.Getenv("XDG_STATE_HOME")` with home fallback; `ConfigDir()` similarly for `workspace.yaml` (`$XDG_CONFIG_HOME/slis`). `EnsureDirs` = `os.MkdirAll` each.

**Step 4 — PASS. Step 5 — commit:** `feat(config): XDG state/config paths`

---

### Task 2b: `slis init` — repo scan + multi-select picker

> **Position:** foundational — do right after Task 2 (despite the "2b" label). Slice discovery (Task 7) and the whole TUI assume a `workspace.yaml` this task generates. Depends only on Tasks 1–2.

**Goal:** scan a project root for git repos, let the user pick the set every slice spans ("always duplicated"), write `workspace.yaml`.

**Files:** Create `internal/config/scan.go`, `internal/config/scan_test.go`, `internal/cli/init.go`

**Step 1 — failing test** (`scan_test.go`): `ScanRepos(root)` over a temp dir containing `root/a/.git`, `root/b/.git`, `root/c` (no `.git`), and a loose file → returns candidates `["a","b"]` (sorted, `c` excluded), each with `Name`, `Path`, and `DefaultBranch` detected via `git symbolic-ref refs/remotes/origin/HEAD` falling back to current branch then `"main"`.
```go
func TestScanReposFindsOnlyGitDirs(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "a"))     // helper: git init in subdir
	mustInitRepo(t, filepath.Join(root, "b"))
	os.MkdirAll(filepath.Join(root, "c"), 0o755)   // not a repo
	got, err := ScanRepos(root)
	if err != nil { t.Fatal(err) }
	names := []string{}
	for _, r := range got { names = append(names, r.Name) }
	if !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Fatalf("got %v", names)
	}
}
```

**Step 2 — run, FAIL** (undefined: ScanRepos).

**Step 3 — implement** `scan.go`: `Candidate{Name, Path, DefaultBranch string}`. `ScanRepos(root)` — `os.ReadDir(root)`, for each dir check `<dir>/.git` exists (`os.Stat`), detect default branch via `git.Run`. `BuildWorkspace(root, selected []Candidate) Workspace` — assemble `Workspace{Root: root, Repos: map[...]}`.

**Step 4 — run, PASS.**

**Step 5 — picker + command** (`internal/cli/init.go`): use **`charmbracelet/huh`** `NewMultiSelect[string]()` to present scanned repo names (no test for the interactive form itself — it requires a TTY; keep the form thin and the testable logic in `scan.go`/`BuildWorkspace`). Flow: `root := arg or cwd` → `ScanRepos` → if none, error with guidance → huh multiselect (all pre-checked except dirs named `infra`/`terraform`/`ops-*`? no — default none checked, let the user choose) → `BuildWorkspace` → `config.SaveWorkspace(ConfigDir()/workspace.yaml)` → print the written path + selected repos. Re-run reconciles: pre-check repos already in the existing `workspace.yaml`.
```bash
go get github.com/charmbracelet/huh
```

**Step 6 — manual verify** (interactive): `mkdir -p /tmp/demo/{x,y}/.git 2>/dev/null; (cd /tmp/demo/x && git init -q); (cd /tmp/demo/y && git init -q); ./slis init /tmp/demo` → picker lists `x`,`y` → select `x` → `workspace.yaml` written with only `x`.

**Step 7 — commit:** `git add -A && git commit -m "feat(init): repo scan + multi-select picker → workspace.yaml"`

---

## Phase 1 — git plumbing

### Task 3: git argv builder, exec wrapper, temp-repo testutil

**Files:** Create `internal/git/git.go`, `internal/git/git_test.go`, `testutil/repo.go`

**Step 1 — testutil** (`testutil/repo.go`) — not a test, the shared scaffolding:
```go
package testutil

import (
	"os/exec"
	"testing"
)

// NewRepo makes a temp git repo with one commit on `main`. Returns its path.
func NewRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

// AddWorktree creates `<repo>/../<branch-leaf>` worktree on a new branch.
func AddWorktree(t *testing.T, repo, branch, path string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
}
```

**Step 2 — failing test** (`git_test.go`): `TestArgvBuilderInjectionProof` — `NewCmd("switch").Arg("--detach").Arg(userInput).Argv()` returns a `[]string` (never a shell string); feeding `"; rm -rf /"` lands as one literal arg. `TestRunInDir` — against `testutil.NewRepo(t)`, `Run(repo, "rev-parse", "--abbrev-ref", "HEAD")` returns `"main"`.

**Step 3 — implement** (`git.go`):
```go
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Cmd struct{ args []string }

func NewCmd(sub string) *Cmd      { return &Cmd{args: []string{sub}} }
func (c *Cmd) Arg(a string) *Cmd  { c.args = append(c.args, a); return c }
func (c *Cmd) ArgIf(cond bool, a string) *Cmd {
	if cond { c.args = append(c.args, a) }
	return c
}
func (c *Cmd) Argv() []string { return append([]string(nil), c.args...) }

// Run executes `git -C dir <args...>` and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	return RunCtx(context.Background(), dir, args...)
}
func RunCtx(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
```

**Step 4 — PASS. Step 5 — commit:** `feat(git): argv builder, exec wrapper, temp-repo testutil`

---

### Task 4: worktree porcelain parser

**Files:** Create `internal/git/worktree.go`, `internal/git/worktree_test.go`

**Step 1 — test:** `ParseWorktreeList` over a captured `git worktree list --porcelain -z` byte slice (records `\0`-separated, double-`\0` between entries). Build the fixture from raw fields: a main entry `worktree <path>\0HEAD <sha>\0branch refs/heads/main\0\0` and a detached entry `worktree <p2>\0HEAD <sha>\0detached\0\0`. Assert: 2 entries; first `Branch=="main"`, `Detached==false`; second `Branch==""`, `Detached==true`. Also add `TestListWorktreesLive` using `testutil.NewRepo`+`AddWorktree`, calling the real git through `Run`, asserting ≥2 worktrees and the new branch present.

**Step 2 — FAIL.**

**Step 3 — implement:** `type Worktree struct{ Path, Branch, HeadSHA string; Detached, Bare, Locked, Prunable bool }`. `ParseWorktreeList([]byte) []Worktree` — split on `\x00\x00` for records, `\x00` for attrs; first attr is `worktree <path>`; `branch refs/heads/<x>` → strip prefix; presence-only booleans `bare`/`detached`/`locked`/`prunable` (label alone or with trailing reason). `ListWorktrees(dir)` = `Run(dir, "worktree", "list", "--porcelain", "-z")` → `ParseWorktreeList`.

**Step 4 — PASS. Step 5 — commit:** `feat(git): worktree --porcelain -z parser`

---

### Task 5: dirty-state check (status porcelain)

**Files:** Create `internal/git/status.go`, `internal/git/status_test.go`

**Step 1 — test:** `IsDirty(repo)` false on a clean `testutil.NewRepo`; write an untracked file → true; (`git status --porcelain -z` non-empty ⇒ dirty). `RevParse(repo, "HEAD")` returns a 40-char sha. `CurrentBranch(repo)` returns `"main"` on a normal checkout, `""` when detached.

**Step 2 — FAIL.**

**Step 3 — implement:** `IsDirty` = `Run(dir,"status","--porcelain","-z")` non-empty. `RevParse(dir, rev)`. `CurrentBranch(dir)` = `Run(dir,"symbolic-ref","--quiet","--short","HEAD")` (empty string + no error-treatment when detached: detect exit and return "").

**Step 4 — PASS. Step 5 — commit:** `feat(git): dirty check, rev-parse, current-branch`

---

## Phase 2 — discovery & model

### Task 6: slice model types

**Files:** Create `internal/model/model.go` (+ trivial `model_test.go` for enum String()).

**Step 1 — test:** `SessionStatus` enum (`None,Running,WaitingInput,Done`) has stable `String()`; `Slice.Repos()` returns member repo names sorted.

**Step 2 — FAIL. Step 3 — implement:**
```go
package model

type SessionStatus int
const (
	SessNone SessionStatus = iota
	SessRunning
	SessWaitingInput
	SessDone
)

type SliceMember struct {
	Repo, Branch, WorktreePath, TipSHA string
}
type Slice struct {
	Name, Base string
	Members    map[string]SliceMember // keyed by repo
	Active     bool                   // currently swapped into primary
}
```
`Repos()` → sorted keys.

**Step 4 — PASS. Step 5 — commit:** `feat(model): slice + member types`

---

### Task 7: discovery & branch-name grouping

**Files:** Create `internal/discovery/discovery.go`, `internal/discovery/discovery_test.go`

**Step 1 — test:** Build 3 temp repos via `testutil`; in `web` and `api` add a worktree on branch `jonny/checkout`, in `ops` add `jonny/other`. With `strip_prefix:"jonny/"`, `Discover(workspace)` yields a slice `"checkout"` with members `{web,api}` (not ops), and a slice `"other"` with `{ops}`. Assert member `WorktreePath` and `TipSHA` populated.

**Step 2 — FAIL.**

**Step 3 — implement:** for each repo in workspace, `git.ListWorktrees(primary)`, skip the primary worktree itself and detached/bare ones, derive slice key = `strings.TrimPrefix(branch, stripPrefix)`, bucket into `map[string]*model.Slice`, fill `TipSHA` via `git.RevParse(worktreePath,"HEAD")`. Return sorted `[]model.Slice`.

**Step 4 — PASS. Step 5 — commit:** `feat(discovery): branch-name worktree grouping`

---

### Task 8: overrides (apply + persist)

**Files:** Create `internal/discovery/overrides.go`, `internal/discovery/overrides_test.go`

**Step 1 — test:** Given discovered slices, applying an override that maps slice `checkout` → `{web:jonny/checkout, api:jonny/checkout-api}` regroups `api`'s differently-named worktree under `checkout`. `SaveOverrides`/`LoadOverrides` round-trip a struct to `overrides.yaml` in `t.TempDir()`.

**Step 2 — FAIL. Step 3 — implement:** `Overrides map[string]map[string]string` (slice→repo→branch). `Apply(slices, ov)` rebuilds membership. YAML load/save to `StatePaths().Overrides`.

**Step 4 — PASS. Step 5 — commit:** `feat(discovery): manual grouping overrides`

---

## Phase 3 — swap engine (heaviest tests — data safety)

### Task 9: swap journal (active.json)

**Files:** Create `internal/swap/journal.go`, `internal/swap/journal_test.go`

**Step 1 — test:** `Journal{Slice string; Repos []RepoState}` where `RepoState{Repo, Primary, PriorBranch, PriorSHA, StashRef, TargetSHA string; Reconciled bool}`. `Save`/`Load` round-trip via a path in `t.TempDir()`; `Load` of a missing file returns `(nil, nil)` (no active swap). `Clear` removes the file.

**Step 2 — FAIL. Step 3 — implement** JSON marshal to `StatePaths().ActiveJournal` (injectable path for tests).

**Step 4 — PASS. Step 5 — commit:** `feat(swap): activation journal`

---

### Task 10: lockfile hashing / dep-reconcile decision

**Files:** Create `internal/swap/deps.go`, `internal/swap/deps_test.go`

**Step 1 — test:** `LockfilesChanged(repoDir, fromSHA, toSHA, []lockfiles)` — in a temp repo, commit `pnpm-lock.yaml` v1 (fromSHA), commit v2 (toSHA) → true; same content → false; lockfile absent in both → false. (Read blob at each rev via `git show <sha>:<path>` through `git.Run`, hash, compare.)

**Step 2 — FAIL. Step 3 — implement** using `git show <sha>:<lockfile>` (treat "path not in rev" as empty), sha256 compare per lockfile, OR-reduce.

**Step 4 — PASS. Step 5 — commit:** `feat(swap): lockfile-change detection for dep-reconcile`

---

### Task 11: single-repo activate (detached-primary)

**Files:** Create `internal/swap/engine.go`, `internal/swap/engine_test.go`

**Step 1 — test** `TestActivateRepoDetachesPrimaryNotWorktree`:
- temp repo `r`; add worktree `wt` on branch `feat`; commit a file in `wt` so `feat` tip ≠ main.
- `st, err := activateRepo(RepoPlan{Primary:r, Branch:"feat", Stash:false})`
- assert: `r` HEAD is **detached** at `feat`'s tip (`git.CurrentBranch(r)==""` and `RevParse(r,"HEAD")==RevParse(wt,"HEAD")`); **`wt` is untouched** (`git.CurrentBranch(wt)=="feat"`, still has its branch); `st.PriorBranch=="main"`.
- `TestActivateRefusesDirtyWithoutStash`: make `r` dirty, `Stash:false` → error, HEAD unchanged.
- `TestActivateStashesDirty`: dirty + `Stash:true` → succeeds, `st.StashRef` set, `r` clean at target.

**Step 2 — FAIL.**

**Step 3 — implement** `activateRepo`:
1. `prior := git.CurrentBranch(primary)`; `priorSHA := git.RevParse(primary,"HEAD")`.
2. if `git.IsDirty(primary)`: if `!plan.Stash` → `return error("primary dirty; pass --stash")`; else `git.Run(primary,"stash","push","-u","-m", stashMsg)` and record `StashRef` (resolve to `stash@{0}`'s commit sha via `git rev-parse -q --verify stash@{0}` to pin it exactly).
3. `target := git.RevParse(primary, plan.Branch)` (resolves the worktree's branch tip from the shared object db).
4. `git.Run(primary,"switch","--detach", target)` — **commit, not branch → worktree keeps the branch.**
5. return `RepoState{...}`.
*Never* `--force`. The worktree is never addressed.

**Step 4 — PASS. Step 5 — commit:** `feat(swap): single-repo detached-primary activate`

---

### Task 12: single-repo deactivate (exact restore)

**Files:** Modify `internal/swap/engine.go`; add tests to `engine_test.go`

**Step 1 — test** `TestDeactivateRestoresExactly`: after `activateRepo` (clean case), `deactivateRepo(st)` returns `r` to branch `main` at `priorSHA`. With stash case: file edits reappear after deactivate; assert content matches pre-activate. `TestDeactivateStashConflictSurfaces`: force a conflicting change at target so pop conflicts → returns a typed `ErrStashConflict`, does **not** silently discard (stash still present).

**Step 2 — FAIL.**

**Step 3 — implement** `deactivateRepo`:
1. `git.Run(primary,"switch", st.PriorBranch)` (or `switch --detach st.PriorSHA` if prior was detached).
2. if `st.StashRef != ""`: find the matching stash entry by its pinned sha (`git stash list --format=%H`), `git stash pop <that index>`; on non-zero exit → `ErrStashConflict` (leave stash intact).

**Step 4 — PASS. Step 5 — commit:** `feat(swap): single-repo restore with exact stash round-trip`

---

### Task 13: multi-repo atomic activate/deactivate + dep-reconcile hook

**Files:** Modify `internal/swap/engine.go`; add `Activate`, `Deactivate`; tests.

**Step 1 — test** `TestActivateSliceAtomicRollback`: 3 temp repos in a slice; inject a failure on repo #3 (e.g. pass a non-existent branch for repo #3) → `Activate` returns error and repos #1,#2 are **rolled back** to their prior branches; no journal written. `TestActivateSliceWritesJournal`: success path writes a journal with 3 RepoStates. `TestDeactivateSliceClearsJournal`. `TestDepReconcileInvokesInstaller`: when `LockfilesChanged` true, the injected `installer func(repoDir string) error` is called; when false, not called. (Installer is an interface/func field so tests don't run pnpm.)

**Step 2 — FAIL.**

**Step 3 — implement** `Activate(slice, opts{Stash bool; Installer func(string) error; Reconcile bool})`:
- For each member: `activateRepo`; collect states. On first error, `deactivateRepo` all already-activated (best-effort) and return the error.
- After all switched: for each member, if `Reconcile && LockfilesChanged(...)` → call `Installer(primary)` (prompt is the caller's job in CLI/TUI; engine just invokes the injected func).
- `journal.Save(...)`. `Deactivate` loads journal, `deactivateRepo` each (continue past `ErrStashConflict`, aggregate), then `journal.Clear`.

**Step 4 — PASS. Step 5 — commit:** `feat(swap): atomic multi-repo activate/deactivate + dep-reconcile`

---

### Task 14: crash recovery + refresh

**Files:** Modify `internal/swap/engine.go`; tests.

**Step 1 — test:** `RecoverState()` reads the journal and returns the in-progress `*Journal` (or nil). `Refresh(slice)` re-resolves each member branch tip and `switch --detach`es primary to the new tip (simulate a new commit in the worktree after activate → `Refresh` moves primary forward; assert prior journal updated with new TargetSHA).

**Step 2 — FAIL. Step 3 — implement** `RecoverState` = `journal.Load`. `Refresh` = for active journal, re-`RevParse` each branch, `switch --detach`, update journal.

**Step 4 — PASS. Step 5 — commit:** `feat(swap): crash recovery + refresh-to-new-tip`

---

## Phase 4 — Graphite reader (read-only)

### Task 15: `gt state` JSON parser

**Files:** Create `internal/gt/gt.go`, `internal/gt/gt_test.go`, `internal/gt/testdata/state.json`

**Step 1 — test:** `ParseState([]byte)` over a captured `gt state` payload (commit a real sample to `testdata/`; if unavailable at execution, hand-author from design §9: array of `{name, trunk:bool, needs_restack:bool, parents:[{ref,sha}]}`). Assert trunk flagged, parent edges parsed, `needs_restack` surfaced. Add `StripBanner([]byte)` test: leading non-JSON banner lines are dropped before the `[`/`{`.

**Step 2 — FAIL.**

**Step 3 — implement** `StripBanner` (scan to first `[` or `{`), `ParseState` (`json.Unmarshal` into typed structs), and `ReadState(repoDir)` = run `gt state --no-interactive` via `exec` (skip test if `gt` absent), `StripBanner`, `ParseState`. Build a `Stack` view model: ordered branches with depth + status.

**Step 4 — PASS. Step 5 — commit:** `feat(gt): read-only gt state JSON parser`

---

### Task 16: ref-metadata fallback reader

**Files:** Create `internal/gt/refs.go`, `internal/gt/refs_test.go`

**Step 1 — test:** In a temp repo, write a `refs/branch-metadata/<branch>` blob (`git update-ref` to a `git hash-object -w` of the JSON `{"parentBranchName":"main","parentBranchRevision":"<sha>"}`); `ReadRefMetadata(repo)` returns the parent map. Pure git, zero writes by the reader.

**Step 2 — FAIL. Step 3 — implement** `for-each-ref refs/branch-metadata/** --format=%(refname)` then `git cat-file -p <ref>` → parse JSON → `map[branch]parent`. Wire `ReadStack(repo)` to try `gt state`, fall back to refs.

**Step 4 — PASS. Step 5 — commit:** `feat(gt): refs/branch-metadata fallback reader`

---

## Phase 5 — CLI twin (cobra)

> From here: specs + representative code. **Verify cobra/bubbletea/gopsutil APIs against pinned versions at execution.**

### Task 17: cobra root + `ls`/`show` with `--json`

**Files:** Create `internal/cli/root.go`, `internal/cli/ls.go`, `internal/cli/show.go`, tests; modify `cmd/slis/main.go` to call `cli.Execute()`.

**Steps (TDD):** Test `ls --json` against a fake workspace (3 temp repos + worktrees) → JSON array of slices with name/members/active. Implement `Execute()` wiring config load → discovery → render (table for humans, `encoding/json` for `--json`). `show <slice>` adds stack (Task 15) + members. Commit `feat(cli): root, ls, show with --json`.

### Task 18: `activate`/`deactivate`/`refresh`/`create` CLI

**Files:** `internal/cli/activate.go`, `deactivate.go`, `refresh.go`, `create.go`, tests.

**Steps:** `activate <slice> [--stash] [--no-reconcile]` → `swap.Activate` with an `Installer` that prompts then runs the configured install cmd. `create <slice>` = `git worktree add` across **all selected repos** (the "always duplicated" set from `workspace.yaml`), branch `<strip_prefix><slice>`, + tmux session (Task 23). Tests use temp repos; `create` test asserts a worktree appears in **every** selected repo. Commit per command.

---

## Phase 6 — TUI shell (Bubble Tea)

### Task 19: app skeleton + slice list

**Files:** `internal/tui/app.go`, `internal/tui/slicelist.go`, `internal/tui/app_test.go`

**Steps:** Pin bubbletea, **verify v2 API first**. Model holds `[]model.Slice`, focus index, a `refreshMsg`. `Init` dispatches a `Cmd` that runs discovery off-loop; `Update` handles `tea.KeyPressMsg` (j/k nav, q quit) and `refreshMsg`. Use `teatest` golden test for the list render + a unit test sending keys asserting focus moves. Commit `feat(tui): app skeleton + slice list`.

### Task 20: tabbed detail + data-driven keymap + help

**Files:** `internal/tui/detail.go`, `internal/tui/keys.go`, tests.

**Steps:** Tabs Stack/Summary/Changes/Sessions/Processes (lazy-load each tab's data via `Cmd`). Keymap as `[]Binding{Keys,Help,Handler}` → `?` renders help from the slice. Worker/UI discipline: all data fetches are `Cmd`s returning Msgs; never mutate model off-loop. Commit `feat(tui): tabbed detail + keymap + help`.

---

## Phase 7 — diff viewer

### Task 21: combined slice diff data

**Files:** `internal/diff/diff.go`, `internal/diff/diff_test.go`

**Steps:** `SliceDiff(slice, base)` → per repo: `git diff --numstat <base>...<branch>` for file/+/- summary and `git diff <base>...<branch>` for hunks. Test against temp repos with known edits → assert file counts and +/- numbers. Commit `feat(diff): combined per-slice diff data`.

### Task 22: diff TUI pane + open-external

**Files:** `internal/tui/diffpane.go`, tests.

**Steps:** Render hunks with `chroma` (verify API), file-tree nav, `[o]` opens `$EDITOR`/`lazygit` via `exec`, `[y]` writes combined patch to clipboard (shell `pbcopy` on darwin). teatest golden for a small diff. Commit `feat(tui): diff viewer pane`.

---

## Phase 8 — sessions (tmux)

### Task 23: tmux control

**Files:** `internal/tmuxctl/tmux.go`, `internal/tmuxctl/tmux_test.go`

**Steps (skip if no `tmux`):** `EnsureSession(slice, members)` → `tmux new-session -d -s slis/<name> -c <wt0>` then a window per repo (`new-window -c <wt>`); idempotent (`has-session`). `Attach` = `switch-client` if `$TMUX` set else exec `tmux attach`. `PanePIDs(slice)` = `list-panes -t ... -F '#{pane_pid}'`. `SessionExists`. Test against a real tmux server in CI (linux runner has tmux): create → assert `has-session` → kill. Commit `feat(tmux): session create/attach/list`.

### Task 24: session status into slice list

**Files:** modify `internal/tui/slicelist.go`, `internal/discovery` enrichment.

**Steps:** Combine tmux `SessionExists` + event-store status (Task 27) → badge enum (●/⏸/✓/○). Unit-test the badge resolver. Commit `feat(tui): session-status badges`.

---

## Phase 9 — processes

### Task 25: process sampler

**Files:** `internal/proc/proc.go`, `internal/proc/proc_test.go`

**Steps:** **Verify gopsutil API.** `SliceProcs(panePIDs)` → walk descendants (`process.Children()` recursively), read `CPUPercent`, `MemoryInfo`, `CreateTime`, `Cmdline`. Test: spawn `sh -c 'while :; do :; done'` as a child, find it in the tree, assert CPU>0 after a sample interval, then `Kill(pid)` and assert it exits. `KillSubtree(pid)` SIGKILLs descendants then root. Commit `feat(proc): per-pane process sampler + kill`.

### Task 26: processes view + global overlay

**Files:** `internal/tui/procpane.go`, `internal/tui/procoverlay.go`, tests.

**Steps:** Per-slice tab table sorted by CPU; global `[P]` overlay across all slices; ⚠ badge when subtree CPU > `cpu_warn_pct`; `[k]`/`[K]` kill with confirm. teatest golden. Commit `feat(tui): process view + CPU overlay + kill`.

---

## Phase 10 — notifications (Claude hooks)

### Task 27: hook handler + event store

**Files:** `internal/hooks/handler.go`, `internal/notify/events.go`, tests; `internal/cli/hook.go`.

**Steps:** `slis hook <event>` reads Claude hook JSON from stdin (`{session_id, cwd, ...}`), maps `cwd` → slice by matching against discovered worktree paths, appends `Event{Slice,Type,Time}` to `EventsDir`. Test: feed sample `Notification` JSON with a cwd inside a known worktree → assert event written with right slice + `WaitingInput`. `Stop` → `Done`. Commit `feat(hooks): hook handler + event store`.

### Task 28: `init-hooks` (opt-in, idempotent)

**Files:** `internal/hooks/install.go`, `internal/cli/inithooks.go`, tests.

**Steps:** With `HOME=t.TempDir()`, `InitHooks()` merges `Notification` + `Stop` hook entries (running `slis hook <event>`) into `~/.claude/settings.json` **without clobbering** existing hooks; idempotent (second run no-ops); prints what it changed; refuses silently — requires the explicit subcommand. Test the merge + idempotency. Commit `feat(hooks): opt-in init-hooks installer`.

### Task 29: event watch → badges + desktop notify

**Files:** `internal/notify/notify.go`, modify `internal/tui/app.go`, tests.

**Steps:** fsnotify watch on `EventsDir` → emits a `tea.Msg` updating slice badges; on `WaitingInput` fire desktop notification (`osascript -e 'display notification ...'` on darwin; no-op + log elsewhere) gated by config; tmux `display-message`. Test the notifier with an injected `runner` (assert command built, not executed). Commit `feat(notify): event watch + desktop notification`.

---

## Phase 11 — summary, skill, release

### Task 30: slice summary

**Files:** `internal/summary/summary.go`, tests; `internal/cli/summary.go`.

**Steps:** Default = `git log --format=%s <base>..<branch>` aggregated across members (test against temp repos). `--ai` = pipe combined diff to `claude -p "<prompt>"` (skip test if `claude` absent; unit-test prompt assembly + glamour render with a canned string). Commit `feat(summary): commit + claude -p summaries`.

### Task 31: Claude skill bundle

**Files:** `skill/slis/SKILL.md` (+ frontmatter), docs.

**Steps:** Author a skill documenting the CLI verbs and `slis init-hooks`, so Claude can create/activate slices and report processes. No test (docs); validate frontmatter shape. Commit `docs(skill): ship slis Claude skill`.

### Task 32: GoReleaser + Homebrew tap + README

**Files:** `.goreleaser.yaml`, `README.md`, `.github/workflows/release.yml`.

**Steps:** GoReleaser builds darwin/linux × amd64/arm64, `CGO_ENABLED=0`, archives + checksums + `brews:` block targeting a tap repo; release workflow on tag. `goreleaser check` in CI. README = quickstart (`workspace.yaml`, `slis init-hooks`, key verbs). Commit `chore(release): goreleaser + homebrew tap + README`.

---

## Build order / dependencies

```
0 → 1 → 2 → 2b                  (2b: slis init — generates workspace.yaml; foundational)
2b,3 → 4,5
4 → 7 → 8                       (7 discovers over the selected repos from 2b)
3 → 9,10,11 → 12 → 13 → 14      (swap chain)
3 → 15 → 16                     (gt)
2b,7,15 → 17 → 18               (18 create spans ALL selected repos)
17 → 19 → 20 → 22               (tui)
21 → 22
23 → 24
25 → 26
27 → 28 ; 27 → 29
7 → 30
all → 31,32
```

Phases 3 (swap) and 9/10 (proc/hooks) are independent of the TUI and can land first behind the CLI, keeping every commit shippable and green.
