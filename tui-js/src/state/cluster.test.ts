import { describe, expect, test } from "bun:test";
import type { Slice } from "../rpc/types";
import type { SliceView } from "./derive";
import {
  attentionIndices,
  buildRows,
  clampFocus,
  clusterByStack,
  clusterByUnit,
  collapsedUnitOf,
  focusIndexForSlice,
  isGatherableStackSlice,
  missingSliceNames,
  nextAttentionIndex,
  nextAttentionRow,
  searchAwareNav,
  selectableIndices,
  stackRootOf,
  stepSelectable,
  unitHeaderIndex,
  unitOfRow,
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

describe("stack actions", () => {
  const views = [
    view("standalone"),
    view("parent", { stack_id: "web\x00root", stack_order: 1 }),
    view("child", { stack_id: "web\x00root", stack_order: 2 }),
  ];

  test("only enables gather for a slice with stack siblings", () => {
    expect(isGatherableStackSlice(views, "standalone")).toBe(false);
    expect(isGatherableStackSlice(views, "parent")).toBe(true);
    expect(isGatherableStackSlice(views, "child")).toBe(true);
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

  test("restores focus by slice identity after the browser remounts", () => {
    expect(focusIndexForSlice(rows, "b")).toBe(1);
    expect(focusIndexForSlice(rows, "gone")).toBe(0);
    expect(focusIndexForSlice(rows, null)).toBe(0);
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

describe("searchAwareNav (M3 — n/N modal decision)", () => {
  // calm · wait(attention) · calm2 — index 1 is the only attention row.
  const rows: BrowserRow[] = buildRows(
    [view("calm"), view("wait", {}, { status: "waiting-input" }), view("calm2")],
    [],
  );

  test("no active search → attention-hop (same as nextAttentionRow)", () => {
    expect(searchAwareNav(false, rows, 0, 1)).toBe(nextAttentionRow(rows, 0, 1));
    // from calm(0) forward lands on the single attention row (1).
    expect(searchAwareNav(false, rows, 0, 1)).toBe(1);
  });

  test("active search → step through matches (same as stepSelectable)", () => {
    // With a search filter the whole list is matches, so n/N walk row by row —
    // stepping onto calm2(2), which attention-hop would never visit.
    expect(searchAwareNav(true, rows, 1, 1)).toBe(stepSelectable(rows, 1, 1));
    expect(searchAwareNav(true, rows, 1, 1)).toBe(2);
    expect(searchAwareNav(true, rows, 2, -1)).toBe(1);
  });

  test("the two modes diverge — search stepping reaches non-attention rows", () => {
    expect(searchAwareNav(true, rows, 1, 1)).not.toBe(searchAwareNav(false, rows, 1, 1));
  });
});

describe("clusterByUnit", () => {
  test("keeps a unit together under one header, stack-ordered inside, after loose slices", () => {
    const views = [
      view("alpha"),
      view("lean", { unit: "latency", stack_id: "mw\u0000root", stack_order: 2 }),
      view("beta", { stack_id: "web\u0000b", stack_order: 1 }),
      view("latency", { unit: "latency", stack_id: "mw\u0000root", stack_order: 1 }),
      view("gamma", { stack_id: "web\u0000b", stack_order: 2 }),
    ];
    const c = clusterByUnit(views);
    expect(c.ordered.map((v) => v.slice.name)).toEqual([
      "alpha",
      "beta",
      "gamma",
      "latency",
      "lean",
    ]);
    expect(c.unitLeaders.get("latency")).toEqual({ unit: "latency", count: 2 });
    expect(c.unitLeaders.size).toBe(1);
    expect(c.leaders.get("latency")?.count).toBe(2);
    expect(c.leaders.get("beta")?.count).toBe(2);
  });
});

describe("unit rows", () => {
  // The screenshot bug: a unit followed by an unrelated slice must not read as
  // one block. The unit gets its own header row and its members are marked,
  // with the last one closing the unit, so `after` is visibly outside it.
  const views = [
    view("before"),
    view("rate", { unit: "platform" }),
    view("rate-api", { unit: "platform" }),
    view("after"),
  ];
  const c = clusterByUnit(views);
  const opts = (collapsed: string[] = [], expandAll = false) => ({
    unitLeaders: c.unitLeaders,
    collapsed: new Set(collapsed),
    expandAll,
  });
  const shape = (rows: BrowserRow[]) =>
    rows.map((r) =>
      r.kind === "unit"
        ? `unit:${r.unit}${r.collapsed ? "+" : "-"}`
        : r.kind === "slice"
          ? `${r.view.slice.name}${r.unit ? (r.unit.last ? "└" : "├") : ""}`
          : `missing:${r.name}`,
    );

  test("loose slices first, then the unit header and its members", () => {
    expect(shape(buildRows(c.ordered, [], opts()))).toEqual([
      "before",
      "after",
      "unit:platform-",
      "rate├",
      "rate-api└",
    ]);
  });

  test("units sit below loose slices and close before the next unit", () => {
    const two = clusterByUnit([
      view("rate", { unit: "platform" }),
      view("query-cost"),
      view("arabic", { unit: "ai" }),
    ]);
    const rows = buildRows(two.ordered, [], {
      unitLeaders: two.unitLeaders,
      collapsed: new Set(),
    });
    expect(shape(rows)).toEqual(["query-cost", "unit:platform-", "rate└", "unit:ai-", "arabic└"]);
  });

  test("collapsed unit hides members but keeps the header selectable", () => {
    const rows = buildRows(c.ordered, [], opts(["platform"]));
    expect(shape(rows)).toEqual(["before", "after", "unit:platform+"]);
    expect(selectableIndices(rows)).toEqual([0, 1, 2]);
    expect(stepSelectable(rows, 0, 1)).toBe(1);
    expect(collapsedUnitOf(rows, "rate-api")).toBe("platform");
    expect(collapsedUnitOf(rows, "after")).toBeNull();
  });

  test("search expands every unit so matches stay visible", () => {
    expect(shape(buildRows(c.ordered, [], opts(["platform"], true)))).toContain("rate├");
  });

  test("h from a member finds the unit and its header", () => {
    const rows = buildRows(c.ordered, [], opts());
    expect(unitOfRow(rows[4])).toBe("platform");
    expect(unitOfRow(rows[2])).toBe("platform");
    expect(unitOfRow(rows[0])).toBeNull();
    expect(unitHeaderIndex(rows, "platform")).toBe(2);
  });

  test("a collapsed unit with a waiting member is an attention stop", () => {
    const waiting = clusterByUnit([
      view("calm"),
      view("w", { unit: "u" }, { status: "waiting-input" }),
    ]);
    const rows = buildRows(waiting.ordered, [], {
      unitLeaders: waiting.unitLeaders,
      collapsed: new Set(["u"]),
    });
    expect(nextAttentionRow(rows, 0, 1)).toBe(1);
  });
});
