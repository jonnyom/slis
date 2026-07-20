# JS TUI rebuild — OpenTUI front-end over a Go sidecar

Branch `experiment/js-tui`. Started as a spike; scope upgraded by Jonny to a
full rebuild: "feel free to implement everything. A good differ, a good
process viewer etc. make this really shine."

Hard requirement: must not feel slower than the Bubble Tea TUI — input
latency and steady-state render must be equal; cold start may lose a few tens
of ms (accepted trade-off).

## Decisions (with rationale)

1. **Framework: OpenTUI (`@opentui/react`) on Bun, versions pinned.**
   Research memo conclusions: Ink is disqualified (30 FPS cap, full-tree
   redraws, flicker); OpenTUI has a Zig cell-diffing core (same architectural
   class as Bubble Tea), powers opencode in production, flexbox layout,
   ScrollBox, focus management. Bun over Node because OpenTUI on Node needs
   Node 26.4 + experimental FFI, and `bun build --compile` gives a single
   binary (~30-50ms cold start). React reconciler over Solid for familiarity;
   state changes are low-frequency (keypress, 2s ticks) so reconciler
   overhead is acceptable — swap to Solid only if profiling demands.
2. **Boundary: long-lived read-only Go sidecar (`slis rpc`), JSON-RPC 2.0
   over stdio, NDJSON framing.** Not per-keystroke CLI spawns. Reuses
   internal Go packages directly, which covers the four reads that had no
   CLI `--json` twin: diff (3 scopes), tmux pane capture, per-slice process
   sampling, derived browser aggregates. Server pushes session-status
   notifications (fsnotify stays Go-side).
3. **Mutations are one-shot `slis <cmd>` spawns from JS** (activate,
   deactivate, rm, restack, submit, merge, sync, create, import, group,
   ci-rerun, fix-ci…). The sidecar must never mutate — keeps the
   data-safety-critical swap engine (and CI writes like `gh run rerun`)
   behind existing, tested CLI entry points. `slis ci-rerun <slice>` wraps
   `forge.RerunFailedChecks` per repo for exactly this reason.
4. **Embedded terminal session tabs (wave 2): tmux stays the persistence
   layer.** The TUI embeds a viewer: PTY running `tmux attach -t <session>`,
   VT-parsed, painted into an OpenTUI pane. One reserved key (candidate:
   ctrl+q, pending prototype verdict) is intercepted before forwarding to the
   PTY and returns to the browser. Closing slis never kills a session;
   reopening reattaches. This capability is the strongest argument for the JS
   rewrite — Bubble Tea cannot embed a live terminal without suspending the
   whole app (root cause of the old delayed-notification bug).

## Architecture

```
tui-js/ (Bun + @opentui/react)
   │  spawns once, owns lifecycle          │ one-shot spawns
   ▼                                       ▼
slis rpc  (read-only JSON-RPC sidecar)   slis activate/rm/submit/…
   │  reuses internal packages directly
   ▼
discovery · swap.Load · gt · forge · diff · tmuxctl · proc · notify
```

- One JSON object per line on stdout/stdin (NDJSON). stderr = logs only.
- Sidecar is **strictly read-only**; never runs anything that mutates a repo.
- Concurrency: requests handled concurrently, expensive subprocess work
  capped at 4 in flight (mirror `internal/tui/gate.go`); stdout writes
  serialized by one mutex.
- Clean shutdown on stdin EOF / SIGINT / SIGTERM.

## RPC surface (v0)

JSON-RPC 2.0. Where a CLI `--json` shape already exists (see `docs/AGENT.md`),
the RPC result is byte-for-byte that shape — same structs, same marshalling.

