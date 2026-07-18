// Background refresh tick gating (parity gap G7). A 30s ticker force-refreshes
// PR / CI / stack data, but must not fight the bulk-load lazy mode or a focused
// PTY tab. `tickPlan` is the pure decision — which slices (if any) a tick should
// refresh — so the gating is unit-testable and the app.tsx interval stays a thin
// shell around it.

import type { BulkPhase } from "./bulkload";

export interface TickContext {
  // A PTY terminal tab is focused, or a blocking prompt (bulk-load) is open.
  paused: boolean;
  phase: BulkPhase;
  focusedSlice: string | null;
  slices: string[];
}

export type TickPlan = { run: false } | { run: true; slices: string[] };

export function tickPlan(ctx: TickContext): TickPlan {
  if (ctx.paused) return { run: false };
  if (ctx.phase === "lazy") {
    return ctx.focusedSlice ? { run: true, slices: [ctx.focusedSlice] } : { run: false };
  }
  if (ctx.slices.length === 0) return { run: false };
  return { run: true, slices: ctx.slices };
}
