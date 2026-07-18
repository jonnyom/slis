// Theme mirrored from the Bubble Tea TUI (internal/tui/detail.go). The Go side
// uses lipgloss ANSI-256 palette indices; these are their exact xterm hex
// equivalents so the JS TUI reads identically. Glyphs are the literal runes the
// Go side renders (sliceGlyph / sessionBadge).

import type { SessionStatus } from "./rpc/types";

export const color = {
  // Accents
  title: "#5fafff", // 75  — "slis" title / focused panel border
  live: "#00d787", // 42  — ● live
  synced: "#00af5f", // 35  — ✓ in-review / approved
  wait: "#ffaf00", // 214 — ⏸ waiting / stale / conflict
  done: "#5fffff", // 87  — ✦ done
  ready: "#87ff87", // 120 — ♻ ready to clear
  merged: "#af87ff", // 141 — merged
  missing: "#ff5f5f", // 203 — missing / error / changes-requested
  candidate: "#87d7ff", // 117 — new-worktree / create-name
  restack: "#ff8700", // 208 — ⟳ needs restack / cpu warn
  repoHeader: "#0087ff", // 33  — repo name headers
  code: "#afffff", // 159 — inline code
  // Structure
  border: "#585858", // 240 — unfocused panel border
  borderFocus: "#5fafff", // 75  — focused panel border
  boxBorder: "#5f5fd7", // 62  — help / empty-state box border
  dim: "#808080", // 244 — dim headers / faint rows
  stackHeader: "#949494", // 246 — stack-cluster group header
  diffHeader: "#808080", // 244 — diff file/hunk header
  // Foregrounds
  fg: "#c0c0c0", // terminal-default-ish light gray
  white: "#ffffff", // 231
  cursorBar: "#5fafff", // 75  — ▎ focus marker
} as const;

// Diff colors (colorizePatch in the Go TUI).
export const diffColor = {
  add: "#00af5f", // green +
  del: "#ff5f5f", // red -
  hunk: "#5fafff", // blue @@
  header: "#808080", // dim file header
} as const;

// Slice-row glyph per combined session/work state (sliceGlyph).
export const glyph = {
  waiting: "⏸",
  done: "✦",
  ciFail: "❌",
  ready: "♻",
  inReview: "✓",
  live: "●",
  running: "●",
  idle: "·",
  restack: "⟳",
  selected: "✓",
  focusBar: "▎",
  filterMarker: "▸",
  arrow: "→",
} as const;

// Standalone session badge (sessionBadge) — used in the cockpit Session panel.
export function sessionBadge(status: SessionStatus): { glyph: string; color: string } {
  switch (status) {
    case "waiting-input":
      return { glyph: "⏸", color: color.wait };
    case "running":
      return { glyph: "●", color: color.live };
    case "done":
      return { glyph: "✓", color: color.done };
    case "none":
    default:
      return { glyph: "○", color: color.dim };
  }
}

export function sessionLabel(status: SessionStatus): string {
  switch (status) {
    case "waiting-input":
      return "waiting for input";
    case "running":
      return "running";
    case "done":
      return "done";
    case "none":
    default:
      return "no session";
  }
}
