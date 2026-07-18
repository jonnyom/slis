import { describe, expect, test } from "bun:test";
import type { Slice } from "../rpc/types";
import type { SliceView } from "./derive";
import {
  attentionIndices,
  buildRows,
  clampFocus,
  clusterByStack,
  missingSliceNames,
  nextAttentionIndex,
  nextAttentionRow,
  selectableIndices,
  stackRootOf,
  stepSelectable,
  type BrowserRow,
} from "./cluster";

function view(
  name: string,
  extra: Partial<Slice> = {},
  vextra: Partial<SliceView> = {},
): SliceView {
  return {
    slice: { name, base: "", active: false, stale: false, members: [], ...extra },
    status: "none",
    ...vextra,
  };
}

describe("stackRootOf", () => {
  test("splits on the NUL separator", () => {
    expect(stackRootOf("web\x00jonny/checkout-base")).toBe("jonny/checkout-base");
  });
  test("empty / missing / no-separator → empty string", () => {
    expect(stackRootOf(undefined)).toBe("");
    expect(stackRootOf("")).toBe("");
    expect(stackRootOf("nosep")).toBe("");
  });
});

describe("clusterByStack", () => {
  test("groups stack siblings adjacently, trunk-first by stack_order", () => {
    const views = [
      view("a"), // solo, no stack
      view("child", { stack_id: "web\x00root", stack_order: 2 }),
      view("b"), // solo
      view("parent", { stack_id: "web\x00root", stack_order: 1 }),
    ];
    const { ordered, leaders } = clusterByStack(views);
    // The group leads at its first-appearance position (where `child` was), and
    // within the group the lower stack_order (parent) comes first.
    expect(ordered.map((v) => v.slice.name)).toEqual(["a", "parent", "child", "b"]);
    expect(leaders.get("parent")).toEqual({ root: "root", count: 2 });
    expect(leaders.has("child")).toBe(false);
  });

  test("solo slices carry no header and keep order", () => {
    const views = [view("x"), view("y")];
    const { ordered, leaders } = clusterByStack(views);
    expect(ordered.map((v) => v.slice.name)).toEqual(["x", "y"]);
    expect(leaders.size).toBe(0);
  });

  test("distinct stack ids (different repos) do not merge", () => {
    const views = [
      view("p", { stack_id: "web\x00r", stack_order: 1 }),
      view("q", { stack_id: "api\x00r", stack_order: 1 }),
    ];
    const { leaders } = clusterByStack(views);
    expect(leaders.size).toBe(0); // each id has a single member
  });
});

describe("attention navigation", () => {
  const views = [
    view("calm"), // rank 99
    view("waiting", {}, { status: "waiting-input" }), // rank 0
    view("also-calm"),
    view("finished", {}, { status: "done" }), // rank 2
  ];

  test("attentionIndices finds only slices needing you", () => {
    expect(attentionIndices(views)).toEqual([1, 3]);
  });

  test("n wraps forward across attention slices", () => {
    expect(nextAttentionIndex(views, 1, 1)).toBe(3);
    expect(nextAttentionIndex(views, 3, 1)).toBe(1); // wrap
  });

  test("N wraps backward", () => {
    expect(nextAttentionIndex(views, 3, -1)).toBe(1);
    expect(nextAttentionIndex(views, 1, -1)).toBe(3); // wrap
  });

  test("off an attention slice: n→first, N→last", () => {
    expect(nextAttentionIndex(views, 0, 1)).toBe(1);
    expect(nextAttentionIndex(views, 0, -1)).toBe(3);
  });

  test("nothing needs attention → null", () => {
    expect(nextAttentionIndex([view("a"), view("b")], 0, 1)).toBeNull();
  });
});

describe("missingSliceNames", () => {
  test("dedupes by slice name, preserves first-seen order", () => {
    expect(
      missingSliceNames([
        { slice: "b" },
        { slice: "a" },
        { slice: "b" },
      ]),
    ).toEqual(["b", "a"]);
  });
  test("undefined → empty", () => {
    expect(missingSliceNames(undefined)).toEqual([]);
  });
});

describe("browser rows (missing-row navigation skip)", () => {
  const rows: BrowserRow[] = buildRows([view("a"), view("b")], ["gone", "vanished"]);

  test("missing rows are appended and non-selectable", () => {
    expect(rows.map((r) => r.kind)).toEqual(["slice", "slice", "missing", "missing"]);
    expect(selectableIndices(rows)).toEqual([0, 1]);
  });

  test("stepSelectable never lands on a missing row and clamps", () => {
    expect(stepSelectable(rows, 0, 1)).toBe(1);
    expect(stepSelectable(rows, 1, 1)).toBe(1); // clamps at last slice, skips missing
    expect(stepSelectable(rows, 1, -1)).toBe(0);
    expect(stepSelectable(rows, 0, -1)).toBe(0);
  });

  test("stepSelectable from a missing row snaps back to a slice", () => {
    expect(stepSelectable(rows, 2, -1)).toBe(1);
    expect(stepSelectable(rows, 3, 1)).toBe(1);
  });

  test("clampFocus pulls a missing/out-of-range index onto a slice", () => {
    expect(clampFocus(rows, 2)).toBe(1);
    expect(clampFocus(rows, 99)).toBe(1);
    expect(clampFocus(rows, 0)).toBe(0);
  });

  test("interleaved missing row is skipped", () => {
    const mixed: BrowserRow[] = [
      { kind: "slice", view: view("a") },
      { kind: "missing", name: "gap" },
      { kind: "slice", view: view("b") },
    ];
    expect(stepSelectable(mixed, 0, 1)).toBe(2);
    expect(stepSelectable(mixed, 2, -1)).toBe(0);
  });

  test("nextAttentionRow wraps over slice rows only", () => {
    const attn: BrowserRow[] = buildRows(
      [view("calm"), view("wait", {}, { status: "waiting-input" })],
      ["gone"],
    );
    expect(nextAttentionRow(attn, 0, 1)).toBe(1);
    expect(nextAttentionRow(attn, 1, 1)).toBe(1); // single attention row, wraps to itself
    expect(nextAttentionRow(buildRows([view("calm")], []), 0, 1)).toBeNull();
  });

  test("all-missing list has no selectable rows", () => {
    const allMissing = buildRows([], ["x", "y"]);
    expect(selectableIndices(allMissing)).toEqual([]);
    expect(stepSelectable(allMissing, 0, 1)).toBe(0);
    expect(clampFocus(allMissing, 1)).toBe(0);
  });
});
