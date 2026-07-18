# slis JS TUI — usability review

Repo `/Users/jonathanomahony/personal/slis`, branch `experiment/js-tui` @ `f60339d`.
Method: drove the real app (`src/index.tsx`) in a Bun PTY with `ghostty-opentui`
under `SLIS_FAKE=1`, walking every flow at 120×35 (typical) and 200×55 (large).
Driver + all captures: `scratchpad/drive.ts`, `scratchpad/caps/`, `scratchpad/caps-lg/`.
Lenses: L1 familiarity, L2 review-ergonomics, L3 problem-first, L4 predictability.

Counts: **0 blocker · 4 major · 3 medium · 7 minor/polish.**

The bones are good. Navigation is vim-clean (j/k/g/G/h/l, esc/h back everywhere),
the breadcrumb always answers *where am I*, hint bars are truthful for the keys they
show, and loading / empty / disconnected states are all handled. The lens-2 review
ergonomics targets are **met**: cold-launch → whole diff of the slice that needs me =
**2 keystrokes** (`enter` cockpit → `enter` rich diff, because boot focuses the
waiting-input slice); → CI log = **3** (`enter`, `2`, `v`); → CPU hog identified =
**1** (`P`). The findings below are mostly about the *edges*: the help screen, the
one broken overlay, and a few key/label/colour choices that fight muscle memory or
hide problem-first actions one level too deep.

---

## MAJOR

### M1 · All-slices process overlay (`P`) header is garbled — L2/L4
`P` from the browser is the headline "what's eating my CPU" answer (1 keystroke, the
lens-2c target). But the sort-hint line and the column header render **on the same
row**, interleaved character-by-character and unreadable:

```
│ soPID cpu ·CPU%ycCPU~ j/k moveMEM(MB)llMEM~ kill trCMD· P/esc close        █ │
```

(that is `sort: cpu · s cycle · j/k move · … kill tree · P/esc close` and
`PID CPU% CPU~ MEM(MB) MEM~ CMD` collided). Capture: `caps/overlays2-10-procs-all.txt`.
The cockpit's Processes panel composes the *same two elements* (`ProcsRight`) inside a
`<scrollbox>` and renders cleanly (`caps/cockpit-08-jump-procs.txt`), so the fix is to
match that structure.
- **Fix location:** `src/components/procoverlay.tsx:125-128` (the bare hint `<text>` +
  `<ProcTableHeader>` as direct flex children of the padded box). Give them explicit
  rows / move inside the scroll region as the cockpit pane does.

### M2 · Help overlay truncates its own descriptions mid-word — L4/L1
The help card is a fixed `width={60}` (clamped only *downward*), so at **every**
terminal size the description column is ~38 cols and long entries are chopped:
`create a new slice (worktrees per repo` (no `)`), `stack actions: restack / submit /
merg`, `conflict radar (files changed by >1 sl`, `clear finished slice(s): [y] remove
·` (loses `[f] force`), and the legend/`ctrl+q` footer are cut too. Captures:
`caps/overlays-01-help.txt`, `caps-lg/overlays-01-help.txt` (still clipped at 200 cols).
This matters more here than in most TUIs because the hint bar only shows 4-6 keys —
**~15 browser actions and most cockpit actions are reachable only by reading `?`**, and
`?` is where they're unreadable.
- **Fix location:** `src/components/help.tsx` (widen `Card width` to ~78-84, which the
  Card already clamps to `termWidth-2`; and/or wrap the help column instead of relying
  on `wrapMode="none"` clipping). Keys use `padEnd(18)` — budget the remainder.

### M3 · `n`/`N` steals the vim/less search-repeat reflex — L1
`/` is a first-class search (`caps/search-*`), and this is a technical audience whose
fingers expect `n`/`N` to **repeat the last search**. Here `n`/`N` instead jump to the
next/prev *attention* slice (`nextAttentionRow`) and do so even while a search filter is
active — so after `/pay⏎`, `n` does not step search matches. It's documented
("next / prev slice needing you"), but overloading the two most ingrained search keys
for something else, when search exists, doesn't earn its place.
- **Fix location:** `src/views/browser.tsx:657-666`. Recommend: when a search is active,
  `n`/`N` walk matches; give attention-hopping a non-colliding key (e.g. reuse `]`/`[`,
  or Tab-cycle within the Inbox filter). This is a keymap decision — flag for the wave,
  don't just silently swap.

### M4 · Red CI is shown in the browser but only actionable in the cockpit — L3
The browser preview shows the failing state right on the slice (`web … #8107 open ✗·2 ✗
changes`, `caps/browser-00-boot.txt`), but there is **no key from there** to see *why*.
The log (`v`), fix-ci (`F`) and re-run (`ctrl+r`) live only in the cockpit **PRs** panel.
Honest cold-launch counts: log = `enter,2,v` (3), fix-ci = `enter,2,F` (3). Problem-first
says the red should be one key from where it's displayed.
- **Fix location:** `src/views/browser.tsx` key handler — add a browser affordance that
  opens the focused slice straight into the PRs panel with the CI log (and expose `F`),
  rather than making the user rediscover the path each time. Pairs with M2 (surface it).

---

## MEDIUM

### D1 · One `⚠` glyph, three meanings — L4
`glyph.dirty` (`⚠`) simultaneously denotes **needs-restack**, **primary-behind (stale)**,
and **file-overlap**. On a browser row `⏸ checkout ● ⚠` the `⚠` is the *stale* flag,
while "needs restack" is only in the header count and the cockpit stack node — the row
can't tell you which condition it is. Predictability wants one glyph = one meaning.
- **Fix location:** `src/theme.ts` glyph set + `src/views/browser.tsx` `SliceRow` /
  `previewLines`. Give restack / stale / overlap distinct marks (or colours + a legend
  entry per meaning; the help legend currently defines none of the three).

