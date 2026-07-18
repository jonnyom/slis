import { expect, test } from "bun:test";
import type { DiffLine } from "./parse";
import { buildSideBySide } from "./sidebyside";

function lines(spec: Array<[DiffLine["type"], string]>): DiffLine[] {
  return spec.map(([type, content]) => ({ type, content }));
}

test("context lines appear on both sides", () => {
  const rows = buildSideBySide(lines([["context", "a"]]));
  expect(rows).toEqual([
    { left: { type: "context", content: "a" }, right: { type: "context", content: "a" }, kind: "context" },
  ]);
});

test("balanced del/add run becomes change rows", () => {
  const rows = buildSideBySide(
    lines([
      ["del", "o1"],
      ["del", "o2"],
      ["add", "n1"],
      ["add", "n2"],
    ]),
  );
  expect(rows.map((r) => r.kind)).toEqual(["change", "change"]);
  expect(rows[0]!.left!.content).toBe("o1");
  expect(rows[0]!.right!.content).toBe("n1");
});

test("surplus deletion produces a del row with blank right", () => {
  const rows = buildSideBySide(
    lines([
      ["del", "o1"],
      ["del", "o2"],
      ["add", "n1"],
    ]),
  );
  expect(rows[1]!.kind).toBe("del");
  expect(rows[1]!.left!.content).toBe("o2");
  expect(rows[1]!.right).toBeUndefined();
});

test("pure additions produce add rows with blank left", () => {
  const rows = buildSideBySide(lines([["add", "n1"]]));
  expect(rows).toEqual([{ right: { type: "add", content: "n1" }, kind: "add" }]);
});
