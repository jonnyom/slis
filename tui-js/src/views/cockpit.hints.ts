// Pure cockpit logic — panel cycling, breadcrumb section labels, and the
// per-panel contextual hint sets (spec §3.2, §3.4). Kept React-free so the
// behaviour is unit-testable without booting OpenTUI.

import type { Hint } from "../components/hintbar";

export type PanelId = "stack" | "prs" | "session" | "procs";

export const PANEL_ORDER: PanelId[] = ["stack", "prs", "session", "procs"];

// How the cockpit should open when entered from the browser (M4). Lets a red-CI
// slice jump straight to the PRs panel with its failing-CI log already loaded.
export interface CockpitEntry {
  panel?: PanelId;
  ciLog?: boolean;
}

// Breadcrumb segment shown after the slice name (spec §3.2 mockup: `Stack`,
// not the eyebrow's louder `REPOS & STACK`).
export const SECTION_LABEL: Record<PanelId, string> = {
  stack: "Stack",
  prs: "PRs",
  session: "Session",
  procs: "Processes",
};

// Cycle panels in either direction, wrapping (parity gap G9 — Go cockpit.go
// tab/L forward, shift+tab/H back).
export function cyclePanel(current: PanelId, delta: number): PanelId {
  const n = PANEL_ORDER.length;
  const i = PANEL_ORDER.indexOf(current);
  return PANEL_ORDER[(((i + delta) % n) + n) % n]!;
}

// Zoom appends ` › zoom` to the breadcrumb path (spec §3.2).
export function breadcrumbSection(panel: PanelId, zoomed: boolean): string {
  const label = SECTION_LABEL[panel];
  return zoomed ? `${label} › zoom` : label;
}

// The Stack panel's right-pane sub-mode (F3): the branch diff, the lazy file
// tree, or a file's content.
export type ReviewMode = "diff" | "tree" | "file";

export interface CockpitHintState {
  scope: string; // short scope name for the stack panel (working/parent/trunk)
  zoomed: boolean;
  killPending: boolean;
  // Stack-panel review context (F3).
  reviewMode: ReviewMode;
  onMember: boolean; // the selected branch is the slice's own (member) branch
  stackReview: boolean; // the sidecar supports branchDiff/tree/file
}

// The 4–6 actions relevant to the current focus. `HintBar` always appends
// `? more`, so these never include it.
export function cockpitHints(panel: PanelId, s: CockpitHintState): Hint[] {
  if (s.killPending)
    return [
      { key: "y", label: "confirm" },
      { key: "n", label: "cancel" },
    ];
  if (s.zoomed)
    return [
      { key: "enter", label: "unzoom" },
      { key: "j/k", label: "move" },
      { key: "^d/u", label: "scroll" },
      { key: "a", label: "agent" },
      { key: "C", label: "launch" },
      { key: "t", label: "shell" },
      { key: "w", label: "swap" },
    ];
  switch (panel) {
    case "stack":
      if (s.reviewMode === "file")
        return [
          { key: "j/k", label: "line" },
          ...(s.onMember ? [{ key: "e", label: "edit" }] : []),
          { key: "c", label: "comment" },
          { key: "V", label: "review" },
          { key: "C", label: "launch" },
          { key: "^d/u", label: "page" },
          { key: "esc", label: "tree" },
        ];
      if (s.reviewMode === "tree")
        return [
          { key: "j/k", label: "move" },
          { key: "l", label: "open/expand" },
          { key: "h", label: "collapse" },
          ...(s.onMember ? [{ key: "e", label: "edit" }] : []),
          { key: "o/E", label: "repo/slice" },
          { key: "C", label: "launch" },
          { key: "esc", label: "diff" },
        ];
      return [
        { key: "j/k", label: "branch" },
        ...(s.stackReview ? [{ key: "f", label: "files" }] : []),
        { key: "enter", label: "rich diff" },
        ...(s.onMember ? [{ key: "b", label: `scope: ${s.scope}` }] : []),
        { key: "V", label: "review" },
        { key: "C", label: "launch" },
        { key: "w", label: "swap" },
      ];
    case "prs":
      return [
        { key: "C", label: "launch" },
        { key: "j/k", label: "pr" },
        { key: "enter", label: "zoom" },
        { key: "v", label: "CI log" },
        { key: "F", label: "fix-ci" },
        { key: "y", label: "copy URL" },
        { key: "O", label: "open PR" },
      ];
    case "session":
      return [
        { key: "tab", label: "panel" },
        { key: "enter", label: "zoom" },
        { key: "a", label: "agent" },
        { key: "C", label: "launch" },
        { key: "t", label: "shell" },
        { key: "r", label: "reload" },
        { key: "w", label: "swap" },
      ];
    case "procs":
      return [
        { key: "C", label: "launch" },
        { key: "j/k", label: "proc" },
        { key: "h/l", label: "fold" },
        { key: "s", label: "sort" },
        { key: "x/X", label: "kill" },
        { key: "enter", label: "zoom" },
      ];
  }
}
