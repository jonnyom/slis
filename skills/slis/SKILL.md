---
name: slis
description: Use when managing multi-repo worktree "slices" — listing what's open, the session status of each slice's Claude, creating a feature slice across repos, swapping a slice into the primary checkouts to test it, reviewing the whole-slice diff/PRs, fixing failing CI, or driving the Graphite stack (restack/submit/merge). Drives the `slis` CLI; every action is headless and most reads emit `--json`.
---

# slis — multi-repo worktree slices

A **slice** is a named feature that spans multiple repositories: each repo has a
git worktree (and matching branch) for that feature. `slis` treats those
worktrees as one unit — list them, create them together, swap the running dev
servers onto one, review their combined diff/PRs, drive the Graphite stack, and
track which slice's Claude session needs your attention.

**Mental model.** Each repo has a "primary" checkout your dev servers read from.
`slis activate` puts those primaries on a `slis/live/<slice>` branch at your
slice's branch *tips* so the servers rebuild against your feature; `slis
deactivate` restores them (and deletes the temp branch). The real worktrees are
never touched — activate is a reversible swap.

While a slice is active each primary is on its `slis/live/<slice>` temp branch —
do your commits and `gt` work in the slice's *worktrees* (`SLIS_WORKTREES`), not
the primaries. `gt` *reads* work in the primary, but `gt` *mutations* refuse
there ("branch not tracked") by design — that's what the worktrees are for.
`deactivate` refuses a primary that drifted off its temp branch (you switched it
away) with zero state change, and refuses when you *committed* on the temp branch
(those commits are safe on that named branch — it lists them so you can graft
them onto the slice branch in the worktree); `deactivate --force` restores anyway
and renames a committed-on temp branch to `slis/rescue/<slice>-<repo>` (never
deletes it) so nothing is lost. If the branch advances, `slis refresh`
fast-forwards the temp branch (it refuses a dirty primary or a diverged branch).

## Driving slis as an agent

slis is built to be driven headlessly. Two rules:

1. **Read with `--json`, parse the data — don't screen-scrape tables and don't
   branch on the exit code** (it's a flat `1` on any failure today). Every read
   command takes `--json`: `ls show status pr pr-stack summary conflicts comments
   doctor candidates`.
2. **Know what mutates before you run it.** See the safety map below. Prefer
   `--json` / `--dry-run` to inspect first; never run a remote/destructive
   mutator (`submit`, `merge`, `sync`, `rm --force`) without explicit intent.

Full JSON schemas, the session-status data flow, and the mutation table live in
[`docs/AGENT.md`](../../docs/AGENT.md). Read it before scripting against slis.

## Command reference

Legend: **read** = no state change · **mutate** = changes git/worktrees/remote/files.

| Command | Kind | `--json` | Purpose |
|---|---|---|---|
| `slis` | — | — | Launch the TUI (no subcommand) |
| `slis init [root]` | mutate | no | Scan repos → write `workspace.yaml` |
| `slis init-hooks` | mutate | no | Install Claude Code Notification/Stop hooks (idempotent). The hook process fires the desktop banner itself when a slice changes to waiting-input/done, so notifications work with no TUI running and while a tmux session is attached. Clicking a banner runs `slis focus <slice>` to jump your tmux client to that slice (terminal-notifier only; set `notify.activate` to a terminal app bundle id to also foreground it) |
| `slis init-skill` | mutate | no | Install this skill (+ `references/AGENT.md`) for an agent harness. `--harness claude\|codex\|both` (default both): claude → `~/.claude/skills/slis/`, codex → `~/.agents/skills/slis/`. Idempotent (content-hash version stamp). Also run by `slis init` unless `--no-skill` |
| `slis ls` | read | **yes** | List all slices + members + active flag (`● stale` when the active slice's branches advanced past the primaries; `● partial` / `"partial": true` when the active swap covers only some member repos — a crash mid-activate). `--json` object: `slices` + `skipped` + `repo_errors` + `candidates` + `missing`; each slice carries optional Graphite `stack_id`/`stack_order` — siblings share a `stack_id` |
| `slis candidates` | read | **yes** | List discovered-but-unmanaged worktrees awaiting opt-in import |
| `slis show <slice>` | read | **yes** | One slice in detail incl. per-repo Graphite stack |
| `slis status [slice]` | read | **yes** | Each slice's Claude session status (none/running/waiting-input/done), plus optional Claude `session_id`/`cwd` for recovery |
| `slis summary <slice>` | read | **yes** | Per-repo commit subjects (`--ai` for prose, markdown only) |
| `slis pr <slice>` | read | **yes** | Per-repo PR: number, state, CI pass/fail/pending, comment count |
| `slis pr-stack <slice>` | read | **yes** | Shareable PR stack (markdown; `--copy` to clipboard; `--json` rows carry `stack_order` and are ordered trunk-first by Graphite depth) |
| `slis share <slice>` | clipboard | no | Copy every PR across every repo stack with parent-relative `+added` / `-deleted` totals as Markdown (`--stdout` prints it instead) |
| `slis comments [slice]` | read | **yes** | Cached PR review/inline comments (persists after `rm`) |
| `slis review list [slice]` | read | **yes** | List pending inline-review comments awaiting delivery to a slice's agent |
| `slis conflicts` | read | **yes** | Files touched by >1 slice (merge-overlap radar) |
| `slis doctor` | read | **yes** | Workspace health findings incl. hidden/detached/prunable worktrees + orphaned `.slis/worktrees` dirs, swap-journal health (stale journal, deleted prior branch, orphaned `slis/live` branch/detach tagged auto-fixable vs needs-manual-attention, partial swap where the journal covers only some member repos, and a repo swapped-in but un-journaled), and a Graphite section (gt installed? repos initialised? branches tracked?). `--fix` auto-repairs the safe ones (incl. deleting a stale journal only when every primary is on a branch, and clearing a contained orphaned `slis/live` branch); never prunes worktrees |
| `slis edit <slice>` | read* | no | Open worktrees in your editor (`--print` prints the path) |
| `slis create <slice>` | mutate | no | Create worktrees + branch across all repos (`--no-worktrees` dry-run); in a Graphite-native repo also `gt track`s the new branch (best-effort) |
| `slis adopt [branch]` | mutate | no | Adopt an existing branch into a managed slice (creates worktrees); in a Graphite-native repo also `gt track`s it (best-effort) |
| `slis import [path]` | mutate | no | Register a candidate worktree (or `--all`) as a managed slice — registry only, never git |
| `slis ignore <path-or-glob>` | mutate | no | Add a path/glob to `grouping.ignore` so matching worktrees are never ingested |
| `slis forget <slice>` | mutate | no | Drop a slice from the registry (does not touch git — use for a missing slice) |
| `slis activate <slice>` | mutate | no | Put all primaries on a `slis/live/<slice>` branch at the slice's branch tips (`--stash` if dirty) |
| `slis deactivate` | mutate | no | Restore primaries to their prior branches and delete the temp branch; refuses a drifted primary or one you committed on (zero state change). `--force` restores anyway, renaming a committed-on temp branch to `slis/rescue/<slice>-<repo>` first (never deletes it) |
| `slis refresh` | mutate | no | Fast-forward active primaries' temp branches to the slice branches' new tips (refuses a dirty primary or a diverged branch) |
| `slis restack <slice>` | mutate | no | `gt restack` across the slice's repos (refuses dirty worktrees) |
| `slis sync <slice>` | mutate | no | `gt sync` per repo — **repo-wide**: may overwrite trunk, delete merged branches |
| `slis submit <slice>` | mutate | no | `gt submit` — **force-pushes** the stack + opens/updates PRs |
| `slis merge <slice>` | mutate | no | `gt merge` — triggers Graphite's **server-side merge queue** |
| `slis fix-ci <slice>` | mutate | no | Point the harness at failing CI in the worktree — `claude -p` or `codex exec` per `sessions.harness` (`--dry-run` previews) |
| `slis rm <slice>` | mutate | no | Remove worktrees + kill tmux + delete merged branches (`--force`, `--dry-run`) |
| `slis group <name> <slice>...` | mutate | no | Manually group slices under one name (writes `overrides.yaml`) |
| `slis ungroup <name>` | mutate | no | Undo a manual grouping |
| `slis gather <name> <slice>` | mutate | yes | Collapse the Graphite stack `<slice>` belongs to into one slice named `<name>`, represented by the stack **tip**; intermediate branches are folded (hidden as their own slices, worktrees untouched). Per repo. `--json` reports `{name, gathered:[{repo,tip,folded,linear}]}` |
| `slis scatter <name>` | mutate | no | Undo a gather (folded branches reappear as their own slices) |
| `slis editor [set\|clear]` | mutate | no | Show/set/clear the editor used by `edit` |
| `slis focus <slice>` | mutate | no | Switch the active tmux client to the slice's session (creates it if missing); prints `tmux attach -t …` when no client is attached. This is what a clicked desktop notification runs |
| `slis review add <slice>` | mutate | no | Add a pending review comment on a line or range (`--repo --file --line [--end-line] --body [--hunk]`); branch is resolved from the slice member. Store only, never git |
| `slis review rm <slice> <id>` | mutate | no | Remove one pending review comment by id (guarded to the named slice) |
| `slis review clear <slice>` | mutate | no | Discard all of a slice's pending review comments |
| `slis review send <slice>` | mutate | no | Compose pending comments, create/reuse the slice session and start the configured agent if needed, verify that agent owns the active pane, inject via bracketed paste + Enter, then clear (`--keep` preserves). Startup/readiness failure keeps comments pending |
| `slis review agent <slice> --agent <name>` | mutate | no | Start the selected configured or PATH-detected reviewer in a dedicated slice tmux window and return immediately; it reviews the whole stack, stores attributed findings, and delivers only those new findings to the working agent. Failed reviews remain visible in tmux |

`*` `edit` opens an editor / prints a path; it does not change repo state.
`slis hook <event>` exists but is hidden and machine-invoked by Claude Code — never call it by hand.

## Setup

```sh
slis init /path/to/project --repos api,web,mobile --strip-prefix jonny/
slis init-hooks   # optional: feed `slis status` with live session state
```

`--strip-prefix` strips a branch-name prefix (e.g. `jonny/`) when deriving the
slice name from the branch name.

## Agent harness (claude / codex)

slis drives an agent harness for launching sessions, `fix-ci`, and `summary
--ai`. Configure it under `sessions:` in `workspace.yaml`:

```yaml
sessions:
  harness: claude   # or "codex". Empty defaults to "claude".
  agent: ""         # explicit launch command; wins verbatim over harness.
  autostart: false  # launch the harness automatically when a session is attached.