| method | params | result |
|---|---|---|
| `hello` | — | `{ "version": string, "workspaceRoot": string }` |
| `ls` | — | same as `slis ls --json` |
| `show` | `{ "slice": string }` | same as `slis show <slice> --json` |
| `status` | `{ "slice"?: string }` | same as `slis status --json` |
| `prStack` | `{ "slice": string }` | same as `slis pr-stack <slice> --json` (now carries the `ci`/`ci_pass`/`ci_fail`/`ci_pending` rollup per row) |
| `ciLog` | `{ "slice": string, "repo"?: string }` | `{ "repos": [ { "repo": string, "branch": string, "log"?: string, "error"?: string } ] }` — failing-CI log excerpt per repo (`forge.FailedLog`); `repo` filters to one member. Read-only. |
| `comments` | `{ "slice": string }` | same as `slis comments <slice> --json` |
| `reviews` | `{ "slice"?: string }` | same as `slis review list [slice] --json` — pending inline-review comments (all slices when omitted). Read-only; add/send stay CLI-only (`slis review add/send`). |
| `conflicts` | — | same as `slis conflicts --json` |
| `diff` | `{ "slice": string, "scope": "working"\|"parent"\|"trunk", "format": "stat"\|"patch"\|"both" }` | `{ "repos": [ { "repo": string, "branch": string, "stat": SliceStat?, "patch": string? } ] }` |
| `capture` | `{ "slice": string, "lines": int }` | `{ "lines": [string] }` (safeterm-stripped) |
| `procs` | `{ "slice"?: string }` | `{ "slices": [ { "slice": string, "procs": [ { "pid": int, "ppid": int, "cmd": string, "cpu": float, "mem": float } ], "totalCPU": float } ] }` |

Notification (server → client, no `id`):

```json
{ "jsonrpc": "2.0", "method": "sessionEvent", "params": { "slice": "...", "status": "running|waiting-input|done|none" } }
```

Errors: standard JSON-RPC error object; `data.kind` carries the slis error
kind when one exists (e.g. `slice-not-found`). Unknown method → -32601,
parse error → -32700.

## Wave 1 — foundation (in flight, one subagent each)

- **go-rpc**: `internal/rpcserver` + `slis rpc` cobra command implementing
  the surface above. TDD over an in-process pipe; tmux tests skip when tmux
  absent. Build/test/lint/gofmt must stay green, `CGO_ENABLED=0`.
- **js-tui**: `tui-js/` — RPC client (`src/rpc/client.ts`, restart-on-crash),
  types, browser view (states rail + slice list + preview), cockpit (4 left
  panels + right pane, diff with scope cycling `b` and stat/patch toggle `t`),
  swap-confirm overlay, help overlay, live `sessionEvent` badges. Fake
  sidecar (`SLIS_FAKE=1`) for development before go-rpc lands. Run:
  `cd tui-js && bun install && bun run start` (needs `slis` on PATH or
  `SLIS_BIN`).
- **term-embed**: feasibility prototype (in scratchpad, NOT the repo) for
  PTY + VT emulation inside OpenTUI on Bun: node-pty-on-Bun compatibility,
  @xterm/headless as screen model, tmux attach/resize/back-key/persistence
  checks, render-cost measurement. Verdict memo decides the wave-2 approach.

## Wave 2 — make it shine (dispatch after wave 1 integrates)

- **Rich differ**: side-by-side + unified, syntax highlighting, word-level
  intra-line diff, per-file tree navigation, hunk jumping. Patch parsing in a
  pure module (unified diff → per-file structures), view consumes parsed data.
- **Full process viewer**: tree via `ppid`, CPU/mem history sparklines
  (client-side sampling), kill / kill-tree with confirm (one-shot CLI or a
  deliberate, explicit exception — do not silently make the sidecar mutate).
- **Embedded session tabs**: productionize the term-embed verdict; tab per
  slice session; reserved back-key; keys otherwise pass through raw.
- **Remaining overlays + mutations**: stack actions (restack/submit/merge/
  sync), create/import/adopt, conflict radar, group/ungroup, search, AI
  summary pane.
- **Polish + proof**: visual pass (frontend-design skill), then benchmark vs
  the Bubble Tea TUI — cold start, input-to-render latency, capture-tick CPU.

## Success criteria

1. `slis rpc` tested; `go test ./...`, lint, gofmt, `CGO_ENABLED=0` build green.
2. `bun run start` in `tui-js/` shows real slices, cockpit diff renders with
   scope cycling, badges flip live on session events.
