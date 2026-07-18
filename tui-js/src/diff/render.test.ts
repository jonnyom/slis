import { expect, test } from "bun:test";
import { classifyPatchLine, styleLine } from "./render";
import { wordDiff } from "./words";

test("classifyPatchLine tags add/del/hunk/meta/context", () => {
  expect(classifyPatchLine("@@ -1,3 +1,5 @@")).toBe("hunk");
  expect(classifyPatchLine("diff --git a/x b/x")).toBe("meta");
  expect(classifyPatchLine("+++ b/x")).toBe("meta");
  expect(classifyPatchLine("--- a/x")).toBe("meta");
  expect(classifyPatchLine("+  added")).toBe("add");
  expect(classifyPatchLine("-  removed")).toBe("del");
  expect(classifyPatchLine("   context")).toBe("context");
});

test("cells reassemble to the original content", () => {
  const cells = styleLine("const b = 3;", "ts");
  expect(cells.map((c) => c.text).join("")).toBe("const b = 3;");
});

test("without a word diff nothing is flagged changed", () => {
  const cells = styleLine("const b = 3;", "ts");
  expect(cells.every((c) => !c.changed)).toBe(true);
});

test("word-diff change flag lands on the differing token only", () => {
  const wd = wordDiff("const b = 2;", "const b = 3;");
  const cells = styleLine("const b = 3;", "ts", wd.new);
  const changed = cells.filter((c) => c.changed).map((c) => c.text).join("");
  expect(changed).toBe("3");
});

test("syntax kind survives the merge with a change flag", () => {
  const wd = wordDiff("x", "foo");
  const cells = styleLine("foo(1)", "ts", wd.new);
  const foo = cells.find((c) => c.text === "foo");
  expect(foo?.kind).toBe("function");
});
