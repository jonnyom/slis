import { expect, test } from "bun:test";
import { parseUnifiedDiff, statusGlyph } from "./parse";

test("modified file has status modified and counts", () => {
  const patch = [
    "diff --git a/src/x.ts b/src/x.ts",
    "index 111..222 100644",
    "--- a/src/x.ts",
    "+++ b/src/x.ts",
    "@@ -1,3 +1,3 @@",
    " const a = 1;",
    "-const b = 2;",
    "+const b = 3;",
    " const c = 4;",
  ].join("\n");
  const [file] = parseUnifiedDiff(patch);
  expect(file!.status).toBe("modified");
  expect(file!.added).toBe(1);
  expect(file!.deleted).toBe(1);
  expect(file!.path).toBe("src/x.ts");
});

test("added file (--- /dev/null) has status added", () => {
  const patch = [
    "diff --git a/new.ts b/new.ts",
    "new file mode 100644",
    "--- /dev/null",
    "+++ b/new.ts",
    "@@ -0,0 +1,2 @@",
    "+line one",
    "+line two",
  ].join("\n");
  const [file] = parseUnifiedDiff(patch);
  expect(file!.status).toBe("added");
  expect(statusGlyph(file!.status)).toBe("A");
});

test("deleted file (+++ /dev/null) has status deleted", () => {
  const patch = [
    "diff --git a/gone.ts b/gone.ts",
    "deleted file mode 100644",
    "--- a/gone.ts",
    "+++ /dev/null",
    "@@ -1,1 +0,0 @@",
    "-was here",
  ].join("\n");
  const [file] = parseUnifiedDiff(patch);
  expect(file!.status).toBe("deleted");
  expect(statusGlyph(file!.status)).toBe("D");
});

test("renamed file keeps status renamed even with edits", () => {
  const patch = [
    "diff --git a/old.ts b/new.ts",
    "similarity index 90%",
    "rename from old.ts",
    "rename to new.ts",
    "--- a/old.ts",
    "+++ b/new.ts",
    "@@ -1,1 +1,1 @@",
    "-old",
    "+new",
  ].join("\n");
  const [file] = parseUnifiedDiff(patch);
  expect(file!.status).toBe("renamed");
  expect(file!.oldPath).toBe("old.ts");
  expect(file!.path).toBe("new.ts");
  expect(statusGlyph(file!.status)).toBe("R");
});

test("parses multiple files", () => {
  const patch = [
    "diff --git a/one.ts b/one.ts",
    "--- a/one.ts",
    "+++ b/one.ts",
    "@@ -1 +1 @@",
    "-a",
    "+b",
    "diff --git a/two.ts b/two.ts",
    "--- a/two.ts",
    "+++ b/two.ts",
    "@@ -1 +1 @@",
    "-c",
    "+d",
  ].join("\n");
  const files = parseUnifiedDiff(patch);
  expect(files.map((f) => f.path)).toEqual(["one.ts", "two.ts"]);
});
