// Word-level intra-line diff. Pure and total: given the old and new text of a
// changed line pair, it returns per-side segments flagged changed / unchanged
// so the differ can background-highlight only the words that actually differ.
//
// The algorithm is a classic LCS over word tokens (identifiers, whitespace
// runs, and single punctuation chars). For pathological line pairs the O(n·m)
// matrix is capped — beyond the cap the whole line is reported changed rather
// than burning time on a keystroke.

export interface WordSegment {
  text: string;
  changed: boolean;
}

export interface WordDiffResult {
  old: WordSegment[];
  new: WordSegment[];
}

const TOKEN_RE = /(\s+|[A-Za-z0-9_$]+|[^\sA-Za-z0-9_$])/g;

// Above this token-count product the LCS matrix is too big to build per
// keystroke; fall back to "whole line changed".
const LCS_CELL_CAP = 40000;

export function tokenizeWords(text: string): string[] {
  return text.match(TOKEN_RE) ?? [];
}

function mergeSegments(tokens: string[], changed: boolean[]): WordSegment[] {
  const segments: WordSegment[] = [];
  for (let i = 0; i < tokens.length; i++) {
    const flag = changed[i]!;
    const last = segments[segments.length - 1];
    if (last && last.changed === flag) last.text += tokens[i]!;
    else segments.push({ text: tokens[i]!, changed: flag });
  }
  return segments;
}

function wholeLine(text: string, changed: boolean): WordSegment[] {
  return text === "" ? [] : [{ text, changed }];
}

/**
 * Diff two lines at word granularity. Common tokens are reported unchanged on
 * both sides; tokens present only in the old line are changed there, tokens
 * only in the new line are changed there.
 */
export function wordDiff(oldLine: string, newLine: string): WordDiffResult {
  if (oldLine === newLine) {
    return { old: wholeLine(oldLine, false), new: wholeLine(newLine, false) };
  }
  const a = tokenizeWords(oldLine);
  const b = tokenizeWords(newLine);

  if (a.length * b.length > LCS_CELL_CAP) {
    return { old: wholeLine(oldLine, true), new: wholeLine(newLine, true) };
  }

  const n = a.length;
  const m = b.length;
  // dp[i][j] = LCS length of a[i..] and b[j..].
  const dp: number[][] = Array.from({ length: n + 1 }, () =>
    new Array<number>(m + 1).fill(0),
  );
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i]![j] = a[i] === b[j]
        ? dp[i + 1]![j + 1]! + 1
        : Math.max(dp[i + 1]![j]!, dp[i]![j + 1]!);
    }
  }

  const aChanged = new Array<boolean>(n).fill(false);
  const bChanged = new Array<boolean>(m).fill(false);
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      i++;
      j++;
    } else if (dp[i + 1]![j]! >= dp[i]![j + 1]!) {
      aChanged[i++] = true;
    } else {
      bChanged[j++] = true;
    }
  }
  while (i < n) aChanged[i++] = true;
  while (j < m) bChanged[j++] = true;

  return { old: mergeSegments(a, aChanged), new: mergeSegments(b, bChanged) };
}
