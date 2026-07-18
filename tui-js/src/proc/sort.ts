// Process sort ordering shared by the tree view and the all-slices overlay.

import type { ProcEntry } from "../rpc/types";

export type ProcSort = "cpu" | "mem" | "pid";

export const SORT_ORDER: ProcSort[] = ["cpu", "mem", "pid"];

export const SORT_LABEL: Record<ProcSort, string> = {
  cpu: "cpu",
  mem: "mem",
  pid: "pid",
};

export function nextSort(sort: ProcSort): ProcSort {
  return SORT_ORDER[(SORT_ORDER.indexOf(sort) + 1) % SORT_ORDER.length]!;
}

/** Comparator over the sort key, with pid as a stable tiebreak. */
export function procComparator(sort: ProcSort): (a: ProcEntry, b: ProcEntry) => number {
  switch (sort) {
    case "mem":
      return (a, b) => b.mem - a.mem || a.pid - b.pid;
    case "pid":
      return (a, b) => a.pid - b.pid;
    case "cpu":
    default:
      return (a, b) => b.cpu - a.cpu || a.pid - b.pid;
  }
}

export function sortProcs(procs: ProcEntry[], sort: ProcSort): ProcEntry[] {
  return [...procs].sort(procComparator(sort));
}
