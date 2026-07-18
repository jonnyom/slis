# slis JS TUI — UI Redesign Spec

**Target:** `tui-js/` (OpenTUI React on Bun). Skin + layout + affordance redesign. Every existing feature stays reachable; nothing about the RPC/data layer changes.

**One-line thesis:** slis is mission-control for parallel feature work. The whole UI answers one question first — *what needs me right now?* — and everything else recedes until you ask for it.

---

## 1. Design principles (5)

1. **The left edge is always the status.** One consistent scannable column carries state across every surface — the semantic glyph in the browser list, the focus/attention bar on cockpit panels, the result bar on overlays. You learn to read one column and you know the whole board.
2. **Debox. Separate with space and hairlines, not borders.** Borders are expensive attention. Reserve them for the two things that genuinely are "a thing on top": the cockpit's content stage (right pane) and floating overlays. Everything else is grouped by an eyebrow label + whitespace + at most a 1px hairline.
3. **Four hues carry all meaning.** green = good/clean/live, amber = attention/dirty/waiting, red = failure, violet = merged/terminal — plus one blue for *focus and identity*. If a color doesn't map to one of those five, it doesn't ship. (Today's palette has ~12 competing accents.)
4. **Show the keys you can press now, not all of them.** A contextual hint bar shows the 4–6 relevant actions for the current focus; `?` opens the full, categorised reference. No wall of comma-separated keys.
5. **Motion only marks async.** A braille spinner while work runs, a short toast fade on completion. No decorative animation, no per-frame reflow.

---

## 2. Palette + semantic tokens (theme.ts-shaped)

Dark-terminal-first, truecolor. OpenTUI accepts hex strings directly (`RGBA.fromHex` under the hood), so tokens stay plain hex.

### Neutral ramp
| token | hex | role |
|---|---|---|
| `bg` | `#0b0d12` | app background (often left transparent to the terminal) |
| `surface` | `#14181f` | raised panel / overlay card fill |
| `surfaceAlt` | `#1c2029` | focused/selected row background tint |
| `hairline` | `#262b34` | dividers, unfocused borders |
| `border` | `#3a414c` | stronger rule when a real border is needed |
| `textFaint` | `#5b6472` | unfocused eyebrows, tertiary meta |
| `textDim` | `#8a93a3` | secondary text, labels |
| `text` | `#c3cad6` | body |
| `textBright` | `#f4f7fb` | headings, focused row name, emphasis |

### Focus / identity
| token | hex | role |
|---|---|---|
| `focus` | `#4c9dff` | focus borders + focus bar, `slis` wordmark, selection, "your turn" (session done), new/candidate |
| `focusDim` | `#2d5c8f` | focus bar when panel is unfocused-but-active, subtle underlines |

### Semantic (the only four hues)
| token | hex | means |
|---|---|---|
| `good` | `#34d399` | clean, live/swapped-in, running, ready-to-clear, added, CI pass, approved |
| `attn` | `#f5a623` | Claude waiting-input, dirty, stale/behind, needs-restack, CI pending |
| `bad` | `#ff5d5d` | CI fail, missing worktree, changes-requested, deleted, repo error |
| `merged` | `#b28bff` | merged PR, terminal "done" states |

### Diff (derived — no new hues)
| token | hex | |
|---|---|---|
| `diff.add` / `diff.addBg` | `#34d399` / `#10281c` | + line fg / changed-word wash |
| `diff.del` / `diff.delBg` | `#ff5d5d` / `#2e1214` | − line fg / changed-word wash |
| `diff.hunk` | `#4c9dff` | `@@` hunk header |
| `diff.header` / `diff.gutter` | `#8a93a3` / `#3a414c` | file header / line-number gutter |
| syntax tokens | keep current hues but **re-pin** keyword→`merged`, string→`good`, number→`attn`, type/function→`focus`, comment/punct→`textFaint`/`textDim` | so the differ shares the app's five-hue system |

### Attention model (new helper — drives the "left edge")
`attention(view) → { level, color, glyph }`, collapsing today's `workState` + session status:

| level | when | color | glyph |
|---|---|---|---|
| `needs-you` (3) | waiting-input / changes-requested / CI-fail | `attn` (waiting/dirty) or `bad` (CI/changes) | `⏸` / `✗` |
| `active` (2) | live/swapped-in, agent running | `good` | `●` |
| `info` (1) | ready-to-clear, in-review, done-for-review | `good` (ready) / `merged` (done) / `focus` (review-you) | `♻` / `✦` / `✓` |
| `idle` (0) | nothing pending | `textDim` | `·` |

### Glyphs (trimmed, each pinned to one color-by-context)
`waiting ⏸` · `done ✦` · `ready ♻` · `review ✓` · `changes ✗` · `ci-fail ✗` · `live/run ●` · `idle ·` · `restack ⟳` · `dirty ⚠` · `selected ✓` · `focusBar ▎` · `arrow ›` · `new ＋`. Drop the emoji ❌/✅/⏳ CI badges in favour of glyph `✓ ✗ ⋯` in `good`/`bad`/`attn` (keeps the five-hue discipline; emoji render at inconsistent widths).

---

## 3. Per-surface spec

Mockups use real data: slices `payroll-ssp-fix`, `checkout-refactor`, `tips-v2`, `invoice-pdf`; repos `nory/Node-Middleware`, `nory/Web-App`, `nory/nory`; branches `jonny/payroll-ssp-fix`.

### 3.1 Browser — the calm overview

Deboxed. Header stat-strip → filter rail + slice list (hairline-separated in one left column) → vertical hairline → preview → contextual hint bar. Focus (tab toggles rail↔list) shown by a **bright eyebrow + focused-row bg tint + `▎` bar**; unfocused eyebrows are `textFaint`.

```
 slis    ⏸ 3 need you    ● 1 live    ♻ 2 ready                              v0.4.2
────────────────────────────────────────────────────────────────────────────────────
 FILTERS                     │ payroll-ssp-fix
 ▸ Needs you       3         │ ──────────────────────────────────────────────────────
   Live            1         │ STATE
   Ready           2         │  ⏸ waiting for you   ⚠ nory/Web-App dirty
   In review       4         │
   Restack         1         │ REPOS
   Idle            5         │  nory/Node-Middleware   jonny/payroll-ssp-fix  #4821 open ✓
   All            12         │  nory/Web-App           jonny/payroll-ssp-fix  #4822 open ✗ changes
                             │
 SLICES  12                  │ CHANGES  vs working tree
▎⏸ payroll-ssp-fix   ⚠       │  nory/Node-Middleware   +142 −38 · 6 files
  ✗ checkout-refactor        │  nory/Web-App           +51  −4  · 3 files
  ● tips-v2  ●               │
  · rota-export              │ SESSION  live tail
  · menu-sync                │  › running accrual test suite…
  stack: tips-v2 → …         │  › 2 failing in accrual.spec.ts
  ♻ invoice-pdf              │
  ✓ discount-engine          │
  · loyalty-tiers            │
────────────────────────────────────────────────────────────────────────────────────
 enter open    a term    w swap    space select    / search    ? more
```

- **Stat-strip** replaces PulseBar: `slis` wordmark in `focus` bold, then *only the non-zero urgent counts* (need-you `attn`, live `good`, ready `good`, restack `attn`, hidden/errors `bad`) with even `  ` spacing, version right-aligned in `textFaint`. Calm when nothing's urgent (just `slis   12 slices                v0.4.2`).
- **Filter rail** loses its box. Eyebrow `FILTERS`; active filter marked `▸` + `textBright` bold + count in its own semantic color; others `textDim`. Search state shows inline: `/ payr…` under the rail.
- **Slice list**: leftmost cell is the **attention glyph** (the scannable column). Level-3 rows put the name in the semantic colour + bold so they pop; idle rows are quiet `textDim` names. Focused row: `▎` `focus` bar + `surfaceAlt` bg + `textBright` bold name. `space`-selected rows show `✓` in `good` before the glyph. Stack clusters keep the dim `stack: … → …` header.
- **Preview** (deboxed, `flexGrow`): slice name as a bold `textBright` heading + hairline, then eyebrow'd sections `STATE / REPOS / CHANGES / SESSION`. Session tail in `textDim`, prefixed `›`. Empty preview → "Pick a slice to preview it."
- Vertical `│` between list column and preview = `border={["right"]}` `hairline` on the left column.

### 3.2 Cockpit — the dense power view

Keeps the lazygit stacked-left + big-right model (familiar, good) but the four left panels become **one column of hairline-separated sections**, not four rounded boxes. The right pane is the only bordered region — it's the content stage and it echoes the focused section's identity. Breadcrumb header replaces the ad-hoc title line.

```
 slis › payroll-ssp-fix › Stack       ● LIVE · swapped in            esc back   ? help
──────────────────────────────────────────────┬──────────────────────────────────────
▎REPOS & STACK                     2 repos     │ nory/Node-Middleware › Changes · working
  nory/Node-Middleware                         │
    master  [trunk]                            │  accrual.ts                          +64 −12
    jonny/payroll-ssp-base                      │  @@ -18,7 +18,9 @@ computeAccrual(band)
▎   jonny/payroll-ssp-fix  ‹you›  ⟳ restack    │    const rate = band.rate
  nory/Web-App                                 │  + const carry = prior.carryOver
    master  [trunk]                            │  + if (carry > 0) total += carry
    jonny/payroll-ssp-fix                       │    return total
 ─────────────────────────────────────────────│  …
  PRS                              2 open       │
   Node-Middleware  #4821 open  ✓ ci  ✓        │
   Web-App          #4822 open  ✗ ci·2  ✗      │
 ─────────────────────────────────────────────│
  SESSION                          ⏸ waiting    │
   › 2 failing in accrual.spec.ts              │
 ─────────────────────────────────────────────│
  PROCESSES                        Σ 38%        │
   ● bun test   32%                            │
──────────────────────────────────────────────┴──────────────────────────────────────
 tab panel   j/k repo   enter rich diff   b scope: working   w swap   R stack   ? more
```

- **Breadcrumb** `slis › <slice> › <section>` (`focus` on `slis`, `textBright` on slice, `textDim` `›`, focused section name `textBright`). Live/stale badges to the right of it in `good`/`attn`. Right-aligned `esc back  ? help`.
- **Left column**: focused section gets a full-height `▎` `focus` bar + `textBright` bold eyebrow; unfocused sections `textFaint` eyebrow, no bar. Section count/summary right-aligned in the eyebrow row (`2 repos`, `2 open`, `⏸ waiting`, `Σ 38%`) so each panel telegraphs its headline without expanding. Hairlines between sections; no per-panel border.
- **Right pane**: single `rounded` border in `focus`, title = `<repo> › <panel-content>`. Scrollbox unchanged. Zoom (`enter` on non-stack panels) hides the left column and widens the pane; breadcrumb appends ` › zoom`.
- **Rich diff** (`DiffView`) and **CI log** keep their structure; only palette tokens swap (diff.* above).

### 3.3 Overlays — floating cards with a scrim

All overlays share one `Card`: centred, `surface` fill, single `rounded` `focus` border, `padding={1}`, header (title `focus` bold + optional `textDim` subtitle), body, then a **HintBar row** of `[key] label` chips. A **Scrim** (absolute full-screen box, `backgroundColor` `bg` at `opacity ~0.5`, lower zIndex than the card) dims the workspace so the modal pops — this is new and is the single biggest "modal feels intentional" win.

```
        ╭ Swap · payroll-ssp-fix ─────────────────────────────╮
        │  Swap IN payroll-ssp-fix?                            │
        │  Puts each primary on slis/live/payroll-ssp-fix at   │
        │  the slice tip. Reversible — worktrees untouched.    │
        │                                                      │
        │  ⚠ nory/Web-App has uncommitted work                 │
        │    stash it and it's popped back on swap-out.        │
        │                                                      │
        │  [y] confirm    [s] stash + swap    [esc] cancel     │
        ╰──────────────────────────────────────────────────────╯
             · · · workspace dimmed behind (scrim) · · ·
```

- **Result overlay**: gains a status left-bar — `good` bar + `✓` title on success, `bad` bar + `✗` on failure. Body scrolls as today.
- **Working overlay**: keep braille spinner (extract to shared `Spinner`), `focus` colour.
- **Candidates / Conflict radar / Summary**: same layout, adopt eyebrows + the shared HintBar; radar's overlap rows use `bad`/`attn` for the shared-file warning. Copy stays as-is except tightened.
- **Help**: bindings grouped under eyebrows (`NAVIGATE / ACT / STACK & PRS / SESSION`) instead of one flat list; the legend maps glyphs to the five-hue system.

### 3.4 Hint bar (shared, contextual)

Bottom line on browser & cockpit. Array of `{ key, label }` → renders `key` in `focus` + `label` in `textDim`, `   ` gaps, always ending in `? more`. Content switches by focus (browser list vs rail; cockpit panel; zoom; kill-pending). Replaces both today's long footer strings. Never wraps — truncate with `…` before overflow.

```
 enter open    a term    w swap    space select    / search    ? more      (browser/list)
 tab panel    j/k repo    enter rich diff    b scope: working    ? more     (cockpit/stack)
 y confirm    n cancel                                                       (kill pending)
```

### 3.5 Toasts (new, transient)

Bottom-right absolute stack, high zIndex, one short opacity fade in/out, auto-dismiss ~2.5s. For quick confirmations that don't deserve a modal: yank-to-clipboard, swap done, refresh done. Left glyph in semantic colour.

```
                                               ╭ ✓ Copied PR-stack markdown ╮
                                               ╰─────────────────────────────╯
```

The **sidecar-disconnected** banner becomes a persistent top toast (`bad`, `⚠ reconnecting…`) rather than a bare absolute text row.

### 3.6 Loading & empty states

```
 loading:   ⠹  loading workspace…                     (centred, spinner + textDim)
 per-item:  loading…                                  (inline textDim, no spinner churn)

 empty (filter):   SLICES  0
                   Nothing matches "Needs you" — you're all caught up.

 empty (workspace): No slices yet.
                    Press  c  to create a feature slice across your repos,
                    or     i  to import existing worktrees.
```

Empty states are directive (name the key), never a bare "(none)".

---

## 4. Component inventory changes

New shared components (all in `tui-js/src/components/`):

| component | purpose | notes |
|---|---|---|
| `Eyebrow` | dim uppercase section label; brightens + optional `▎` bar when focused | takes `label`, `focused`, `trailing` (right-aligned count/summary) |
| `HintBar` | contextual key hints | takes `hints: {key,label}[]`; truncates to width |
| `Badge` | small state token (`● live`, `⏸ waiting`, `✗ ci·2`, `#4821 open`) | glyph+text in one semantic colour |
| `StatusGlyph` | the attention glyph for a `SliceView` | wraps the new `attention()` helper |
| `Breadcrumb` | `slis › slice › section` + trailing badges | cockpit header |
| `Divider` | full-width hairline `─` (or vertical via `border` side) | replaces most `Panel` borders |
| `StatStrip` | browser header attention summary | only non-zero counts, semantic-coloured |
| `Scrim` | absolute dim backdrop behind overlays | `opacity` blend, static (see risks) |
| `Card` | overlay shell (header/subtitle/body/hintbar + status bar) | refactor of `Overlay` |
| `Toast` + `useToasts` | transient confirmations | timer-driven, fade once |
| `Spinner` | braille spinner | extracted from `WorkingOverlay` |

Changed:
- `Panel` gains a `variant: "seamless" | "bordered"`. **Seamless** (cockpit left sections, browser rail/list/preview) = `Eyebrow` + optional `▎` focus bar + hairline, no box. **Bordered** (cockpit right pane, overlay cards) keeps the rounded border. This is the debox lever.

---

## 5. Implementation notes (per file)

- **`theme.ts`** — replace `color` with the neutral ramp + `focus` + four semantics; re-derive `diffColor`/`syntaxColor` from them; add `attention(view)` and `badgeFor(state)` helpers; trim `glyph`; keep `sessionBadge`/`sessionLabel` but re-point colours. Everything downstream imports from here, so this is the keystone change.
- **`components/panel.tsx`** — add `variant`; seamless path renders `Eyebrow` + children + optional left bar instead of the bordered box.
- **New**: `eyebrow.tsx`, `hintbar.tsx`, `badge.tsx`, `statusglyph.tsx`, `breadcrumb.tsx`, `divider.tsx`, `statstrip.tsx`, `scrim.tsx`, `card.tsx`, `toast.tsx`, `spinner.tsx`.
- **`views/browser.tsx`** — swap `PulseBar`→`StatStrip`; debox `StatesRail`/`Slices`/`Preview` to seamless with eyebrows + vertical hairline; slice rows use `StatusGlyph` + focus bg tint; footer→`HintBar` with list/rail-contextual hints. Keys/handlers unchanged.
- **`views/cockpit.tsx`** — header→`Breadcrumb`; four `Panel`s→seamless sections in one column with `border={["right"]}` divider and per-section trailing summary; right pane keeps bordered `Panel`/scrollbox; footer→`HintBar` per panel. Keys/handlers unchanged.
- **`components/overlay.tsx`** — becomes `Card` + `Scrim`; add status-bar variant. `overlays/overlays.tsx` — each card adopts `Card`/`Badge`/`HintBar`; `ResultOverlay` gets success/error bar; `WorkingOverlay` uses `Spinner`.
- **`components/help.tsx`** — group bindings under eyebrows; legend re-mapped to five hues.
- **`app.tsx`** — mount `useToasts()` + `ToastLayer` (high zIndex); route yank/swap/refresh confirmations to toasts; disconnected banner→persistent top toast.
- **`components/diffpane.tsx` / `diffview.tsx` / `procview.tsx`** — palette-token swaps only (diff.*, semantic hues); structure untouched. `procview` sparkline already hand-built — recolour to `good`/`attn` by CPU threshold.

---

## 6. Risks & OpenTUI limits

- **No per-frame reflow.** OpenTUI runs a real Yoga layout; the deboxed hairline/eyebrow structure is *fewer* nested boxes than today's four bordered panels, so layout cost drops. Keep the slice list rendering all rows (N is small); if a workspace ever has hundreds of slices, wrap the list in `scrollbox` with `viewportCulling` (available).
- **Scrim opacity is a buffer post-op.** Animating the scrim's `opacity` every frame would repaint the whole backdrop. **Spec it static** (fixed ~0.5) — dim appears/disappears with the modal, no tween. Reserve `opacity` tweening for the one-shot toast fade (a handful of frames on a single small box).
- **Toast timers** re-render on mount/dismiss only — one `setTimeout` per toast, cleared on unmount. No polling.
- **No gradient / shadow / progress / gauge / sparkline components in OpenTUI.** The spec uses none of these as primitives — depth comes from the neutral ramp (bg/surface/surfaceAlt) not shadows; the proc sparkline stays hand-built (already is). Don't introduce a `<Spinner/>`/`<Progress/>` assumption — `Spinner` here is our own braille-frame component.
- **Border styles limited to single/double/rounded/heavy.** Standardise on `rounded` (right pane + cards) and per-side `border={["left"|"right"]}` for focus bars/dividers — both confirmed available. Hairline *dividers between stacked sections* are drawn as a `─`-filled `text` row (a per-side border can't sit mid-column).
- **Emoji width.** Dropping ❌/✅/⏳ for glyph+colour avoids the double-width/′variation-selector inconsistency across terminals.
- **Focus must never depend on colour alone.** Focus = `▎` bar **and** bg tint **and** bold — three redundant signals — so it survives low-contrast terminal themes and colour-blindness.
- **Backwards compat.** This is skin/layout only; the RPC types, key handlers, and overlay state machine (`useOverlays`) are untouched, so parity with the Go TUI's *behaviour* is preserved while the *look* diverges as mandated.
