# slis

`slis` ("slice", from the Irish *slis*) is a terminal cockpit for managing one
feature across several Git repositories.

It groups the feature's branches and worktrees into a single **slice**, then
puts its diffs, stacked branches, pull requests, CI, terminal sessions, processes,
and coding agents in one place. The OpenTUI interface is backed by a Go CLI, so
the same workflows are available interactively, from scripts, or to an agent.

<img width="1539" height="1395" alt="2026-08-12 16 00 29" src="https://github.com/user-attachments/assets/c280c0f3-5707-4521-8cc0-b577667d1b9d" />


Slis is primarily a worktree and workspace manager. Coding-agent support is
useful, but entirely optional.

## Quick start

Slis expects one workspace directory containing your repositories:

```text
~/work/acme/
├── web/
├── api/
└── worker/
```

Install Slis, initialize that directory once, then open the TUI:

```sh
brew trust --cask jonnyom/tap/slis
brew install jonnyom/homebrew-tap/slis

cd ~/work/acme
slis init .
slis
```

Daily work happens inside the TUI:

1. Press `c` in the hub to create a slice. Enter a feature name such as
   `checkout`. Slis creates matching branches and worktrees in each repository.
2. Select the slice with `j` or `k`, then press `enter` to open its cockpit.
3. Press `C` to launch your default coding agent, `L` to choose another agent,
   or `t` to open a shell.
4. Select a branch and press `enter` to review its diff. GitHub review comments
   appear beside the relevant code when `gh` is installed and authenticated.
5. Press `d` when the slice is finished. Slis checks the worktrees before it
   removes anything.

Press `?` in any main view for the keys available there. Press `esc` to move
back one level.

If your branches use a personal prefix, initialize with
`slis init . --strip-prefix your-name/`. Slis removes that prefix when it groups
branches into slices.

## What a slice is

Suppose a feature called `checkout` requires changes in three repositories:

```text
checkout
├── web       → worktree on branch checkout
├── api       → worktree on branch checkout
└── payments  → worktree on branch checkout
```

Without Slis, those worktrees, pull requests, terminals, and branch states are
separate things you have to keep aligned yourself. Slis puts them under one
entry in the hub. Open that entry to review the whole feature, run its agents,
swap it into primary checkouts, or clear it when the work is done.

The TUI calls that single-feature view the **cockpit**. It first shows stack
position, PR and CI state, changed files, session status, and processes. It
loads a full diff or file browser only when requested.

Slis also works in a single repository. The multi-repo workflow is where the
slice abstraction becomes most useful.

## Why I built it

I built Slis for a very specific personal problem: I work across multiple
repositories a lot. A single feature might involve a frontend, a Rails API, an
MCP service, several worktrees, a Graphite stack, a handful of PRs, and one or
more coding agents. All the individual tools worked, but keeping the whole unit
of work in my head did not.

Slis is the cockpit I wanted for that workflow. It treats a feature, not a
repository or branch, as the thing I am working on. It works with coding
agents, but they are not the point; the worktree and slice model is useful
without them.

I also want to be completely candid: **I 100% vibe coded this.** I built it with
coding agents because I had a concrete problem I wanted solved, and at the
beginning code completeness mattered much less to me than making the workflow
real. The project has since gained a substantial test suite, safety checks, and
release tooling because I use it for actual work. I am not interested in
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
- you want a separate terminal or coding-agent session for each feature;
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
- you want a shared, hosted project-management system. Slis is a local developer
  cockpit, not a team planning database;
- you want Slis to hide Git entirely. It adds guardrails, but worktrees,
  branches, rebases, and dirty files still matter;
- you need a graphical desktop application rather than a terminal interface;
- you require Windows binaries. Current releases target macOS and Linux;
- you expect every integration without installing its tool. Graphite, GitHub,
  and coding-agent features degrade independently when their binaries are
  unavailable.

You also do not need Slis merely to use an AI coding agent. Its value comes from
managing the surrounding Git and multi-repo workflow.

## Integrations

The Homebrew package includes Slis and its private session runtime. You do not
need a separate terminal multiplexer. Slis starts without the tools below and
hides or disables the features that need them.

| Tool | Enables |
|---|---|
| `gh` | Pull-request, review-comment, and CI views |
| `gt` | Graphite stack reading, restacking, submission, sync, and merge |
| Claude Code or Codex | Agent sessions, AI summaries, and `fix-ci` |

## TUI guide

### The hub

