# slis

`slis` ("slice", from the Irish *slis*) is a terminal cockpit for managing one
feature across several Git repositories.

It groups the feature's branches and worktrees into a single **slice**, then
puts its diffs, stacked branches, pull requests, CI, tmux sessions, processes,
and coding agents in one place. The OpenTUI interface is backed by a Go CLI, so
the same workflows are available interactively, from scripts, or to an agent.

> **Demo GIF coming soon**
>
> The demo will show the hub, a slice cockpit, changed files, a rich diff, and an
> attached agent session.

Slis is primarily a worktree and workspace manager. Coding-agent support is
useful, but entirely optional.

## Contents

- [The idea](#the-idea)
- [Why I built it](#why-i-built-it)
- [Good use cases](#good-use-cases)
- [Bad use cases](#bad-use-cases)
- [Install](#install)
- [How to use Slis](#how-to-use-slis)
- [Configure Slis](#configure-slis)
- [Command reference](#command-reference)
- [Upgrading](#upgrading)
- [Safety model](#safety-model)
- [Contributing](#contributing)
- [Project status](#project-status)

## The idea

Suppose a feature called `checkout` requires changes in three repositories:

```text
checkout
├── web       → worktree on branch checkout
├── api       → worktree on branch checkout
└── payments  → worktree on branch checkout
```

Without Slis, those worktrees, pull requests, terminals, and branch states are
separate things you have to keep aligned yourself. Slis treats them as one unit:

```sh
slis create checkout
slis activate checkout
slis pr checkout
slis rm checkout
```

The TUI calls that single-feature view the **cockpit**. It shows an operational
summary first—stack position, PR and CI state, changed files, session status,
and processes—then loads a full diff or file browser only when requested.

Slis also works in a single repository. The multi-repo workflow is where the
slice abstraction becomes most useful.

## Why I built it

I built Slis for a very specific personal problem: I work across multiple
repositories a lot. A single feature might involve a frontend, a Rails API, an
MCP service, several worktrees, a Graphite stack, a handful of PRs, and one or
more coding agents. All the individual tools worked, but keeping the whole unit
of work in my head did not.

Slis is the cockpit I wanted for that workflow. It is deliberately opinionated
around treating a feature—not a repository or a branch—as the thing I am
actually working on. It works with coding agents, but they are not the point;
the worktree and slice model is useful without them.

I also want to be completely candid: **I 100% vibe coded this.** I built it with
coding agents because I had a concrete problem I wanted solved, and at the
beginning code completeness mattered much less to me than making the workflow
real. The project has since gained a substantial test suite, safety checks, and
release tooling because I use it for actual work—but I am not interested in
pretending it emerged from a solemn, perfectly planned software process.

I am sharing it because other people may have the same problem, and because I
would genuinely like to hear where the model works, where it breaks, and what
people would do differently.

## Good use cases

Slis is a good fit when:

- one product or feature regularly spans two or more repositories;
- you keep several Git worktrees open and want to know which ones belong
  together;
- you use stacked branches or stacked pull requests, particularly with
  Graphite;
- you want a separate tmux session—or coding-agent session—for each feature;
- you need to review changed files, PR status, CI, and local processes without
  visiting several tools;
- your development servers run from primary checkouts and you want to swap a
  feature into all of them together, then restore them reliably;
- you want the same operations available through a TUI, a normal CLI, and
  structured JSON for scripts or agents.

Some concrete examples:

- a frontend, API, and worker changed by one product feature;
- a Rails application plus a TypeScript MCP or agent service;
- several related Graphite stacks that need to be reviewed and submitted
  together;
- running Claude Code or Codex on multiple independent features without losing
  track of which terminal owns which worktree;
- comparing overlapping files across active features before they become merge
  conflicts.

## Bad use cases

Slis is probably the wrong tool when:

- you work in one checkout on one branch at a time and Git already feels simple;
- a monorepo gives you all the isolation you need and you do not use worktrees;
- you want a shared, hosted project-management system—Slis is a local developer
  cockpit, not a team planning database;
- you want Slis to hide Git entirely. It adds guardrails, but worktrees,
  branches, rebases, and dirty files still matter;
- you need a graphical desktop application rather than a terminal interface;
- you require Windows binaries. Current releases target macOS and Linux;
- you expect every integration without installing its tool. Graphite, GitHub,
  tmux, and coding-agent features degrade independently when their binaries are
  unavailable.

You also do not need Slis merely to use an AI coding agent. Its value comes from
managing the surrounding Git and multi-repo workflow.

## Install

### Homebrew (recommended)

```sh
brew install jonnyom/homebrew-tap/slis
```

The Homebrew release includes matching `slis` and `slis-ui` binaries for macOS
and Linux. Bare `slis` launches the OpenTUI interface.

### Go

```sh
go install github.com/jonnyom/slis/cmd/slis@latest
```

This installs the Go CLI and legacy Bubble Tea TUI. If a matching `slis-ui`
binary is not installed beside it, Slis explains the fallback and opens the
legacy interface.

### Optional integrations

Slis starts without these tools and hides or disables the features that need
them:

| Tool | Enables |
|---|---|
| `tmux` | Per-slice terminal and agent sessions |
| `gh` | Pull-request, review-comment, and CI views |
| `gt` | Graphite stack reading, restacking, submission, sync, and merge |
| Claude Code or Codex | Agent sessions, AI summaries, and `fix-ci` |

## How to use Slis

### 1. Initialise a workspace

Point Slis at the directory containing your repositories:

```sh
slis init ~/your-project
```

If branches share a personal prefix, tell Slis to remove it when deriving slice
names:

```sh
slis init ~/your-project --strip-prefix jonny/
```

Slis scans for repositories and writes the workspace configuration to
`$XDG_CONFIG_HOME/slis/workspace.yaml`, normally
`~/.config/slis/workspace.yaml`.

Check the result before doing anything else:

```sh
slis ls
slis doctor
```

### 2. Open the hub

Run Slis without a subcommand:

```sh
slis
```

The **hub** lists every managed slice and highlights work that needs attention,
is active, is in review, or is ready to clear. Use `j`/`k` to navigate and
`enter` to open a slice's cockpit. Press `?` anywhere for contextual help.

Unknown worktrees are not silently adopted after initial migration. They appear
as candidates so you can choose what Slis should manage:

```sh
slis candidates
slis import /path/to/worktree
slis ignore '/path/or/glob/**'
```

### 3. Create a slice

Create a new branch and worktree in every configured repository:

```sh
slis create checkout
```

Use `--dry-run` to inspect the plan first. Slis records only worktrees that were
actually created, so a partial failure does not invent nonexistent slice
members or tmux panes.

To bring existing work into Slis instead:

```sh
# Create managed worktrees for an existing branch
slis adopt existing-branch

# Register a worktree that already exists where it is
slis import /path/to/existing/worktree
```

In Graphite-initialised repositories, newly created or adopted branches are
tracked best-effort. A Graphite failure does not block the Git worktree.

### 4. Work from the cockpit

Open a slice from the hub or inspect it headlessly:

```sh
slis show checkout
slis show checkout --json
```

The cockpit has four operational areas:

| Area | What it shows |
|---|---|
| Stack | Current worktree branch, downstack ancestry, health, summary, and changed files |
| PRs | Pull requests, reviews, comments, CI, rerun/fix actions, and merge readiness |
| Session | The slice's tmux or coding-agent session |
| Processes | Processes rooted in the slice, including CPU history and guarded termination |

The Stack area deliberately excludes sibling and upstack branches checked out
in other worktrees. Graphite metadata provides context; it does not redefine
which branches belong to the current slice.

Press `enter` on a stack branch to load its rich diff, or `f` to browse files at
that revision. Rich diffs support unified and split layouts, syntax-aware
rendering, line selection, and pending review comments.

Useful cockpit keys:

| Key | Action |
|---|---|
| `tab` / `1`–`4` | Cycle panels or jump directly to one |
| `j` / `k` | Move within the focused panel |
| `enter` / `l` | Open the selected branch's rich diff |
| `f` | Browse files at the selected revision |
| `b` | Cycle working-tree, parent, and trunk summary scopes |
| `c` / `V` | Add a review comment / manage pending comments |
| `w` | Activate the slice or restore the primaries |
| `a` / `C` | Attach to the agent terminal / launch the configured agent |
| `,` | Configure the default launch agent |
| `T` | Cycle System, Midnight, Violet, and Light themes |
| `esc` / `h` | Return to the hub |

### 5. Use a terminal or coding agent

With tmux installed, each slice can have an isolated session rooted in its own
worktrees. From the hub, press `a` to attach or `C` to launch the configured
agent.

The equivalent CLI workflow is available through `slis focus`, `slis status`,
and the review commands. For example:

```sh
slis status checkout --json
slis review list checkout
slis review send checkout
```

Run this once if you use Claude Code and want per-slice notifications when an
agent stops or needs input:

```sh
slis init-hooks
```

See [Configure coding agents](#configure-coding-agents) for Claude, Codex, and
custom commands.

### 6. Activate a slice in your primary checkouts

Worktrees are ideal for isolation, but a development server may already be
running from each repository's primary checkout. Activate the slice to move all
primaries to temporary `slis/live/<slice>` branches at the slice tips:

```sh
slis activate checkout
```

Slis journals the original branch in every repository. When finished, restore
all primaries together:

```sh
slis deactivate
```

If the slice advances while active, update the primaries with:

```sh
slis refresh
```

Activation refuses dirty primary checkouts unless you explicitly pass
`--stash`. That exact stash entry is restored during deactivation. Slis also
warns when common lockfiles differ, since your running application may need its
dependencies reinstalled.

### 7. Review, submit, and monitor the work

Use the TUI or the equivalent commands:

```sh
slis pr checkout
slis pr-stack checkout
slis summary checkout
slis conflicts
```

When Graphite is available:

```sh
slis restack checkout
slis submit checkout
slis merge checkout
```

`submit`, `merge`, and `sync` can change remote or repo-wide state. Inspect the
slice and command help before running them.

Every read command supports structured JSON where applicable, allowing an agent
or script to use the same data as the TUI. The full machine-facing contract is
documented in [docs/AGENT.md](docs/AGENT.md), and Slis ships an installable agent
skill in [skills/slis](skills/slis).

### 8. Clear completed work

Preview cleanup, then remove the slice's worktrees, merged local branches, and
tmux session:

```sh
slis rm checkout --dry-run
slis rm checkout
```

Cleanup is idempotent. It removes empty Slis-managed parent directories after a
successful removal, but refuses dirty worktrees, untracked files, locked
worktrees, and directories Git does not recognise. Use `--force` only after
inspecting the work that Git would otherwise protect.

PR comments remain cached after cleanup so review history is not lost.

## Configure Slis

### Workspace configuration

The main configuration file is:

```text
$XDG_CONFIG_HOME/slis/workspace.yaml
~/.config/slis/workspace.yaml   # default
```

`slis init` creates it. Restart Slis after editing it so the workspace and
session configuration are reloaded.

### Configure coding agents

For a single Claude Code or Codex integration:

```yaml
sessions:
  harness: codex       # claude (default) or codex
  layout: repos        # repos, root, or both
  autostart: false
```

To launch a custom command, set `agent`. A non-empty value takes precedence for
interactive sessions, while `harness` still selects the compatible integration
for AI summaries and `fix-ci`:

```yaml
sessions:
  harness: claude
  agent: "claude --resume"
  autostart: false
```

To choose at launch time, provide multiple named commands:

```yaml
sessions:
  default_agent: Codex
  agents:
    - name: Claude
      cmd: [claude, --resume]
    - name: Codex
      cmd: [codex, --full-auto]
```

Slis also detects installed `claude`, `codex`, `gemini`, `cursor-agent`, and
`opencode` binaries and adds them to the launch picker without replacing custom
configured commands. Press `C` from the TUI to launch an agent; the current
default is marked in the picker. Press `,` from any main TUI view to enter agent
settings, then press `Enter` to make the focused agent the default. Slis writes
that choice to `sessions.default_agent` in `workspace.yaml`; subsequent presses
of `C` launch it immediately without reopening the picker.
The last launched agent is remembered as well. Run `slis agent` to inspect the
saved choice, or `slis agent clear-default` to return to first-launch selection.
`layout: repos` creates one tmux window inside each member worktree; `root`
creates a shared parent window; `both` provides both. Multi-repo slices default
to `repos`, which avoids accidentally running Git or Graphite in an enclosing
repository outside the workspace.

### Themes and saved preferences

The default **System** theme asks the terminal whether its background is light
or dark and follows live profile changes when supported. Midnight is the
fallback when the terminal cannot report its appearance.

Press `T` in the hub, cockpit, or diff view to cycle System, Midnight, Violet,
and Light. You can also pin the launch theme:

```sh
SLIS_THEME=violet slis
SLIS_THEME=light slis
SLIS_THEME=auto slis
NO_COLOR=1 slis
```

Supported canonical names are `midnight`, `violet`, `light`, and `mono`;
`auto` or `system` follows the terminal. Common aliases such as `dark`, `blue`,
`purple`, and `monochrome` are accepted.

Theme, the legacy coding-agent fallback, diff layout, and diff scope are stored in:

```text
$XDG_STATE_HOME/slis/prefs.json
~/.local/state/slis/prefs.json   # default
```

Environment variables override saved preferences for that launch. `NO_COLOR`
always disables chromatic themes.

## Command reference

Most TUI actions have a CLI equivalent. Run `slis <command> --help` for all
options.

| Command | Purpose |
|---|---|
| `slis init [root]` | Scan a workspace and write its configuration |
| `slis ls` | List managed slices and discovery warnings |
| `slis show <slice>` | Show slice members and per-repo stack context |
| `slis create <slice>` | Create a branch and worktree in every repository |
| `slis adopt [branch]` | Create managed worktrees for existing work |
| `slis candidates` | List discovered but unmanaged worktrees |
| `slis import [path]` | Register an existing worktree as a slice |
| `slis ignore <glob>` | Exclude unknown worktrees from discovery |
| `slis forget <slice>` | Remove registry ownership without touching Git |
| `slis activate <slice>` | Put all primaries on the slice tips |
| `slis deactivate` | Restore every primary to its journalled branch |
| `slis refresh` | Advance active primaries to newer slice tips |
| `slis rm <slice>` | Remove completed worktrees, branches, and session |
| `slis pr <slice>` | Show pull-request and CI status |
| `slis pr-stack <slice>` | Produce a shareable PR-stack summary |
| `slis summary <slice>` | Show commit or AI-generated prose summaries |
| `slis review ...` | Add, list, remove, clear, or send review comments |
| `slis conflicts` | Find files changed by more than one slice |
| `slis status [slice]` | Show per-slice agent/session status |
| `slis restack/submit/merge/sync` | Run Graphite stack operations |
| `slis ci-rerun <slice>` | Rerun failed GitHub Actions jobs |
| `slis branch-diff/tree/cat` | Inspect stack revisions without checkout |
| `slis group/ungroup` | Override automatic branch-name grouping |
| `slis edit <slice>` | Open all member worktrees in one editor workspace |
| `slis agent` | Show, set, or clear the default coding agent |
| `slis doctor` | Diagnose configuration and workspace health |
| `slis init-hooks` | Install Claude Code status hooks |
| `slis init-skill` | Install the Slis skill for Claude Code or Codex |
| `slis ui` | Explicitly launch the OpenTUI front-end |
| `slis rpc` | Run the JSON-RPC sidecar used by the front-end |

Read-oriented commands—including `ls`, `show`, `status`, `pr`, `pr-stack`,
`summary`, `conflicts`, `comments`, `doctor`, `candidates`, `branch-diff`,
`tree`, and `cat`—support `--json`.

## Upgrading

### Homebrew

```sh
brew update
brew upgrade slis
```

Release archives contain a matching Go core and standalone OpenTUI front-end.
Keep `slis` and `slis-ui` from the same release together; the front-end starts
the Go binary as its sidecar.

### Existing workspaces

No migration command is required.

On the first registry-aware launch, Slis records existing discovered worktrees
so an upgrade does not make a working setup disappear. Later launches:

- backfill older Slis-created worktrees into an existing registry;
- refresh saved branch and path identities after legitimate changes;
- quarantine malformed legacy registries as
  `registry.yaml.broken-<timestamp>` and rebuild from healthy worktrees;
- remove one exact stale Git administrative record when a Slis-owned checkout
  is already gone;
- remove missing managed registry entries and empty directory litter only under
  `<workspace>/.slis/worktrees`.

Startup repair never deletes a live or non-empty worktree, an external/imported
missing worktree, a branch ref, or a commit. Missing external worktrees remain
visible for manual recovery, including worktrees on temporarily unavailable
volumes.

Existing workspace configuration—including `sessions.default_agent`—remains in
the XDG config directory. Theme and diff preferences, plus the legacy agent
fallback, remain in the XDG state directory and are reused by new releases.

After upgrading, a useful smoke check is:

```sh
slis doctor
slis ls
```

## Safety model

Slis coordinates operations across repositories, so it is conservative by
default:

- worktree removal uses Git ownership checks and refuses ambiguous directories;
- dirty primary checkouts block activation unless `--stash` is explicit;
- deactivation uses a journal to restore exact prior branches and stash entries;
- a committed-on temporary activation branch is rescued rather than discarded;
- registry writes are atomic;
- automatic repair is restricted to missing Slis-owned paths and empty
  directories;
- startup repair never untracks Graphite branches merely because they are not
  members of the selected slice;
- `create`, `rm`, and `fix-ci` provide dry-run workflows;
- remote Graphite and GitHub operations remain explicit commands.

The cockpit shows only a member's current branch and its downstack ancestry.
Sibling or upstack branches appearing in Graphite metadata may be valid work in
other worktrees; hiding them from the slice is safer than automatically
untracking or deleting them.

## Contributing

Bug reports, workflow descriptions, design feedback, and pull requests are all
welcome. Slis grew from one specific multi-repo workflow, so examples of where
its assumptions do not hold are particularly useful.

When reporting a bug, please include:

- operating system and installation method;
- `slis` version;
- relevant output from `slis doctor`;
- whether `tmux`, `gh`, or `gt` is involved;
- the smallest reproducible repository/worktree layout;
- screenshots for visual TUI issues, with sensitive repository information
  removed.

Do not include repository contents, tokens, private PR data, or unredacted agent
prompts in an issue.

### Development setup

The Go core requires Go 1.25 or newer. The OpenTUI front-end requires Bun
1.3.14 or newer.

```sh
git clone https://github.com/jonnyom/slis
cd slis

go build -o slis ./cmd/slis

cd tui-js
bun install --frozen-lockfile
cd ..
```

Bun 1.3.10 can produce an apparently successful `slis-ui` build that crashes
while loading bundled OpenTUI worker assets. The build script rejects versions
older than 1.3.14.

### Run locally

Run the Go CLI or legacy TUI:

```sh
go run ./cmd/slis ls
SLIS_TUI=go go run ./cmd/slis
```

Run the OpenTUI source against the built Go sidecar:

```sh
cd tui-js
SLIS_BIN=../slis bun run start

# Fixture data only; does not need a workspace or sidecar
bun run start:fake
```

Exercise the normal launcher from the repository root:

```sh
SLIS_TUI_DIR="$PWD/tui-js" ./slis
```

Compile a standalone front-end beside the Go binary:

```sh
cd tui-js
bun run ./scripts/require-bun-version.ts 1.3.14
bun build --compile ./src/index.tsx --outfile ../slis-ui
cd ..
./slis
```

`slis-ui` embeds Bun, OpenTUI, and Ghostty's native libraries, but still needs
the matching `slis` executable as its Go sidecar.

### Test changes

```sh
# Go core, CLI, RPC, reports, and legacy TUI
go test ./...
CGO_ENABLED=0 go build ./...

# OpenTUI types and unit tests
cd tui-js
bun run typecheck
bun test

# Optional tmux/review smoke tests
bun run term:e2e
bun run term:picker:e2e
bun run review:e2e
```

Before opening a pull request:

- add regression coverage for behavioral changes;
- run the relevant Go and OpenTUI suites;
- update this README or [docs/AGENT.md](docs/AGENT.md) when a user-facing or
  machine-facing contract changes;
- keep cleanup and migration behavior conservative—never infer permission to
  delete branches, commits, non-empty directories, or external worktrees;
- keep `slis` and `slis-ui` compatibility in mind when changing RPC data.

The main implementation areas are:

```text
cmd/slis/           Go executable
internal/cli/       CLI commands
internal/discovery/ worktree discovery and durable slice membership
internal/report/    data shared by CLI, RPC, and TUIs
internal/tui/       legacy Bubble Tea interface
tui-js/src/         OpenTUI front-end
docs/AGENT.md       JSON and agent automation contract
```

Release builds use `scripts/build-slis-ui.sh` to cross-compile the OpenTUI
front-end for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64`.
GoReleaser packages it with the matching Go binary and updates the Homebrew tap.

## Project status

Slis is an opinionated personal tool that is now being shared for others who
have the same problem. It is used regularly, but you should still expect rough
edges and occasional breaking changes while the workflow settles.

The project was built extensively with coding agents. Changes are reviewed and
covered by automated tests, but the project does not claim the maturity or
support guarantees of a commercial developer platform.

Slis is released under the [MIT License](LICENSE).
