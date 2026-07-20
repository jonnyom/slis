import { describe, expect, test } from "bun:test";
import { tickPlan } from "./tick";

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