```

- **`harness`** picks the binary and the launch shape: `claude` gets a
  `--append-system-prompt '<slice context>'`; `codex` gets **no** positional
  prompt and no such flag (`codex exec` for headless work).
- **`agent`** is an escape hatch — a non-empty value is used verbatim (e.g.
  `claude --resume`), overriding `harness`.
- **`autostart`** launches the harness in a slice's session the first time it's
  attached (the same as pressing `C` in the TUI). Legacy `autostart_claude` is
  accepted as an alias.

## SLIS_* environment contract

When slis launches the harness in a slice's tmux session, it prefixes these env
vars onto the launch command. An agent running **inside** a slis session can
trust them:

| Var | Meaning |
|---|---|
| `SLIS_SLICE` | the slice name |
| `SLIS_ROOT` | the workspace root |
| `SLIS_ACTIVE` | `1` if the slice is swapped into the primaries, else `0` |
| `SLIS_HARNESS` | `claude` or `codex` |
| `SLIS_WORKTREES` | comma-separated `repo=worktree_path` pairs (make edits here, never the primaries) |
| `SLIS_TERMINAL_APP` | originating terminal when detected (currently `ghostty`), used to route notification clicks |

## Agent recipes

### Which slice needs my input?
```sh
slis status --json    # → [{slice, status, session_id?, cwd?}]
```
Requires `slis init-hooks` once so Claude Code writes the events. `slis status
<slice>` is a direct lookup for one slice. The TUI Sessions panel offers
`resume` when a waiting/done Claude process is gone but its recorded session can
be recovered.

### Fix failing CI on a slice
```sh
slis pr <slice> --json            # find rows where .ci == "fail"
slis fix-ci <slice> --dry-run     # preview the prompt + worktree
slis fix-ci <slice>               # runs the harness (claude -p / codex exec) in each failing repo's worktree
```

### Slice lifecycle, end to end
```sh
slis create <slice>                  # worktrees across all repos
slis edit <slice> --print            # cd into a worktree to work
slis summary <slice> --json          # review per-repo commits
slis pr <slice> --json               # PR + CI state
slis submit <slice>                  # force-push stack + open/update PRs
slis merge <slice>                   # Graphite server-side merge queue
slis rm <slice>                      # clean up after merge
```

### Test a slice in the running dev servers
```sh
slis activate <slice> --stash   # primaries → slis/live branch at slice tips; stash dirty work
# ... exercise the servers; `slis refresh` if new commits land ...
slis deactivate                 # restore primaries (pops the stash)
```

### Before merging: catch cross-slice conflicts
```sh
slis conflicts --json   # → {overlaps:[{repo,path,slices}], incomplete:[...]}
```

### A worktree slis found but didn't ingest (opt-in)
```sh
slis candidates --json          # → [{repo,path,branch,slice}] not-yet-managed
slis import /path/to/worktree   # register it as a slice (or --all)
slis ignore '**/scratch/**'     # or hide matching worktrees for good
```
Ingestion is opt-in: only managed worktrees (under `<root>/.slis/worktrees/**`
or in the registry) become slices. A registered slice whose worktree vanished
shows up as `missing` in `slis ls --json`; `slis forget <slice>` drops it from
the registry (git untouched).

## Safety map (read before any mutator)

- `activate --stash` touches **uncommitted** primary work (stash pinned by SHA,
  popped on `deactivate`; pop conflict leaves the stash intact).
- `submit` **force-pushes** the stack to the remote and opens/updates PRs.
- `merge` hands the squash/merge/restack to Graphite's **server-side queue**.
- `sync` is **repo-wide**: may fast-forward/overwrite trunk and delete merged
  branches — not scoped to the slice.
- `rm --force` removes **dirty** worktrees; `rm` also deletes merged branches.
- Everything in the **read** column is safe to run anytime and is the right way
  to inspect state before deciding to mutate.

## Notes

- Only one slice can be active at a time; `activate` errors (zero state change)
  if another is active — `deactivate` first.
- `slis` (no subcommand) launches the TUI; every TUI action has the CLI twin above.
- `gt`/`forge` reads are best-effort: a repo with no PR or no `gh` simply omits
  that data rather than failing the whole command.