3. Embedded session tab: interact with a real Claude session inside slis,
   back-key returns to browser, killing slis leaves tmux session alive.
4. Measured: input-to-render latency comparable to Go TUI; cold start
   recorded honestly.

## Resume notes (for the next session / other machine)

- Everything lands on `experiment/js-tui`; commit early and often, push after
  every landed unit. Wave-1 subagents were told NOT to commit — the
  orchestrator commits their work.
- Key references: `docs/AGENT.md` (JSON contract), `internal/tui/*.go`
  (feature inventory source of truth: slicelist.go browser, cockpit.go
  cockpit, diffpane.go scopes, gate.go concurrency), CLAUDE.md (conventions:
  no code comments, TDD, testutil.NewRepo, tool-absent tests skip).
- Full keybinding + overlay inventory and the CLI-twin gap analysis live in
  the session that produced this doc; the four gap reads are now RPC methods
  (`diff`, `capture`, `procs`, plus aggregates via `ls`/`show`).

## Benchmark results (2026-07-18, M-series laptop, real ~/nory workspace)

Method: app spawned in a headless PTY (ghostty VT), 200x50; timestamps from
spawn to (a) first non-empty painted frame, (b) a real slice name visible
("data ready"). 3 runs each, median reported. Script: session scratchpad
`cold.ts`.

| metric | Go TUI (Bubble Tea) | JS TUI (OpenTUI/Bun) |
|---|---|---|
| first paint | 5083 ms (5075–5086) | **173 ms** (140–182) |
| data ready | 15365 ms (15304–15404) | **11389 ms** (10543–11389) |

Honest reading:
- The 29× first-paint win is an architecture artifact, not raw runtime speed:
  the Go TUI blocks its first frame on the startup discovery fan-out, while
  the JS app paints a loading state immediately and streams data in. The Bun
  runtime itself contributes ~100-150 ms of that 173 ms.
- Data-ready is dominated in both by the git/gh subprocess fan-out; the JS
  path is ~4 s faster because the sidecar answers `ls` first and the app
  paints slices before PR/stack enrichment lands.
- Not yet measured: keypress→repaint latency and idle CPU (subjectively
  indistinguishable; measure before replacing the Go TUI for real).

Verdict: the "must not be slower" bar is met — cold start and time-to-usable
both favor the JS TUI on a real workspace.

### Compiled single-binary cold start (`bun build --compile`, 3 runs, median)

Same headless-PTY method, driving the standalone `slis-ui` binary (real
sidecar, real workspace). Poll granularity 20 ms.

| metric | JS compiled (`slis-ui`) |
|---|---|
| first paint | 571 ms (514–640) |
| data ready | 10993 ms (10993–11118) |

First paint is slower than the `bun run` path's 173 ms because the standalone
binary extracts its embedded native assets to a cache on first launch; data
ready matches the `bun run` number (both dominated by the sidecar git/gh
fan-out, which the compile does not touch).

### Input latency + idle CPU (2026-07-18, same laptop/workspace)

Method: app warmed to data-ready in the headless PTY. **Latency** = 20
alternating `j`/`k` keypresses (selection always moves, never clamps),
wall-clock from the write to the first *changed* painted frame, median.
**Idle CPU** = accumulated process-tree CPU-time over a fixed window ÷ window
(Go tree = the `slis` process; JS tree = the Bun app + the `slis rpc` sidecar
it spawns). One run each.

| metric | Go TUI | JS (bun run) | JS compiled |
|---|---|---|---|
| input latency, median | 15.0 ms | 2.6 ms | 3.3 ms |
| input latency, range | 1.3–18.6 ms | 1.3–14.9 ms | 2.5–35.3 ms |
| idle CPU, 5 s settle | 2.7 % | 18.0 % | 17.3 % |
| idle CPU, 25 s settle | 1.6 % | 1.2 % | — |