### D2 · Refusals wear a green success ✓ — L3/L4
"Cannot clear — checkout is live" and other guards route through `overlays.info()` which
sets `ok:true`, so the card title gets the green success glyph and a green left status
bar: `╭─✓ Cannot clear───╮` (`caps/overlays2-01-clear-live-refused.txt`). A blocked
action reads as a success.
- **Fix location:** `src/overlays/useOverlays.tsx` (`info` → `ok:true`) and the browser/
  cockpit `d`-handlers that call `overlays.info("Cannot clear", …)`. Add a neutral/`warn`
  status, or route refusals through `error`.

### D3 · The restack action is hidden from where restack is flagged — L3
"Needs restack" is a header stat (`⟳ 1 restack`), a filter (6), and a per-row/stack `⚠`,
but `R` (stack actions) is **not** in the browser hint bar and restack is two keys
(`R` then `r`). The single most-flagged stack problem is only discoverable via `?`.
- **Fix location:** `src/views/browser.tsx` `LIST_HINTS` — surface `R` contextually when
  the focused slice needs restack (like the cockpit stack panel already does with
  `R stack`).

---

## MINOR / POLISH

### P1 · Waiting-input isn't signposted as "answer" — L4
A waiting-input slice shows `⏸ needs you`; the one key that takes you to the agent to
answer (`a`, attach) is labelled `a term`. The one-key-to-session path exists and is
good — it just isn't tied to the "needs you" state. Consider a contextual `a answer`
hint / label when the focused slice is `waiting-input`. (`src/views/browser.tsx`.)

### P2 · Overlay hint rows / bodies clip at the card edge — L4/polish
On the narrower cards the trailing hint clips: stack-actions loses `? more` → `? mor`
and the conflict-radar footer loses `Committed changes only.` (`caps/overlays-05`,
`caps/overlays-07`). The affordance that teaches discoverability (`? more`) is itself
truncated. `src/components/card.tsx` / `hintbar.tsx` — size cards to content or wrap.

### P3 · Rich-diff file panel overruns into the footer — polish
In the rich diff the file-list panel extends one row too far; its bottom rule collides
with the footer hint line for the panel's width (`j/k─file─·─[─]─…`, box-rule chars
replacing spaces). Captures: `caps/cockpitDiff-06`, `-09`. Layout in
`src/components/diffview.tsx` (reserve the footer row).

### P4 · `enter` in the rich diff is a near-noop — L4/polish
Everywhere else `enter` drills in / zooms; in the rich diff it just `scrollTo(0)` and
isn't listed in `DIFF_BINDINGS`. Pressing it on a file does nothing visible.
`src/components/diffview.tsx:213`.

### P5 · CI-fail count formatting differs across screens — polish
Browser preview renders `✗·2` (middle dot); cockpit PRs renders `✗2`. One format.
`src/views/browser.tsx` `prBadge` vs `src/views/cockpit.tsx` `ciBadge`.

### P6 · Two overlapping "what needs me" buckets — L4/minor
Filter 2 "Needs you" and filter 8 "Inbox" (= needsYou ∪ restack ∪ stale) both frame
attention, alongside the header "N need you"; default filter is "All". Consider whether
both filters need to exist or one should be the default landing. `src/state/derive.ts`.

### P7 · `q` quits from any depth, no confirm — polish
`q` exits the whole app even from the cockpit / rich diff (matches lazygit, and `esc`/`h`
is the level-up key), but a user treating `q` as "back" can quit unexpectedly. Low; keep
unless testers trip on it. `src/app.tsx` `quit`.

---

## Strengths to preserve (don't regress in the fix wave)
- Breadcrumb `slis › checkout › Stack [› zoom]` — always answers *where am I*.
- `esc`/`h` = back at every level; `?` = help everywhere; consistent panel `tab`/`1-4`.
- Boot focuses the waiting-input slice, so the ≤3-keystroke review target is hit.
- Truthful contextual hint bars; good empty ("Press c … or i …"), loading, and
  `⚠ sidecar disconnected` states; scrim behind modals (present, just invisible to a
  text capture).

---

## Top 10 fixes, ordered (one implementation wave)
1. **M1** Fix the `P` all-slices overlay garbled header (match the cockpit scrollbox layout).
2. **M2** Widen/wrap the help overlay so no description truncates — it's the reference for every hidden key.
3. **M4** Make red CI actionable from the browser: one key from a red slice into the PRs panel + CI log (and expose `F`).
4. **M3** Resolve `n`/`N` vs search-repeat — decide keymap: `n`/`N` repeat matches when a search is active, attention-hop moves keys.
5. **D2** Stop refusals ("Cannot clear …") showing a green success ✓ — neutral/warn status.
6. **D1** Disambiguate the `⚠` glyph (restack vs stale vs overlap) in rows/preview + add legend entries.
7. **D3** Surface `R` (restack/stack) in the browser hint bar, contextually when the focused slice needs restack.
8. **P1** Signpost the waiting-input action (`a answer` / contextual hint on `needs you`).
9. **P2** Stop overlay hint rows and card bodies clipping (`? more`, `cancel`, radar footer).
10. **P3/P4** Rich-diff polish: reserve the footer row so the file panel stops colliding; make `enter` meaningful or drop it from the diff.
