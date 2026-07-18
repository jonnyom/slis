import { describe, expect, test } from "bun:test";
import { clampScroll, maxScroll } from "./scroll";

describe("maxScroll", () => {
  test("content shorter than viewport → 0", () => {
    expect(maxScroll(3, 10)).toBe(0);
  });
  test("content taller than viewport → overflow", () => {
    expect(maxScroll(30, 10)).toBe(20);
  });
});

describe("clampScroll", () => {
  test("clamps to [0, maxScroll]", () => {
    expect(clampScroll(-5, 30, 10)).toBe(0);
    expect(clampScroll(5, 30, 10)).toBe(5);
    expect(clampScroll(999, 30, 10)).toBe(20);
  });
  test("no overflow pins to 0", () => {
    expect(clampScroll(4, 5, 10)).toBe(0);
  });
});