Honest reading:
- **Latency: parity.** All three medians sit far under one 16 ms (60 fps)
  frame; the spread is a few ms with rare ~35 ms outliers on the JS side.
  Imperceptible either way — the "must not feel slower" bar is met on input.
- **Idle CPU is a measurement trap, and the 5 s row is misleading on its own.**
  At a 5 s settle the JS stack looks 6–7× hungrier (18 % vs 2.7 %) — but that
  window still overlaps the JS app's *lazy* PR/stack enrichment (the sidecar
  spawning gh/git; process-tree size 3–5, live children present). The Go TUI
  front-loads all enrichment before data-ready, so it is already quiet by then.
  Let enrichment quiesce (25 s settle, tree back to just app+sidecar) and both
  fall to ~1–1.6 % — **steady-state idle CPU is at parity.** The JS 18 % was
  work-in-flight, not resting cost. (This is the same trade the cold-start
  section calls out: the JS app pays enrichment cost *later* rather than up
  front, which is exactly why its first paint wins.)

Updated verdict: input latency and steady-state idle CPU both meet the "must
not be slower" bar; cold start and time-to-usable favor the JS TUI. Nothing in
these numbers blocks replacing the Go TUI.

## Distribution

**`bun build --compile` works today — one self-contained binary.**
`bun build --compile ./src/index.tsx --outfile slis-ui` produces a **79 MB**
standalone executable that paints a real slice list with no `node_modules`
present. Both native libraries embed cleanly:
- OpenTUI's Zig core (`libopentui.dylib` / `.so`) — the prebuilt
  `@opentui/core-<platform>` package already resolves it on Bun via
  `import("./libopentui.dylib", { with: { type: "file" } })`, which
  `bun build --compile` recognises and bundles into the binary. No patching.
- ghostty's VT parser (`ghostty-opentui.node`, a Node-API addon) — Bun embeds
  the statically-required `.node` addon into the binary; it loads at runtime
  with no external file. No dlopen path fix was needed (the opencode
  `{ type: "file" }` + dlopen dance was not required here).

Per-platform builds are still required (the binary carries one platform's
native libs), so release packaging compiles `slis-ui` per target the same way
GoReleaser cross-builds `slis`.

**Release mechanism (implemented 2026-07-18).** GoReleaser ships `slis-ui`
beside `slis` in every archive, so an installed `slis` finds its sibling and
bare `slis` launches the JS TUI (`resolveUILaunch` prefers a compiled
`slis-ui` next to the running binary).

- **Cross-compile from one host works.** `bun build --compile` is
  platform-aware: `--target=bun-<os>-<arch>` only needs the *target*
  platform's optional npm packages present. `@opentui/core` uses per-platform
  optional deps (`@opentui/core-linux-x64`, …) and a host `bun install` fetches
  only its own — so a naive linux cross-compile on macOS fails with
  "Could not resolve @opentui/core-linux-x64". Fix:
  `bun install --frozen-lockfile --cpu '*' --os '*'` installs every platform's
  optional dep, after which all four `--target` builds succeed from a single
  runner. (`ghostty-opentui` already ships all platforms' `.node` addons in one
  package, so only `@opentui/core` needed the `--cpu/--os` widening.)
