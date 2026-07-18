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

export interface CockpitHintState {
  scope: string; // short scope name for the stack panel (working/parent/trunk)
  showPatch: boolean;
  zoomed: boolean;
  killPending: boolean;
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
      { key: "a", label: "term" },
      { key: "w", label: "swap" },
    ];
  switch (panel) {
    case "stack":
      return [
        { key: "tab", label: "panel" },
        { key: "j/k", label: "repo" },
        { key: "enter", label: "rich diff" },
        { key: "b", label: `scope: ${s.scope}` },
        { key: "t", label: s.showPatch ? "stat" : "patch" },
        { key: "w", label: "swap" },
        { key: "R", label: "stack" },
      ];
    case "prs":
      return [
        { key: "tab", label: "panel" },
        { key: "j/k", label: "pr" },
        { key: "enter", label: "zoom" },
        { key: "v", label: "CI log" },
        { key: "F", label: "fix-ci" },
        { key: "O", label: "open PR" },
        { key: "w", label: "swap" },
      ];
    case "session":
      return [
        { key: "tab", label: "panel" },
        { key: "enter", label: "zoom" },
        { key: "a", label: "term" },
        { key: "r", label: "reload" },
        { key: "w", label: "swap" },
      ];
    case "procs":
      return [
        { key: "tab", label: "panel" },
        { key: "j/k", label: "proc" },
        { key: "h/l", label: "fold" },
        { key: "s", label: "sort" },
        { key: "x/X", label: "kill" },
        { key: "enter", label: "zoom" },
      ];
  }
}
