import { describe, expect, test } from "bun:test";
import type { ProcEntry } from "../rpc/types";
import {
  buildProcTree,
  flattenTree,
  subtreePids,
  totalCpu,
  totalMem,
} from "./tree";

const P = (
  pid: number,
  ppid: number,
  cpu: number,
  mem: number,
  cmd = `p${pid}`,
): ProcEntry => ({ pid, ppid, cmd, cpu, mem });

// tmux pane (100) → node (200) → esbuild (300); plus a sibling go (400).
const slice = [
  P(100, 1, 1, 10, "tmux pane"),
  P(200, 100, 50, 200, "node vite"),
  P(300, 200, 5, 80, "esbuild"),
  P(400, 100, 40, 150, "go run"),
];

describe("buildProcTree", () => {
  test("roots are procs whose parent is outside the set", () => {
    const roots = buildProcTree(slice, "cpu");
    expect(roots.map((r) => r.proc.pid)).toEqual([100]);
  });

  test("rolls CPU/mem up the subtree", () => {
    const root = buildProcTree(slice, "cpu")[0]!;
    expect(root.subtreeCpu).toBe(1 + 50 + 5 + 40);
    expect(root.subtreeMem).toBe(10 + 200 + 80 + 150);
  });

  test("orders siblings by subtree cpu desc", () => {
    const root = buildProcTree(slice, "cpu")[0]!;
    // node subtree (55) > go (40)
    expect(root.children.map((c) => c.proc.pid)).toEqual([200, 400]);
  });

  test("treats an orphan (parent missing) as a root", () => {
    const orphan = [P(200, 100, 50, 200), P(300, 200, 5, 80)];
    const roots = buildProcTree(orphan, "cpu");
    expect(roots.map((r) => r.proc.pid)).toEqual([200]);
  });

  test("survives a ppid cycle without infinite recursion", () => {
    const cyclic = [P(1, 2, 1, 1), P(2, 1, 1, 1)];
    const roots = buildProcTree(cyclic, "cpu");
    // Both reference each other; one becomes the root, the other its child.
    expect(roots.length).toBeGreaterThanOrEqual(1);
  });
});

describe("flattenTree", () => {
  test("pre-order with indent guides", () => {
    const rows = flattenTree(buildProcTree(slice, "cpu"));
    expect(rows.map((r) => r.proc.pid)).toEqual([100, 200, 300, 400]);
    expect(rows.map((r) => r.depth)).toEqual([0, 1, 2, 1]);
    // node is not the last child of pane → ├─ ; go is last → └─
    expect(rows[1]!.prefix).toBe("├─");
    expect(rows[3]!.prefix).toBe("└─");
    // esbuild sits under node (not-last), so a pipe carries down.
    expect(rows[2]!.prefix).toBe("│ └─");
  });

  test("collapsing a node hides its subtree", () => {
    const rows = flattenTree(buildProcTree(slice, "cpu"), new Set([200]));
    expect(rows.map((r) => r.proc.pid)).toEqual([100, 200, 400]);
    expect(rows.find((r) => r.proc.pid === 200)!.collapsed).toBe(true);
    expect(rows.find((r) => r.proc.pid === 200)!.hasChildren).toBe(true);
  });
});

describe("subtreePids", () => {
  test("deepest-first with the root last", () => {
    expect(subtreePids(slice, 100)).toEqual([300, 200, 400, 100]);
  });

  test("leaf subtree is just itself", () => {
    expect(subtreePids(slice, 300)).toEqual([300]);
  });

  test("unknown pid yields nothing", () => {
    expect(subtreePids(slice, 999)).toEqual([]);
  });
});

describe("totals", () => {
  test("sum cpu and mem", () => {
    expect(totalCpu(slice)).toBe(96);
    expect(totalMem(slice)).toBe(440);
  });
});