The **hub** lists every managed slice and highlights work that needs attention,
is active, is in review, or is ready to clear. Use `j`/`k` to navigate and
`enter` to open a slice's cockpit. Press `?` anywhere for contextual help.

Unknown worktrees appear as candidates instead of entering the workspace
silently. Press `i` to import discovered worktrees or `I` to adopt an existing
branch as a managed slice.

Useful hub keys:

| Key | Action |
|---|---|
| `j` / `k` | Move between slices |
| `enter` | Open the focused slice |
| `c` | Create a slice |
| `/` | Search slices |
| `w` | Swap the slice into or out of primary checkouts |
| `d` | Clear a finished slice |
| `C` / `L` / `t` | Launch the default agent, choose an agent, or open a shell |
| `s` | Browse all running sessions |
| `R` | Open Graphite stack actions |

### Create or import a slice

Press `c` in the hub, enter a name, and confirm. Slis creates one branch and
worktree in every configured repository. Creation runs in the background, so
you can keep using the hub while it works.

For work that already exists, press `i` to import discovered worktrees or `I`
to adopt a branch. Slis shows the candidates before it changes the workspace.

### The cockpit

The cockpit has five panels:

| Area | What it shows |
|---|---|
| Stack | Current worktree branch, downstack ancestry, health, summary, and changed files |
| PRs | Pull requests, reviews, comments, CI, rerun/fix actions, and merge readiness |
| Reviews | Local comments waiting to be sent to an agent |
| Session | The slice's terminal and coding-agent sessions |
| Processes | Processes rooted in the slice, including CPU history and guarded termination |

The Stack area deliberately excludes sibling and upstack branches checked out
in other worktrees. Graphite metadata provides context; it does not redefine
which branches belong to the current slice.

Press `enter` on a stack branch to load its rich diff, or `f` to browse files at
that revision. Rich diffs support unified and split layouts, syntax-aware
rendering, wrapped source lines, line selection, and pending review comments.

GitHub review comments appear below the lines they refer to. The file list keeps
the diff totals and comment count visible beside each path, even when the path
itself is too long to fit. Press `2` from the diff to open the selected PR's full
comment summary. Long bot comments use a shorter inline preview so they do not
push the surrounding code off screen.

Useful cockpit keys:

| Key | Action |
|---|---|
| `tab` / `1` through `5` | Cycle panels or jump directly to one |
| `j` / `k` | Move within the focused panel |
| `enter` / `l` | Open the selected branch's rich diff |
| `f` | Browse files at the selected revision |
| `b` | Cycle working-tree, parent, and trunk summary scopes |
| `c` / `V` | Add a local review comment / manage comments waiting for the agent |
| `2` in a diff | Open GitHub comments for the selected PR |
| `w` | Activate the slice or restore the primaries |
| `a` / `C` | Open the existing agent tab / launch the default agent |
| `L` / `t` | Launch another agent type / open a shell tab |
| `,` | Configure the default launch agent |
| `T` | Cycle System, Midnight, Violet, and Light themes |
| `esc` / `h` | Return to the hub |

### The agent dock

Each slice has an isolated, persistent Slis session rooted at the directory
containing its repo worktrees. Slis ships and manages its own session runtime,
so you do not need to install or configure a terminal multiplexer.

The agent dock follows the slice selected in the hub or cockpit. Press `a` to
open an existing agent tab, `C` to launch the default agent, `L` to choose a
different agent, or `t` to open a shell. A slice can keep several agent and
shell tabs alive at once. Click a tab to switch to it, click its `×` to stop
that tab, press `ctrl+g` to toggle focus between Slis and the dock, or
press `ctrl+q` to hide the dock without stopping anything. New shell tabs start
with your login shell from `$SHELL`. While the dock has focus, press
`ctrl+shift+right` for the next tab or `ctrl+shift+left` for the previous tab.

Set `sessions.layout: repos` if you want one terminal tab per repository.

