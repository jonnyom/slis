---
name: slis
description: Use when managing multi-repo worktree "slices" — listing what's open, creating a feature slice across repos, swapping a slice into the primary checkouts to test it, reviewing a whole-slice diff, or checking which slice's Claude session needs input. Drives the `slis` CLI.
---

# slis — multi-repo worktree slices

A **slice** is a named feature that spans multiple repositories: each repo has a git worktree (and matching branch) for that feature. `slis` treats all of those worktrees as one unit — you can list them, create them together, swap the running dev servers over to one, review their combined diff, and track which Claude session needs your attention.

The mental model: your repos each have a "primary" checkout that your dev servers read from. `slis activate` detaches those primaries to your slice's branch tips so the servers rebuild against your feature. `slis deactivate` restores them. The actual worktrees are never touched.

## When to use

- You want to see all open feature slices and which one is currently active.
- You are starting a new feature that touches more than one repo and want matching worktrees created everywhere.
- You want to test a slice end-to-end in the running servers without manually checking out branches.
- You have finished work and want to review the whole cross-repo diff before sharing.
- You are monitoring multiple Claude sessions and want to know which one is waiting for input.
- You need to set up the Claude Code notification hooks so slis can surface "needs input" signals.

## Setup

```sh
# Scan a project root and write workspace.yaml
slis init /path/to/project --strip-prefix jonny/

# Optionally, install Claude Code notification hooks (idempotent)
slis init-hooks
```

`--strip-prefix` strips a branch-name prefix (e.g. `jonny/`) when deriving the slice name from the branch name.

## Core commands

| Command | Purpose |
|---|---|
| `slis` | Launch the TUI (no subcommand) |
| `slis init [root]` | Scan repos and write `workspace.yaml` |
| `slis ls` | List all slices |
| `slis show <slice>` | Detail for one slice, including per-repo Graphite stack |
| `slis activate <slice>` | Detach all repo primaries to the slice's branch tips |
| `slis deactivate` | Restore primaries to their prior branches |
| `slis refresh` | Advance active primaries to the slice branches' new tips |
| `slis create <slice>` | Create worktrees across all repos for a new slice |
| `slis summary <slice>` | Show commit log summary (or AI prose with `--ai`) |
| `slis init-hooks` | Install Claude Code Notification/Stop hooks |
| `slis hook <event>` | Internal — invoked by Claude Code hooks, not by hand |

### Flags worth knowing

```sh
slis init [root] --repos api,web,mobile   # skip interactive picker
slis init [root] --strip-prefix jonny/    # strip branch prefix for slice names

slis ls --json                            # machine-parseable output
slis show <slice> --json                  # machine-parseable output

slis activate <slice> --stash             # auto-stash dirty primaries first
slis activate <slice> --no-reconcile      # skip dep-reconcile (lockfile installs)

slis create <slice> --no-worktrees        # dry-run: print what would be created

slis summary <slice> --ai                 # AI prose via `claude -p`
slis summary <slice> --base develop       # diff against a different base branch
```

## Common workflows (with exact commands)

### Discover what's open
```sh
slis ls --json
```
Returns a JSON array with `name`, `active`, and `members` (repo, branch, worktree_path, tip_sha). Use this for scripting or when you need to parse slice state.

### Create a feature slice across all repos
```sh
slis create checkout-revamp
```
Creates a git worktree in each repo under `.slis/worktrees/checkout-revamp/<repo>/` with a new branch (prefix + slice name from `workspace.yaml`).

### Test a slice in the running servers
```sh
slis activate checkout-revamp   # detach primaries to slice tips
# ... use the servers ...
slis deactivate                 # restore primaries to prior branches
```
If new commits land on the slice branch while it is active: `slis refresh` advances the primaries.

### Review before sharing
```sh
slis show checkout-revamp --json   # full detail incl. Graphite stack per repo
slis summary checkout-revamp       # formatted commit log across all repos
slis summary checkout-revamp --ai  # AI prose summary via claude -p
```

### Set up needs-input notifications
```sh
slis init-hooks
```
Edits `~/.claude/settings.json` to call `slis hook <event>` on Claude Code Notification and Stop events. Idempotent — safe to run more than once.

## Notes / safety

- `activate` is a **reversible detached-HEAD swap**. The actual worktrees are untouched. If a primary has uncommitted changes, pass `--stash` to auto-stash before switching.
- Only one slice can be active at a time. If another is already active, `activate` will error and tell you to `deactivate` first.
- `ls` and `show` both support `--json` for machine parsing — prefer this over screen-scraping the table output.
- `slis hook` is hidden and machine-invoked; do not call it directly.
- The TUI (bare `slis`) provides a live view of slices, diffs, process CPU, and session status.
