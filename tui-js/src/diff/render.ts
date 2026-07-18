// Merge a line's syntax tokens with its word-level change segments into a flat
// list of cells the view paints directly. Each cell carries a syntax `kind`
// (→ foreground colour) and a `changed` flag (→ background highlight). Pure and
// total; the view owns the colour mapping so this stays theme-agnostic and
// unit-testable.

import { tokenizeLine, type Lang, type TokenKind } from "./tokenize";
import type { WordSegment } from "./words";

export interface Cell {
  text: string;
  kind: TokenKind;
  changed: boolean;
}

function changedMask(length: number, segments: WordSegment[] | undefined): boolean[] {
  const mask = new Array<boolean>(length).fill(false);
  if (!segments) return mask;
  let offset = 0;
  for (const seg of segments) {
    const end = Math.min(length, offset + seg.text.length);
    if (seg.changed) {
      for (let k = offset; k < end; k++) mask[k] = true;
    }
    offset = end;
  }
  return mask;
}

/**
 * Style one line: tokenize for syntax colour, then split tokens at word-diff
 * change boundaries so a single cell is uniform in both colour and highlight.
 */
export function styleLine(
  content: string,
  lang: Lang,
  changed?: WordSegment[],
): Cell[] {
  const mask = changedMask(content.length, changed);
  const tokens = tokenizeLine(content, lang);
  const cells: Cell[] = [];
  let offset = 0;
  for (const token of tokens) {
    let runStart = 0;
    for (let k = 1; k <= token.text.length; k++) {
      const boundary =
        k === token.text.length || mask[offset + k] !== mask[offset + runStart];
      if (boundary) {
        cells.push({
          text: token.text.slice(runStart, k),
          kind: token.kind,
          changed: mask[offset + runStart] ?? false,
        });
        runStart = k;
      }
    }
    offset += token.text.length;
  }
  return cells;
}