See [Configure coding agents](#configure-coding-agents) for Claude, Codex, and
custom commands.

### Swap a slice into your primary checkouts

Worktrees are ideal for isolation, but a development server may already be
running from each repository's primary checkout. Press `w` to swap the focused
slice into those checkouts. Press `w` again to restore their original branches.
Slis records the original branch in every repository.

Slis refuses dirty primary checkouts unless you choose the stash option in the
confirmation dialog. It restores that exact stash when you swap back. It also
warns when common lockfiles differ, since your running application may need its
dependencies reinstalled.

### Review and ship

The PRs panel shows review state and CI. Press `v` to inspect failing CI, `F` to
send the failure to an agent, or `O` to open the focused PR in your browser.

Press `R` for Graphite actions such as restack, submit, sync, and merge. Slis
shows the command before it changes remote or repository-wide state.

In a diff, select code with `v` or `space`, extend the range with `j` or `k`,
then press `c` to write a note for the agent. Press `V` to inspect, delete, or
send pending notes. GitHub comments from teammates appear inline beside their
code and in the PR comment summary.

### Clear completed work

Press `d` on a finished slice in the hub or cockpit. Slis shows what it will
remove before it touches the worktrees, local branches, or session.

Cleanup is idempotent. It removes empty Slis-managed parent directories after a
successful removal, but refuses dirty worktrees, untracked files, locked
worktrees, and directories Git does not recognise. Force removal remains an
explicit choice in the confirmation dialog.

PR comments remain cached after cleanup so review history is not lost.

## Configure Slis

### Workspace configuration

Slis selects the workspace that contains the current directory. Each workspace
has separate configuration, preferences, session state, and zmx sessions.

`slis init` creates:

```text
$XDG_CONFIG_HOME/slis/workspaces/<workspace-id>/workspace.yaml
~/.config/slis/workspaces/<workspace-id>/workspace.yaml   # default
```

Run `slis init .` once from each workspace root. Running Slis outside an
initialized workspace shows the setup prompt instead of opening another
workspace. Restart Slis after editing its configuration.

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
      cmd: [codex]
```

Slis also detects installed `claude`, `codex`, `gemini`, `cursor-agent`, and
`opencode` binaries and adds them to the launch picker without replacing custom
configured commands. Press `C` from the TUI to launch an agent; the current
default is marked in the picker. Press `,` from any main TUI view to enter agent
settings, then press `Enter` to make the focused agent the default. Slis writes
that choice to `sessions.default_agent` in `workspace.yaml`; subsequent presses
of `C` launch it immediately without reopening the picker.
The last launched agent is remembered as well. Press `,` whenever you want to
change the default.
`layout: repos` creates one terminal tab inside each member worktree; `root`
creates a shared parent tab; `both` provides both. Multi-repo slices default
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

Theme, the legacy coding-agent fallback, diff layout, and diff scope are stored
per workspace in:

```text
$XDG_STATE_HOME/slis/workspaces/<workspace-id>/prefs.json
~/.local/state/slis/workspaces/<workspace-id>/prefs.json   # default
```

Environment variables override saved preferences for that launch. `NO_COLOR`
always disables chromatic themes.

## CLI and automation

The CLI backs the TUI and exists for setup, scripts, and coding agents. Most
people only need it once:

```sh
slis init ~/work/acme
slis doctor
```

Run `slis --help` to see the full command set. Read commands support JSON where
applicable. The machine-facing contract lives in
[docs/AGENT.md](docs/AGENT.md), and Slis ships an agent skill in
[skills/slis](skills/slis).

## Upgrading

### Homebrew

```sh
brew update
brew upgrade slis
```

Release archives contain a matching Go core, OpenTUI front-end, and private
session runtime. Keep all three files from the same release together.

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

Existing workspace configuration, including `sessions.default_agent`, remains
at the legacy XDG path and keeps its existing state. New workspaces use separate
workspace directories under the XDG config and state directories.

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
- whether `gh`, `gt`, or a coding agent is involved;
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

Build the Go CLI and private session runtime:

```sh
make build
./slis ls
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
# Go core, CLI, RPC, reports, and session management
go test ./...
CGO_ENABLED=0 go build ./...

# OpenTUI types and unit tests
cd tui-js
bun run typecheck
bun test

# Optional terminal/review smoke tests
bun run term:e2e
bun run term:picker:e2e
bun run review:e2e
```

Before opening a pull request:

- add regression coverage for behavioral changes;
- run the relevant Go and OpenTUI suites;
- update this README or [docs/AGENT.md](docs/AGENT.md) when a user-facing or
  machine-facing contract changes;
- keep cleanup and migration behavior conservative. Never infer permission to
  delete branches, commits, non-empty directories, or external worktrees;
- keep `slis` and `slis-ui` compatibility in mind when changing RPC data.

The main implementation areas are:

```text
cmd/slis/           Go executable
internal/cli/       CLI commands
internal/discovery/ worktree discovery and durable slice membership
internal/report/    data shared by CLI, RPC, and UI
internal/session/   Slis session ownership and lifecycle
internal/zmxctl/    private session-runtime adapter
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
