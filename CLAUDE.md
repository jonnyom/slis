# CLAUDE.md — slis

Guidance for Claude agents working in this repo. Read this first.

## What slis is

`slis` ("slice", Irish for *a slice*) is a **multi-repo worktree cockpit**: a lazygit-style TUI **and** a scriptable CLI. The unit of work is a **slice** — a feature's git worktrees across *several* repos, treated as one named unit.

Core capabilities:
- **Discover** slices (worktrees grouped by branch name across the configured repos).
- **Swap** a slice into the repos' *primary* checkouts so running dev servers rebuild that feature — by putting each primary on a `slis/live/<slice>` branch at the slice branch's tip commit (reversible; worktrees never touched).
- **Review** the whole-slice diff, read the **Graphite** stack (read-only), and generate commit / `claude -p` summaries.
- **tmux sessions** per slice (attach/detach), **process** view + kill (find CPU hogs), and **"Claude needs input"** notifications via Claude Code hooks.
- **GitHub PRs** over the stack: per-branch PR link, CI status, comment counts, shareable markdown, and `fix-ci` (points Claude at failing CI).

Module: `github.com/jonnyom/slis`. Single static binary. Entry point: `cmd/slis/main.go` → `internal/cli.Execute()`. Running bare `slis` (no subcommand) launches the TUI.

Design + the full phased build plan live in `docs/plans/2026-06-22-slis-*`. A Claude skill for driving slis is in `skills/slis/SKILL.md`.

## Commands (use these exactly)

```sh
# Build (static)
CGO_ENABLED=0 go build -o slis ./cmd/slis

# Test (all packages)
go test ./...

# Lint — IMPORTANT: install golangci-lint with THIS repo's Go toolchain.
# A prebuilt golangci-lint binary refuses our go.mod directive if it was built
# with an older Go than the directive. Installing via `go install` makes it match.
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
"$(go env GOPATH)/bin/golangci-lint" run ./...

# Format check
gofmt -l .

# Run the TUI
go run ./cmd/slis            # or: ./slis

# Exercise the CLI (agent-friendly; --json on all read commands)
./slis ls --json
./slis status --json   # which slice's Claude is waiting-input?
```

CI (`.github/workflows/ci.yml`) runs build + test + lint on ubuntu & macos. **Green CI is the bar.** CI configures a global git identity (tests create throwaway repos) and installs golangci-lint via `go install` for the reason above. Lowering the `go.mod` `go` directive to satisfy a prebuilt linter will break the build (deps require ≥ the current directive) — don't; fix lint by matching the linter's build toolchain instead.

## Package map (`internal/`)

| Package | Responsibility |
|---|---|
| `config` | `workspace.yaml` load/save, XDG paths, repo scan for `slis init` |
| `git` | injection-proof argv builder, `Run`, porcelain parsers, dirty/rev-parse/current-branch |
| `model` | `Slice`, `SliceMember`, `SessionStatus` |
| `discovery` | group worktrees → slices by branch name (+ manual overrides) |
| `swap` | **the data-safety-critical engine** — activate/deactivate/refresh, journal, dep-reconcile |
| `gt` | **read-only** Graphite stack reader (`gt state` JSON + refs fallback) |
| `tmuxctl` | per-slice tmux session create/attach/list, pane PIDs, `CapturePane`. Window layout via `SessionOpts` — `sessions.layout: root` (default when a workspace root is set; one window at root to run Claude across the stack) / `repos` (one per worktree) / `both` |
| `proc` | process tree sampler (gopsutil) + kill |
| `hooks` | Claude Code hook handler (`slis hook`) + `init-hooks` installer |
| `notify` | per-slice status event store + desktop notification + fsnotify watch |
| `summary` | commit summary + `claude -p` AI summary (glamour render) |
| `forge` | **read-only** `gh` wrapper: PR info, CI status, comments, stack markdown |
| `diff` | combined per-slice diff (numstat + patch); `SliceDiff`/`SliceStat`/`CommitSummary` take `base=""` to auto-detect each repo's trunk |
| `cleanup` | remove a finished slice: `git worktree remove` (refuses dirty unless force) + optional `git branch -d` (merged-only unless force) + kill tmux session. `slis rm` (TUI `d`) |
| `restack` | run `gt restack` across a slice's repos (refuses dirty worktrees; conflicts left for the user). `slis restack` + CLI-level `gt` wrappers `slis submit` (stack→PRs) / `slis merge` (Graphite server-side queue) / `slis sync` (TUI `R` → stack-actions overlay) |
| `tui` | Bubble Tea app — two levels: **browser** (slice cards: repos/stack-health/PR/session/overview) and **cockpit** (lazygit-style stacked left panels Stack/PRs/Session/Processes whose focus drives a big right pane) + overlays. `app.go` routes; `slicelist.go`=browser, `cockpit.go`=cockpit, `detail.go`=stack loader+styles |
| `cli` | cobra commands; `Execute()`; bare `slis` launches the TUI |
| (`testutil`) | shared test scaffolding: temp git repos + worktrees |

## Non-negotiable conventions

