import { expect, test } from "bun:test";
import { styleLine } from "./render";
import { wordDiff } from "./words";

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
