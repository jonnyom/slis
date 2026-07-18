# slis — OpenTUI front-end (spike)

An experimental TUI for [`slis`](../README.md) built with
[OpenTUI](https://github.com/anomalyco/opentui) (React reconciler) on
[Bun](https://bun.sh). It talks to the Go core through a long-lived,
read-only `slis rpc` sidecar (JSON-RPC 2.0 over NDJSON on stdio); mutations
(swap in / out) run as one-shot `slis` spawns.

See `../docs/plans/2026-07-18-js-tui-spike.md` for the architecture + RPC
contract this implements, and `../docs/AGENT.md` for the JSON shapes.

## Run

Requires **Bun 1.3+** and the `slis` binary on `PATH` (or point `SLIS_BIN` at
it — e.g. the freshly built `../slis`).

```sh
bun install

# Against the real sidecar (`slis rpc`):
bun run start
SLIS_BIN=../slis bun run start        # explicit binary

# Against in-process fixtures (no sidecar, no repos needed):
bun run start:fake

# Type-check:
bun run typecheck
```

> First paint against a real workspace waits for `ls`, which is a cold
> multi-repo scan (≈10s for a 21-slice workspace here) — the "loading
> workspace…" splash is expected until it returns. PR / stack / diff / capture
> data then fills in lazily.

## Environment

| Var | Meaning |
|---|---|
| `SLIS_BIN` | Path to the `slis` binary. Default `slis` (from `PATH`). |
| `SLIS_FAKE=1` | Use in-process fixtures instead of spawning the sidecar. |

## Keybindings

**Browser**

| Key | Action |
|---|---|
| `tab` | toggle States rail / Slices list focus |
| `j` / `k` | move down / up |
| `1`–`8` | jump to filter (All, Needs you, In review, Ready, In progress, Needs restack, Live, Inbox) |
| `g` / `G` | first / last slice |
| `enter` / `l` | open the slice cockpit |
| `w` | swap the slice in / out (live) |
| `r` | refresh workspace |
| `?` | help overlay |
| `q` | quit |

**Cockpit**

| Key | Action |
|---|---|
| `tab` | next panel |
| `1`–`4` | Repos&Stack / PRs / Session / Processes |
| `j` / `k` | move selection within the focused panel |
| `b` | cycle diff scope: working → parent → trunk |
| `t` | toggle diff stat / patch |
| `ctrl+d` / `ctrl+u` | scroll the right pane |
| `g` / `G` | top / bottom of the right pane |
| `w` | swap the slice in / out (live) |
| `esc` / `h` | back to the browser |
| `q` | quit |

**Overlays** — help closes with `?`/`esc`; swap confirms with `y`/`enter`,
cancels with `n`/`esc`; the result card closes with `enter`/`esc`.

## Layout

```
src/
  index.tsx            entry: createCliRenderer + createRoot(App)
  app.tsx              client lifecycle, workspace data, routing, overlays
  theme.ts             glyphs + ANSI-256→hex colors mirrored from the Go TUI
  rpc/
    types.ts           TS mirrors of the slis JSON shapes + the RpcClient contract
    client.ts          real client: spawns `slis rpc`, NDJSON JSON-RPC, reconnect
    fake.ts            fixture client (SLIS_FAKE=1), incl. simulated sessionEvents
    mutate.ts          one-shot `slis activate`/`deactivate` spawns
    index.ts           factory: real vs fake
  diff/parse.ts        pure unified-diff parser → per-file / per-hunk structures
  state/derive.ts      slice work-state classification for the states rail
  util/ansi.ts         strip SGR escapes from tmux capture lines
  components/          panel, overlay, help, diffpane, shared UI helpers
  views/
    browser.tsx        pulse bar · states rail · slice list · preview
    cockpit.tsx        header · 4 stacked panels · focus-driven right pane
```

The diff pane (`components/diffpane.tsx`) is self-contained: it takes the raw
`stat`/`patch` for one repo and renders via the pure parser in `diff/parse.ts`.
A richer differ (side-by-side, syntax highlighting, file-tree nav) can replace
the render without touching the cockpit — it receives the same parsed
`FileDiff[]`.

## Notes

- Live session badges are applied from `sessionEvent` notifications the moment
  they arrive.
- The 2s capture poll runs only while a Session pane is visible; the process
  table polls only while the Processes panel is focused. Slice rows are
  memoized so a capture tick never re-renders the list.
- The real client restarts the sidecar with backoff on crash and shows a
  "sidecar disconnected" banner while reconnecting.

## Out of scope (this spike)

Restack / submit / merge, tmux attach handover, process kill, group / ungroup,
create / import, and AI summaries — the swap-confirm and help overlays are the
only ones wired.
