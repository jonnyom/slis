import { describe, expect, test } from "bun:test";
import { BULK_LOAD_THRESHOLD, bulkLoadPlan } from "./bulkload";

describe("bulkLoadPlan", () => {
  test("under threshold always fans out, never prompts", () => {
    expect(bulkLoadPlan(BULK_LOAD_THRESHOLD, "unprompted")).toEqual({
      prompt: false,
      fanOut: true,
    });
    expect(bulkLoadPlan(3, "unprompted")).toEqual({ prompt: false, fanOut: true });
  });

  test("over threshold + unprompted → prompt, no fan-out", () => {
    expect(bulkLoadPlan(BULK_LOAD_THRESHOLD + 1, "unprompted")).toEqual({
      prompt: true,
      fanOut: false,
    });
  });

  test("over threshold + all → fan out, no prompt", () => {
    expect(bulkLoadPlan(200, "all")).toEqual({ prompt: false, fanOut: true });
  });

  test("over threshold + lazy → neither prompt nor fan-out", () => {
    expect(bulkLoadPlan(200, "lazy")).toEqual({ prompt: false, fanOut: false });
  });
});
