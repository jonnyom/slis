# slis

Multi-repo worktree cockpit for developers working across many repositories at once.

A **slice** is the unit of work in slis: it groups together the matching worktrees from every repo in your project into a single named feature unit. When you activate a slice, slis detaches each repo's primary checkout to the slice's branch tip so your running dev servers immediately see the feature code — no manual `git checkout` per repo.

---

## Install

**Homebrew** (recommended):
```sh
brew install jonnyom/tap/slis
```
> Note: the tap repo `jonnyom/homebrew-tap` must exist for this to work. See [Status](#status--disclaimer).

**Go install**:
```sh
go install github.com/jonnyom/slis/cmd/slis@latest
```

**Release binary**: download a pre-built tarball from the [Releases](https://github.com/jonnyom/slis/releases) page and place `slis` on your `$PATH`.

---

## Quickstart

```sh
# 1. Point slis at your project root and tell it which branch-name prefix to strip
slis init ~/yourproject --strip-prefix jonny/

# 2. See all discovered slices (grouped by matching branch names across repos)
slis ls

# 3. Create a new slice — creates a worktree in every tracked repo
slis create my-feature

# 4. Activate it — detaches each repo's primary to the slice branch tip
slis activate my-feature

# 5. Work; restart your dev servers if needed — they now see the feature code

# 6. Deactivate — restores every repo primary back to its prior branch
slis deactivate

# Optional: install Claude Code hooks so slis records AI tool events per-slice
slis init-hooks
```

---

## The TUI

Run `slis` with no arguments to launch the interactive TUI:

```sh
slis
```

**Key bindings**:

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `tab` / `l` | Next tab |
| `shift+tab` / `h` | Previous tab |
| `a` | Attach tmux session for the focused slice |
| `P` | Open process overlay (CPU/memory for slice processes) |
| `s` | Trigger AI summary (on the Summary tab) |
| `r` | Refresh slice data |
| `?` | Show/hide help |
| `q` | Quit |

The detail panel has tabs: **Info**, **Changes** (diff viewer), **Sessions**, **Processes**, **Summary** (commit log + optional AI prose).

---

## Commands

| Command | Description |
|---------|-------------|
| `slis init [root]` | Initialise a workspace by scanning for git repos under `root` |
| `slis ls` | List all slices in the workspace |
| `slis show <slice>` | Show details of a slice including per-repo gt stacks |
| `slis activate <slice>` | Activate a slice — detach all repo primaries to the slice's branch tips |
| `slis deactivate` | Deactivate the current slice — restore all repo primaries to their prior branches |
| `slis refresh` | Refresh the active slice — advance primaries to new branch tips |
| `slis create <slice>` | Create worktrees for all repos in a new slice |
| `slis summary <slice>` | Show commit summary (or AI prose summary) for a slice |
| `slis init-hooks` | Install Claude Code Notification/Stop hooks that call `slis hook <event>` |
| `slis --version` | Print the slis version |

---

## How the swap works

When you run `slis activate <slice>`, slis reads each repo's primary worktree path from the workspace config and runs `git checkout --detach <branch-tip-sha>` there. The primary checkout ends up in detached-HEAD state pointing at the slice's commit — your running servers (Next.js, Rails, etc.) hot-reload onto the feature code without you touching them. Worktrees are never moved or modified. If a lockfile (`package.json`, `Gemfile.lock`, etc.) changed between the old HEAD and the new one, slis records that so you know to re-run your package manager. `slis deactivate` re-attaches each primary to its original branch exactly (recorded in a journal), making the swap fully reversible.

---

## Status / Disclaimer

`slis` was built with [Claude Code](https://claude.ai/code) and is a personal productivity tool. It works, but expect rough edges.

- The Homebrew tap (`jonnyom/homebrew-tap`) must be created as a public GitHub repo before `brew install jonnyom/tap/slis` will work.
- The release workflow (`release.yml`) uses `GITHUB_TOKEN` for the GitHub release and needs a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret (a PAT with `repo` write access to the tap) to push the updated Homebrew formula.
- Requires `tmux` for session features; `claude` CLI for AI summaries.
