import { describe, expect, test } from "bun:test";
import type { ProcEntry } from "../rpc/types";
import { killProc, killSubtree } from "./kill";

const P = (pid: number, ppid: number): ProcEntry => ({
  pid,
  ppid,
  cmd: `p${pid}`,
  cpu: 0,
  mem: 0,
});

// Spawn a real, harmless child so we exercise process.kill against a live pid
// rather than mocking it (the signal path is the whole point of these tests).
function spawnSleeper(): number {
  const child = Bun.spawn({ cmd: ["sleep", "30"], stdout: "ignore", stderr: "ignore" });
  return child.pid;
}

describe("killProc", () => {
  test("signals a live process", () => {
    const pid = spawnSleeper();
    const out = killProc(pid);
    expect(out.ok).toBe(true);
    expect(out.killed).toBe(1);
  });

  test("reports ESRCH for a pid that is gone", () => {
    const out = killProc(2_000_000_000);
    expect(out.ok).toBe(false);
    expect(out.errors[0]!.kind).toBe("ESRCH");
  });
});

describe("killSubtree", () => {
  test("aggregates results across the subtree order", () => {
    // Two dead pids with a parent link: both fail ESRCH, none killed.
    const procs = [P(1_900_000_001, 1), P(1_900_000_002, 1_900_000_001)];
    const out = killSubtree(procs, 1_900_000_001);
    expect(out.killed).toBe(0);
    expect(out.errors.map((e) => e.kind)).toEqual(["ESRCH", "ESRCH"]);
    expect(out.ok).toBe(false);
  });

  test("falls back to the root pid when it is not in the proc set", () => {
    const out = killSubtree([], 2_000_000_001);
    expect(out.errors[0]!.pid).toBe(2_000_000_001);
  });
});
