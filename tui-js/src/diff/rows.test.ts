import { expect, test } from "bun:test";
import { parseUnifiedDiff } from "./parse";
import { buildFileRows } from "./rows";

const PATCH = [
  "diff --git a/src/x.ts b/src/x.ts",
  "--- a/src/x.ts",
  "+++ b/src/x.ts",
  "@@ -1,3 +1,3 @@ header",
  " const a = 1;",
  "-const b = 2;",
  "+const b = 3;",
  " const c = 4;",
].join("\n");

function firstFile() {
  return parseUnifiedDiff(PATCH)[0]!;
}

test("unified rows start with a hunk header then one row per line", () => {
  const rows = buildFileRows(firstFile(), "ts").unified;
  expect(rows[0]!.kind).toBe("hunk");
  const lineRows = rows.filter((r) => r.kind === "line");
  expect(lineRows).toHaveLength(4);
});

test("unified paired change highlights only the changed word", () => {
  const rows = buildFileRows(firstFile(), "ts").unified;
  const addRow = rows.find((r) => r.kind === "line" && r.lineType === "add");
  const changed =
    addRow!.kind === "line" ? addRow!.cells.filter((c) => c.changed).map((c) => c.text).join("") : "";
  expect(changed).toBe("3");
});

test("side-by-side aligns the change on one row with word highlights", () => {
  const rows = buildFileRows(firstFile(), "ts").sideBySide;
  const changeRow = rows.find(
    (r) => r.kind === "line" && r.left.lineType === "del" && r.right.lineType === "add",
  );
  expect(changeRow).toBeDefined();
  if (changeRow?.kind === "line") {
    expect(changeRow.left.cells.filter((c) => c.changed).map((c) => c.text).join("")).toBe("2");
    expect(changeRow.right.cells.filter((c) => c.changed).map((c) => c.text).join("")).toBe("3");
  }
});

test("hunk offsets point at hunk rows", () => {
  const built = buildFileRows(firstFile(), "ts");
  for (const off of built.unifiedHunkOffsets) {
    expect(built.unified[off]!.kind).toBe("hunk");
  }
  for (const off of built.sbsHunkOffsets) {
    expect(built.sideBySide[off]!.kind).toBe("hunk");
  }
});

test("gutter numbers are populated per side", () => {
  const rows = buildFileRows(firstFile(), "ts").unified;
  const ctx = rows.find((r) => r.kind === "line" && r.lineType === "context");
  if (ctx?.kind === "line") {
    expect(ctx.oldNumber).toBe(1);
    expect(ctx.newNumber).toBe(1);
  }
});
