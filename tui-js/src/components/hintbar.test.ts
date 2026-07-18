import { describe, expect, test } from "bun:test";
import { fitHints, type Hint } from "./hintbar";

const HINTS: Hint[] = [
  { key: "enter", label: "open" },
  { key: "a", label: "term" },
  { key: "w", label: "swap" },
  { key: "space", label: "select" },
  { key: "/", label: "search" },
];

describe("fitHints", () => {
  test("keeps every hint when width is ample", () => {
    const { shown, truncated } = fitHints(HINTS, 200);
    expect(shown).toEqual(HINTS);
    expect(truncated).toBe(false);
  });

  test("drops hints from the end when width is tight", () => {
    const { shown, truncated } = fitHints(HINTS, 30);
    expect(truncated).toBe(true);
    expect(shown.length).toBeLessThan(HINTS.length);
    // dropped from the tail — kept prefix stays in order
    expect(shown).toEqual(HINTS.slice(0, shown.length));
  });

  test("a tiny width shows no leading hints but still marks truncation", () => {
    const { shown, truncated } = fitHints(HINTS, 6);
    expect(shown).toEqual([]);
    expect(truncated).toBe(true);
  });

  test("empty hints never truncate", () => {
    expect(fitHints([], 80)).toEqual({ shown: [], truncated: false });
  });

  test("shown hints plus '? more' never exceed the width", () => {
    for (const w of [8, 12, 16, 24, 40, 60]) {
      const { shown } = fitHints(HINTS, w);
      const segs = shown.map((h) => h.key.length + 1 + h.label.length);
      const gaps = shown.length * 3; // gap before each shown hint / before more
      const more = "? more".length;
      expect(segs.reduce((a, b) => a + b, 0) + gaps + more).toBeLessThanOrEqual(w);
    }
  });
});
