# Building a lazygit-style multi-repo worktree + Graphite TUI — Research Report

Research-only report. No project code written. Scope: a solo-dev TUI that navigates git worktrees
across **multiple repos**, models a feature as a "slice" spanning N repos, integrates with the
Graphite `gt` CLI for stacked branches, and can "swap" which branch is active in a designated
main dev directory.

---

## 1. TUI framework landscape & architecture

### 1.1 Rust: ratatui (+ crossterm)

ratatui is **immediate-mode**: you own all application state and rebuild/re-render the entire UI
from that state every frame; ratatui diffs buffers and writes only changed cells, so "redraw
everything" is cheap. Blocking the loop freezes the UI, so slow work (git/`gt`) must run off-loop
(`tokio::process` or a worker thread). crossterm is the default cross-platform backend; ratatui
0.30+ split it into a `ratatui-crossterm` crate to avoid version conflicts.

- Rendering / immediate mode: https://ratatui.rs/concepts/rendering/
- Backends: https://ratatui.rs/concepts/backends/

Canonical loop is **draw -> poll event -> map event to Action(s) -> apply Action(s) to state**,
multiplexing crossterm's `EventStream` with `tokio::select!` (separate tick and frame rates):
https://ratatui.rs/recipes/apps/terminal-and-event-handler/

**Official Component architecture** co-locates event handling + update + draw per component, wired
through an `mpsc::unbounded_channel::<Action>()`; the `App` holds `Vec<Box<dyn Component>>` and runs
`handle_events -> handle_actions -> draw`, broadcasting each Action to every component. The `Action`
enum derives Serialize/Deserialize so config-file keybindings map to action names.
- Concept: https://ratatui.rs/concepts/application-patterns/component-architecture/
- Template: https://ratatui.rs/templates/component/ and https://github.com/ratatui/templates/tree/main/component

**Real apps:**
- **gitui** — closest reference for this project (lazygit alternative on ratatui+crossterm, separate
  `asyncgit` crate so git never blocks the UI). Its own `Component` trait splits `commands()` /
  `event()` / focus / visibility; key dispatch walks the component list until one returns
  `EventState::Consumed`; the active tab is a `usize` index where show/hide == focus;
  cross-component messaging via `Queue = Rc<RefCell<VecDeque<InternalEvent>>>` with `NeedsUpdate`
  bitflags. https://github.com/extrawurst/gitui (also https://github.com/gitui-org/gitui)
- **ATAC** — Postman-style client; alternative focus model: one big `AppState` enum where the
  current variant gates which keybindings apply. https://github.com/Julien-cpsn/ATAC
- Showcase / ecosystem: https://ratatui.rs/showcase/apps/ , https://github.com/ratatui/awesome-ratatui

Two dominant focus patterns: (a) gitui/template — list of components, dispatch key down the list,
focused one consumes; (b) ATAC/bottom — explicit active-pane enum gates keybindings. For a lazygit
clone, **gitui is the closest reference implementation**.

Learning-curve hazards: immediate-mode mental model + the borrow checker fighting "components
mutate shared state" (the reason gitui uses an `Rc<RefCell<VecDeque>>` queue and the template uses
an mpsc action channel — both route cross-component effects through messages, not direct mutation),
plus async discipline.

