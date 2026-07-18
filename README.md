# slis

`slis` ("slice", from the Irish *slis*) is a cockpit for working across several git repos at once: an OpenTUI front-end backed by a Go core, plus a CLI that mirrors every TUI action so agents and scripts can drive it headlessly. The original Bubble Tea TUI is still available as a fallback.

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

Prebuilt binaries for macOS (Intel and Apple Silicon) and Linux. The Homebrew cask installs both the Go `slis` binary and its matching standalone `slis-ui` front-end.

### Go

```sh
go install github.com/jonnyom/slis/cmd/slis@latest
```

This installs the Go CLI and legacy TUI only. Bare `slis` falls back to that TUI when it cannot find a compiled `slis-ui` beside itself.

### From source

```sh
git clone https://github.com/jonnyom/slis && cd slis
CGO_ENABLED=0 go build -o slis ./cmd/slis
```

That builds the Go CLI and legacy TUI. To build and run the OpenTUI front-end too, follow [Development and building](#development-and-building) below.

tmux powers the session features, a configured coding-agent CLI powers agent sessions and AI summaries, `gh` the PR/CI views, and `gt` (Graphite) the stack reader. None are required to start — slis just hides the features that need a tool you don't have.

### Upgrading existing workspaces

Upgrades are designed to keep existing slices working without a migration
command. On the first registry-aware launch, Slis records the worktrees it
already knows about; later launches backfill older Slis-created worktrees and
refresh their saved branch and path when either changes. A malformed legacy
registry is moved aside with a `.broken-<timestamp>` suffix and rebuilt from
healthy worktrees rather than making the cockpit appear empty.

Normal discovery also performs narrowly-scoped housekeeping left behind by
older removal behavior:

- stale Git worktree administration is removed one exact Slis-owned path at a
  time, only after its checkout directory is already gone;
- missing registry entries are forgotten automatically only for paths inside
  `<workspace>/.slis/worktrees`, and empty managed-directory litter is removed;
- missing external/imported worktrees remain visible for manual recovery, and
  non-empty directories are never removed;
- branch refs and commits are never deleted by startup repair.

`slis rm` is idempotent, removes empty managed parent directories after a
successful cleanup, and still refuses dirty or untracked work unless explicitly
forced. Graphite metadata is used for context, but the cockpit shows only the
current worktree branch and its downstack ancestors—siblings and upstack
branches from other worktrees are not slice members.

## Development and building

### Prerequisites and setup

The Go core requires Go 1.25 or newer. The OpenTUI front-end uses Bun; use **Bun 1.3.14 or newer**, then install its locked dependencies:

```sh
git clone https://github.com/jonnyom/slis && cd slis
go build -o slis ./cmd/slis

cd tui-js
bun install --frozen-lockfile
cd ..
```

Bun 1.3.10 can appear to compile `slis-ui` successfully but produces an executable that crashes while OpenTUI loads its bundled worker assets (`loadedPath.startsWith` / `normalizeLoadedFilePath`). The build script now rejects compiler versions older than 1.3.14; rebuild any affected `slis-ui` with a current Bun.

### Run the Go CLI and legacy TUI

Run a command directly from source, or force the original Bubble Tea interface:

```sh
go run ./cmd/slis ls
SLIS_TUI=go go run ./cmd/slis

# The already-built equivalent:
SLIS_TUI=go ./slis
```

Like an installed copy, these commands use the workspace created by `slis init`.

### Run the OpenTUI front-end from source

The front-end starts a long-lived `slis rpc` sidecar for reads and invokes the same Go binary for mutations. Point it at the binary you just built:

```sh
cd tui-js
SLIS_BIN=../slis bun run start

# UI fixtures only: no workspace, repos, or Go sidecar needed
bun run start:fake
```

You can also exercise the integrated launcher from the repository root. `SLIS_TUI_DIR` tells `slis` to run the Bun source when no sibling `slis-ui` has been compiled:

```sh
SLIS_TUI_DIR="$PWD/tui-js" ./slis
# `./slis ui` uses the same resolution rules.
```

Bare `slis` prefers a compiled `slis-ui` beside the Go binary. If it cannot resolve or start that front-end it explains why and falls back to the Go TUI; `SLIS_TUI=go` skips the lookup.

### Compile a standalone `slis-ui`

For the current machine, compile the front-end next to `slis` so the default launcher finds it:

```sh
cd tui-js
bun run ./scripts/require-bun-version.ts 1.3.14
bun install --frozen-lockfile
bun build --compile ./src/index.tsx --outfile ../slis-ui
cd ..
./slis
```

`slis-ui` embeds Bun plus OpenTUI's and Ghostty's native libraries; it does not need Bun at runtime. It still launches the sibling Go binary as its sidecar, so distribute `slis` and `slis-ui` together.

To produce all release targets from one machine, run:

```sh
./scripts/build-slis-ui.sh
```

The script installs all platform-specific optional packages and cross-compiles `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64` into `tui-js/dist/<goos>-<goarch>/slis-ui`. On a `vX.Y.Z` tag, the release workflow installs Bun 1.3.14 and Go, then GoReleaser runs this script before building the matching Go binaries. Each archive contains the corresponding `slis` + `slis-ui` pair, and the generated Homebrew cask installs both.

### Tests

```sh
# Go core, CLI, RPC sidecar, review store, and reports
go test ./...
CGO_ENABLED=0 go build ./...

# OpenTUI types and unit tests
cd tui-js
bun run typecheck
bun test

# Optional terminal/review smoke tests (tmux required for terminal tests)
bun run term:e2e
bun run term:picker:e2e
bun run review:e2e
```

CI runs the Go build, tests, and lint on macOS and Linux, plus the Bun typecheck and unit suite. Terminal embedding E2E is opt-in through the `RUN_TUI_E2E` repository variable.

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

## Configure coding agents

`slis init` writes the workspace configuration to `~/.config/slis/workspace.yaml` (or `$XDG_CONFIG_HOME/slis/workspace.yaml` when `XDG_CONFIG_HOME` is set). Add a `sessions` block there to choose what the TUI launches when you press `C` on a slice.

For a single Claude or Codex agent, set the harness:

```yaml
sessions:
  harness: codex       # claude (the default) or codex
  layout: repos        # repos, root, or both
  autostart: false
```

To launch a custom command or pass arguments, use `agent`. A non-empty `agent` command takes precedence over `harness` for interactive sessions:

```yaml
sessions:
  harness: claude
  agent: "claude --resume"
  autostart: false
```

To choose between several agents at launch time, configure the picker list as names plus command argument arrays:

```yaml
sessions:
  agents:
    - name: Claude
      cmd: [claude, --resume]
    - name: Codex
      cmd: [codex, --full-auto]
  autostart: false
```

In the TUI, `a` attaches to the slice's tmux session and `C` launches an agent. When `sessions.agents` contains more than one entry, `C` opens the agent picker and remembers the last selection for the next launch; otherwise it launches the single `sessions.agent`/`sessions.harness` default. Set `autostart: true` to launch the remembered/default agent automatically the first time a session is attached. Restart `slis` after editing the workspace file so it reloads the configuration.

The `harness` setting also selects the integration used by `slis fix-ci` and AI summaries. If you use a custom interactive command, keep `harness` set to the compatible `claude` or `codex` integration.

`layout` controls the tmux windows. `repos` creates one safe window inside each configured repo worktree, `root` creates one window above all of them, and `both` provides both forms. When omitted, multi-repo slices default to `repos`; this avoids accidentally running Git or Graphite commands in an enclosing repository that is not part of the Slis workspace.

## The TUI

Run `slis` with no arguments. It launches the OpenTUI front-end when `slis-ui` is installed beside it, otherwise it falls back to the legacy Go TUI.

It opens on the **hub** — a list of your slices. Each card shows the repos and branch, stack health, PR and CI state, and whether a session is waiting on you. The rail on the left filters by state (needs you, ready to merge, in progress, and so on). Press `enter` or `l` on a slice to open its **cockpit**.

The cockpit is the single-slice view: four panels down the left — Stack, PRs, Session, Processes — and a wide right pane that shows detail for whichever panel is focused. The Stack panel is an operational summary of the selected branch: its downstack ancestry (never sibling or upstack branches from another worktree), stack position and health, PR/CI state, and changed files. Press `enter` to load a full unified or side-by-side diff only when you need it, or `f` to browse the tree at that revision. Focus the diff with `enter`/`tab`, move its visible line cursor with `j`/`k`, select a range with `v`/`space`, and press `c` to comment; comments collect into a pending review that can be sent to the slice's agent session. `tab` cycles the cockpit panels, `1`–`4` jump to one, and the data refreshes itself as you move around so you're not leaning on `r`.

Agent sessions run in embedded terminal tabs using the [coding-agent configuration](#configure-coding-agents). Process views include tree navigation, CPU history, sorting, and guarded process/subtree termination.

### Hub keys

| Key | Action |
|-----|--------|
| `j` / `k` | Move within the focused panel |
| `tab` | Switch focus: state rail ⇄ slice list |
| `enter` / `l` | Open the slice cockpit |
| `n` / `N` | Jump to the next / previous slice needing attention |
| `c` | Create a new slice (worktrees across every repo) |
| `i` / `I` | Import discovered worktrees / adopt an arbitrary branch |
| `w` | Swap the slice into the primaries / deactivate |
| `R` | Stack actions: restack, submit, merge, sync (Graphite) |
| `d` | Clear a finished slice (worktrees, branches, session) |
| `a` / `C` | Open the tmux session / launch a configured agent in it |
| `e` / `o` | Open the whole slice workspace in the configured editor |
| `Y` | Copy a PR-stack markdown summary |
| `/` | Search by name |
| `P` / `!` | Process overlay / cross-slice conflict radar |
| `T` | Cycle System, Midnight, Violet, and Light themes |
| `r` · `?` · `q` / `ctrl+c` | Refresh · help · quit |

### Cockpit keys

| Key | Action |
|-----|--------|
| `tab` / `1`–`4` | Focus next panel / jump to a panel |
| `j` / `k` | Select within the focused panel |
| `enter` / `l` | Open the selected stack branch's rich diff |
| `f` | Browse files at the selected stack branch revision |
| `c` / `C` | Add an inline review comment / manage and send pending comments |
| `↑↓` `^d`/`^u` `g`/`G` | Scroll the right pane |
| `enter` | Zoom the right pane |
| `b` | Cycle the Stack summary scope: working tree / parent / trunk |
| `s` / `S` | Commit summary / force an AI summary |
| `w` | Swap into the primaries / deactivate |
| `O` / `v` / `ctrl+r` / `F` | Open PR / view failing-CI logs / rerun CI / fix CI (PRs panel) |
| `x` / `X` | Kill the selected process / process subtree (Processes panel) |
| `T` | Cycle System, Midnight, Violet, and Light themes |
| `esc` / `h` | Back to the hub |

### Themes

By default, the OpenTUI asks the terminal whether its background is light or
dark and follows that appearance, including live profile changes when the
terminal supports them. Midnight is the fallback when the terminal cannot
report its appearance.

Press `T` (`shift+t`) in the hub, cockpit, or diff view to cycle System,
Midnight, Violet, and Light. The selection is remembered across launches;
System continues following live terminal appearance changes.

Pin a startup theme with `SLIS_THEME`, or use the standard `NO_COLOR` variable
for a monochrome dark/light palette:

```sh
SLIS_THEME=violet slis
SLIS_THEME=light slis
SLIS_THEME=auto slis       # follow the terminal again
NO_COLOR=1 slis            # monochrome, still follows light/dark appearance
```

Supported theme names are `midnight`, `violet`, `light`, and `mono`.
`auto`/`system`, `dark`/`blue`, `purple`, and `monochrome` are accepted aliases.
`NO_COLOR` always prevents chromatic themes, including after a theme-cycle
keypress.

Theme, the last selected coding agent, diff layout (unified or split), and diff
scope are stored in `$XDG_STATE_HOME/slis/prefs.json` (normally
`~/.local/state/slis/prefs.json`). Command-line environment variables still
override the saved theme for that launch.

## Commands

Every TUI action has a CLI twin, and read commands such as `ls`, `show`, `status`, `pr`, `pr-stack`, `summary`, `conflicts`, `comments`, `branch-diff`, `tree`, `cat`, and `doctor` take `--json`, so you can script slis or hand it to an agent.

| Command | What it does |
|---------|--------------|
| `slis init [root]` | Scan `root` for repos and write the workspace config |
| `slis ls` | List the slices in the workspace |
| `slis show <slice>` | Slice detail, including each repo's Graphite stack |
| `slis status [slice]` | Each slice's Claude session status (none/running/waiting-input/done) |
| `slis create <slice>` | Create a worktree for the slice in every repo |
| `slis adopt <branch>` | Adopt an existing branch into a managed slice |
| `slis activate <slice>` | Swap the slice into every repo's primary checkout |
| `slis deactivate` | Restore every primary to where it was |
| `slis refresh` | Advance the live primaries to the latest branch tips |
| `slis rm <slice>` | Remove a finished slice (worktrees, merged branches, session) |
| `slis pr <slice>` | PR + CI status per repo |
| `slis summary <slice>` | Commit summary, or an AI prose summary |
| `slis ui` | Launch the OpenTUI front-end explicitly |
| `slis rpc` | Run the OpenTUI JSON-RPC sidecar over stdio |
| `slis review ...` | Add, list, remove, clear, or send pending inline review comments |
| `slis ci-rerun <slice>` | Re-trigger failed GitHub Actions runs across a slice |
| `slis branch-diff` / `tree` / `cat` | Inspect a stack branch's diff and files without checking it out |
| `slis restack` / `submit` / `merge` / `sync` | Graphite stack operations across the slice |
| `slis group` / `ungroup` | Fix grouping when one feature spans differently-named branches |
| `slis doctor` | Read-only sanity checks; `--fix` applies them |
| `slis init-hooks` | Install the Claude Code hooks for per-slice notifications |

Run `slis <command> --help` for the full flag set.

### Driving slis with agents

slis ships a [Claude skill](skills/slis/SKILL.md) and an agent contract,
[`docs/AGENT.md`](docs/AGENT.md), covering the JSON output shapes, the
session-status data flow (`slis status`), the mutate-vs-read map, and how errors
surface. An agent can poll `slis status --json` for the slice whose Claude is
`waiting-input`, find failing CI with `slis pr <slice> --json` and hand it to
`slis fix-ci`, or run a slice from `create` to `merge` — all headless.

## How the swap works

`slis activate <slice>` reads each repo's primary worktree path from the workspace config and runs `git switch --detach <branch-tip-sha>` there. The primary ends up in detached HEAD at the slice's commit, so a running dev server (Next.js, Rails, whatever you're on) hot-reloads onto the feature without you touching it. The linked worktrees are never moved or modified.

If a lockfile changed between the old HEAD and the new one (`package.json`, `Gemfile.lock`, and friends), slis tells you so you know to reinstall. If the primary has uncommitted work, activate refuses unless you pass `--stash`, and that stash is popped back by its exact entry on deactivate. `slis deactivate` re-attaches each primary to its original branch, recorded in a journal, so the whole thing is reversible.

## Status

slis was built with [Claude Code](https://claude.ai/code) as a personal tool. I use it daily; it works, but expect rough edges and the occasional breaking change.

Releases are cut by tagging `vX.Y.Z`: the GoReleaser workflow builds the Go core and platform-matched standalone OpenTUI front-ends, publishes both in each GitHub release archive, and updates the Homebrew cask in [`jonnyom/homebrew-tap`](https://github.com/jonnyom/homebrew-tap). The cask push needs a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret — a PAT with write access to the tap repo.