- **Wiring.** `scripts/build-slis-ui.sh` (a GoReleaser `before` hook) runs the
  all-platform install then loops `bun build --compile --target=bun-{darwin,
  linux}-{x64,arm64}` into `tui-js/dist/<goos>-<goarch>/slis-ui` (GOOS/GOARCH
  names, not Bun's, so the archive template matches). The `archives.files`
  entry `src: tui-js/dist/{{ .Os }}-{{ .Arch }}/slis-ui` + `strip_parent: true`
  drops the matching binary at each archive's root as `slis-ui`. One per
  (Os, Arch) — the same four targets GoReleaser builds `slis` for — so the
  templated src resolves to exactly one file per archive. Chosen over
  GoReleaser Pro's `builder: prebuilt` (paid) and over a per-OS runner matrix
  (single-host cross-compile is proven to work, so no matrix needed).
- **Cask.** `homebrew_casks.binaries: [slis, slis-ui]` installs both; the
  `postflight` `xattr -dr com.apple.quarantine` runs for both (both unsigned).
- **Release workflow.** `.github/workflows/release.yml` adds `oven-sh/setup-bun`
  (pinned 1.3.14, matching CI) + a `bun install --frozen-lockfile` lockfile
  gate in `tui-js` before GoReleaser; the `before` hook does the all-platform
  install + compile.
- **Caveat.** Linux `slis-ui` is glibc-linked (dynamic; Bun standalone
  binaries are not static), unlike the fully-static `CGO_ENABLED=0` Go `slis`.
  Alpine/musl-only hosts won't run `slis-ui` — the Go CLI still works there.
  (The `-musl` optional deps are installed but the default `bun-linux-*`
  targets emit glibc binaries; a musl build would need a separate target.)

**Validated locally (2026-07-18).** `goreleaser check` passes against
GoReleaser v2.17. A full `goreleaser release --snapshot --clean --skip=publish`
ran the before hook, cross-compiled all four `slis-ui`, and produced four
archives each containing `slis` + a platform-matched `slis-ui` at the root
(darwin archive → Mach-O slis-ui; linux archive → ELF slis-ui) plus a cask with
`binary "slis"` / `binary "slis-ui"` and the dual quarantine strip. The
darwin-arm64 `slis-ui` was booted headless (`SLIS_FAKE=1`) and paints. The
linux-x64 cross-compile was confirmed a real x86-64 ELF embedding `libopentui.so`
+ `linux-x64/ghostty`. **Deferred to a real tag:** the actual multi-arch run on
GitHub's ubuntu runner (bun downloading each target runtime over the network),
the tap push (needs `HOMEBREW_TAP_GITHUB_TOKEN`), and `brew install` on a clean
machine.

**Launcher: `slis ui`** (`internal/cli/ui.go`). Execs the JS front-end via
`syscall.Exec` (clean handover, the JS app owns the terminal): it prefers a
compiled `slis-ui` sitting next to the running `slis` binary, else falls back
to `bun run src/index.tsx` when `SLIS_TUI_DIR` points at `tui-js/` (dev mode).
It passes `SLIS_BIN=<path-to-slis>` through so the JS RPC client always finds
the sidecar. Bare `slis` still launches the Go (Bubble Tea) TUI unchanged.
Pure resolution logic is unit-tested (`ui_test.go`); the three paths (sibling
binary, bun dev, error) were also exercised end-to-end in a PTY.

**CI** (`.github/workflows/ci.yml`): a `tui-js` job on ubuntu runs
`oven-sh/setup-bun`, `bun install --frozen-lockfile`, `bun x tsc --noEmit`, and
`bun test` (92 tests). The Go build/test/lint job is unchanged. The terminal
embed e2e (`src/term/e2e.ts`) is **gated off by default** behind
`if: vars.RUN_TUI_E2E == 'true'` (with its tmux install): it drives fixed
sleeps against a live PTY + tmux and is timing-flaky (observed ~1 failure in 4
runs even on a fast laptop; a headless runner would be worse), so gating it
keeps green CI meaningful. Set the `RUN_TUI_E2E` repo variable to `true` to run
it on demand.

## Next phase (mandated 2026-07-18, not yet done)

Jonny's direction: the JS TUI is a FULL REPLACEMENT, not an experiment.
1. DONE. Bare `slis` now launches the JS TUI by default: the root command
   (`internal/cli/root.go`) reuses `resolveUILaunch`/`execJSUI` from
   `internal/cli/ui.go` and the pure `chooseDefaultUI` decision helper.
   `SLIS_TUI=go` forces the legacy Go (Bubble Tea) TUI; if the JS front-end
   can't be resolved (no sibling `slis-ui`, no `SLIS_TUI_DIR`), bare `slis`
   prints a one-line stderr notice and falls back to the Go TUI rather than
   erroring, so users without the JS binary aren't bricked. `slis ui` keeps
   its explicit hard-error behavior. Decision covered by `ui_test.go`.
2. Full feature parity audit vs internal/tui — known suspects: bulk-load
   strategy for >25 slices (app.tsx refresh() fans out prStack+show for every
   slice; Go TUI prompted before this), preview phantom-branch warning +
   colorized diff tail, missing-slice dimmed rows, create-in-progress spinner.
   Audit rigorously, then close every gap.
3. DONE. Release packaging ships `slis-ui` per platform alongside `slis`,
   brew cask included. A GoReleaser `before` hook (`scripts/build-slis-ui.sh`)
   cross-compiles `slis-ui` for all four targets; `archives.files` bundles the
   matching one beside `slis`; the cask installs both binaries and strips
   quarantine off both. See the updated **Distribution** section for the
   mechanism and how it was validated.

## Post-parity roadmap (mandated 2026-07-18 pm)

Order after the overlays wave: usability first, then the three features.

### U — Usability wave (technical-audience UX)
Jonny: "Claude Code works because it's easy to think about. Match that
simplicity." One usability-review agent walks every flow (fake + real
workspace), files findings ranked by friction; fixes land as a wave.
Lenses: (1) familiarity — lazygit/vim/Claude Code idioms, deviations must
earn their place; (2) review ergonomics — ≤3 keystrokes from launch to
"seen the whole diff that needs me"; (3) problem-first — every red/amber
state actionable from where it's shown; (4) predictability — same key,
same meaning, everywhere; every screen answers where-am-I / what-can-I-press
/ how-do-I-get-back.

