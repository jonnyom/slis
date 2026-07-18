import { describe, expect, test } from "bun:test";
import type { ProcEntry } from "../rpc/types";
import { nextSort, sortProcs } from "./sort";

const P = (pid: number, cpu: number, mem: number): ProcEntry => ({
  pid,
  ppid: 1,
  cmd: `p${pid}`,
  cpu,
  mem,
});

const procs = [P(30, 10, 500), P(10, 90, 100), P(20, 90, 300)];

describe("sortProcs", () => {
  test("cpu desc, pid tiebreak", () => {
    expect(sortProcs(procs, "cpu").map((p) => p.pid)).toEqual([10, 20, 30]);
  });

  test("mem desc, pid tiebreak", () => {
    expect(sortProcs(procs, "mem").map((p) => p.pid)).toEqual([30, 20, 10]);
  });

  test("pid asc", () => {
    expect(sortProcs(procs, "pid").map((p) => p.pid)).toEqual([10, 20, 30]);
  });

  test("does not mutate the input", () => {
    const copy = [...procs];
    sortProcs(procs, "mem");
    expect(procs).toEqual(copy);
  });
});

describe("nextSort", () => {
  test("cycles cpu → mem → pid → cpu", () => {
    expect(nextSort("cpu")).toBe("mem");
    expect(nextSort("mem")).toBe("pid");
    expect(nextSort("pid")).toBe("cpu");
  });
});
