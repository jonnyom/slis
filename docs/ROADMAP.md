# slis roadmap — parallel-agent mission control

**Thesis:** the crowded part of the space is *spawning* parallel agents (Claude Squad, Conductor, Superset — almost all single-repo). The real, repeatedly-cited bottleneck is **human review/merge bandwidth**, and it's worse across multiple repos. slis is uniquely placed at the multi-repo + Graphite-stack + PR/CI layer, so it should own **convergence**: triage → review → merge → clean across many slices.

## Phase 1 — Converge faster ✅ shipped
- **Inbox / triage queue** — `n`/`N` jump to the next/prev slice needing you (waiting-input → CI-red → needs-restack → ready); "Inbox" state filter, urgency-sorted.
- **Batch actions** — `space`/`A` select; `d` clear and `R` restack act on the selection (else focused). Clear-all-ready in a few keys.

## Phase 2 — Close the loop to shipped ✅ shipped
- **`gt submit`** — create/update a slice's PRs from its Graphite stack (`R` → `[p]`).
- **`gt merge`** — merge the stack via **Graphite's server-side queue** (`R` → `[m]`). Chosen over local `gh pr merge`: Graphite handles squash/merge/restack on its servers, so slis triggers and walks away — no local waiting/reloading. Also `gt sync` (`R` → `[s]`).

## Phase 3 — Prevent pain + scale the fleet
- **Cross-slice conflict radar** — warn when two in-flight slices edit the same files across repos, before merge (uses the multi-repo diffs slis already computes).
- **Launch agents from slis** — "new slice" → create worktrees across repos + spawn a `claude` session with a task prompt. The control-plane leap (multi-repo — no competitor does this).

## Deferred (not scoped)
- Per-slice env isolation (ports / DB branches) so several slices run live at once — biggest lift, most moving parts.
- Fleet analytics / retro (throughput, what's stuck, CI pass rate) — useful, not bottleneck-critical.
