// Pair adjacent deleted/added lines within a hunk so the differ can run a
// word-level diff on the pairs that most likely represent an in-place edit. A
// maximal run of deletions immediately followed by a run of additions is
// paired positionally (index 0 with 0, 1 with 1, …); surplus lines on either
// side are left unpaired. Pure and total.

import type { DiffLine } from "./parse";

export interface LinePairing {
  oldIndex: number; // index into the hunk's lines array of the deleted line
  newIndex: number; // index of the added line it pairs with
}

export function pairChangedLines(lines: DiffLine[]): LinePairing[] {
  const pairs: LinePairing[] = [];
  let i = 0;
  const n = lines.length;
  while (i < n) {
    if (lines[i]!.type !== "del") {
      i++;
      continue;
    }
    const delStart = i;
    while (i < n && lines[i]!.type === "del") i++;
    const delEnd = i;
    const addStart = i;
    while (i < n && lines[i]!.type === "add") i++;
    const addEnd = i;

    const count = Math.min(delEnd - delStart, addEnd - addStart);
    for (let k = 0; k < count; k++) {
      pairs.push({ oldIndex: delStart + k, newIndex: addStart + k });
    }
  }
  return pairs;
}

/** Convenience: a Map from a line's index to its counterpart's index. */
export function pairingMap(lines: DiffLine[]): Map<number, number> {
  const map = new Map<number, number>();
  for (const p of pairChangedLines(lines)) {
    map.set(p.oldIndex, p.newIndex);
    map.set(p.newIndex, p.oldIndex);
  }
  return map;
}
