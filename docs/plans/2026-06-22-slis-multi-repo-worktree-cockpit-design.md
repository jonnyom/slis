# slis — Multi-Repo Worktree Cockpit (Design)

- **Date:** 2026-06-22
- **Status:** Approved — ready for implementation planning
- **Author:** Jonathan O'Mahony (jonny@nory.ai), with Claude
- **Name:** `slis` — Irish (Gaeilge) for *a slice / sliver*. The tool's unit of work is a **slice**: one feature materialised across three repos.

---

## 1. Problem & Goals

I work across **three separate repositories** (three codebases, three deployment flows), stack branches with **Graphite** (`gt`), and use **git worktrees** heavily — often created on the fly by `claude --worktree`, Conductor, or by hand. The result:

- I lose track of what's open and what I'm working on.
- Worktrees pile up in the background and are hard to actually *test*.
- A unit of work almost always spans **all three repos**, but the tools treat each repo (and each worktree) in isolation.
- Background agents (Claude) silently run tests in worktrees that peg my CPU, and I can't easily see which one.
- I can't tell at a glance which Claude session is blocked waiting for my input.

**Goal:** a lazygit-style terminal cockpit — plus a scriptable CLI twin — that treats a **slice** (a feature's worktrees across all three repos) as the first-class unit, and lets me:

1. **See my work** — all slices, their branches, status, and which Claude sessions are live.
2. **Review a slice as a whole** — a built-in diff across all three repos.
3. **See the Graphite stack** per repo, read-only, before handing work to teammates.
4. **Swap a slice into the primary tree** so my already-running dev servers rebuild *that* slice's code — without spinning up a second set of servers.
5. **Move between work** by attaching/detaching to each slice's Claude session (home-base).
6. **See CPU-intensive processes** per slice and kill runaways.
7. **Get notified when Claude needs input** (questions, approvals, idle-waiting).

---

## 2. Core Concepts

| Concept | Definition |
|---|---|
| **Repo** | One of the three codebases. Each has a fixed **primary tree** (where dev servers run, layout "A") plus N worktrees. |
| **Primary tree** | The repo's main working directory. Servers/watchers are pinned here. `slis` makes it temporarily reflect a slice's committed code; never the destination of long-term work. |
| **Worktree** | A `git worktree` of a repo. `slis` **discovers** these; it does not own their creation. The place real work (and Claude) happens. |
| **Slice** | A named bucket grouping **one worktree per repo** for the same feature. May be **partial** (a repo with no matching branch simply stays on its mainline). The novel unit no prior tool models. |
| **Session** | A **tmux** session per slice (`slis/<name>`), one window per repo (cwd = that repo's worktree), where Claude runs. The substrate for attach/detach, process visibility, and status. |

A Graphite stack is a slice of branches *within* one repo; `slis` generalises that to branches *across* repos.

---

## 3. Non-Goals / Out of Scope (v1)

- **No `gt` mutations** from the TUI (restack/submit/sync/create/track). Read-only. You run `gt` inside the worktrees yourself.
- **No "full mirror" swap** (uncommitted changes). Swap is **committed-only** by deliberate choice — fits a Graphite/stack flow where you commit constantly.
- **No per-worktree server farm.** The whole point is to reuse the running primary servers.
- **No PR creation, no CI integration.**
- **No symlink-based swap by default** (see §7 — it's a documented fallback, not the default).
- Config supports N repos, but the UX is tuned for three.

---

## 4. Architecture Overview

Three surfaces, one core:

```
            ┌──────────────────────────────────────────────┐
            │                core (Go)                       │
            │  discovery · slice model · swap engine ·       │
            │  gt-state reader · git argv builder ·          │
            │  tmux ctl · process sampler · event store      │
            └──────────────────────────────────────────────┘
              ▲                  ▲                    ▲
              │                  │                    │
   ┌──────────┴───────┐  ┌───────┴────────┐  ┌────────┴─────────┐
   │  TUI (Bubble Tea)│  │ CLI twin (cobra)│  │ hook handler      │
   │  interactive     │  │ slis ls/show/…  │  │ slis hook <event> │
   │  home-base       │  │ --json for      │  │ (Claude Notif/Stop│
   │                  │  │  agents         │  │  → event store)   │
   └──────────────────┘  └─────────────────┘  └───────────────────┘
```

- **Language:** **Go**. Rationale (and independently confirmed by research): the app is fundamentally CLI-orchestration (shell out to `git`/`gt`/`tmux`/`claude`, parse, reflect state), and Go's `os/exec` story is the most proven that exists (lazygit, gh-dash are readable references). For an **open-source, easy-to-build** tool, Go is the *easier* choice — seconds-long builds, one small toolchain, **pure-Go deps → fully static cross-platform binaries with `CGO_ENABLED=0`**, GoReleaser + Homebrew tap in one config. Lowest barrier for contributors and users.
- **TUI framework:** **Bubble Tea** (+ Bubbles, Lipgloss). Our UI is modal/tabbed (slice list + tabbed detail) rather than lazygit's dense fixed grid, so the Charm ecosystem fits — and `glamour` (markdown for summaries) and `chroma` (syntax highlight for diffs) come from the same stable. `gh-dash` is the multi-pane reference to copy. (Alternative considered: `gocui`, what lazygit uses — better for a dense fixed grid; not needed here.)
- **CLI twin:** every TUI action has a non-interactive equivalent (cobra), with `--json` for clean agent parsing → **agent-native**.
- **Hook handler:** a tiny `slis hook <event>` subcommand that Claude Code hooks pipe into; it maps the event's `cwd` → slice and appends to the event store.

### Architectural patterns to copy (from lazygit / gitui / gh-dash)
- **View / Context / Controller / Presentation** split; **context-stack focus** (push/pop → Escape & popup semantics fall out).
- **Data-driven keybindings** (binding = keys + handler + description + disabled-reason → free help menu).
- **Git argv builder** (`NewGitCmd("switch").Arg(...).ToArgv()`) — injection-proof, never string-concatenate shell.
- **Porcelain parsing**: `git worktree list --porcelain -z` / `git status --porcelain -z`, split on NUL.
- **Worker/UI-thread discipline**: all slow work (git, gt, ps, tmux) runs off the UI loop via Bubble Tea `Cmd func() Msg` (never raw goroutines mutating state); view mutation only on UI thread.
- **Scoped, on-demand refresh** (not polling) + **param-hash dedupe / at-most-one-pending job** per scope (gitui's debounce) for snappy navigation. A filesystem watcher (fsnotify) catches external changes (new worktrees, hook events).

---

## 5. Data Model & Config

Two layers, YAML. **Workspace topology** is authored once; **slices are auto-discovered** at runtime, with only manual overrides persisted.

```yaml
# ~/.config/slis/workspace.yaml  (authored once)
repos:
  web: { primary: ~/code/web, default_branch: main }
  api: { primary: ~/code/api, default_branch: main }
  ops: { primary: ~/code/ops, default_branch: main }

grouping:
  strategy: branch-name        # auto-group worktrees whose branch matches across repos
  strip_prefix: "jonny/"       # jonny/checkout-revamp  →  slice "checkout-revamp"

swap:
  # per-repo lockfile(s) to hash for dep-reconcile, + install command to run on change
  dep_reconcile:
    web: { lockfiles: [pnpm-lock.yaml], install: "pnpm install" }
    api: { lockfiles: [poetry.lock],    install: "poetry install" }
  post_activate: ""            # optional shell hook after a successful swap

sessions:
  autostart_claude: false      # plain shells by default; true = launch claude per repo window
notify:
  needs_input: { desktop: true,  tui: true }
  done:        { desktop: false, tui: true }
processes:
  cpu_warn_pct: 150            # badge a slice whose subtree sustains > this %CPU
```

```yaml
# ~/.local/state/slis/overrides.yaml  (written by the TUI; only when you correct the heuristic)
overrides:
  checkout-revamp:             # force this grouping
    web: jonny/checkout-revamp
    api: jonny/checkout-api    # different branch name on api
```

**In-memory model** (derived each run, the research's many-to-many join + worktree axis):

```
Slice { name, base, members: map[repo] -> SliceMember }
SliceMember { repo, branch, worktree_path, tip_sha, stack_status, session_status }
ActiveState { slice, per-repo: { prior_branch_or_sha, stash_ref, target_sha, reconciled } }  // journalled
```

State/journal lives under `~/.local/state/slis/` (XDG): `overrides.yaml`, `active.json` (swap journal), `events/` (hook events).

---

## 6. Slice Discovery & Grouping

On launch (and on fsnotify change), for each repo:

1. `git -C <primary> worktree list --porcelain -z` → enumerate worktrees + their branches.
2. Bucket worktrees into slices by **branch name** with `strip_prefix` applied (default heuristic).
3. Apply **overrides** (merge/split/attach-stray/rename) from `overrides.yaml`.

This automatically picks up worktrees created by `claude --worktree`, Conductor, or by hand — `slis` owns nothing. Manual fixes in the TUI are persisted to `overrides.yaml`. A slice can include only 1–2 repos.

---

## 7. Swap Engine — *the crux* (detached-primary, committed-only, reversible)

**Goal:** make a repo's primary tree reflect a slice's **committed** code so the running watchers rebuild — **without a restart** and **without ever mutating the worktree** (so Claude keeps running there on its real branch).

**Why "detached-primary" and not the obvious approaches** (validated by research):
- A branch checked out in a worktree **cannot** be checked out a second time in the primary — git refuses. But git **allows checking out the *commit*** that a worktree has at its branch tip. So we put the **primary** into detached HEAD at that commit; the **worktree is untouched**.
- A **symlink swap** is safest at the git-data layer but **forces a server restart** (Node resolves real paths; a running server keeps serving the old target). That defeats the core goal. Kept only as a documented fallback if a specific watcher ever chokes on in-place switching.
- In-place switch keeps the **inode stable** → watchers survive and see a change-storm → that *is* the rebuild we want.

### Activate slice S (atomic across all repos)
For each repo member:
1. **Preflight** — `git -C <primary> status --porcelain -z`. If dirty and `--stash` not chosen, **abort** (never auto-force, never blind data loss). Resolve `target_sha = git rev-parse <member.branch>`.
2. **Save** — record primary's current branch (or detached SHA). If dirty + opted in: `git -C <primary> stash push -u -m "slis:auto:<slice>:<ts>"` (captures **and removes** tracked + untracked — this also resolves the "untracked carryover" hazard).
3. **Dep-reconcile check** — hash configured lockfile(s) at current HEAD vs `target_sha`. Flag if changed.
4. **Switch** — `git -C <primary> switch --detach <target_sha>`. Files change on disk → watchers rebuild.
5. **Reconcile deps** — if a lockfile changed, **prompt** to run the configured install command in the primary (never silent).
6. **Journal** — append the per-repo `{prior_branch_or_sha, stash_ref, target_sha, reconciled}` to `active.json`.

**Atomicity:** if any step fails on repo N, roll back repos already switched (restore them per below). **Crash recovery:** on startup, if `active.json` shows a half-applied or stale swap, offer to complete the restore.

### Deactivate / "push back" (restore)
1. `git -C <primary> switch <prior_branch>` (or restore the detached prior SHA).
2. If a stash was recorded: pop **that exact stash ref** (never a blind `stash pop`); on conflict, stop and surface it — don't guess.
3. If a lockfile differs from the restored state, offer dep-reconcile again.
4. Clear the journal record.

### Notes & honest caveats
- **Committed-only.** A worktree's uncommitted edits do **not** appear in primary. The TUI badges an active slice so you know to **commit in the worktree** first.
- **No auto-follow.** If the agent adds commits in the worktree while the slice is active, primary won't move — **`[r]efresh`** re-checks-out the new tip. Explicit by design.
- **`node_modules`/build cache is branch state** — handled by the dep-reconcile step (lockfile-hash gated).
- Only **one slice active at a time** in v1 (a slice may span all three primaries; activating swaps all of them together).

---

## 8. Diff Viewer (built-in, library-backed)

A built-in, scrollable diff for the whole slice — **not** hand-rolled:
- **Overview**: per-repo file list with +/- counts (vs each branch's base/parent).
- **Inline diff**: `chroma` for syntax highlighting; unified hunks; file-tree navigation across all three repos.
- `[o]` opens the full diff in your pager / `lazygit` / `$EDITOR`; `[y]` copies a combined patch.
- Source via the git argv builder; diff against the member branch's base (the slice `base`, default `main`, or gt parent when available).

---

## 9. Graphite Stack (read-only)

Show the current stack per repo before handing work to teammates. **Read-only — never mutate gt metadata.**

- **Primary read path:** `gt state --no-interactive` → strict JSON (full DAG: `trunk`, `needs_restack`, `parents[].ref` + `sha`). Research **live-confirmed on `gt 1.6.1`** (the installed version). Pin the gt version, strip banner lines before the JSON.
- **Fallback read path:** `refs/branch-metadata/<branch>` JSON via pure git (`for-each-ref` + `cat-file -p`) — zero-write, works on the refs backend (pre-1.8; current install).
- **Render:** per repo, the stack tree with current branch, parents/children, `needs_restack`, and PR/submit status where available.
- **Worktree note:** gt metadata is **shared across worktrees** (refs in the common `.git`); only current-branch is per-worktree. Read-only, so the pre-1.8.4 "operates on branches in other worktrees" hazard does not apply.

> **Uncertainty:** `gt state` is undocumented/unsupported and gt moved storage to SQLite in v1.8.0 — pin the version; revisit the read path if upgrading past 1.8.

---

## 10. Slice Summary

The "is this ready for teammates" glance, per slice:
- **Default (instant, free):** aggregated commit subjects across the slice's branches.
- **`[s]` on demand:** shell to `claude -p` over the combined diff → richer markdown summary, rendered with `glamour`. Cached per slice+tip. No AI cost unless asked; no hard dependency on `claude` for the default view.

---

## 11. Sessions — tmux home-base (attach/detach)

The tool owns Claude sessions in **tmux**, turning the TUI into a switchboard.

- **One tmux session per slice** (`slis/<name>`), **one window per repo** (cwd = that repo's worktree). Switch *slices* = switch tmux sessions; switch *repos within a slice* = switch tmux windows.
- **Create** — `[c]` / `slis create <name>` spins the worktrees (the "vertical split" across three repos) **and** the tmux session. Windows are plain shells by default; `claude` auto-launched per window only if `sessions.autostart_claude: true`.
- **Attach** — `[a]`: if the TUI runs inside tmux → `tmux switch-client` (instant); if standalone → suspend TUI, `tmux attach`, return on detach (`prefix-d`).
- **Status** (combined with §13): ● running · ⏸ waiting-for-input · ✓ done · ○ no session — shown in the slice list.
- Sessions persist across TUI restarts and exist independently of the tool.

---

## 12. Processes — CPU view + kill

Because each slice's work runs in its tmux session, processes attribute precisely to a slice:

- For each slice: `tmux list-panes -t slis/<name> -F '#{pane_pid}'` → walk the descendant process tree → read **CPU% / mem / elapsed / command** via **gopsutil** (pure-Go → keeps the static binary; CPU% needs delta sampling, which gopsutil handles). `ps` fallback.
- **Per-slice Processes tab** + a **global `[P]` overlay sorted by CPU** — when the machine crawls, find *which slice* and *which command* (e.g. `vitest` pegging 8 cores) in one keystroke.
- **Auto ⚠ badge** on any slice whose subtree exceeds `processes.cpu_warn_pct`.
- **`[k]` kill** selected process (SIGTERM); **`[K]`** SIGKILL whole subtree (confirm) — for forked test runners.
- **Caveat (documented):** fully-detached/daemonised processes that leave the pane's process tree won't be attributed. The common test-hog case (direct child of the pane) is caught.

---

## 13. Notifications — "Claude needs input"

Robust signal, **not** screen-scraping: **Claude Code hooks**.

- `slis init-hooks` (**opt-in, with consent** — it edits `~/.claude/settings.json`; never silent) installs:
  - a **`Notification`** hook — fires for questions / permission / approval prompts **and** when Claude is idle waiting on you;
  - a **`Stop`** hook — turn done.
- Each hook pipes Claude's JSON to **`slis hook <event>`**; the handler maps the event's **`cwd` → slice** (via the worktree path — no tmux needed for the mapping) and appends to `~/.local/state/slis/events/`.
- The TUI watches the event store (fsnotify) and:
  - sets the slice-list badge (⏸ waiting-for-input / ● running / ✓ done / ○ none);
  - fires a **desktop notification** on waiting-for-input (macOS `osascript`, zero-dep; configurable) + a tmux `display-message`;
  - **Enter on the badge → jump straight into that slice's session.**
- Shipped in the Claude skill so setup is one command for others.

---

## 14. TUI Layout & Navigation (Bubble Tea, keyboard-first)

```
┌ Slices ─────────────────┐┌ feature-x ──────────────────────────────────────┐
│> checkout-revamp ● ⏸     ││ [Stack] Summary  Changes  Sessions  Processes     │
│  refactor-auth     ● ⚠   ││ Stack (web)   main                                │
│  spike-billing     ○     ││   └ jonny/base-x        ● submitted PR#123         │
│                          ││     └ jonny/checkout-…  ◉ current · needs restack  │
│ [c]reate [a]ttach        ││ Stack (api)   main                                 │
│ [A]ctivate→primary       ││   └ jonny/checkout-api  ◉ current                  │
│ [d]eactivate  [r]efresh  ││ Stack (ops)   (on main — not in slice)             │
│ [P]rocesses  [?]help     ││                                                   │
└──────────────────────────┘└───────────────────────────────────────────────────┘
 ● running  ⏸ needs-input  ✓ done  ○ no session  ⚠ high-CPU      slice ● = active in primary
```

- **Left:** slice list. Badges show session status (●/⏸/✓/○), CPU warning (⚠), and which slice is currently activated in primary (●).
- **Right:** tabbed detail — **Stack / Summary / Changes (diff) / Sessions / Processes**.
- Navigate panes/tabs with arrows/hjkl + Tab; single-key actions; `?` = data-driven help menu.

---

## 15. CLI Surface + Claude Skill (agent-native)

Every TUI action has a non-interactive twin; `--json` everywhere for agents:

```
slis ls                       # list slices + status (--json)
slis show <slice>             # detail: members, branches, stack, sessions
slis create <slice>           # spin worktrees across repos + tmux session ("vertical split")
slis activate <slice>         # swap slice into primary (detached-primary; --stash to allow dirty)
slis deactivate               # restore primary
slis refresh                  # re-checkout active slice's new tips
slis diff <slice>             # combined diff (--json / patch)
slis summary <slice> [--ai]   # commit summary, or claude -p summary
slis ps [<slice>]             # processes (--json), sorted by CPU
slis kill <pid> [--subtree]   # kill a process
slis attach <slice>           # tmux attach/switch
slis init-hooks               # opt-in: install Claude Code Notification/Stop hooks
slis hook <event>             # internal: hook handler (reads Claude JSON on stdin)
```

We ship a **Claude skill** documenting these so Claude can create a slice (vertical split across three repos), activate one, summarise work, or report processes on request — and the skill bundles the hook setup.

---

## 16. Tech Stack & Libraries

| Concern | Choice | Notes |
|---|---|---|
| Language | **Go** | static binary, fast builds, best `os/exec` story |
| TUI | **Bubble Tea** + Bubbles + Lipgloss | `gh-dash` as reference architecture |
| Syntax highlight | **chroma** | diff viewer |
| Markdown render | **glamour** | Claude summaries |
| CLI | **cobra** | CLI twin + subcommands |
| Process info | **gopsutil** | pure-Go CPU%/mem/children; `ps` fallback |
| FS watch | **fsnotify** | external worktree changes + hook events |
| git / gt / tmux / claude | **`os/exec`** + argv builder | no native linking → `CGO_ENABLED=0` |

> **Uncertainty:** Bubble Tea had a v2 line with type renames (`tea.KeyPressMsg`, `View() tea.View`) in 2025/26 — pin and check the version against its docs before coding.

---

## 17. Distribution

- Single **static binary**, `CGO_ENABLED=0`, cross-compiled for darwin/linux × amd64/arm64.
- **GoReleaser** → GitHub Releases + checksums + a **Homebrew tap**. `go install` also works.
- No daemon. State under XDG dirs. Open-source-friendly: `git clone && go build` in seconds.

---

## 18. Testing Strategy

- **Swap engine** (highest risk): integration tests against **throwaway temp git repos + worktrees** — assert no worktree mutation, correct detach/restore, exact stash round-trip, dep-reconcile gating, atomic rollback on injected failure, and crash-recovery from a half-written journal. This is where correctness matters most (data safety).
- **Discovery/grouping:** table-driven tests over porcelain fixtures (matching branches, partial slices, overrides).
- **gt reader:** parse fixtures from real `gt state` output (pinned version); fallback ref-reader against a seeded repo.
- **Process sampler:** spawn a known CPU-burner under a tmux pane, assert attribution + kill.
- **Hook handler:** feed sample Claude hook JSON, assert cwd→slice mapping + event write.
- **TUI:** Bubble Tea model unit tests (send Msgs, assert state) + `teatest` golden snapshots for key views.
- **CI:** GitHub Actions matrix (macOS + Linux), `go test ./...`, `golangci-lint`, GoReleaser dry-run. **Green CI is the bar.**

---

## 19. Key Risks & Open Uncertainties

1. **Swap data safety** — the one place a bug can lose work. Mitigations baked in: clean-check, named/exact stash round-trip (never blind pop), never auto-force, atomic rollback, journalled crash-recovery, worktree never mutated. Heaviest test coverage.
2. **`gt state` is undocumented** and gt moved to SQLite at v1.8.0 — pin gt; ref-reader fallback; revisit past 1.8.
3. **Bubble Tea v2 type renames** — pin and verify before coding.
4. **Watcher behaviour under in-place switch** is server-specific (Vite/Vitest can be finicky). In-place switch keeps inode stable (no restart), but document the **symlink + restart** fallback if a specific stack misbehaves; `node_modules` staleness handled by dep-reconcile.
5. **Process attribution** misses daemonised/detached procs — documented limitation; covers the common case.

---

## 20. References (research)

- ratatui app patterns & component architecture — https://ratatui.rs/recipes/apps/terminal-and-event-handler/ · https://ratatui.rs/concepts/application-patterns/component-architecture/
- gitui (Rust lazygit alt; asyncgit, single-pending-job debounce) — https://github.com/extrawurst/gitui
- Bubble Tea / gh-dash (multi-pane TEA reference) — https://github.com/charmbracelet/bubbletea · https://github.com/dlvhdr/gh-dash
- lazygit internals (View/Context/Controller, data-driven keymap, argv builder, porcelain parsing) — https://github.com/jesseduffield/lazygit ; gocui rationale — https://github.com/jesseduffield/lazygit/issues/2705
- Graphite — command reference https://graphite.com/docs/command-reference · changelog https://graphite.com/docs/cli-changelog · preserved fork https://github.com/searleser97/graphite-cli
- Multi-repo prior art — Google `repo` https://gerrit.googlesource.com/git-repo/ · mani https://github.com/alajmo/mani · gita https://github.com/nosarthur/gita · mu-repo https://github.com/fabioz/mu-repo · vcstool https://github.com/dirk-thomas/vcstool

Full research report: `scratchpad/tui-worktree-research-report.md`.

---

## Appendix — Implementation phase seeds (for the plan)

1. **Skeleton** — Go module, cobra CLI, config loader (workspace.yaml), XDG state, `slis ls` reading discovery. Static-binary CI.
2. **Discovery & model** — porcelain parsing, branch-name grouping, overrides.
3. **Swap engine** — detached-primary activate/deactivate/refresh, stash round-trip, dep-reconcile, journal + crash-recovery. (Heavy tests.)
4. **gt reader** — `gt state` JSON + ref fallback; stack render data.
5. **TUI shell** — Bubble Tea app, slice list + tabbed detail, keymap, refresh discipline.
6. **Diff viewer** — chroma, combined per-slice diff, open-external.
7. **Sessions** — tmux create/attach/status.
8. **Processes** — gopsutil sampler, per-slice + global view, kill.
9. **Notifications** — `init-hooks`, `hook` handler, event store, fsnotify, badges + desktop notify.
10. **Summary** — commit aggregation + `claude -p`/glamour.
11. **Claude skill + hooks bundle**, docs, GoReleaser + Homebrew tap.
