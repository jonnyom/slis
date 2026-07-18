// View-ready row builders: fold parse + pair + side-by-side + word-diff +
// syntax into flat arrays of styled rows that the DiffView renders one-to-one.
// Doing the heavy lifting here (a) keeps the JSX a trivial map and (b) makes
// the whole render pipeline unit-testable without a terminal. Pure and total.
//
// Call these once per selected file (memoized in the view). They never touch
// the whole patch — only the one FileDiff handed in.

import type { DiffLine, DiffLineType, FileDiff } from "./parse";
import { pairingMap } from "./pair";
import { buildSideBySide } from "./sidebyside";
import { styleLine, type Cell } from "./render";
import { wordDiff } from "./words";
import { type Lang } from "./tokenize";

export type { Cell } from "./render";

export type UnifiedRow =
  | { kind: "hunk"; hunkIndex: number; header: string }
  | {
      kind: "line";
      hunkIndex: number;
      lineType: DiffLineType;
      oldNumber?: number;
      newNumber?: number;
      cells: Cell[];
    };

export interface SbsSide {
  lineType: DiffLineType | "blank";
  oldNumber?: number;
  newNumber?: number;
  cells: Cell[];
}

export type SbsRowR =
  | { kind: "hunk"; hunkIndex: number; header: string }
  | { kind: "line"; hunkIndex: number; left: SbsSide; right: SbsSide };

const BLANK_SIDE: SbsSide = { lineType: "blank", cells: [] };

function buildUnifiedRows(file: FileDiff, lang: Lang): UnifiedRow[] {
  const rows: UnifiedRow[] = [];
  file.hunks.forEach((hunk, hunkIndex) => {
    rows.push({ kind: "hunk", hunkIndex, header: hunk.header });
    const counterpart = pairingMap(hunk.lines);
    hunk.lines.forEach((line, idx) => {
      let cells: Cell[];
      const mate = counterpart.get(idx);
      if (mate !== undefined && (line.type === "add" || line.type === "del")) {
        const other = hunk.lines[mate]!;
        const [oldLine, newLine] =
          line.type === "del" ? [line, other] : [other, line];
        const wd = wordDiff(oldLine.content, newLine.content);
        cells = styleLine(line.content, lang, line.type === "del" ? wd.old : wd.new);
      } else {
        cells = styleLine(line.content, lang);
      }
      rows.push({
        kind: "line",
        hunkIndex,
        lineType: line.type,
        oldNumber: line.oldNumber,
        newNumber: line.newNumber,
        cells,
      });
    });
  });
  return rows;
}

function sideFor(line: DiffLine, lang: Lang): SbsSide {
  return {
    lineType: line.type,
    oldNumber: line.oldNumber,
    newNumber: line.newNumber,
    cells: styleLine(line.content, lang),
  };
}

function buildSbsRows(file: FileDiff, lang: Lang): SbsRowR[] {
  const rows: SbsRowR[] = [];
  file.hunks.forEach((hunk, hunkIndex) => {
    rows.push({ kind: "hunk", hunkIndex, header: hunk.header });
    for (const row of buildSideBySide(hunk.lines)) {
      if (row.kind === "change" && row.left && row.right) {
        const wd = wordDiff(row.left.content, row.right.content);
        rows.push({
          kind: "line",
          hunkIndex,
          left: {
            lineType: row.left.type,
            oldNumber: row.left.oldNumber,
            newNumber: row.left.newNumber,
            cells: styleLine(row.left.content, lang, wd.old),
          },
          right: {
            lineType: row.right.type,
            oldNumber: row.right.oldNumber,
            newNumber: row.right.newNumber,
            cells: styleLine(row.right.content, lang, wd.new),
          },
        });
        continue;
      }
      rows.push({
        kind: "line",
        hunkIndex,
        left: row.left ? sideFor(row.left, lang) : BLANK_SIDE,
        right: row.right ? sideFor(row.right, lang) : BLANK_SIDE,
      });
    }
  });
  return rows;
}

export interface FileRows {
  unified: UnifiedRow[];
  sideBySide: SbsRowR[];
  /** Row index of each hunk header within the unified list, for hunk jumps. */
  unifiedHunkOffsets: number[];
  /** Row index of each hunk header within the side-by-side list. */
  sbsHunkOffsets: number[];
}

function hunkOffsets(rows: Array<{ kind: string }>): number[] {
  const offsets: number[] = [];
  rows.forEach((r, i) => {
    if (r.kind === "hunk") offsets.push(i);
  });
  return offsets;
}

/** Build both render models for a file in one pass (memoize by file+lang). */
export function buildFileRows(file: FileDiff, lang: Lang): FileRows {
  const unified = buildUnifiedRows(file, lang);
  const sideBySide = buildSbsRows(file, lang);
  return {
    unified,
    sideBySide,
    unifiedHunkOffsets: hunkOffsets(unified),
    sbsHunkOffsets: hunkOffsets(sideBySide),
  };
}
