import { describe, expect, test } from "bun:test";
import { newlyDiscoveredSliceNames, shouldRefreshDiscovery, tickPlan } from "./tick";

describe("tickPlan", () => {
  test("paused (PTY tab / prompt) never runs", () => {
    expect(tickPlan({ paused: true, focusedSlice: "a" })).toEqual({
      run: false,
    });
  });

  test("refreshes only the focused slice", () => {
    expect(tickPlan({ paused: false, focusedSlice: "a" })).toEqual({
      run: true,
      slices: ["a"],
    });
  });

  test("with no focus does not run", () => {
    expect(tickPlan({ paused: false, focusedSlice: null })).toEqual({
      run: false,
    });
  });

});

describe("discovery refresh", () => {
  test("runs whenever background work is not paused", () => {
    expect(shouldRefreshDiscovery({ paused: false, focusedSlice: null })).toBe(true);
    expect(shouldRefreshDiscovery({ paused: true, focusedSlice: "a" })).toBe(false);
  });

  test("returns only slices introduced by the latest discovery result", () => {
    expect(newlyDiscoveredSliceNames(["a", "b"], ["b", "c", "d"])).toEqual(["c", "d"]);
  });
});
