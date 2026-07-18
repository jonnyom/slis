import { describe, expect, test } from "bun:test";
import { tickPlan } from "./tick";

describe("tickPlan", () => {
  const slices = ["a", "b", "c"];

  test("paused (PTY tab / prompt) never runs", () => {
    expect(tickPlan({ paused: true, phase: "all", focusedSlice: "a", slices })).toEqual({
      run: false,
    });
  });

  test("all mode refreshes every slice", () => {
    expect(tickPlan({ paused: false, phase: "all", focusedSlice: "a", slices })).toEqual({
      run: true,
      slices,
    });
  });

  test("unprompted (small workspace fan-out) refreshes every slice", () => {
    expect(tickPlan({ paused: false, phase: "unprompted", focusedSlice: null, slices })).toEqual({
      run: true,
      slices,
    });
  });

  test("lazy mode refreshes only the focused slice", () => {
    expect(tickPlan({ paused: false, phase: "lazy", focusedSlice: "b", slices })).toEqual({
      run: true,
      slices: ["b"],
    });
  });

  test("lazy mode with no focus does not run", () => {
    expect(tickPlan({ paused: false, phase: "lazy", focusedSlice: null, slices })).toEqual({
      run: false,
    });
  });

  test("empty workspace does not run", () => {
    expect(tickPlan({ paused: false, phase: "all", focusedSlice: null, slices: [] })).toEqual({
      run: false,
    });
  });
});
