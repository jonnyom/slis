import { describe, expect, test } from "bun:test";
import { sparkline } from "./sparkline";

describe("sparkline", () => {
  test("empty input is an empty string", () => {
    expect(sparkline([])).toBe("");
  });

  test("a flat series renders the floor tick", () => {
    expect(sparkline([5, 5, 5])).toBe("▁▁▁");
  });

  test("maps low → high across the 5-level ramp on a 0..max scale", () => {
    // max = 100, so 0→▁, 100→▇, 50→middle.
    const s = sparkline([0, 25, 50, 75, 100]);
    expect(s.length).toBe(5);
    expect(s[0]).toBe("▁");
    expect(s[4]).toBe("▇");
  });

  test("width keeps only the most recent samples", () => {
    const s = sparkline([0, 0, 0, 100], { width: 2, max: 100 });
    expect(s).toBe("▁▇");
  });

  test("clamps out-of-range and non-finite samples", () => {
    const s = sparkline([-10, Number.NaN, 200], { min: 0, max: 100 });
    // NaN dropped; -10 clamps to floor, 200 clamps to ceiling.
    expect(s).toBe("▁▇");
  });
});
