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
`slis activate` detaches those primaries to your slice's branch *tips* (detached
HEAD) so the servers rebuild against your feature; `slis deactivate` restores
them. The real worktrees are never touched — activate is a reversible swap.

## Driving slis as an agent

slis is built to be driven headlessly. Two rules:

1. **Read with `--json`, parse the data — don't screen-scrape tables and don't
   branch on the exit code** (it's a flat `1` on any failure today). Every read
   command takes `--json`: `ls show status pr pr-stack summary conflicts comments
   doctor`.
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
| `slis init-hooks` | mutate | no | Install Claude Code Notification/Stop hooks (idempotent). The hook process fires the desktop banner itself when a slice changes to waiting-input/done, so notifications work with no TUI running and while a tmux session is attached |
| `slis ls` | read | **yes** | List all slices + members + active flag (`--json` is an object: `slices` + `skipped` + `repo_errors`) |
| `slis show <slice>` | read | **yes** | One slice in detail incl. per-repo Graphite stack |
| `slis status [slice]` | read | **yes** | Each slice's Claude session status (none/running/waiting-input/done) |
| `slis summary <slice>` | read | **yes** | Per-repo commit subjects (`--ai` for prose, markdown only) |
| `slis pr <slice>` | read | **yes** | Per-repo PR: number, state, CI pass/fail/pending, comment count |
| `slis pr-stack <slice>` | read | **yes** | Shareable PR stack (markdown; `--copy` to clipboard) |
| `slis comments [slice]` | read | **yes** | Cached PR review/inline comments (persists after `rm`) |
| `slis conflicts` | read | **yes** | Files touched by >1 slice (merge-overlap radar) |
| `slis doctor` | read | **yes** | Workspace health findings incl. hidden/detached/prunable worktrees + orphaned `.slis/worktrees` dirs (`--fix` auto-repairs the safe ones; never prunes) |
| `slis edit <slice>` | read* | no | Open worktrees in your editor (`--print` prints the path) |
| `slis create <slice>` | mutate | no | Create worktrees + branch across all repos (`--no-worktrees` dry-run) |
| `slis adopt [branch]` | mutate | no | Adopt an existing branch into a managed slice |
| `slis activate <slice>` | mutate | no | Detach all primaries to the slice's branch tips (`--stash` if dirty) |
| `slis deactivate` | mutate | no | Restore primaries to their prior branches |
| `slis refresh` | mutate | no | Advance active primaries to the slice branches' new tips |
| `slis restack <slice>` | mutate | no | `gt restack` across the slice's repos (refuses dirty worktrees) |
| `slis sync <slice>` | mutate | no | `gt sync` per repo — **repo-wide**: may overwrite trunk, delete merged branches |
| `slis submit <slice>` | mutate | no | `gt submit` — **force-pushes** the stack + opens/updates PRs |
| `slis merge <slice>` | mutate | no | `gt merge` — triggers Graphite's **server-side merge queue** |
| `slis fix-ci <slice>` | mutate | no | Point `claude -p` at failing CI in the worktree (`--dry-run` previews) |
| `slis rm <slice>` | mutate | no | Remove worktrees + kill tmux + delete merged branches (`--force`, `--dry-run`) |
| `slis group <name> <slice>...` | mutate | no | Manually group slices under one name (writes `overrides.yaml`) |
| `slis ungroup <name>` | mutate | no | Undo a manual grouping |
| `slis editor [set\|clear]` | mutate | no | Show/set/clear the editor used by `edit` |

`*` `edit` opens an editor / prints a path; it does not change repo state.
`slis hook <event>` exists but is hidden and machine-invoked by Claude Code — never call it by hand.

## Setup

```sh
slis init /path/to/project --repos api,web,mobile --strip-prefix jonny/
slis init-hooks   # optional: feed `slis status` with live session state
```

`--strip-prefix` strips a branch-name prefix (e.g. `jonny/`) when deriving the
slice name from the branch name.

## Agent recipes

### Which slice needs my input?
```sh
slis status --json    # → [{slice, status}] ; act on status == "waiting-input"
```
Requires `slis init-hooks` once so Claude Code writes the events. `slis status
<slice>` is a direct lookup for one slice.

### Fix failing CI on a slice
```sh
slis pr <slice> --json            # find rows where .ci == "fail"
slis fix-ci <slice> --dry-run     # preview the prompt + worktree
slis fix-ci <slice>               # runs `claude -p` in each failing repo's worktree
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
slis activate <slice> --stash   # detach primaries to slice tips; stash dirty work
# ... exercise the servers; `slis refresh` if new commits land ...
slis deactivate                 # restore primaries (pops the stash)
```

### Before merging: catch cross-slice conflicts
```sh
slis conflicts --json   # → {overlaps:[{repo,path,slices}], incomplete:[...]}
```

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
