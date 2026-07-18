// Semantic theme system for the JS TUI (redesign spec §2). OpenTUI accepts hex
// strings directly, so tokens stay plain hex. Widgets consume the mutable
// `theme` object below; switching a theme updates it in place so imports remain
// stable while React repaints with the new values.
//
// Each palette has a neutral ramp, one focus/identity accent, and four semantic
// slots (good / attn / bad / merged). Diff and syntax colors are derived from
// these rather than introducing widget-local hues. `attention(view)` collapses work-state +
// session status into the scannable "left edge" glyph; `badgeFor(state)` maps a
// state keyword to a Badge's glyph+color.
//
// Back-compat: the pre-redesign `color` map is preserved but re-pointed onto the
// new tokens so existing views keep compiling and immediately render the new
// palette. New code should import `theme` (and the helpers) rather than `color`.

import type { SessionStatus } from "./rpc/types";
import type { FileStatus } from "./diff/parse";
import type { TokenKind } from "./diff/tokenize";
import type { SliceView } from "./state/derive";

// ── Palettes ────────────────────────────────────────────────────────────────

export interface ThemeTokens {
  bg: string;
  surface: string;
  surfaceAlt: string;
  hairline: string;
  border: string;
  textFaint: string;
  textDim: string;
  text: string;
  textBright: string;
  focus: string;
  focusDim: string;
  good: string;
  attn: string;
  bad: string;
  merged: string;
  diffAddBg: string;
  diffAddChangeBg: string;
  diffDelChangeBg: string;
}

export const themes = {
  midnight: {
    bg: "#0b0d12", surface: "#14181f", surfaceAlt: "#1c2029",
    hairline: "#262b34", border: "#3a414c", textFaint: "#5b6472",
    textDim: "#8a93a3", text: "#c3cad6", textBright: "#f4f7fb",
    focus: "#4c9dff", focusDim: "#2d5c8f", good: "#34d399",
    attn: "#f5a623", bad: "#ff5d5d", merged: "#b28bff",
    diffAddBg: "#0d2117", diffAddChangeBg: "#16452e", diffDelChangeBg: "#2e1214",
  },
  violet: {
    bg: "#100e17", surface: "#191624", surfaceAlt: "#242033",
    hairline: "#302b40", border: "#49425e", textFaint: "#6e6681",
    textDim: "#a39bb5", text: "#d2ccdf", textBright: "#fffaff",
    focus: "#c084fc", focusDim: "#704b93", good: "#4ade80",
    attn: "#fbbf24", bad: "#fb7185", merged: "#67e8f9",
    diffAddBg: "#10251a", diffAddChangeBg: "#174b2a", diffDelChangeBg: "#35151e",
  },
  light: {
    bg: "#f7f5f2", surface: "#ffffff", surfaceAlt: "#ebe7e1",
    hairline: "#d8d3cc", border: "#9b948b", textFaint: "#716a62",
    textDim: "#57514b", text: "#292622", textBright: "#11100e",
    focus: "#075fc8", focusDim: "#8eb7e3", good: "#087443",
    attn: "#9a5800", bad: "#bd202d", merged: "#7248a5",
    diffAddBg: "#e8f5ed", diffAddChangeBg: "#c5e8d2", diffDelChangeBg: "#f4ccd0",
  },
  mono: {
    bg: "#101010", surface: "#181818", surfaceAlt: "#252525",
    hairline: "#383838", border: "#666666", textFaint: "#777777",
    textDim: "#a0a0a0", text: "#d0d0d0", textBright: "#ffffff",
    focus: "#ffffff", focusDim: "#888888", good: "#d8d8d8",
    attn: "#ffffff", bad: "#a8a8a8", merged: "#c0c0c0",
    diffAddBg: "#202020", diffAddChangeBg: "#383838", diffDelChangeBg: "#383838",
  },
  "mono-light": {
    bg: "#f7f7f7", surface: "#ffffff", surfaceAlt: "#e8e8e8",
    hairline: "#d0d0d0", border: "#929292", textFaint: "#696969",
    textDim: "#505050", text: "#282828", textBright: "#101010",
    focus: "#101010", focusDim: "#888888", good: "#303030",
    attn: "#101010", bad: "#585858", merged: "#404040",
    diffAddBg: "#e8e8e8", diffAddChangeBg: "#d0d0d0", diffDelChangeBg: "#d0d0d0",
  },
} as const satisfies Record<string, ThemeTokens>;

export type ThemeName = keyof typeof themes;
const CYCLING_THEMES: ThemeName[] = ["midnight", "violet", "light"];

export function resolveThemeName(value?: string, noColor = false): ThemeName {
  const normalized = value?.trim().toLowerCase();
  if (noColor) return normalized === "light" || normalized === "mono-light" ? "mono-light" : "mono";
  if (normalized === "dark" || normalized === "blue") return "midnight";
  if (normalized === "purple") return "violet";
  if (normalized === "no-color" || normalized === "monochrome") return "mono";
  if (normalized && normalized in themes) return normalized as ThemeName;
  return "midnight";
}

