// Slice classification for the browser's states rail. Mirrors the Bubble Tea
// browser's work-state buckets. `prStack` now carries a per-PR CI rollup, but
// the buckets here still key off review decision and session status (a CI-fail
// signal is surfaced per-row in the cockpit's PRs panel, not the states rail).

import type {
  PrStackEntry,
  SessionStatus,
  ShowResult,
  Slice,
} from "../rpc/types";

export type WorkState = "needs-you" | "in-review" | "ready" | "in-progress";

export interface SliceView {
  slice: Slice;
  status: SessionStatus;
  prs?: PrStackEntry[];
  show?: ShowResult;
}

function hasPrs(prs: PrStackEntry[] | undefined): prs is PrStackEntry[] {
  return !!prs && prs.some((p) => p.number !== undefined);
}

function allMerged(prs: PrStackEntry[] | undefined): boolean {
  if (!hasPrs(prs)) return false;
  const withPr = prs.filter((p) => p.number !== undefined);
  return withPr.every((p) => p.state === "MERGED");
}

function anyOpenPr(prs: PrStackEntry[] | undefined): boolean {
  return !!prs && prs.some((p) => p.state === "OPEN");
}

function changesRequested(prs: PrStackEntry[] | undefined): boolean {
  return !!prs && prs.some((p) => p.review_decision === "CHANGES_REQUESTED");
}

export function needsRestack(view: SliceView): boolean {
  if (!view.show) return false;
  return view.show.members.some((m) =>
    (m.stack ?? []).some((n) => n.needs_restack),
  );
}

export function needsYou(view: SliceView): boolean {
  return (
    view.status === "waiting-input" ||
    view.status === "done" ||
    changesRequested(view.prs)
  );
}

export function workState(view: SliceView): WorkState {
  if (needsYou(view)) return "needs-you";
  if (allMerged(view.prs)) return "ready";
  if (anyOpenPr(view.prs)) return "in-review";
  return "in-progress";
}

export interface Filter {
  key: string; // "1".."8"
  label: string;
  match: (view: SliceView) => boolean;
}

export const FILTERS: Filter[] = [
  { key: "1", label: "All", match: () => true },
  { key: "2", label: "Needs you", match: (v) => needsYou(v) },
  {
    key: "3",
    label: "In review",
    match: (v) => workState(v) === "in-review",
  },
  { key: "4", label: "Ready", match: (v) => workState(v) === "ready" },
  {
    key: "5",
    label: "In progress",
    match: (v) => workState(v) === "in-progress",
  },
  { key: "6", label: "Needs restack", match: (v) => needsRestack(v) },
  { key: "7", label: "Live", match: (v) => v.slice.active },
  {
    key: "8",
    label: "Inbox",
    match: (v) => needsYou(v) || needsRestack(v) || v.slice.stale,
  },
];

/** Sort key for the Inbox filter: most-urgent first. */
export function attentionRank(view: SliceView): number {
  if (view.status === "waiting-input") return 0;
  if (changesRequested(view.prs)) return 1;
  if (view.status === "done") return 2;
  if (needsRestack(view)) return 3;
  if (view.slice.stale) return 4;
  return 99;
}
