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
   deactivate, rm, restack, submit, merge, sync, create, import, group…).
   The sidecar must never mutate — keeps the data-safety-critical swap
   engine behind its existing, tested CLI entry points.
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
| `prStack` | `{ "slice": string }` | same as `slis pr-stack <slice> --json` |
| `comments` | `{ "slice": string }` | same as `slis comments <slice> --json` |
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