**Swap engine (`internal/swap`) — data safety.** This manipulates real repos with uncommitted user work. Invariants, enforced by tests:
- NEVER a force git switch, `-B`/`-C`, or `git stash drop`/`clear`; never run git against a *worktree* dir — only the *primary*. `git branch -D` is used in exactly one place: deleting the temp branch, and only after re-verifying its tip still equals the journal's `TargetSHA` immediately before the delete (provably nothing to lose).
- Activate = `git switch -c slis/live/<slice> <commit-sha>` (create-only `-c`) — a real, named temp branch at the slice branch's tip, so Graphite stays usable in the primary, an accidental commit is never orphaned, and the worktree's branch checkout is never contended. A pre-existing `slis/live/<slice>` → refuse with zero state change (doctor cleans it). Deactivate deletes the temp branch (clean) or, under `--force` after commits, *renames* it to `slis/rescue/<slice>-<repo>` (never deletes). Refresh fast-forwards the temp branch (`merge --ff-only`, never reset). Legacy detached-HEAD journals (no temp branch) still restore via the detached path.
- Dirty primary + no `--stash` → error with zero state change.
- Stash is pinned by commit SHA (and message) and popped by that exact entry; pop conflict → `ErrStashConflict`, stash left intact.
- Journal is written incrementally; multi-repo activate is atomic (rollback on partial failure deletes each just-created temp branch and restores the prior branch); deactivate only clears the journal when every repo restored cleanly. If you change this engine, keep the heavy tests green and prefer adding adversarial tests.

**TUI (`internal/tui`).**
- MUST NOT import `internal/cli` (cli imports tui to launch it — importing back is a cycle).
- All slow work (git, gh, gt, proc, tmux) runs inside `tea.Cmd` closures, never in `Update`/`View`. `View` must be pure.
- Bubble Tea **v1** API (`tea.KeyMsg`, `Update(tea.Msg)(tea.Model,tea.Cmd)`, `View() string`).
- **Do not render markdown with glamour's `WithAutoStyle` inside the running program** — it queries the terminal (OSC background detection), which blocks forever under Bubble Tea and hangs the tab. Use `summary.RenderMarkdownFixed` (fixed style, no query).

**Detection & grouping.** Discovery lists each repo's **linked worktrees** (skipping the primary checkout, detached/bare/branch-less) and groups them into slices keyed by **branch name** minus `strip_prefix`. So every slice row is a real worktree; the heuristic is the *grouping by branch name*. It breaks when one feature spans repos under different branch names (→ separate slices) or when unrelated work shares a name (→ false merge). Fix manually: `slis group <name> <slice>...` / `slis ungroup <name>` (TUI: `space` to select, `m` to group, `u` to ungroup) — these write `overrides.yaml` (`slice→repo→branch`), applied by `discovery.Apply` over the auto-grouping (one branch per repo per group).

**Per-repo trunk.** A slice spans repos with *different* trunks (one on `master`, another on `main`), so there is no single slice-wide base. `git.DetectBase(worktree)` resolves each repo's trunk (origin/HEAD → main/master/develop/trunk → fallback). Diff/summary callers pass `base=""` to auto-detect per repo; `model.Slice.Base` is an optional whole-slice override only (left empty by discovery). The cockpit Stack panel scopes to the member branch's lineage via `gt.State.Lineage(branch)` — never the whole repo's branch list.

**Integrations.** `forge` is read-only (`gh pr view` only); `forge.PRForBranch` returns `(nil,nil)` when there's no PR or `gh` is absent — callers tolerate per-repo failures. `gt` is read-only *except* `gt.Restack` (the one mutator), always run behind a TUI confirm / explicit `slis restack`; the `internal/restack` engine refuses dirty worktrees and never auto-stashes/aborts (conflicts are left for the user to resolve + `gt continue`). `slis sync` / `slis submit` / `slis merge` shell out to interactive `gt` per repo (CLI-level, not via the `gt` package), sharing the `gtPerRepo` helper. `merge` triggers Graphite's server-side merge queue (`gt merge`) so slis doesn't babysit the squash/merge/restack locally; `sync` is repo-wide (may overwrite trunk, delete merged branches); `submit` force-pushes the stack + opens/updates PRs.

**Tests.** TDD. Tests that need external tools (`git` always; `gh`/`gt`/`tmux`/`claude` optionally) must `t.Skip` when the tool is absent. Create repos via `testutil.NewRepo` (it sets *local* git identity so commits — including in linked worktrees — work on machines/CI with no global git config). `CGO_ENABLED=0` must keep building (deps are pure-Go; don't introduce cgo).

**Agent-native.** Every TUI action has a non-interactive CLI twin; **every read command** (`ls`/`show`/`status`/`pr`/`pr-stack`/`summary`/`conflicts`/`comments`/`doctor`) supports `--json`. Keep that invariant — any new read command ships with `--json`. `slis status [slice] --json` exposes per-slice Claude session state (none/running/waiting-input/done) from the `notify` event store so agents poll "which slice needs input" without reading raw files. The agent contract (JSON shapes, session-status flow, mutation map, error model) lives in `docs/AGENT.md`; the driving skill in `skills/slis/SKILL.md`. Update both when the surface changes.

## Gotchas / environment

- Homebrew install is a **prebuilt cask** in `jonnyom/homebrew-tap` (`brew install jonnyom/homebrew-tap/slis`), published by GoReleaser (the `homebrew_casks` block in `.goreleaser.yaml`) on every `vX.Y.Z` tag. The release workflow needs a `HOMEBREW_TAP_GITHUB_TOKEN` repo secret (PAT with write access to the tap) to push the cask. The binary is unsigned, so the cask's `postflight` strips the macOS quarantine flag. (Cut a release by tagging: `git tag vX.Y.Z && git push origin vX.Y.Z`.)
- `gh` must be authenticated for `forge`/PR features; if a shell's keyring auth is flaky, `export GH_TOKEN="$(gh auth token)"` before running.
- `slis init [root] --repos a,b,c --strip-prefix jonny/` writes `workspace.yaml`; slices are then auto-discovered.