let activeTheme: ThemeName = resolveThemeName(process.env.SLIS_THEME, process.env.NO_COLOR !== undefined);
export const theme: ThemeTokens = { ...themes[activeTheme] };

export function themeName(): ThemeName {
  return activeTheme;
}

// ── Back-compat `color` map (old names → new tokens) ─────────────────────────

export const color = {
  // Accents
  title: theme.focus,
  live: theme.good,
  synced: theme.good,
  wait: theme.attn,
  done: theme.merged,
  ready: theme.good,
  merged: theme.merged,
  missing: theme.bad,
  candidate: theme.focus,
  restack: theme.attn,
  repoHeader: theme.focus,
  code: theme.good,
  // Structure
  border: theme.hairline,
  borderFocus: theme.focus,
  boxBorder: theme.focus,
  overlayBg: theme.surface,
  dim: theme.textDim,
  stackHeader: theme.textFaint,
  diffHeader: theme.textDim,
  // Foregrounds
  fg: theme.text,
  white: theme.textBright,
  cursorBar: theme.focus,
};

// ── Diff colors (derived — no new hues) ──────────────────────────────────────

export const diffColor = {
  add: theme.good,
  del: theme.bad,
  hunk: theme.focus,
  header: theme.textDim,
  // A quiet full-row tint makes additions scannable even when syntax colours
  // override the green foreground; changed words get the stronger layer.
  addBg: theme.diffAddBg,
  addChangeBg: theme.diffAddChangeBg,
  delChangeBg: theme.diffDelChangeBg,
  gutter: theme.border,
};

// ── Syntax tokens (re-pinned to the five-hue system) ─────────────────────────

export const syntaxColor: Record<TokenKind, string> = {
  keyword: theme.merged,
  string: theme.good,
  number: theme.attn,
  type: theme.focus,
  function: theme.focus,
  comment: theme.textFaint,
  punct: theme.textDim,
  plain: theme.text,
};

function syncDerivedColors(): void {
  Object.assign(color, {
    title: theme.focus, live: theme.good, synced: theme.good, wait: theme.attn,
    done: theme.merged, ready: theme.good, merged: theme.merged, missing: theme.bad,
    candidate: theme.focus, restack: theme.attn, repoHeader: theme.focus, code: theme.good,
    border: theme.hairline, borderFocus: theme.focus, boxBorder: theme.focus,
    overlayBg: theme.surface, dim: theme.textDim, stackHeader: theme.textFaint,
    diffHeader: theme.textDim, fg: theme.text, white: theme.textBright, cursorBar: theme.focus,
  });
  Object.assign(diffColor, {
    add: theme.good, del: theme.bad, hunk: theme.focus, header: theme.textDim,
    addBg: theme.diffAddBg, addChangeBg: theme.diffAddChangeBg,
    delChangeBg: theme.diffDelChangeBg, gutter: theme.border,
  });
  Object.assign(syntaxColor, {
    keyword: theme.merged, string: theme.good, number: theme.attn,
    type: theme.focus, function: theme.focus, comment: theme.textFaint,
    punct: theme.textDim, plain: theme.text,
  });
}

export function setTheme(name: ThemeName): ThemeName {
  // NO_COLOR is a contract, not merely a startup suggestion.
  const next = process.env.NO_COLOR !== undefined
    ? name === "light" || name === "mono-light" ? "mono-light" : "mono"
    : name;
  Object.assign(theme, themes[next]);
  activeTheme = next;
  syncDerivedColors();
  return activeTheme;
}

export function cycleTheme(): ThemeName {
  if (process.env.NO_COLOR !== undefined)
    return setTheme(activeTheme === "mono-light" ? "midnight" : "light");
  const index = CYCLING_THEMES.indexOf(activeTheme);
  return setTheme(CYCLING_THEMES[(index + 1) % CYCLING_THEMES.length]!);
}

export function colorForKind(kind: TokenKind): string {
  return syntaxColor[kind];
}

// File-tree status glyph colors (A/M/D/R).
export function statusColor(status: FileStatus): string {
  switch (status) {
    case "added":
      return theme.good;
    case "deleted":
      return theme.bad;
    case "renamed":
      return theme.focus;
    case "modified":
    default:
      return theme.attn;
  }
}

// ── Glyphs (trimmed; each pinned to one color-by-context) ────────────────────

export const glyph = {
  waiting: "⏸",
  done: "✦",
  ready: "♻",
  inReview: "✓",
  changes: "✗",
  ciFail: "✗",
  ciPass: "✓",
  ciPending: "⋯",
  live: "●",
  running: "●",
  idle: "·",
  restack: "⟳",
  dirty: "⚠",
  stale: "↓",
  overlap: "⧉",
  selected: "✓",
  focusBar: "▎",
  filterMarker: "▸",
  arrow: "›",
  new: "＋",
  comment: "✎",
} as const;

// ── Result-overlay outcome styling (D2) ──────────────────────────────────────
//
// A finished action reports one of three outcomes. A refusal / guard that
// blocked with no error is a neutral **warn** (amber ⚠) — never dressed as a
// green success ✓, and never the red error ✗.

