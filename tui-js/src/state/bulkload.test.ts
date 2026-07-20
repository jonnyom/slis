import { describe, expect, test } from "bun:test";
import { BULK_LOAD_THRESHOLD, bulkLoadPlan, loadSlicesSequentially } from "./bulkload";

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

test("loadSlicesSequentially leaves capacity between slice loads", async () => {
  const started: string[] = [];
  const releases: Array<() => void> = [];
  const load = (slice: string) => {
    started.push(slice);
    return new Promise<void>((resolve) => releases.push(resolve));
  };

  const loading = loadSlicesSequentially(["a", "b", "c"], load);
  await Promise.resolve();
  expect(started).toEqual(["a"]);

  releases.shift()?.();
  await Promise.resolve();
  expect(started).toEqual(["a", "b"]);

  releases.shift()?.();
  await Promise.resolve();
  releases.shift()?.();
  await loading;
  expect(started).toEqual(["a", "b", "c"]);
});