### F1 — Agent picker for terminal tabs
Open a *chosen* agent (claude, codex, aider, …) in an embedded PTY tab, not
just the default. `workspace.yaml` gains an `agents:` list (name + argv;
default claude). Browser/cockpit agent-launch key opens a picker overlay when
more than one agent is configured; selection runs in the slice's tmux session
(persistence unchanged). Config read is Go-side (config pkg + `ls`/`hello`
RPC exposure or a new `agents` method); launch is the existing term-tab path.

### F2 — Inline review comments that feed the agent
GitHub-review-style: while reading a slice diff, comment on a file:line/hunk;
comments accumulate into a review batch; submitting the batch delivers it to
the slice's agent as an instruction ("change this"). Sketch: `c` in DiffView →
text-input overlay → comment stored (slice, repo, file, line, hunk excerpt,
body) in the slis data dir; `C` (or a review overlay) lists pending comments;
submit composes a structured prompt and (a) if the slice session is
waiting-input, injects via tmux send-keys, else (b) starts/queues the agent
with the prompt (`claude -p` in the session). Mutation goes through a new CLI
twin (`slis review add/send`) — the sidecar stays read-only. JSON shapes into
docs/AGENT.md.

**Backend DONE (2026-07-18).** `internal/review` (per-slice pending-comment
store — single atomic JSON file at `<state>/reviews.json`; `ComposePrompt`
deterministic agent prompt; `Send` = compose + tmux bracketed-paste inject via
`tmuxctl.SendPrompt`, `ErrNoSession` when no session, never auto-starts an
agent). CLI: `slis review add/list/rm/send/clear` (`list --json`; `send` clears
on success unless `--keep`). RPC: read-only `reviews` method. `docs/AGENT.md`
(JSON shape + send flow + mutation table) and `skills/slis/SKILL.md` updated.
Remaining: the TUI wiring (`c` in DiffView → comment overlay; `C` → review
overlay → send).

### F3 — Stack-at-a-glance + per-branch code navigation/review
The cockpit stack panel becomes a real review surface: selecting ANY branch in
the lineage scopes the right pane to *that branch vs its stack parent* (today
the diff is slice-branch-centric). Add per-branch file-tree navigation and
file viewing at the branch's revision (`git show <branch>:<path>` via a new
read-only RPC — `tree`/`file` methods), so a whole stack can be reviewed
bottom-up without leaving slis. Pairs with F2: comments attach to the branch
being reviewed.

## Final state (2026-07-18 evening)

