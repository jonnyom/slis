# slis

`slis` ("slice", from the Irish *slis*) is a cockpit for working across several git repos at once: a lazygit-style TUI, plus a CLI that mirrors every TUI action so agents and scripts can drive it headlessly.

The unit of work is a **slice** — one feature's worktrees across all your repos, treated as a single named thing. With a slice you can review the combined diff, read the Graphite stack, track the PRs and their CI, run a tmux session, and *swap* the feature into your repos' primary checkouts so your already-running dev servers rebuild it. No checking out a branch in each repo by hand.

The swap is the part that earns its keep, and it's careful with your work: it detaches each primary to the slice's branch tip (it never touches the worktrees), refuses to run over uncommitted changes unless you ask it to stash, and keeps a journal so deactivating puts everything back exactly as it was.

### Disclaimer
This tool was built for a very specific personal purpose. I work across multiple repositories a lot. This works with LLM agents, but isn't a requirement. Think of it as a worktree / slice manager more than anything.

I will also announce that I have 100% vibe coded this. It's a personal project, I had a specific problem to solve, and the code completeness wasn't very important to me. I'm mostly sharing so other people can use it if they find it useful.

## Install

### Homebrew (recommended)

```sh
brew install jonnyom/homebrew-tap/slis
brew upgrade slis   # later
```

Prebuilt static binaries for macOS (Intel and Apple Silicon) and Linux.

### Go

```sh
go install github.com/jonnyom/slis/cmd/slis@latest
```

### From source

```sh
git clone https://github.com/jonnyom/slis && cd slis
CGO_ENABLED=0 go build -o slis ./cmd/slis
```

slis is one static binary. tmux powers the session features, the `claude` CLI the AI summaries, `gh` the PR/CI views, and `gt` (Graphite) the stack reader. None are required to start — slis just hides the features that need a tool you don't have.

## Quickstart

```sh
# Point slis at your project root; strip a shared branch prefix if you use one
slis init ~/yourproject --strip-prefix jonny/

# List the slices it discovered (worktrees grouped by branch name across repos)
slis ls

# Start a new feature — a worktree in every tracked repo
slis create my-feature

# Swap it into the primaries so your running dev servers pick it up
slis activate my-feature

# ... work ...

# Put every repo back where it was
slis deactivate
```

For per-slice "Claude needs input" notifications, run `slis init-hooks` once to install the Claude Code hooks.

## The TUI

Run `slis` with no arguments.

It opens on the **hub** — a list of your slices. Each card shows the repos and branch, stack health, PR and CI state, and whether a session is waiting on you. The rail on the left filters by state (needs you, ready to merge, in progress, and so on). Press `enter` or `l` on a slice to open its **cockpit**.

The cockpit is the single-slice view: four panels down the left — Stack, PRs, Session, Processes — and a wide right pane that shows detail for whichever panel is focused. `tab` cycles the panels, `1`–`4` jump to one, and the data refreshes itself as you move around so you're not leaning on `r`.

### Hub keys

| Key | Action |
|-----|--------|
| `j` / `k` | Move within the focused panel |
| `tab` | Switch focus: state rail ⇄ slice list |
| `enter` / `l` | Open the slice cockpit |
| `n` / `N` | Jump to the next / previous slice needing attention |
| `c` | Create a new slice (worktrees across every repo) |
| `i` | Adopt an existing branch as a slice |
| `w` | Swap the slice into the primaries / deactivate |
| `R` | Stack actions: restack, submit, merge, sync (Graphite) |
| `d` | Clear a finished slice (worktrees, branches, session) |
| `a` / `C` | Attach the tmux session / launch `claude` in it |
| `o` / `e` | Open the slice in your editor |
| `Y` | Copy a PR-stack markdown summary |
| `/` | Search by name |
| `P` / `!` | Process overlay / cross-slice conflict radar |
| `r` · `?` · `q` | Refresh · help · quit |

### Cockpit keys

| Key | Action |
|-----|--------|
| `tab` / `1`–`4` | Focus next panel / jump to a panel |
| `j` / `k` | Select within the focused panel |
| `↑↓` `^d`/`^u` `g`/`G` | Scroll the right pane |
| `enter` | Zoom the right pane |
| `t` / `b` | Toggle split diff / diff base (Stack panel) |
| `s` / `S` | Commit summary / force an AI summary |
| `w` | Swap into the primaries / deactivate |
| `O` / `v` / `ctrl+r` / `F` | Open PR / view failing-CI logs / rerun CI / fix CI (PRs panel) |
| `x` | Kill the selected process (Processes panel) |
| `esc` / `h` | Back to the hub |

## Commands

Every TUI action has a CLI twin, and `ls` / `show` / `pr` take `--json`, so you can script slis or hand it to an agent.

| Command | What it does |
|---------|--------------|
| `slis init [root]` | Scan `root` for repos and write the workspace config |
| `slis ls` | List the slices in the workspace |
| `slis show <slice>` | Slice detail, including each repo's Graphite stack |
| `slis create <slice>` | Create a worktree for the slice in every repo |
| `slis adopt <branch>` | Adopt an existing branch into a managed slice |
| `slis activate <slice>` | Swap the slice into every repo's primary checkout |
| `slis deactivate` | Restore every primary to where it was |
| `slis refresh` | Advance the live primaries to the latest branch tips |
| `slis rm <slice>` | Remove a finished slice (worktrees, merged branches, session) |
| `slis pr <slice>` | PR + CI status per repo |
| `slis summary <slice>` | Commit summary, or an AI prose summary |
| `slis restack` / `submit` / `merge` / `sync` | Graphite stack operations across the slice |
| `slis group` / `ungroup` | Fix grouping when one feature spans differently-named branches |
| `slis doctor` | Read-only sanity checks; `--fix` applies them |
| `slis init-hooks` | Install the Claude Code hooks for per-slice notifications |

Run `slis <command> --help` for the full flag set.

## How the swap works

`slis activate <slice>` reads each repo's primary worktree path from the workspace config and runs `git switch --detach <branch-tip-sha>` there. The primary ends up in detached HEAD at the slice's commit, so a running dev server (Next.js, Rails, whatever you're on) hot-reloads onto the feature without you touching it. The linked worktrees are never moved or modified.

If a lockfile changed between the old HEAD and the new one (`package.json`, `Gemfile.lock`, and friends), slis tells you so you know to reinstall. If the primary has uncommitted work, activate refuses unless you pass `--stash`, and that stash is popped back by its exact entry on deactivate. `slis deactivate` re-attaches each primary to its original branch, recorded in a journal, so the whole thing is reversible.

## Status

slis was built with [Claude Code](https://claude.ai/code) as a personal tool. I use it daily; it works, but expect rough edges and the occasional breaking change.

Releases are cut by tagging `vX.Y.Z`: the GoReleaser workflow builds the binaries, publishes a GitHub release, and updates the Homebrew formula in [`jonnyom/homebrew-tap`](https://github.com/jonnyom/homebrew-tap). The formula push needs a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret — a PAT with write access to the tap repo.
