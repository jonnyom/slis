# slis TUI Parity Audit — Go (`internal/tui`) vs JS (`tui-js/src`)

Branch `experiment/js-tui`. Goal: the JS OpenTUI/React (Bun) TUI must fully replace the Go Bubble Tea TUI. This is the exhaustive gap list. Source is verified in code, not filenames. Line refs are `file:line`.

Architecture note (shapes several findings): the JS TUI talks to a long-lived read-only Go sidecar (`slis rpc`) over JSON-RPC/NDJSON for all *reads*, and runs every *mutation* as a separate one-shot `slis <cmd>` spawn captured into a modal (`tui-js/src/rpc/mutate.ts`). The Go TUI runs reads in-process and mutations either in-process (`restack`, `cleanup`) or via `tea.ExecProcess` (suspends the TUI, hands over the TTY). This TTY difference is the root of the single highest-impact gap (#1).

---

## 1. CONFIRMED GAPS (ranked by user impact)

### G1 — Interactive mutations run non-interactively (captured, stdin ignored, no timeout) [HIGH]
- **Go**: `submit`, `sync`, `merge`, `adopt`, `fix-ci` run via `tea.ExecProcess` which suspends the TUI and gives the child a real TTY (app.go:695-724 `slisSubmitCmd`/`slisMergeCmd`/`slisSyncCmd`; app.go:1311-1318 `slisAdoptCmd`; prview.go:211 `fixCICmd`). These commands are *interactive*: `gt submit/sync/merge` can prompt; `slis fix-ci` launches `claude` (a full interactive session); `slis adopt` is an interactive branch picker.
- **JS**: all go through `mutate.ts run()` → `spawnCapture` with `stdin: "ignore"`, stdout/stderr captured into the Result overlay (mutate.ts:19-43, 77-100, 122-124). No PTY, no interactivity, **and no timeout on `proc.exited`** — a child that blocks on a prompt hangs the "working…" overlay forever.
- **Impact**: `fix-ci` and `adopt` are effectively broken (they need a live terminal); `submit`/`sync`/`merge` lose prompts and risk hanging.
- **Fix location**: route these through the embedded terminal layer (`tui-js/src/term/manager.ts` + `term/tabs.tsx`) as PTY tabs instead of `spawnCapture`, mirroring how attach/agent already open a tab. At minimum add a timeout + treat interactive commands specially in `overlays/useOverlays.tsx` `runMutation`.

### G2 — No `>25` bulk-load prompt; cold start fans out show+prStack for every slice [HIGH]
- **Go**: `bulkLoadThreshold = 25` (gate.go:17). On a cold load of >25 slices it raises a modal prompt (`renderBulkPrompt` cockpit.go:696; `updateBulkPromptKeys` app.go:1072) — `[y]` load all, `[n]`/esc → `lazyCards` mode that loads only the focused slice and never re-prompts. Background card/PR/proc loaders are gated to 4 concurrent (`bgGate`, gate.go:21-30). Behavior locked by `bulkprompt_test.go`.
- **JS**: `app.tsx refresh()` fans out `prStack(name)` + `show(name)` for **every** slice unconditionally (app.tsx:66-78), plus one `conflicts()`. No threshold, no prompt, no lazy mode, no client-side concurrency gate. It relies on the sidecar internally capping subprocess work at 4 — so it is throttled but still issues N `show` + N `prStack` on every full refresh, and the user never gets the "load as you go" choice.
- **Impact**: large workspaces do full-fan-out reads on every `r`/reconnect; no user control.
- **Fix location**: `tui-js/src/app.tsx refresh()` — add threshold + lazy mode + a bulk-load overlay (new entry in `overlays/useOverlays.tsx`).

### G3 — Browser preview shows diffstat only, not the colorized diff tail [MEDIUM]
- **Go**: `previewContent` renders a colorized "── recent changes ──" *patch* per repo (`colorizePatch(rd.Patch)` slicelist.go:886-914).
- **JS**: browser preview loads `diff({scope:"working", format:"stat"})` and renders only `+added / -deleted / N files` (browser.tsx:284, 353-357). No patch body, no coloring.
- **Impact**: the browser preview is far less informative than Go's.
- **Fix location**: `tui-js/src/views/browser.tsx` preview block — fetch `format:"patch"` and render via the existing `diff/render.ts` colorizer.

### G4 — Browser preview is not scrollable [MEDIUM]
- **Go**: preview has its own scroll (`previewScroll`, clamped) driven by `ctrl+d`/`pgdown` / `ctrl+u`/`pgup` in the browser (slicelist.go:1100-1105, 641), title shows scroll offset when it overflows.
- **JS**: browser handles no scroll keys at all (browser.tsx:424-540 has no ctrl+d/ctrl+u/pageup/pagedown); preview is a fixed-height render.
- **Impact**: long previews are truncated with no way to see the rest.
- **Fix location**: `tui-js/src/views/browser.tsx` — add a scrollbox + scroll keys (pairs naturally with G3).

### G5 — Phantom / doubled-prefix branch warning absent [MEDIUM]
- **Go**: preview surfaces `⚠ doubled-prefix branch (phantom) … run slis doctor --fix` (slicelist.go:843-854).
- **JS**: no phantom/doubled-prefix detection anywhere in `browser.tsx` (grep confirms none). Only generic "N hidden / N missing" pulse-bar aggregates exist.
- **Impact**: a real data-hygiene warning the Go TUI gives is silently dropped.
- **Fix location**: `tui-js/src/views/browser.tsx` preview + `state/derive.ts` (derive the flag from show data / branch names).

### G6 — Missing slices not rendered as dimmed rows in the list [LOW-MEDIUM]
- **Go**: missing slices render as dimmed, non-selectable rows inside the slices list (slicelist.go:769-771, 589-591).
- **JS**: `ls.missing` is used only for the pulse-bar count `⚠ N missing` (browser.tsx:142, 187-189); the list navigates only `filtered` real slices — missing slices never appear as rows.
- **Impact**: you see a count but not *which* slices are missing, and can't act on them from the list.
- **Fix location**: `tui-js/src/views/browser.tsx` list render + `state/cluster.ts` visible-set assembly.

### G7 — No periodic background refresh of PR / CI / stack data [MEDIUM]
- **Go**: a 30s ticker force-refreshes PRs/diff/stack/procs (`prsTick` app.go:862-867) plus a 2s capture ticker; `r` also drops the PR cache so merge-state re-fetches (app.go:1161-1175).
- **JS**: `app.tsx` has no `setInterval`/periodic refresh (grep confirms none). Only pane-level polling exists (session capture 2s, procs 2.5s) plus refresh on manual `r`, reconnect, and incremental session events.
- **Impact**: CI status / PR merge-state / restack-health can go stale until the user manually refreshes.
- **Fix location**: `tui-js/src/app.tsx` — add a periodic `refresh()` (or targeted prStack refetch) ticker.

### G8 — Mouse wheel scrolling unsupported [LOW-MEDIUM]
- **Go**: launches with `tea.WithMouseCellMotion()` and handles wheel up/down to scroll (app.go:1013-1018, 1588-1591).
- **JS**: no mouse handling anywhere (grep for mouse/onMouse is empty).
- **Impact**: no mouse scroll; keyboard-only.
- **Fix location**: `tui-js/src/app.tsx` / relevant scrollboxes (OpenTUI mouse events).

### G9 — Reverse panel cycle (`shift+tab` / `H`) absent in cockpit [LOW]
- **Go**: cockpit cycles panels both ways — `tab`/`L` forward, `shift+tab`/`H` back (cockpit.go:804-813).
- **JS**: only `tab` (forward-only wrap) at cockpit.tsx:786-787; no `shift+tab`/`H`.
- **Impact**: minor; can still reach any panel via `1`-`4` or forward wrap.
- **Fix location**: `tui-js/src/views/cockpit.tsx` keyboard handler.

### G10 — No `r` refresh inside cockpit [LOW]
- **Go**: `r` is global; in cockpit it refreshes (Session pane: reload capture only; else full refresh) (app.go:1161-1175).
- **JS**: `r` is bound only in the browser (browser.tsx:444); cockpit has no `r`. Mitigated by pane polling, but there is no manual "refresh now" in the cockpit.
- **Fix location**: `tui-js/src/views/cockpit.tsx`.

---

## 2. PARTIAL / DIVERGENT (behavior exists but differs)

- **D1 — Summary presentation**: Go `s` toggles a commit-summary in the cockpit *right pane* and `S` forces AI (cockpit.go:854-870, `summaryContent`). JS `s`/`S` open a centered *modal overlay* (`useOverlays.tsx:361`, `summarySlice`). Same data, different surface; overlay obscures the panels.
- **D2 — Create is a blocking modal in JS vs non-blocking in Go**: Go runs `slis create` in the *background* with a braille spinner in the pulse bar — you keep navigating (app.go:1273-1281, slicelist.go:583-585). JS runs it through `runMutation` → a blocking "working…" overlay → Result overlay (useOverlays.tsx:130-133). (This is the suspected "create-in-progress spinner" item: JS *has* a spinner, but it blocks the UI rather than being the ambient pulse-bar spinner.)
- **D3 — Adopt scope**: Go browser `I` runs an interactive adopt of an arbitrary branch (app.go:1133 `slisAdoptCmd`). JS has no `I`; adopt is only reachable inside the candidates overlay via `a` on a detected candidate (useOverlays.tsx:327-341) — you cannot adopt an arbitrary non-candidate branch. (Also blocked by G1's interactivity issue.)
- **D4 — Attach / agent launch surface**: Go suspends the TUI via `ExecProcess` to attach tmux / launch the agent (app.go:1185-1252). JS opens an embedded PTY *terminal tab* (`term/tabs.tsx`, back key `ctrl+q`). Functionally richer in JS, but the interaction model and detach guidance differ; verify feature parity of the tmux session semantics.
- **D5 — Concurrency gating**: Go gates background card/PR/proc loaders to 4 client-side (`bgGate` gate.go). JS has no client-side gate and depends on the sidecar capping in-flight subprocess work at 4 (app.tsx:57 comment). Behaviorally similar *if* the sidecar cap holds; no JS-side guarantee.
- **D6 — `s` key overload in cockpit**: Go `s` = summary everywhere in cockpit. JS `s` = proc sort when procs panel is focused, summary otherwise (cockpit.tsx:795 vs :816). Divergent meaning; intentional but worth noting.
- **D7 — `space` in cockpit**: Go `space` = half-page scroll of the right pane (app.go:842). JS cockpit has no `space` (uses ctrl+d/pagedown). Trivial.

---

## 3. VERIFIED PARITY (present and behaviorally equivalent)

- Two-level model: browser hub ↔ cockpit; `enter`/`l`/`right` down, `esc`/`h` up.
- State-filter rail with 8 filters + live counts; `1`-`8` jump; `tab` rail↔list focus.
- Attention navigation `n`/`N` (wrapping) in browser.
- `g`/`G` first/last; `j`/`k` movement.
- Select `space` (one) / `A` (all-visible); group `m` / ungroup `u`.
- Swap confirm overlay: `[y]` / `[s]` stash (shown only when primary dirty) / `[n]`.
- Remove confirm overlay: `[y]` / `[f]` force / `[n]`, live-slice block with toast.
- Stack-actions overlay: `[r]` restack / `[p]` submit / `[m]` merge / `[s]` sync, with conflict-partner warning.
- CI-rerun overlay (`ctrl+r` on PRs panel) → `slis ci-rerun`.
- Fix-CI key `F` on PRs panel; open PR in browser `O`.
- Candidates overlay: import `i`, ignore `x` (adopt `a` — see D3); editor picker overlay with persisted choice.
- Conflict radar `!` (scrollable, honest "committed only" framing).
- Help overlay `?` with per-view bindings + glyph legend + terminal/detach note.
- Yank combined patch `y`; yank PR-stack markdown `Y` (clipboard tool probing pbcopy/wl-copy/xclip/xsel matches Go).
- Cockpit 4 panels (Stack lineage / PRs / Session / Processes) with per-panel right pane; `1`-`4` jump; zoom via `enter` on non-stack panels.
- Diff scope cycle `b` (working→parent→trunk) and split/unified `t`.
- CI-log toggle `v` in right pane; PR detail with CI rollup + review + comments (cached fallback).
- Kill process `x` / kill-subtree `X` with `[y]/[n]` confirm, both in cockpit procs pane and the all-slices proc overlay `P`.
- Session status glyphs (`⏸`/`●`/`✓`/`○`) and work-state row glyphs; LIVE/stale cockpit header.
- Editor: open whole slice `e` / selected repo `o`; `editor set` persistence.
- Agent context env (SLIS_* prefix) on launch.
- Full mutation CLI-twin coverage: every mutation in `rpc/mutate.ts` (activate/deactivate/create/rm/restack/submit/merge/sync/ci-rerun/fix-ci/group/ungroup/import/ignore/adopt/editor-set/edit/summary/pr-stack) is reachable from a keybinding; none orphaned; no UI action lacks a backing command.

---

## 4. JS-ONLY EXTRAS (present in JS, not in Go)

- **Embedded PTY terminal tabs** (`term/tabs.tsx`, `term/manager.ts`) — attach/agent open in-app tabs with `ctrl+q` back, instead of suspending the whole TUI.
- **Rich DiffView** (`components/diffview.tsx`) — hunk navigation `[`/`]`/`n`/`p`, word-level intra-line change highlighting, syntax-token coloring, side-by-side + line numbers + horizontal scroll. Go's `colorizePatch` is fg-only.
- **Process view niceties** — subtree fold `h`/`l`, sort cycle `s`, per-process CPU sparklines (`proc/sparkline.ts`).
- **RPC resilience UX** — disconnect banner + auto-reconnect with exponential backoff (`rpc/client.ts:180`), and a `SLIS_FAKE=1` fixture client for headless testing.

---

## Summary counts
- CONFIRMED GAPS: 10
- PARTIAL/DIVERGENT: 7
- VERIFIED PARITY: ~30 feature areas
- JS-ONLY EXTRAS: 4
