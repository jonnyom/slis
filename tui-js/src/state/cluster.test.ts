import { describe, expect, test } from "bun:test";
import type { Slice } from "../rpc/types";
import type { SliceView } from "./derive";
import {
  attentionIndices,
  clusterByStack,
  nextAttentionIndex,
  stackRootOf,
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
