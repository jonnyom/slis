import { expect, test } from "bun:test";
import type { DiffLine } from "./parse";
import { pairChangedLines, pairingMap } from "./pair";

function lines(spec: Array<[DiffLine["type"], string]>): DiffLine[] {
  return spec.map(([type, content]) => ({ type, content }));
}

test("pairs a del run with the following add run positionally", () => {
  const ls = lines([
    ["context", "a"],
    ["del", "old1"],
    ["del", "old2"],
    ["add", "new1"],
    ["add", "new2"],
    ["context", "b"],
  ]);
  expect(pairChangedLines(ls)).toEqual([
    { oldIndex: 1, newIndex: 3 },
    { oldIndex: 2, newIndex: 4 },
  ]);
});

test("surplus deletions are left unpaired", () => {
  const ls = lines([
    ["del", "old1"],
    ["del", "old2"],
    ["add", "new1"],
  ]);
  expect(pairChangedLines(ls)).toEqual([{ oldIndex: 0, newIndex: 2 }]);
});

test("additions with no preceding deletions pair nothing", () => {
  const ls = lines([
    ["context", "a"],
    ["add", "new1"],
    ["add", "new2"],
  ]);
  expect(pairChangedLines(ls)).toEqual([]);
});

test("pairingMap is symmetric", () => {
  const ls = lines([
    ["del", "old"],
    ["add", "new"],
  ]);
  const map = pairingMap(ls);
  expect(map.get(0)).toBe(1);
  expect(map.get(1)).toBe(0);
});
