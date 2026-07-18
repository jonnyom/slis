// Pure helpers for the inline-review loop (F2). Kept React-free and total so the
// comment-context extraction, the gutter-marker matching, and the review-overlay
// list state are all unit-testable without booting OpenTUI.

import type { FileDiff } from "../diff/parse";
import type { ReviewComment } from "../rpc/types";

export type DiffSide = "old" | "new";

// The context a composed comment carries. `line` is a 1-based line in the NEW
// (post-change) file — the same anchor `slis review add --line` expects and the
// same number the gutter marker matches on.
export interface CommentContext {
  slice: string;
  repo: string;
  branch: string;
  file: string;
  line: number;
  endLine?: number;
  side?: DiffSide;
  hunk: string;
}

// The anchor + excerpt a comment on a diff HUNK captures. `line` is the hunk's
// first added line (else its first context line, else the hunk's new start) so
// the stored comment lands on a line that actually renders in the new file —
// which is also where the gutter marker then shows.
export interface HunkComment {
  line: number;
  endLine?: number;
  hunk: string;
}

function markerFor(type: string): string {
  return type === "add" ? "+" : type === "del" ? "-" : " ";
}

// hunkComment extracts the comment anchor + a short excerpt from one parsed hunk.
// The excerpt is a window of at most `radius*2+1` lines centred on the anchor,
// each prefixed with its +/-/space marker, so the agent sees the change in
// context. Returns null when the file/hunk index is out of range.
export function hunkComment(file: FileDiff, hunkIndex: number, radius = 3): HunkComment | null {
  const hunk = file.hunks[hunkIndex];
  if (!hunk) return null;

  const lines = hunk.lines;
  let anchorIdx = lines.findIndex((l) => l.type === "add");
  if (anchorIdx < 0) anchorIdx = lines.findIndex((l) => l.type === "context");
  if (anchorIdx < 0) anchorIdx = 0;

  const anchor = lines[anchorIdx];
  const line = anchor?.newNumber ?? anchor?.oldNumber ?? hunk.newStart;

  const start = Math.max(0, anchorIdx - radius);
  const end = Math.min(lines.length, anchorIdx + radius + 1);
  const excerpt = lines
    .slice(start, end)
    .map((l) => markerFor(l.type) + l.content)
    .join("\n");

  return { line, hunk: excerpt };
}

// diffRangeComment captures exactly the selected range on either side of the
// patch. Old-side ranges let split view review deletion-only lines; DiffView
// keeps every selection inside one side of one hunk.
export function diffRangeComment(
  file: FileDiff,
  hunkIndex: number,
  fromLine: number,
  toLine: number,
  side: DiffSide = "new",
): HunkComment | null {
  const hunk = file.hunks[hunkIndex];
  if (!hunk) return null;

  const line = Math.min(fromLine, toLine);
  const endLine = Math.max(fromLine, toLine);
  const selected = hunk.lines.filter(
    (l) => {
      const n = side === "old" ? l.oldNumber : l.newNumber;
      return n !== undefined && n >= line && n <= endLine;
    },
  );
  if (selected.length === 0) return null;

  return {
    line,
    endLine: endLine > line ? endLine : undefined,
    hunk: selected.map((l) => markerFor(l.type) + l.content).join("\n"),
  };
}

// fileComment extracts the comment anchor + excerpt for a cursor line in a file
// viewed at a revision (F3 file view). `cursor` is 0-based; the returned line is
// 1-based. The excerpt is the surrounding source lines (no diff markers).
export function fileComment(lines: string[], cursor: number, radius = 3): HunkComment {
  const idx = Math.max(0, Math.min(cursor, Math.max(0, lines.length - 1)));
  const start = Math.max(0, idx - radius);
  const end = Math.min(lines.length, idx + radius + 1);
  return { line: idx + 1, hunk: lines.slice(start, end).join("\n") };
}

// linesWithComments returns the set of NEW-file line numbers in one repo+file
// that carry a pending review comment — the gutter-marker match set. Kept as a
// plain Set so the row renderers do an O(1) `has(newNumber)` per line.
export function linesWithComments(
  comments: ReviewComment[],
  repo: string,
  file: string,
  side: DiffSide = "new",
): Set<number> {
  const out = new Set<number>();
  for (const c of comments) {
    if (c.repo === repo && c.file === file && (c.side ?? "new") === side) {
      const end = Math.max(c.line, c.end_line ?? c.line);
      for (let line = c.line; line <= end; line++) out.add(line);
    }
  }
  return out;
}

// clampReviewSel keeps a review-overlay list selection inside [0, len-1] (or 0
// for an empty list), matching the clamp used by the other list overlays.
export function clampReviewSel(sel: number, len: number): number {
  if (len <= 0) return 0;
  return Math.max(0, Math.min(sel, len - 1));
}
