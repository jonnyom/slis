import { describe, expect, test } from "bun:test";
import { RingBuffer } from "./ring";

describe("RingBuffer", () => {
  test("collects up to capacity in push order", () => {
    const r = new RingBuffer(4);
    r.push(1);
    r.push(2);
    r.push(3);
    expect(r.values()).toEqual([1, 2, 3]);
    expect(r.size).toBe(3);
    expect(r.last()).toBe(3);
  });

  test("evicts the oldest once full", () => {
    const r = new RingBuffer(3);
    for (const v of [1, 2, 3, 4, 5]) r.push(v);
    expect(r.values()).toEqual([3, 4, 5]);
    expect(r.size).toBe(3);
    expect(r.last()).toBe(5);
  });

  test("empty buffer", () => {
    const r = new RingBuffer(2);
    expect(r.values()).toEqual([]);
    expect(r.last()).toBeUndefined();
    expect(r.size).toBe(0);
  });

  test("rejects a zero capacity", () => {
    expect(() => new RingBuffer(0)).toThrow();
  });
});
