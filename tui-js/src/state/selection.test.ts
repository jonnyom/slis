import { describe, expect, test } from "bun:test";
import {
  matchesSearch,
  pruneSelection,
  toggleAllVisible,
  toggleSelected,
  visibleSelection,
} from "./selection";

describe("toggleSelected", () => {
  test("adds an unselected name", () => {
    expect([...toggleSelected(new Set(), "a")]).toEqual(["a"]);
  });
  test("removes an already-selected name", () => {
    expect([...toggleSelected(new Set(["a", "b"]), "a")]).toEqual(["b"]);
  });
  test("does not mutate the input set", () => {
    const input = new Set(["a"]);
    toggleSelected(input, "b");
    expect([...input]).toEqual(["a"]);
  });
});

describe("toggleAllVisible", () => {
  test("selects all when none are selected", () => {
    expect([...toggleAllVisible(new Set(), ["a", "b"])]).toEqual(["a", "b"]);
  });
  test("selects all when only some are selected", () => {
    expect([...toggleAllVisible(new Set(["a"]), ["a", "b"])].sort()).toEqual(["a", "b"]);
  });
  test("clears the visible set when all are already selected", () => {
    expect([...toggleAllVisible(new Set(["a", "b", "c"]), ["a", "b"])]).toEqual(["c"]);
  });
  test("empty visible set is a no-op", () => {
    expect([...toggleAllVisible(new Set(["a"]), [])]).toEqual(["a"]);
  });
});

describe("visibleSelection", () => {
  test("drops selected names hidden by a search or filter", () => {
    expect(visibleSelection(new Set(["a", "b", "c"]), ["c", "d"])).toEqual(["c"]);
  });
  test("follows on-screen order", () => {
    expect(visibleSelection(new Set(["b", "a"]), ["a", "b"])).toEqual(["a", "b"]);
  });
});

describe("pruneSelection", () => {
  test("drops names that no longer exist", () => {
    expect([...pruneSelection(new Set(["a", "gone"]), ["a", "b"])]).toEqual(["a"]);
  });
  test("returns the same set when nothing changed", () => {
    const input = new Set(["a"]);
    expect(pruneSelection(input, ["a", "b"])).toBe(input);
  });
});

describe("matchesSearch", () => {
  test("empty query matches everything", () => {
    expect(matchesSearch("checkout", "")).toBe(true);
  });
  test("case-insensitive substring match", () => {
    expect(matchesSearch("Checkout-Flow", "out")).toBe(true);
    expect(matchesSearch("Checkout-Flow", "OUT")).toBe(true);
  });
  test("non-match returns false", () => {
    expect(matchesSearch("payments", "refund")).toBe(false);
  });
});