Distribution: single static binary via `x86_64-unknown-linux-musl`; cross-compile with `cross`
(Docker-based, https://github.com/cross-rs/cross). Momentum (2026): ratatui ~21k stars, v0.30.2
(June 2026), the maintained successor to archived `tui-rs`.

### 1.2 Go: Bubble Tea / Bubbles / Lipgloss (Charm) — Elm Architecture

> Version caveat: Charm shipped a **v2** line (2025/2026) renaming core types — v2 uses
> `tea.KeyPressMsg` and `View() tea.View`; v1 (what nearly all existing apps target) uses
> `tea.KeyMsg` and `View() string`.

Bubble Tea is The Elm Architecture: `Init() Cmd`, `Update(Msg) (Model, Cmd)`, `View() string`.
State is threaded immutably through `Update`'s return. `Msg` is any event; `Cmd func() Msg` is an IO
operation that returns a message. The three rules: use commands for all I/O, only for I/O, never use
goroutines. `(*Program).Send(msg)` bridges external goroutines in. Async: `tea.Batch`,
`tea.Sequence`, `tea.Tick`. Resize arrives as `tea.WindowSizeMsg` (the hook for responsive panes).
- https://github.com/charmbracelet/bubbletea , https://pkg.go.dev/github.com/charmbracelet/bubbletea
- Commands blog: https://charm.land/blog/commands-in-bubbletea/

**Bubbles** = reusable components (list, table, viewport, textinput, key, help...), each itself a
Model that a parent embeds and delegates to (returning concrete types to avoid type assertions):
https://github.com/charmbracelet/bubbles
**Lipgloss** = CSS-like styling; the load-bearing layout fns are `JoinHorizontal` / `JoinVertical`
(+ `Place`, `Width`/`Height`) to assemble panes from styled boxes:
https://github.com/charmbracelet/lipgloss

**Multi-pane:** no built-in pane manager. Converged recipe: parent holds child models + a focus
marker (`int`/enum); route `KeyMsg` only to the focused child, **broadcast `WindowSizeMsg` to all**;
compose with Lipgloss `Join*` in `View`. Canonical demo: `split-editors`
(https://github.com/charmbracelet/bubbletea/tree/main/examples). Real exemplar: **gh-dash** (~12k
stars) — shared `ProgramContext`, one package per pane, top model holds `currSectionId` focus marker:
https://github.com/dlvhdr/gh-dash (UI under `internal/tui/`). The "composition problem": TEA is
verbose for dense static panel grids (each pane = field + Update branch + View branch); you hand-roll
the router. (Note: k9s uses tview/tcell, not Bubble Tea.)

### 1.3 gocui (what lazygit actually uses)

gocui (Jesse Duffield's fork, on tcell) is imperative/immediate: Views are addressable rectangles
implementing `io.ReadWriter` (you `Fprintln` to draw); a layout-manager callback repositions views
each frame; keybindings are **per-view**; focus is explicit (`SetCurrentView`). For a fixed grid of
always-visible panels with one focused, it maps ~1:1. https://github.com/jesseduffield/gocui

Why lazygit chose it (primary source, issue #2705): needs very high information density / custom
compact widgets; at the time no substantial Charm exemplar existed; gocui is easy to control and
fork. https://github.com/jesseduffield/lazygit/issues/2705 (Note: lazygit now vendors gocui in-tree
at `pkg/gocui` on tcell/v3.)

### 1.4 Comparison & recommendation

| Dimension | Rust + ratatui | Go + Bubble Tea | Go + gocui |
|---|---|---|---|
| Multi-pane UI ease | Medium (gitui pattern; fight borrow checker) | Medium-low (composition problem; hand-roll router) | **High for the lazygit shape** |
| Single binary | Excellent (static musl) | Excellent (CGO-free) | Excellent |
| Cross-compile | Good (`cross`/Docker) | **Best (GOOS/GOARCH)** | Best |
| Shell out + parse | `tokio::process` off-loop; manual | `os/exec`; **proven by lazygit/gh-dash** | Same; lazygit `pkg/commands` is the reference |
| Momentum 2026 | ratatui 21k, gitui 22k | bubbletea 43k + active v2 ecosystem | gocui 325 (niche) but lazygit 79.5k proves it |
| Learning curve | Steepest | Moderate | Gentlest for panels |

**Recommendation: pick Go.** The app is fundamentally CLI-orchestration (shell out to `git`/`gt`,
parse, reflect state) — Go's `os/exec` story is the most proven path in existence (lazygit, gh-dash
are readable references), which de-risks the hardest part. For a solo dev, time-to-prototype and
learning curve favor Go heavily over Rust+ratatui (borrow checker + immediate-mode + async at once).

Between the two Go options:
- **gocui** is the lowest-friction renderer for a *true* lazygit clone (fixed panel grid, one focus),
  and you get lazygit's entire `pkg/gui` as a structural blueprint.
- **Bubble Tea** if you want the larger/livelier ecosystem (Bubbles, Lipgloss, far more momentum),
  expect modal/wizard UI, and value a clean testable state model — accepting the composition
  boilerplate. gh-dash proves a serious multi-pane Charm app is viable.

Reach for **Rust + ratatui (gitui as reference)** only if a hard requirement (max perf, existing Rust
codebase, deliberate learning) outweighs the slower solo ramp. Distribution is a wash either way
(single static binary; GoReleaser https://goreleaser.com for Homebrew/Scoop).

**Concrete starting move:** read lazygit's `pkg/gui`
(https://github.com/jesseduffield/lazygit/tree/master/pkg/gui) and gh-dash's `internal/tui`
(https://github.com/dlvhdr/gh-dash/tree/main/internal/tui). Mirror lazygit's structure on gocui if it
fits; otherwise build in Bubble Tea using gh-dash as template.

---

## 2. How lazygit / gitui / lazydocker are structured (lessons to copy)

### 2.1 Panes / views — lazygit's Context system

lazygit decouples **View** (gocui rectangle that renders text), **Context** (logical mode bound to a
view; owns focus + keybindings), **Controller** (handlers). One view can host different contexts.
- Context interface: https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/types/context.go
  (`HandleFocus`/`HandleFocusLost`/`HandleRender`...; `IBaseContext` has identity;
  `ContextKind` enum: `SIDE_CONTEXT`, `MAIN_CONTEXT`, `PERSISTENT_POPUP`, `TEMPORARY_POPUP`,
  `DISPLAY_CONTEXT`).
- Concrete contexts (one per panel) + **trait/mixin composition** (base, list_context_trait,
  list_renderer, search_trait, filtered_list, history_trait, view_trait):
  https://github.com/jesseduffield/lazygit/tree/master/pkg/gui/context
- **Focus == a context stack** managed by `ContextMgr` (`Push`/`Pop`/`Replace`/`Activate`):
  https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/context.go . Push behavior depends on
  `ContextKind`; Escape semantics fall out of pop rules. Capability interfaces
  (`IListContext`, `ISearchableContext`...) let generic code act on whatever is focused.
- **Presentation layer** = pure model->rows formatting, heavily unit-tested:
  https://github.com/jesseduffield/lazygit/tree/master/pkg/gui/presentation (commits.go, branches.go,
  files.go, graph/...).
- lazydocker mirrors the skeleton but with a flatter older "panels" model (copy lazygit's contexts,
  not lazydocker's panels): https://github.com/jesseduffield/lazydocker/tree/master/pkg/gui
- gitui uses a Component tree (`src/components/`, `src/tabs/`) instead; its standout is `asyncgit`.

### 2.2 Global keymap

Two-tier, assembled in https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/keybindings.go .
`types.Binding{ViewName, Keys, Handler, Description, Tooltip, GetDisabledReason, ...}`; `ViewName==""`
== global, else context-specific. `GetInitialKeybindings` folds in each context's own bindings. Keys
come from **config** (`opts.Config.Universal.*`), not hardcoded — data-driven rebinding. Bindings
carry Description/Tooltip (auto-generate the help menu + i18n) and `GetDisabledReason`
(present-but-disabled-with-explanation UX). Per-context bindings live in **Controllers** (~60 files,
each `GetKeybindings(opts)`); a `baseController` provides no-op defaults so concrete controllers
override only what they need. https://github.com/jesseduffield/lazygit/tree/master/pkg/gui/controllers
(wired via `AttachControllers` in .../controllers.go).

### 2.3 Shelling out to git vs using a library

**lazygit shells out** to the `git` binary (https://github.com/jesseduffield/lazygit/tree/master/pkg/commands):
- argv builder, never a shell string -> injection-proof:
  `NewGitCmd("commit").Arg().ArgIf(cond,...).ToArgv()`:
  https://github.com/jesseduffield/lazygit/blob/master/pkg/commands/git_commands/git_command_builder.go
- execution wraps `os/exec` (`Run`, `RunWithOutput`, `RunAndProcessLines` streaming; PTY +
  credential-prompt detection for push/pull):
  https://github.com/jesseduffield/lazygit/blob/master/pkg/commands/oscommands/cmd_obj_runner.go
- **safe porcelain parse**: `git status --porcelain -z ...` then split on `\x00`, handling the R/C
  two-record rename case:
  https://github.com/jesseduffield/lazygit/blob/master/pkg/commands/git_commands/file_loader.go

**gitui uses libgit2 via git2-rs** (no parsing; maps Delta/Status bitflags):
https://github.com/gitui-org/gitui/blob/master/asyncgit/src/sync/status.rs

Tradeoff: shell-out gives exact parity with the user's git (aliases, hooks, config, credential
helpers, signing) and natural interactivity, at the cost of careful version-sensitive parsing;
libgit2 gives structured data and speed but diverges from user git behavior and needs FFI/feature
reimplementation. For a tool prioritizing **no surprises vs the user's git**, shell-out is the safer
default. (lazydocker is a pragmatic hybrid: Docker SDK for structured calls, shell-out for
`docker compose`.)

### 2.4 State refresh — never block the UI thread

lazygit: scoped, on-demand refresh
(https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/controllers/helpers/refresh_helper.go).
`Refresh(opts)` takes a **Scope** (`COMMITS, BRANCHES, FILES, STASH, WORKTREES, STATUS, ...`) so a
commit reloads only commits+files; mode `SYNC|ASYNC|BLOCK_UI`. Discipline: slow git work on a worker
(`OnWorker`), all view mutation back on the UI thread (`OnUIThread`)
(https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/types/common.go). Mostly on-demand
(after operations / on focus), not polling.

gitui: async jobs + channel notifications, event loop multiplexes input + git results + filesystem
watcher with crossbeam `Select` (https://github.com/gitui-org/gitui/blob/master/src/main.rs). The
`AsyncSingleJob` (https://github.com/gitui-org/gitui/blob/master/asyncgit/src/asyncjob/mod.rs) is an
at-most-one-pending queue that overwrites stale requests; `AsyncStatus`
(https://github.com/gitui-org/gitui/blob/master/asyncgit/src/status.rs) **hashes request params** to
dedupe/skip identical work — excellent debouncing patterns for snappy navigation.

### 2.5 Top lessons to copy

1. Separate View / Context / Controller / Presentation.
2. Model focus as a context stack keyed by kind (Escape/popups fall out for free).
3. Compose contexts from traits/mixins, not inheritance.
4. Keybindings = data, attached to contexts, sourced from user config; carry description/tooltip/
   disabled-reason (free help menu + i18n).
5. Build git commands as an argv builder, never a shell string.
6. Parse porcelain with `--porcelain -z`, split on `\x00`, handle R/C rename records.
7. One rule: slow work on a worker, all view mutation on the UI thread.
8. Refresh by explicit scope, on-demand, not blanket polling.
9. Debounce/dedupe async work (hash params; at-most-one-pending job).
10. Consider a filesystem watcher for external changes; mix library + shell-out pragmatically.

---

## 3. Git worktree manipulation (incl. the risky "swap the main dir" workflow)

### 3.1 Core commands & porcelain parsing

Official docs: https://git-scm.com/docs/git-worktree

- `remove` only succeeds on a clean worktree; needs single `--force` if unclean, double `-f -f` if
  locked. `lock --reason <str>` blocks prune/remove/move (writes `locked` under
  `$GIT_DIR/worktrees/<id>/`). Deleting a worktree dir out-of-band leaves admin files until `prune`
  (shows as `prunable`). After a manual dir move the gitdir pointer breaks — use `git worktree move`
  or follow a raw move with `git worktree repair`.
- **Same-branch rule (hard invariant):** git refuses to check out a branch already checked out in
  another worktree (HEAD/index race). Tools either refuse+redirect to the existing worktree, or use
  `--detach` to inspect the same commit without owning the ref. `--force` overrides but reintroduces
  the race.
- **`git worktree list --porcelain`** = one attribute per line, records separated by a blank line,
  first attribute always `worktree <path>`. Fields: `HEAD <40-hex>`, `branch refs/heads/<name>`
  (only when attached), and **presence-only booleans** `bare`, `detached`, `locked` (optional
  trailing reason), `prunable` (optional trailing reason). Parsing rules: booleans are the label
  alone when true and absent when false (don't parse as key/value); records end at a blank line; the
  last record ends at EOF. **Always use `--porcelain -z`** (records separated by an extra NUL, i.e.
  `\0\0`; fields by `\0`) to avoid quoting ambiguity and survive paths/reasons with spaces/newlines.

### 3.2 The risky workflow: make the MAIN dir reflect worktree X without N dev servers

Two facts drive everything:
- Dev servers / file watchers (Vite, webpack, Next, nodemon; inotify/FSEvents/kqueue) **bind to a
  path/inode at startup**. Anything that replaces directory contents under a running process ->
  stale watches / missed events / watcher pinned to an orphaned inode.
- `node_modules` and build caches (`.next/`, `dist/`, `.vite/`, tsbuildinfo) are **branch state**;
  swapping branch in place without reconciling them yields source/deps mismatch.

**(a) `git checkout`/`git switch` in the MAIN dir** — inode unchanged (watchers stay valid, but a
big checkout fires a change storm -> rebuild thrash). Git refuses to overwrite uncommitted *tracked*
changes (safe against tracked-file loss) — but **untracked files silently carry across** to the new
branch, and `--force`/`--discard-changes` *will* destroy work (a tool must never auto-force). Shared
`node_modules` is wrong after a lock-file change. Verdict: safe vs tracked loss, operationally noisy,
silently wrong for deps/caches.

**(b) Symlink swapping (MAIN is a symlink retargeted at different worktree dirs)** — **safest at the
git/data layer**: you never mutate any working tree, each branch's dirty state stays isolated, ~zero
git data-loss from the swap. BUT the risk moves to the FS layer: Node resolves/caches real paths, so
a server/watcher started against the symlink **keeps watching the old target after retarget** — you
must restart the server bound to the stable path. (Reported: "Vitest and Vite both choked on the
symlinked paths" — Node module resolution follows symlinks and gets confused.) Do **not** symlink
`node_modules` into Node's resolution path; instead use pnpm global virtual store or CoW. Verdict:
safest data-wise; gives "retarget -> restart server at the stable path," not true no-restart.

**(c) `git worktree move`** — **wrong tool**; relocates a worktree, doesn't change which branch shows
at a fixed path; moving a dir out from under a running server is maximally disruptive (invalidated
handles/inotify). Use only to relocate idle worktrees.

**(d) stash + branch switch** — conventional safe in-place switch when dirty, but **real, underrated
data-loss**: default `git stash` does NOT include untracked files (use `-u`); `stash pop` can
conflict and leave markers + an undropped stash; stashes are an unlabeled stack, not backups; auto-
stashing hides work. Same inode-stable watcher behavior as (a) but two churn events. Verdict: safe
only with `-u`, a named stash per branch, and a pop-conflict plan; a quiet data-loss path otherwise.

**(e) Just `cd` into the worktree and run servers there** — genuinely safest (no overwrite, full
isolation, no stale watches, no restart-to-swap), at the price of one server per active worktree
(the cost the user wanted to avoid). The mainstream "switch terminals, not branches" pattern;
`gwq` adds tmux integration for warm per-worktree processes.

### 3.3 Recommendation & guardrails

No mechanism swaps content under a *running* server without a restart (servers/watchers pin
paths/inodes at startup). Choose by constraint:
1. **If a brief restart is acceptable (recommended): symlink-retarget model (b).** MAIN = stable
   symlink; background worktrees = full checkouts; swap = atomic retarget (`ln -sfn` to a temp link +
   rename) then restart the MAIN server. Keep `node_modules` cheap with **pnpm
   `enableGlobalVirtualStore: true`** (content-addressable store; per-worktree installs ~instant;
   trust-boundary caveat) or CoW `node_modules`.
2. **If the dir must be physically stable and you accept watcher churn:** in-place `git switch` (a)
   with strict guardrails, falling back to named-stash (d) only when dirty; reconcile deps when the
   lock file differs.
Avoid (c) for swapping. Use (e) if isolation matters more than server count.

Guardrails a tool should enforce:
1. Never swap a dirty MAIN silently — check `git status --porcelain` (empty == clean); refuse unless
   explicit `--stash`/`--force`.
2. Auto-stash must be named + complete: `git stash push -u -m "tool:auto:<branch>:<ts>"`; record the
   ref; restore the exact stash to the exact branch (never blind `pop`).
3. Never auto-`--force` checkout/switch.
4. Detect same-branch-elsewhere before acting (parse `worktree list --porcelain -z`); refuse +
   redirect, or offer `--detach`.
5. Handle detached HEAD explicitly (no `branch` line; new commits only via reflog) — warn / offer to
   create a branch; block prune/remove of a detached worktree ahead of any ref.
6. Lock the MAIN worktree while a server is attached (`git worktree lock --reason "dev server"`);
   unlock on detach.
7. Quiesce processes around the swap (stop -> retarget -> restart for symlinks; pause/resume watcher
   for in-place).
8. Reconcile deps on swap only when the lock file changed.
9. Use `git worktree move` / `repair`, never raw `mv`.
10. Parse porcelain defensively (`-z`; presence-only booleans; optional trailing reasons).

Prior art: gwq (https://github.com/d-kuro/gwq), wtp (https://github.com/satococoa/wtp), git-wt
(https://github.com/k1LoW/git-wt), pnpm git-worktrees (https://pnpm.io/git-worktrees), Bill Mill's
CoW node_modules (https://notes.billmill.org/blog/2024/03/How_I_use_git_worktrees.html), Dave
Schumaker's symlink/Vite pitfalls
(https://daveschumaker.net/use-git-worktrees-they-said-itll-be-fun-they-said/).

---

## 4. Graphite `gt` integration

> Two big caveats: the original `withgraphite/graphite-cli` GitHub repo was **deleted** (CLI went
> closed-source — historical issues are gone), and `gt` **migrated storage from git refs to SQLite in
> v1.8.0** (2026-03-02), so behavior differs sharply pre/post-1.8. Findings below are marked
> live-confirmed (tested on a real v1.6.1 install), docs/source-confirmed, or uncertain.

### 4.1 Reading the stack programmatically

| Command | Alias | Machine-readable? |
|---|---|---|
| `gt log` | — | No (decorative ASCII; shows worktree paths in 1.7.20+) |
| `gt log short` | `gt ls` | No (compact graph) |
| `gt info [branch]` | `gt branch info` | No (human text; shows `Parent:`) |
| **`gt state`** | — | **YES — strict JSON of the whole tracked stack** |

`gt state` (live-confirmed) emits per branch: `trunk`, `needs_restack`, and `parents[].ref` +
`parents[].sha` (the full DAG). **But it is undocumented/unsupported** — absent from `gt --help --all`
and from https://graphite.com/docs/command-reference . Treat as internal: pin the `gt` version, parse
defensively, and strip banner lines printed before the JSON. Always pass `--no-interactive` (and
maybe `-q`) when scripting.

There is **no `--json` flag** on `gt log`/`ls`/`info`. The only structured stdout is `gt state`.

**On-disk metadata (live-confirmed, pre-1.8 refs backend), inside the repo `.git/`:**
- Git refs `refs/branch-metadata/<branch>` -> JSON blob `{ "parentBranchName": "...",
  "parentBranchRevision": "<sha>" }` — parent/child is an **upward pointer**; derive children by
  inverting. Read with pure git: `git for-each-ref refs/branch-metadata/` + `git cat-file -p <ref>`.
- `.git/.graphite_repo_config` (trunk config), `.git/.graphite_cache_persist` (plain JSON precomputed
  graph with `sha` validity key + `branches` array including **children arrays** and
  `validationResult: TRUNK|VALID`), `.git/.graphite_pr_info` (PR cache).
- Global/user (XDG, not `~/.graphite`): `~/.local/share/graphite/` (`user_config` incl. authToken,
  `aliases`, `feature_flags`, JSONL debug logs under `debug/`).
- **v1.8.0+ SQLite backend (uncertain):** exact DB filename / shared-vs-per-worktree location not
  confirmed. Do NOT hardcode the old ref/cache paths if supporting >=1.8; prefer `gt state` (abstracts
  the backend) if you accept the undocumented-command risk.

Current branch = plain `git rev-parse --abbrev-ref HEAD` (gt uses git HEAD; current branch is the
filled marker in `gt ls`).

### 4.2 Not breaking gt's metadata

What corrupts it (docs: https://github.com/withgraphite/docs/blob/main/guides/graphite-cli/mixing-gt-and-git.md):
simple `git` updates are safe; `git rebase -i` on a branch's own commits is safe **as long as it
keeps the base commit**; a vanilla `git rebase` that **removes the base commit** untracks the branch
and its children (gt anchors on the base commit). Raw `git branch -m`/`-D` drift the metadata.
Repairs: `gt track` / `gt downstack track`, `gt branch edit`, `gt sync`/`gt restack`,
`gt dev cache --clear` (safe, cache-only), `gt init --reset` (nuclear). (https://graphite.com/docs/troubleshooting)

**A read-only integrating tool should:** never run mutating `gt` commands
(create/modify/restack/sync/submit/track/untrack/move/fold/branch edit/init); read via `gt state`
(version-pinned), or — for a zero-write guarantee — read `refs/branch-metadata/*` blobs /
`.graphite_cache_persist` directly with plain git (these cannot corrupt anything); never write the
refs/`.graphite_*` files; avoid raw rebase/branch ops on tracked branches. The load-bearing piece to
preserve is the parent pointer (`parentBranchName` + `parentBranchRevision` = the base commit).

### 4.3 gt + worktrees

Works inside worktrees, with a rocky history (changelog: https://graphite.com/docs/cli-changelog):
experimental support v0.18.7 (2022); update branches across worktrees v1.4.5 (2024); worktree paths
in `gt log` v1.7.20 (2026-02); **v1.8.4 (2026-04-13): commands "only affect the current worktree."**

**Metadata is shared across worktrees** (live-confirmed, refs backend): `refs/branch-metadata/*` live
in the shared common `.git`, visible identically from every worktree — so a branch's stack is
repo-global regardless of which worktree you query; only "which branch is current" is per-worktree
(git HEAD). (For SQLite 1.8+, shared-vs-per-worktree is uncertain, but the v1.8.4 change makes the
*logical behavior* worktree-scoped.)

Pre-1.8.4 core gotcha: because metadata + ops were global, gt would try to restack/checkout branches
checked out in another worktree, hitting `fatal: '<branch>' is already checked out at '<path>'`;
v1.8.4 made `gt get`/`gt sync`/`gt restack` skip non-trunk branches checked out elsewhere. Community
data-loss caveat: be cautious running `gt sync` with unstaged changes on the main worktree
(https://blog.matte.fyi/posts/git-worktrees-with-graphite/). **Recommendation:** require `gt >= 1.8.4`
if you operate near worktrees, else stay strictly read-only.

Refs/sources: https://graphite.com/docs/command-reference , https://graphite.com/docs/cli-changelog ,
https://graphite.com/docs/troubleshooting , preserved fork
https://github.com/searleser97/graphite-cli (`src/wrapper-classes/metadata_ref.ts` confirms
`git update-ref refs/branch-metadata/<branch>`).

---

## 5. Multi-repo "slice" modeling — prior art

Two camps: **command broadcasters** (config is just a repo list; `checkout feature` is broadcast to
all; branch state lives only in each `.git`) and **manifest/snapshot tools** (config pins a revision
per repo; the manifest itself is a slice, but one global slice, no worktrees, no concurrent named
slices).

- **meta** — `.meta` JSON `{ignore, projects: {relpath: url}}`; broadcast `meta git checkout`; no
  branch/slice model; `--parallel`. https://github.com/mateodelnorte/meta
- **mu-repo** — `.mu_repo` flat `name=value`; **partial-name checkout** (`mu co v1.2` matches
  `*v1.2*`); **groups** (switchable named repo subsets — the most slice-adjacent broadcaster idea);
  parallel by default. https://github.com/fabioz/mu-repo
- **gita** — CSV `repos.csv` + groups + `cmds.json` custom subcommands; **async by default**; strength
  is the **branch x repo status matrix** (`gita ll`, color-coded sync/ahead/behind). No slice object.
  https://github.com/nosarthur/gita
- **Google `repo`** — XML `manifest.xml`; **per-`<project>` pinned `revision`** (branch/tag/SHA),
  most-specific-wins; URL derived `${remote.fetch}/${name}.git`; first-class groups (`-g`),
  `notdefault`; `repo start <branch>` creates a topic branch across projects; parallel `sync -j`.
  **Strongest prior art** — the manifest *is* a slice. Limits: one global slice per manifest (concurrent
  slices need separate manifest files/branches); no worktree concept.
  https://gerrit.googlesource.com/git-repo/ ,
  https://gerrit.googlesource.com/git-repo/+/refs/heads/main/docs/manifest-format.md
- **mani** — YAML `mani.yaml` with **projects / specs / targets / tasks** (cleanest separation of
  what/where/how); per-project `branch` pin; tags + `tags_expr` filtering.
  https://github.com/alajmo/mani , https://github.com/alajmo/mani/blob/main/docs/config.md
- **gws** — minimal `.projects.gws` (`path | url`); inspection/broadcast only; folder-hierarchy
  grouping; `update` never deletes (safe). https://github.com/StreakyCobra/gws
- **vcstool** — YAML `.repos` (`type/url/version`); VCS-agnostic; **`vcs export --exact` snapshots
  live SHAs into a reproducible manifest** (capture slice from reality). https://github.com/dirk-thomas/vcstool
- **myrepos (`mr`)** — `.mrconfig` INI of shell snippets; powerful but config==scripts, no
  declarative slice. https://myrepos.branchable.com/
- **git submodules** — `.gitmodules` + gitlink (mode 160000) pins exact SHA per submodule, versioned
  with the parent; but detached-HEAD by default and clumsy two-step (commit in submodule, then bump
  pointer) for active cross-repo feature work. https://git-scm.com/docs/git-submodule

**Steal:** explicit per-repo revision pinning over branch-name broadcasting (`repo`/vcstool/
submodules); derive URLs from named remote + name; separate what/where/how (mani); snapshot live
state to exact SHA (vcstool); first-class groups/tags; the branch x repo status matrix (gita); safe
sync (gws never deletes). **What they all miss (your opportunity):** no named, concurrent slices (one
implicit global slice each), and **no worktree dimension** — none model "repo R, on branch B,
materialized at worktree path P." That worktree axis is genuinely novel here.

---

## 6. Recommendations for this project

### 6.1 Language / framework
**Go.** The core risk is shelling out to `git`/`gt` and parsing output, where Go has the most proven
references (lazygit `pkg/commands`, gh-dash). For a solo dev it minimizes time-to-prototype and
learning curve vs Rust+ratatui. For UI: **gocui** if you're building a true lazygit-shaped fixed
panel grid (clone lazygit's `pkg/gui` structure); **Bubble Tea + Bubbles + Lipgloss** if you want the
livelier ecosystem and more modal/wizard UI (use gh-dash as template, accept the composition
boilerplate). Only choose Rust+ratatui (gitui as reference) if a hard requirement justifies the
slower ramp. Architecture to copy regardless: View/Context/Controller/Presentation split; focus as a
context stack; data-driven keybindings with description/disabled-reason; argv command builder;
`--porcelain -z` parsing; slow-work-on-worker / mutate-on-UI-thread; scoped on-demand refresh; param-
hashing + at-most-one-pending job debouncing; optional filesystem watcher. Ship a single static binary
via GoReleaser.

### 6.2 Safest swap mechanism
No swap is restart-free for a *running* server. Default to **MAIN as a stable retargetable symlink
over isolated per-branch worktrees, with a stop -> atomic-retarget -> restart swap**, and keep
`node_modules` cheap via pnpm `enableGlobalVirtualStore` (or CoW). Provide in-place `git switch` as an
alternative when the dir must be physically stable, behind guardrails. Enforce: clean-check
(`git status --porcelain`) before any swap; named complete auto-stash (`-u -m`) with exact restore;
never auto-`--force`; detect same-branch-elsewhere; explicit detached-HEAD handling; lock MAIN while a
server is attached; quiesce processes around the swap; reconcile deps only on lock-file change; use
`git worktree move`/`repair` not raw `mv`. If "one server only" is not actually a hard constraint, the
*safest* design is one warm server per worktree ("switch terminals, not branches," gwq-style).

### 6.3 Config / data model for multi-repo slices
**YAML, two layers** (mani/vcstool ergonomics; far more humane than `repo`'s XML):
- **Workspace topology** (stable): `remotes` (URL prefix; derive `${remote}/${repo}.git`) + `repos`
  (path, remote, default_branch).
- **Slices** (many named, concurrent): each slice has `description`, `base`, and `members` — a
  **many-to-many join** of `(slice, repo) -> {branch, worktree_path, revision?, status?}`.

Model it as: `Slice(id, name, description, base)`, `Repo(id, path, remote, default_branch)`,
`SliceMember(slice_id, repo_id, branch, worktree_path, revision?, status?)` — one `SliceMember` per
(slice, repo) pair. Properties this buys you that prior art lacks: per-repo branch names may differ
(make branch explicit); partial slices (a repo absent from `members` stays on mainline); **worktree
as a first-class attribute of the join** (the same repo+branch can materialize at different worktrees
per slice; `git worktree add` is the materialization primitive — one main clone per repo + N
worktrees keyed by slice); operations filter by slice (mani-style target, run across members in
parallel); snapshot/restore by resolving+storing `revision` per member (vcstool `--exact`); a
gita-style branch x repo status matrix scoped to the slice. In one line: take `repo`'s per-project
pinned-revision manifest, parameterize it by a **named slice**, borrow mani's YAML + task/target/spec
ergonomics, add vcstool's snapshot-to-exact-SHA, and introduce the one axis none model — a
**worktree attribute on the (slice, repo) join**.

Example sketch:
```yaml
# workspace.yaml
remotes: { origin: git@github.com:acme }
repos:
  web:    { path: services/web, remote: origin, default_branch: main }
  api:    { path: services/api, remote: origin, default_branch: main }
  shared: { path: libs/shared,  remote: origin, default_branch: main }

# slices.yaml
slices:
  checkout-revamp:
    description: "New checkout flow"
    base: main
    members:
      web: { branch: feat/checkout-revamp,     worktree: ~/wt/checkout-revamp/web, revision: a1b2c3d }
      api: { branch: feat/checkout-revamp-api, worktree: ~/wt/checkout-revamp/api }
      # 'shared' absent -> stays on default branch
```

Note the natural alignment with Graphite: a Graphite stack is itself a slice across branches *within*
one repo; your slice generalizes that to branches *across* repos. Read each repo's stack read-only via
`gt state` (version-pinned) or direct `refs/branch-metadata` reads, and remember gt metadata is
repo-global/shared across worktrees while "current branch" is per-worktree.
