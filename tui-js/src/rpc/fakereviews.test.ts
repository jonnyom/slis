import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { fakeReviewsList, resetFakeReviews } from "./fakereviews";
import { reviewAdd, reviewRm, reviewSend } from "./mutate";

// The fake mutations gate on SLIS_FAKE at call time; drive the whole loop
// against the shared in-memory store the fake RPC client reads from.
const prev = process.env["SLIS_FAKE"];

beforeEach(() => {
  process.env["SLIS_FAKE"] = "1";
  resetFakeReviews();
});
afterEach(() => {
  if (prev === undefined) delete process.env["SLIS_FAKE"];
  else process.env["SLIS_FAKE"] = prev;
});

describe("fake review loop", () => {
  test("seeds one comment on checkout", () => {
    expect(fakeReviewsList("checkout").length).toBe(1);
    expect(fakeReviewsList("payments").length).toBe(0);
  });

  test("add appends a comment visible to the list", async () => {
    const res = await reviewAdd({
      slice: "checkout",
      repo: "web",
      branch: "jonny/checkout",
      file: "src/checkout/totals.ts",
      line: 3,
      body: "guard against NaN",
    });
    expect(res.code).toBe(0);
    const list = fakeReviewsList("checkout");
    expect(list.length).toBe(2);
    expect(list.some((c) => c.file === "src/checkout/totals.ts" && c.line === 3)).toBe(true);
  });

  test("rm removes by id", async () => {
    const seeded = fakeReviewsList("checkout")[0]!;
    const res = await reviewRm("checkout", seeded.id);
    expect(res.code).toBe(0);
    expect(fakeReviewsList("checkout").length).toBe(0);
  });

  test("send clears the slice's batch and reports the count", async () => {
    await reviewAdd({ slice: "checkout", repo: "web", file: "a.ts", line: 1, body: "one" });
    expect(fakeReviewsList("checkout").length).toBe(2);
    const res = await reviewSend("checkout");
    expect(res.code).toBe(0);
    expect(res.stdout).toContain("delivered 2");
    expect(fakeReviewsList("checkout").length).toBe(0);
  });

  test("send with nothing pending is a non-zero no-op", async () => {
    const res = await reviewSend("payments");
    expect(res.code).not.toBe(0);
    expect(res.stderr).toContain("no pending review comments");
  });
});
