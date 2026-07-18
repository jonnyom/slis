// Turn a hunk's flat line list into aligned two-column rows for the
// side-by-side view. Context lines appear on both sides; a run of deletions
// followed by additions is aligned row-for-row (surplus lines get a blank
// filler on the empty side). Pure and total — the view maps rows to columns.

import type { DiffLine } from "./parse";

export type SbsRowKind = "context" | "change" | "del" | "add";

export interface SbsRow {
  /** The old-side line, or undefined for a blank filler cell. */
  left?: DiffLine;
  /** The new-side line, or undefined for a blank filler cell. */
  right?: DiffLine;
  kind: SbsRowKind;
}

export function buildSideBySide(lines: DiffLine[]): SbsRow[] {
  const rows: SbsRow[] = [];
  let i = 0;
  const n = lines.length;
  while (i < n) {
    const line = lines[i]!;

    if (line.type === "context" || line.type === "meta") {
      rows.push({ left: line, right: line, kind: "context" });
      i++;
      continue;
    }

    if (line.type === "del") {
      const delStart = i;
      while (i < n && lines[i]!.type === "del") i++;
      const dels = lines.slice(delStart, i);
      const addStart = i;
      while (i < n && lines[i]!.type === "add") i++;
      const adds = lines.slice(addStart, i);

      const rowCount = Math.max(dels.length, adds.length);
      for (let k = 0; k < rowCount; k++) {
        const left = dels[k];
        const right = adds[k];
        rows.push({
          left,
          right,
          kind: left && right ? "change" : left ? "del" : "add",
        });
      }
      continue;
    }

    // A run of additions with no preceding deletions.
    if (line.type === "add") {
      rows.push({ right: line, kind: "add" });
      i++;
      continue;
    }

    i++;
  }
  return rows;
}
