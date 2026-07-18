// Parent → child process tree built from ppid links within a single slice's
// proc list, plus flattening (respecting a collapsed set) with indent guides
// and subtree CPU/mem rollups. Pure and total: cycles are broken, and a proc
// whose parent is outside the slice's set is treated as a root.

import type { ProcEntry } from "../rpc/types";
import type { ProcSort } from "./sort";

export interface ProcNode {
  proc: ProcEntry;
  children: ProcNode[];
  subtreeCpu: number;
  subtreeMem: number;
}

export interface FlatProcRow {
  proc: ProcEntry;
  depth: number;
  hasChildren: boolean;
  collapsed: boolean;
  subtreeCpu: number;
  subtreeMem: number;
  /** Assembled tree-guide prefix (│ ├─ └─) for everything left of the command. */
  prefix: string;
}

function nodeComparator(sort: ProcSort): (a: ProcNode, b: ProcNode) => number {
  switch (sort) {
    case "mem":
      return (a, b) => b.subtreeMem - a.subtreeMem || a.proc.pid - b.proc.pid;
    case "pid":
      return (a, b) => a.proc.pid - b.proc.pid;
    case "cpu":
    default:
      return (a, b) => b.subtreeCpu - a.subtreeCpu || a.proc.pid - b.proc.pid;
  }
}

export function buildProcTree(procs: ProcEntry[], sort: ProcSort = "cpu"): ProcNode[] {
  const byPid = new Map<number, ProcEntry>();
  for (const p of procs) byPid.set(p.pid, p);

  const childrenOf = new Map<number, ProcEntry[]>();
  const roots: ProcEntry[] = [];
  for (const p of procs) {
    const parentInSet = p.ppid !== p.pid && byPid.has(p.ppid);
    if (parentInSet) {
      const list = childrenOf.get(p.ppid) ?? [];
      list.push(p);
      childrenOf.set(p.ppid, list);
    } else {
      roots.push(p);
    }
  }

  const cmp = nodeComparator(sort);
  const build = (p: ProcEntry, seen: Set<number>): ProcNode => {
    seen.add(p.pid);
    const kids = (childrenOf.get(p.pid) ?? [])
      .filter((c) => !seen.has(c.pid))
      .map((c) => build(c, seen))
      .sort(cmp);
    const subtreeCpu = kids.reduce((a, n) => a + n.subtreeCpu, p.cpu);
    const subtreeMem = kids.reduce((a, n) => a + n.subtreeMem, p.mem);
    return { proc: p, children: kids, subtreeCpu, subtreeMem };
  };

  const seen = new Set<number>();
  const result = roots.map((r) => build(r, seen));
  // Procs trapped in a ppid cycle have no external root; surface them anyway.
  for (const p of procs) {
    if (!seen.has(p.pid)) result.push(build(p, seen));
  }
  return result.sort(cmp);
}

/**
 * Flatten the tree to display rows in pre-order, skipping the children of any
 * pid in `collapsed`. `ancestorsLast` tracks, per level, whether the ancestor
 * was the last sibling — that decides pipe vs blank in the indent guide.
 */
export function flattenTree(
  roots: ProcNode[],
  collapsed: Set<number> = new Set(),
): FlatProcRow[] {
  const out: FlatProcRow[] = [];

  const walk = (node: ProcNode, depth: number, ancestorsLast: boolean[], isLast: boolean) => {
    const hasChildren = node.children.length > 0;
    const isCollapsed = collapsed.has(node.proc.pid);

    let prefix = "";
    if (depth > 0) {
      for (const last of ancestorsLast) prefix += last ? "  " : "│ ";
      prefix += isLast ? "└─" : "├─";
    }

    out.push({
      proc: node.proc,
      depth,
      hasChildren,
      collapsed: isCollapsed,
      subtreeCpu: node.subtreeCpu,
      subtreeMem: node.subtreeMem,
      prefix,
    });

    if (hasChildren && !isCollapsed) {
      const nextAncestors = depth > 0 ? [...ancestorsLast, isLast] : ancestorsLast;
      node.children.forEach((child, i) => {
        walk(child, depth + 1, nextAncestors, i === node.children.length - 1);
      });
    }
  };

  roots.forEach((r, i) => walk(r, 0, [], i === roots.length - 1));
  return out;
}

/**
 * PIDs of the subtree rooted at `rootPid`, deepest-first (post-order) with the
 * root last — the order SIGTERM should follow so parents don't respawn a child
 * before it is signalled. Derived from ppid links in `procs`; cycle-safe.
 */
export function subtreePids(procs: ProcEntry[], rootPid: number): number[] {
  const childrenOf = new Map<number, number[]>();
  const pids = new Set(procs.map((p) => p.pid));
  for (const p of procs) {
    if (p.ppid !== p.pid && pids.has(p.ppid)) {
      const list = childrenOf.get(p.ppid) ?? [];
      list.push(p.pid);
      childrenOf.set(p.ppid, list);
    }
  }

  const order: number[] = [];
  const seen = new Set<number>();
  const walk = (pid: number) => {
    if (seen.has(pid)) return;
    seen.add(pid);
    for (const kid of childrenOf.get(pid) ?? []) walk(kid);
    order.push(pid);
  };
  if (pids.has(rootPid)) walk(rootPid);
  return order;
}

/** Total CPU across a proc list (the totals row / slice-total badge). */
export function totalCpu(procs: ProcEntry[]): number {
  return procs.reduce((a, p) => a + p.cpu, 0);
}

/** Total resident memory (MiB) across a proc list. */
export function totalMem(procs: ProcEntry[]): number {
  return procs.reduce((a, p) => a + p.mem, 0);
}
