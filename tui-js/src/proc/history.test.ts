import { describe, expect, test } from "bun:test";
import { ProcHistory } from "./history";

describe("ProcHistory", () => {
  test("accumulates a series per pid across samples", () => {
    const h = new ProcHistory(10);
    h.sample([{ pid: 1, cpu: 10, mem: 100 }]);
    h.sample([{ pid: 1, cpu: 20, mem: 110 }]);
    expect(h.cpuSeries(1)).toEqual([10, 20]);
    expect(h.memSeries(1)).toEqual([100, 110]);
  });

  test("drops pids that vanish from a later sample", () => {
    const h = new ProcHistory(10);
    h.sample([
      { pid: 1, cpu: 1, mem: 1 },
      { pid: 2, cpu: 2, mem: 2 },
    ]);
    h.sample([{ pid: 1, cpu: 3, mem: 3 }]);
    expect(h.trackedPids).toEqual([1]);
    expect(h.cpuSeries(2)).toEqual([]);
  });

  test("respects the ring capacity", () => {
    const h = new ProcHistory(3);
    for (let i = 1; i <= 5; i++) h.sample([{ pid: 1, cpu: i, mem: i }]);
    expect(h.cpuSeries(1)).toEqual([3, 4, 5]);
  });

  test("unknown pid has empty series", () => {
    const h = new ProcHistory();
    expect(h.cpuSeries(42)).toEqual([]);
  });
});