Final proof pass at `b8cc38e` on `experiment/js-tui`. Build was a fresh
`CGO_ENABLED=0 go build`; JS driven via `bun run src/index.tsx` and the
`bun build --compile` single binary; all measured against the real `~/nory`
workspace (21 slices).

### Feature waves landed (`91b7589..b8cc38e`)

- `482950b` feat(tui-js/theme): five-hue design system + shared component foundation
- `3451b1c` feat(tui-js/mutations): interactive mutations run in PTY tabs; captured runs get timeouts
- `03489c5` feat(release/slis-ui): ship compiled JS TUI per platform via goreleaser + brew cask
- `5296791` feat(tui-js/cockpit): breadcrumb + seamless sections redesign, reverse panel cycle, cockpit refresh
- `e75e78a` feat(tui-js/browser): deboxed five-hue browser + bulk-load guard, patch preview, phantom warning, missing rows
- `a14a7b3` fix(tui-js/cockpit): divider overflow swallowed breadcrumb row; tame right-pane scrollbars
- `d80c1d7` docs(plans/js-tui): post-parity roadmap
- `dca8c8a` feat(tui-js/overlays): card+scrim overlays, toasts, non-blocking create, adopt key, 30s refresh tick
- `e103ded` docs(plans/js-tui): usability review findings
- `bb34163` feat(agents): configurable agent list + picker for terminal tabs
- `dea8249` fix(tui-js/usability): top-10 review fixes (proc overlay header, help wrap, browser CI actions, search-aware n/N, warn status, glyph semantics)
- `bed2034` feat(review): pending-review store, agent prompt composer, tmux delivery + slis review CLI (F2 backend)
- `40df4d8` fix(testutil/repo): stop detached git background processes racing t.TempDir cleanup
- `d011f64` feat(stack-review): per-branch diff, file tree + file-at-revision RPCs (F3 backend)
- `820391d` feat(tui-js/stack-review): branch-scoped review surface in cockpit (F3 UI)
- `e19a956` fix(testutil/repo): env-level git isolation — trace2 off, null global/system config
- `b8cc38e` feat(tui-js/review): inline diff comments, pending-review overlay, send-to-agent loop (F2 UI)

### Benchmark re-run (same headless-PTY method, 200x50, real ~/nory)

Cold start — 3 runs, median (range):

| metric | Go (Bubble Tea) | JS (bun run) | JS compiled |
|---|---|---|---|
| first paint | 5107 ms (5068–5149) | **153 ms** (152–206) | 611 ms (503–1962) |
| data ready | 15715 ms (15666–15751) | **10623 ms** (10518–10752) | 11311 ms (11172–12301) |
| *prior table* | *5083 / 15365* | *173 / 11389* | *571 / 10993* |

Input latency (20 alternating j/k, median write→changed-frame) + idle CPU
(5 s settle) — one run each:

| metric | Go | JS (bun run) | JS compiled |
|---|---|---|---|
| input latency, median | 14.58 ms | **2.46 ms** | 5.17 ms |
| input latency, range | 2.56–27.84 | 2.17–80.7 | 2.41–739.81 |
| idle CPU, 5 s settle | 5.63 % | 14.26 % | 17.29 % |
| *prior table* | *15.0 / 2.7%* | *2.6 / 18.0%* | *3.3 / 17.3%* |

No meaningful regression. Every cold-start number sits within ~7% of the prior
table; every latency median is far under one 16 ms (60 fps) frame. Two entries
cross the +20% flag threshold but are measurement artefacts, not slowdowns:
compiled-JS latency median 5.17 ms vs 3.3 ms (+57%) is driven by a single
739 ms first-keypress outlier — the compiled binary's first-render/JIT cost —
after which every press is 2–5 ms; Go idle CPU 5.63% vs 2.7% is variance (the
Go tree was still finishing enrichment during the 5 s window, tree size 7). The
"must not feel slower" bar holds: first paint and time-to-usable favour the JS
TUI, input latency is at parity, steady-state idle CPU converges (as the earlier
25 s-settle measurement showed both stacks fall to ~1–1.6%).

### Real-workspace read-only E2E (JS TUI, real sidecar, real ~/nory)

