# slis OpenTUI front-end

This directory contains the primary terminal interface for
[`slis`](../README.md). It is built with
[OpenTUI](https://github.com/anomalyco/opentui)'s React reconciler and Bun,
and uses the Go `slis` binary for workspace discovery and all mutations.

The UI has two main views:

- The **hub** filters and searches every slice, shows stack/PR/CI/session
  state, previews the focused slice, and exposes batch operations.
- The **cockpit** focuses on one slice, with Stack, PRs, Session, and Processes
  panels plus a context-sensitive detail pane. Its diff viewer supports file
  navigation, unified and side-by-side rendering, range selection, and review
  comments that can be sent to the slice's agent.

Slice sessions and interactive commands run in embedded terminal tabs backed
by tmux. Tabs stay mounted while open, so switching back to the UI does not
stop the shell, agent, or command.

## How it talks to the Go core

The front-end starts a long-lived `slis rpc` sidecar and exchanges JSON-RPC
2.0 messages over newline-delimited JSON on stdio. Reads such as workspace
state, diffs, PRs, stack data, process samples, file trees, and pending reviews
go through that sidecar.

Mutations remain behind the Go CLI's existing safety checks:

- Non-interactive actions run as captured one-shot `slis` processes.
- Commands that may prompt, including submit, sync, merge, adopt, and fix-CI,
  run in an embedded PTY tab.
- tmux is the persistence layer for slice shells and coding-agent sessions.

The sidecar reconnects with backoff after a crash. The UI shows a disconnect
banner and refreshes workspace state once the connection returns.

## Prerequisites

- **Bun 1.3.14 or newer.** Older Bun compilers can produce a standalone binary
  that crashes while OpenTUI loads its embedded worker assets.
- A built `slis` Go binary for real workspace data and mutations.
- `tmux` for embedded session tabs, agent launch, and review delivery.

`gh`, `gt`, a configured editor, and a coding-agent CLI enable their
corresponding PR, stack, editor, and agent features but are not required to
start the UI.

## Run from source

Install the locked dependencies, then point the front-end at the Go binary:

```sh
cd tui-js
bun install --frozen-lockfile
SLIS_BIN=../slis bun run start
```

To exercise the interface without a workspace, repositories, or Go sidecar,
use the in-process fixtures:

```sh
bun run start:fake
```

From the repository root, the Go launcher's development fallback can run the
same source tree:

```sh
SLIS_TUI_DIR="$PWD/tui-js" ./slis
```

Bare `slis` and `slis ui` otherwise look for a compiled `slis-ui` next to the
Go binary. Bare `slis` falls back to the legacy Go TUI when the OpenTUI binary
is unavailable; `SLIS_TUI=go` selects that fallback explicitly.

## Build

Compile a standalone front-end for the current machine next to the Go binary:

```sh
cd tui-js
bun run ./scripts/require-bun-version.ts 1.3.14
bun install --frozen-lockfile
bun build --compile ./src/index.tsx --outfile ../slis-ui
```

The executable embeds Bun and OpenTUI's/Ghostty's native libraries, but it
still needs the matching `slis` binary beside it. To build every release target
(`darwin` and `linux`, `amd64` and `arm64`), run this from the repository root:

```sh
./scripts/build-slis-ui.sh
```

Outputs are written to `tui-js/dist/<goos>-<goarch>/slis-ui`.

## Environment

| Variable | Meaning |
|---|---|
| `SLIS_BIN` | Go sidecar/CLI binary. Defaults to `slis` on `PATH`. |
| `SLIS_FAKE=1` | Use in-process fixtures instead of a real sidecar. The `start:fake` script sets this. |
| `SLIS_THEME` | Startup theme: `auto`, `system`, `midnight`, `violet`, `light`, `mono`, or `mono-light`. `dark`/`blue`, `purple`, and `monochrome` are aliases. |
| `NO_COLOR` | Force a monochrome palette. |
| `SLIS_TERM_BACK_KEY` | Hex byte for the embedded-terminal back key. Defaults to `11` (`ctrl+q`). |

With no explicit theme, the UI asks the terminal for its light/dark appearance
and falls back to Midnight when that query is unsupported. Press `T` outside a
modal or terminal tab to cycle Midnight, Violet, and Light.

## Keyboard model

Press `?` in the hub or cockpit for the complete context-sensitive reference.
Arrow keys work alongside the documented Vim-style motions.

### Hub

| Key | Action |
|---|---|
| `tab` | Switch focus between the state rail and slice list. |
| `j` / `k` | Move within the focused area. |
| `1`–`8` | Select a state filter. |
| `/` | Search slice names; `n` / `N` moves between matches. |
| `enter` / `l` | Open the focused slice's cockpit. |
| `space` / `A` | Select one slice / all visible slices for batch actions. |
| `c` | Create a slice in the background. |
| `i` / `I` | Import discovered worktrees / adopt an existing branch. |
| `m` / `u` | Group selected slices / ungroup the focused slice. |
| `w` | Activate or deactivate the focused slice. Dirty activation can stash first. |
| `R` | Open stack actions: restack, submit, merge, or sync. |
| `d` | Clear finished slices, with guarded force removal when requested. |
| `v` / `F` | Open failing-CI detail / launch the fix-CI flow. |
| `a` / `C` | Attach to the slice terminal / launch a configured agent. |
| `V` / `,` | Manage pending review comments / configure the default agent. |
| `e` / `o` | Open the slice in an editor. |
| `Y` | Copy PR-stack Markdown to the clipboard. |
| `P` / `!` | Show all-slice processes / cross-slice conflict radar. |
| `r` · `T` · `?` · `q` | Refresh · cycle theme · help · quit. |

### Cockpit

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Focus the next / previous panel. |
| `1`–`4` | Jump to Stack, PRs, Session, or Processes. |
| `j` / `k` | Move the selection in the focused panel. |
| `enter` / `l` | Open the selected stack branch's rich diff. On other panels, `enter` zooms the detail pane. |
| `f` | Browse the selected stack branch's file tree at that revision. |
| `t` / `b` | Toggle stat/patch (or unified/split in the rich diff) / cycle diff scope. |
| `c` / `V` | Add an inline comment / manage and send the pending review. |
| `y` / `Y` | Copy the current diff / PR-stack Markdown. |
| `v` / `ctrl+r` / `F` / `O` | View CI logs, rerun failed CI, fix CI, or open the focused PR. |
| `s` / `S` | Show a cached AI summary / force regeneration. |
| `x` / `X` | Send SIGTERM to the selected process / process subtree. |
| `a` / `C` | Open the slice's embedded terminal tab / launch an agent. |
| `,` | Configure the default launch agent. |
| `e` / `o` | Open the whole slice / selected repo in an editor. |
| `esc` / `h` | Step back or return to the hub. |

In the rich diff, `tab`/`enter` moves focus between the file list and diff,
`[`/`]` or `p`/`n` jumps between hunks, `v`/`space` starts a line range, and
`c` comments on the current line or range. In an embedded terminal, all input
goes to the PTY; `ctrl+q` returns to the UI, while tmux's own detach binding is
`ctrl+b d`.

## Tests

```sh
# TypeScript and unit/integration tests
bun run typecheck
bun test

# PTY smoke tests (tmux is required for terminal tests)
bun run term:e2e
bun run term:picker:e2e
bun run review:e2e
```

The normal test suite covers state derivation, diffs, themes, overlays,
reviews, process handling, tmux setup, and keyboard behavior. The smoke tests
drive the real app inside a PTY.

## Source map

```text
src/
  index.tsx             renderer setup, terminal theme detection, root mount
  app.tsx               RPC lifecycle, routing, refresh, overlays, terminal tabs
  theme.ts              semantic palettes, syntax/diff colors, glyphs
  views/
    browser.tsx         hub: state rail, slice list, preview, batch selection
    cockpit.tsx         slice panels, stack/file review, PR/session/process detail
  components/           shared panels, cards, hints, diffs, files, processes, toasts
  diff/                 parsing, row models, tokenization, unified/split rendering
  overlays/             confirmations, inputs, stack actions, reviews, pickers
  rpc/                  JSON-RPC client, fixtures, types, mutation runners
  review/               inline-review context and end-to-end coverage
  state/                derived states, grouping, selection, stack/file navigation
  term/                 tmux setup, PTY sessions, command sessions, terminal tabs
  proc/                 process trees, sampling history, sorting, sparklines, kills
  editor/               editor discovery and selection
```

The protocol and JSON shapes are documented in [`../docs/AGENT.md`](../docs/AGENT.md).
The original design record in
[`../docs/plans/2026-07-18-js-tui-spike.md`](../docs/plans/2026-07-18-js-tui-spike.md)
is historical context, not the current feature reference.
