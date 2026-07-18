import { expect, test } from "bun:test";
import { langForPath, tokenizeLine, type Token } from "./tokenize";

function reassemble(tokens: Token[]): string {
  return tokens.map((t) => t.text).join("");
}

function kindOf(tokens: Token[], text: string): string | undefined {
  return tokens.find((t) => t.text === text)?.kind;
}

test("langForPath maps extensions and special filenames", () => {
  expect(langForPath("src/a.tsx")).toBe("ts");
  expect(langForPath("internal/x.go")).toBe("go");
  expect(langForPath("m.py")).toBe("py");
  expect(langForPath("app/models/user.rb")).toBe("rb");
  expect(langForPath("Gemfile")).toBe("rb");
  expect(langForPath("config.yaml")).toBe("yaml");
  expect(langForPath("data.json")).toBe("json");
  expect(langForPath("README.md")).toBe("md");
  expect(langForPath("LICENSE")).toBe("plain");
});

test("every token round-trips to the original line", () => {
  const line = "  const total = sum(items) + 42; // note";
  expect(reassemble(tokenizeLine(line, "ts"))).toBe(line);
});

test("ts: keywords, strings, comments, numbers, calls", () => {
  const t = tokenizeLine('const s = "hi"; foo(); // x', "ts");
  expect(kindOf(t, "const")).toBe("keyword");
  expect(kindOf(t, '"hi"')).toBe("string");
  expect(kindOf(t, "foo")).toBe("function");
  expect(t.find((x) => x.text.startsWith("//"))?.kind).toBe("comment");
});

test("ts: capitalised identifier is a type", () => {
  const t = tokenizeLine("let x: Money = zero", "ts");
  expect(kindOf(t, "Money")).toBe("type");
});

test("go: func keyword and backtick string", () => {
  const t = tokenizeLine("func Totals() { return `x` }", "go");
  expect(kindOf(t, "func")).toBe("keyword");
  expect(kindOf(t, "Totals")).toBe("function");
  expect(t.find((x) => x.text === "`x`")?.kind).toBe("string");
});

test("py: hash comment and def keyword", () => {
  const t = tokenizeLine("def run():  # go", "py");
  expect(kindOf(t, "def")).toBe("keyword");
  expect(t.find((x) => x.text.startsWith("#"))?.kind).toBe("comment");
});

test("yaml: leading key is a type, value tokenized", () => {
  const t = tokenizeLine("name: true # c", "yaml");
  expect(kindOf(t, "name")).toBe("type");
  expect(kindOf(t, "true")).toBe("keyword");
  expect(t.find((x) => x.text.startsWith("#"))?.kind).toBe("comment");
});

test("json: literals are keywords, strings are strings", () => {
  const t = tokenizeLine('"key": false', "json");
  expect(kindOf(t, '"key"')).toBe("string");
  expect(kindOf(t, "false")).toBe("keyword");
});

test("md: heading line is one keyword token; inline code is string", () => {
  expect(tokenizeLine("## Title", "md")).toEqual([{ text: "## Title", kind: "keyword" }]);
  const t = tokenizeLine("use `code` here", "md");
  expect(t.find((x) => x.text === "`code`")?.kind).toBe("string");
});

test("empty line yields no tokens", () => {
  expect(tokenizeLine("", "ts")).toEqual([]);
});
