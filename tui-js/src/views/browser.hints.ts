// Pure browser hint-bar logic. Kept React-free so the contextual hint set is
// unit-testable without booting OpenTUI. See spec §3.4.

import type { Hint } from "../components/hintbar";
import { needsRestack, type SliceView } from "../state/derive";

// A focused slice has failing CI when any PR reports a failed check or a
// non-zero fail count. Drives the contextual `v why` / `F fix` hints (M4) and
// the auto-CI-log cockpit entry.
export function hasFailingCi(view: SliceView | undefined): boolean {
  return !!view?.prs?.some(
    (p) => p.number !== undefined && (p.ci === "fail" || (p.ci_fail ?? 0) > 0),
  );
}

// The contextual list hint bar. A waiting-input slice signposts the answer key
// (P1); a red-CI slice surfaces `v why` / `F fix` (M4); a slice that needs
// restacking surfaces `R stack` (D3); and while a search is active `n/N` are
// labelled `match` to reflect their search-repeat meaning (M3).
export function listHints(focused: SliceView | undefined, searchActive: boolean): Hint[] {
  const waiting = focused?.status === "waiting-input";
  const hints: Hint[] = [
    { key: "enter", label: "open" },
    { key: "a", label: waiting ? "answer" : "agent" },
    { key: "C", label: "launch" },
    { key: "V", label: "review" },
    { key: "t", label: "shell" },
    { key: "s", label: "sessions" },
  ];
  if (hasFailingCi(focused)) hints.push({ key: "v", label: "why" }, { key: "F", label: "fix" });
  if (focused && needsRestack(focused)) hints.push({ key: "R", label: "stack" });
  hints.push({ key: "w", label: "swap" });
  if (searchActive) hints.push({ key: "n/N", label: "match" });
  else hints.push({ key: "space", label: "select" }, { key: "/", label: "search" });
  hints.push({ key: ",", label: "config" });
  return hints;
}
