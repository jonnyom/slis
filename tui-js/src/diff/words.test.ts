import { expect, test } from "bun:test";
import { tokenizeWords, wordDiff } from "./words";

function reassemble(segs: { text: string }[]): string {
  return segs.map((s) => s.text).join("");
}

test("tokenizeWords splits identifiers, whitespace and punctuation", () => {
  expect(tokenizeWords("a = b + 1")).toEqual(["a", " ", "=", " ", "b", " ", "+", " ", "1"]);
});

test("identical lines report nothing changed", () => {
  const r = wordDiff("const x = 1;", "const x = 1;");
  expect(r.old.every((s) => !s.changed)).toBe(true);
  expect(r.new.every((s) => !s.changed)).toBe(true);
});

test("single-word change is isolated on both sides", () => {
  const r = wordDiff("const b = 2;", "const b = 3;");
  // The only changed token on each side is the number.
  const changedOld = r.old.filter((s) => s.changed).map((s) => s.text);
  const changedNew = r.new.filter((s) => s.changed).map((s) => s.text);
  expect(changedOld).toEqual(["2"]);
  expect(changedNew).toEqual(["3"]);
});

test("segments reassemble to the original text", () => {
  const r = wordDiff("return foo(a, b)", "return bar(a, b, c)");
  expect(reassemble(r.old)).toBe("return foo(a, b)");
  expect(reassemble(r.new)).toBe("return bar(a, b, c)");
});

test("insertion only marks new side", () => {
  const r = wordDiff("a b", "a x b");
  expect(r.old.some((s) => s.changed)).toBe(false);
  expect(r.new.filter((s) => s.changed).map((s) => s.text).join("")).toContain("x");
});

test("shared prefix and suffix stay unchanged", () => {
  const r = wordDiff("const total = sum(items);", "const total = sum(items, tax);");
  expect(r.old.some((s) => s.changed)).toBe(false);
  const changedNew = r.new.filter((s) => s.changed).map((s) => s.text).join("");
  expect(changedNew).toContain("tax");
});