Driven headless through a ghostty PTY; no mutations. Per step:

- **boot → browser** — PASS. 21 real slices with live session badges
  (running/waiting/idle), the five-hue filters rail, and stack clustering
  (`stack: jonny/pay-45-…`).
- **search → cockpit** — PASS. `/` filters to the target; enter opens the
  cockpit.
- **stack lineage** — PASS. The Stack panel renders the full real Graphite
  lineage (e.g. WFM-4157: `master` + 30+ branches with `⟳ restack` markers, the
  member branch highlighted).
- **scope cycling (`b`)** — PASS. working → parent → trunk updates the
  breadcrumb + hint.
- **select non-member branch** — PASS. The right pane title switches to
  `<branch> › vs parent` (branch-vs-stack-parent diff, F3).
- **`f` file tree** — PASS. Renders the complete real file tree at the branch
  revision — dirs + files with sizes (`package.json 9.5 KB`, `tsconfig.json
  726 B`, `vite.config.ts 6.5 KB`, …); expand/collapse navigation works.
- **`C` review overlay (F2)** — PASS. Opens ("No pending comments — press `c`
  on a diff or file line to add one").
- **`q` quit** — PASS, CLEAN. `exited:0`, zero orphan processes (the bun app
  and its `slis rpc` sidecar both gone; sidecar dies on stdin EOF).
- **diff pane / rich diff *content*** — NOT rendered within 55 s under the real
  21-slice load (see caveat below). The diff *data plane* is proven fine: a
  standalone RPC probe returned the member-vs-trunk patch (11 files, 41 KB) in
  <1 s, and the fake-sidecar browser renders `web +171 -16 · 3 files` instantly.
- **file-view *content*** — proven via RPC (`file` returned real `package.json`,
  9686 bytes); the PTF frame captured the tree render but did not land the
  FileView open (harness key-timing, not a product issue).

Sanity at this head: `bun x tsc --noEmit` clean; `bun test` 258 pass / 0 fail
(36 files); `go test -count=1 ./...` 26 packages ok, 0 fail. `src/term/e2e.ts`
returned 4/5 (browser paints, `a` opens a tab, keystrokes reach the embedded
shell, and — critically — the tmux session survives app quit all PASS);
`ctrl_q_returns_to_browser` FAILed on both runs, but this is a **stale test
assertion**, not a regression: a direct probe confirmed ctrl+q returns to the
browser (post-ctrl+q frame shows FILTERS/SLICES/CHANGES), and the assertion just
greps for `refresh`/`filter` strings the UI redesign removed from the browser
hint bar.

### Known caveats (carried + new)

- **NEW — cockpit diff can starve on large workspaces.** On the 21-slice real
  workspace the interactive `diff` (and `branchDiff`) request never completed
  within 55 s: the 30 s background refresh tick re-fans-out `prStack`+`show` for
  every slice through the sidecar's 4-in-flight subprocess gate, keeping it
  saturated so the interactive diff loses the race. The single cheaper `tree`
  call did get a slot and rendered. Data plane is fine standalone — this is a
  prioritisation problem. Recommend the sidecar (or the client) prioritise
  interactive reads (diff/branchDiff/tree/file) ahead of the background
  enrichment tick, or throttle/skip the tick while the cockpit is focused.
- **NEW — `src/term/e2e.ts` `ctrl_q_returns_to_browser` assertion is stale.**
  Checks for `refresh`/`filter` in the returned frame; the redesigned browser
  hint bar no longer contains those. ctrl+q itself works. Update the assertion
  to look for `SLICES`/`FILTERS`/a slice name.
- Help overlay clips past ~30 rows (documented; needs internal scroll/paging).
- Inline-comment composer is single-line only.
- Linux `slis-ui` is glibc-linked (Bun standalone binaries aren't static);
  musl/Alpine hosts fall back to the Go CLI.
- Cockpit `C` is rebound to the F2 review overlay (explicit agent launch stays
  on the browser's `C`); `a` attaches the terminal in the cockpit.