export type ResultStatus = "success" | "warn" | "failure";

export function resultStatusStyle(status: ResultStatus): { color: string; glyph: string } {
  switch (status) {
    case "success":
      return { color: theme.good, glyph: glyph.inReview };
    case "warn":
      return { color: theme.attn, glyph: glyph.dirty };
    case "failure":
      return { color: theme.bad, glyph: glyph.changes };
  }
}

// ── Attention model — drives the "left edge" ─────────────────────────────────
//
// Collapses work-state + session status into one of four levels. See spec §2.
// Levels: needs-you (3) > active (2) > info (1) > idle (0).

export type AttentionLevel = 0 | 1 | 2 | 3;

export interface Attention {
  level: AttentionLevel;
  color: string;
  glyph: string;
  bold: boolean;
}

function changesRequested(view: SliceView): boolean {
  return !!view.prs && view.prs.some((p) => p.review_decision === "CHANGES_REQUESTED");
}

function ciFailing(view: SliceView): boolean {
  return (
    !!view.prs &&
    view.prs.some((p) => p.ci === "fail" || (p.ci_fail ?? 0) > 0)
  );
}

function allPrsMerged(view: SliceView): boolean {
  const withPr = (view.prs ?? []).filter((p) => p.number !== undefined);
  return withPr.length > 0 && withPr.every((p) => p.state === "MERGED");
}

function anyOpenPr(view: SliceView): boolean {
  return !!view.prs && view.prs.some((p) => p.state === "OPEN");
}

export function attention(view: SliceView): Attention {
  // 3 — needs you
  if (view.status === "waiting-input")
    return { level: 3, color: theme.attn, glyph: glyph.waiting, bold: true };
  if (changesRequested(view))
    return { level: 3, color: theme.bad, glyph: glyph.changes, bold: true };
  if (ciFailing(view))
    return { level: 3, color: theme.bad, glyph: glyph.ciFail, bold: true };

  // 2 — active
  if (view.slice.active)
    return { level: 2, color: theme.good, glyph: glyph.live, bold: true };
  if (view.status === "running")
    return { level: 2, color: theme.good, glyph: glyph.running, bold: true };

  // 1 — info
  if (view.status === "done")
    return { level: 1, color: theme.merged, glyph: glyph.done, bold: false };
  if (allPrsMerged(view))
    return { level: 1, color: theme.good, glyph: glyph.ready, bold: false };
  if (anyOpenPr(view))
    return { level: 1, color: theme.focus, glyph: glyph.inReview, bold: false };

  // 0 — idle
  return { level: 0, color: theme.textDim, glyph: glyph.idle, bold: false };
}

// ── Badges — small state tokens (glyph + label in one semantic hue) ──────────

export type BadgeState =
  | "live"
  | "running"
  | "waiting"
  | "done"
  | "dirty"
  | "stale"
  | "restack"
  | "ready"
  | "ci-pass"
  | "ci-fail"
  | "ci-pending"
  | "approved"
  | "changes"
  | "merged"
  | "idle";

export interface BadgeSpec {
  glyph: string;
  color: string;
  label: string;
}

export function badgeFor(state: BadgeState): BadgeSpec {
  switch (state) {
    case "live":
      return { glyph: glyph.live, color: theme.good, label: "live" };
    case "running":
      return { glyph: glyph.running, color: theme.good, label: "running" };
    case "waiting":
      return { glyph: glyph.waiting, color: theme.attn, label: "waiting" };
    case "done":
      return { glyph: glyph.done, color: theme.merged, label: "done" };
    case "dirty":
      return { glyph: glyph.dirty, color: theme.attn, label: "dirty" };
    case "stale":
      return { glyph: glyph.stale, color: theme.attn, label: "stale" };
    case "restack":
      return { glyph: glyph.restack, color: theme.attn, label: "restack" };
    case "ready":
      return { glyph: glyph.ready, color: theme.good, label: "ready" };
    case "ci-pass":
      return { glyph: glyph.ciPass, color: theme.good, label: "ci" };
    case "ci-fail":
      return { glyph: glyph.ciFail, color: theme.bad, label: "ci" };
    case "ci-pending":
      return { glyph: glyph.ciPending, color: theme.attn, label: "ci" };
    case "approved":
      return { glyph: glyph.inReview, color: theme.good, label: "approved" };
    case "changes":
      return { glyph: glyph.changes, color: theme.bad, label: "changes" };
    case "merged":
      return { glyph: glyph.done, color: theme.merged, label: "merged" };
    case "idle":
    default:
      return { glyph: glyph.idle, color: theme.textDim, label: "idle" };
  }
}

// ── Session badge / label (re-pointed to the five-hue system) ────────────────

export function sessionBadge(status: SessionStatus): { glyph: string; color: string } {
  switch (status) {
    case "waiting-input":
      return { glyph: glyph.waiting, color: theme.attn };
    case "running":
      return { glyph: glyph.running, color: theme.good };
    case "done":
      return { glyph: glyph.done, color: theme.merged };
    case "none":
    default:
      return { glyph: "○", color: theme.textDim };
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
