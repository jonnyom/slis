import { describe, expect, test } from "bun:test";
import { parseUnifiedDiff } from "../diff/parse";
import type { ReviewComment } from "../rpc/types";
import {
  clampReviewSel,
  diffRangeComment,
  fileComment,
  hunkComment,
  linesWithComments,
} from "./context";

const CART_PATCH = [
  "diff --git a/src/checkout/cart.tsx b/src/checkout/cart.tsx",
  "--- a/src/checkout/cart.tsx",
  "+++ b/src/checkout/cart.tsx",
  "@@ -10,7 +10,9 @@ export function Cart() {",
  "   const items = useCart();",
  "-  return <List items={items} />;",
  "+  const totals = useTotals(items);",
  "+  return <List items={items} totals={totals} />;",
  " }",
].join("\n");

function cartFile() {
  return parseUnifiedDiff(CART_PATCH)[0]!;
}

describe("hunkComment", () => {
  test("anchors on the hunk's first added line (new-file number)", () => {
    const hc = hunkComment(cartFile(), 0);
    // hunk newStart=10: context 10, del, add 11, add 12, context 13.
    // first added line is new-line 11.
    expect(hc?.line).toBe(11);
  });

  test("excerpt carries diff markers + surrounding lines", () => {
    const hc = hunkComment(cartFile(), 0, 1);
    expect(hc?.hunk).toContain("+  const totals = useTotals(items);");
    // radius 1 around the anchor pulls in the preceding deletion.
    expect(hc?.hunk).toContain("-  return <List items={items} />;");
  });

  test("falls back to a context line when a hunk has no additions", () => {
    const patch = [
      "diff --git a/f.txt b/f.txt",
      "--- a/f.txt",
      "+++ b/f.txt",
      "@@ -5,3 +5,2 @@",
      " keep me",
      "-drop me",
      " keep me too",
    ].join("\n");
    const file = parseUnifiedDiff(patch)[0]!;
    const hc = hunkComment(file, 0);
    expect(hc?.line).toBe(5); // first context line's new number
  });

  test("returns null for an out-of-range hunk index", () => {
    expect(hunkComment(cartFile(), 9)).toBeNull();
  });
});

describe("diffRangeComment", () => {
  test("captures the exact selected new-file lines and their range", () => {
    const selected = diffRangeComment(cartFile(), 0, 11, 12);
    expect(selected).toEqual({
      line: 11,
      endLine: 12,
      hunk:
        "+  const totals = useTotals(items);\n" +
        "+  return <List items={items} totals={totals} />;",
    });
  });

  test("a single selected line omits endLine", () => {
    expect(diffRangeComment(cartFile(), 0, 12, 12)).toEqual({
      line: 12,
      endLine: undefined,
      hunk: "+  return <List items={items} totals={totals} />;",
    });
  });

  test("returns null for a missing hunk", () => {
    expect(diffRangeComment(cartFile(), 9, 11, 12)).toBeNull();
  });

  test("captures deletion-only lines from the old side", () => {
    const selected = diffRangeComment(cartFile(), 0, 11, 11, "old");
    expect(selected).toEqual({
      line: 11,
      endLine: undefined,
      hunk: "-  return <List items={items} />;",
    });
  });
});

describe("fileComment", () => {
  const lines = ["one", "two", "three", "four", "five"];

  test("line is 1-based; excerpt is the surrounding source window", () => {
    const fc = fileComment(lines, 2, 1); // cursor on "three"
    expect(fc.line).toBe(3);
    expect(fc.hunk).toBe("two\nthree\nfour");
  });

  test("clamps the cursor into range", () => {
    expect(fileComment(lines, 99, 0).line).toBe(5);
    expect(fileComment(lines, -3, 0).line).toBe(1);
  });
});

describe("linesWithComments", () => {
  const comments: ReviewComment[] = [
    { id: "a", slice: "s", repo: "web", file: "cart.tsx", line: 12, body: "x", created_at: "" },
    { id: "b", slice: "s", repo: "web", file: "cart.tsx", line: 30, body: "y", created_at: "" },
    { id: "c", slice: "s", repo: "web", file: "other.ts", line: 12, body: "z", created_at: "" },
    { id: "d", slice: "s", repo: "api", file: "cart.tsx", line: 12, body: "w", created_at: "" },
  ];

  test("matches on repo + file, collecting the marked lines", () => {
    const set = linesWithComments(comments, "web", "cart.tsx");
    expect([...set].sort((a, b) => a - b)).toEqual([12, 30]);
  });

  test("does not bleed across repo or file", () => {
    expect(linesWithComments(comments, "web", "cart.tsx").has(12)).toBe(true);
    expect(linesWithComments(comments, "api", "other.ts").size).toBe(0);
  });

  test("marks every line covered by a pending range comment", () => {
    const ranged: ReviewComment = {
      id: "range",
      slice: "s",
      repo: "web",
      file: "cart.tsx",
      line: 20,
      end_line: 22,
      body: "x",
      created_at: "",
    };
    expect([...linesWithComments([ranged], "web", "cart.tsx")]).toEqual([20, 21, 22]);
  });

  test("keeps old-side and new-side gutter markers separate", () => {
    const old: ReviewComment = {
      id: "old",
      slice: "s",
      repo: "web",
      file: "cart.tsx",
      line: 11,
      side: "old",
      body: "x",
      created_at: "",
    };
    expect(linesWithComments([old], "web", "cart.tsx", "old").has(11)).toBe(true);
    expect(linesWithComments([old], "web", "cart.tsx", "new").size).toBe(0);
  });
});

describe("clampReviewSel", () => {
  test("keeps selection inside the list", () => {
    expect(clampReviewSel(5, 3)).toBe(2);
    expect(clampReviewSel(-1, 3)).toBe(0);
    expect(clampReviewSel(1, 3)).toBe(1);
  });
  test("empty list clamps to 0", () => {
    expect(clampReviewSel(4, 0)).toBe(0);
  });
});
