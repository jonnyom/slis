// Lightweight, single-line syntax tokenizer for the differ. Pure and total,
// no dependencies, no tree-sitter / WASM — so it cannot tank cold start and is
// trivially unit-testable. It is deliberately line-local: block comments and
// multi-line strings are only recognised within one line (good enough for a
// diff, where we render one line at a time). Every character of the input is
// emitted in exactly one token, so the output concatenates back to the input.

export type TokenKind =
  | "keyword"
  | "string"
  | "comment"
  | "number"
  | "type"
  | "function"
  | "punct"
  | "plain";

export type Lang = "ts" | "go" | "py" | "rb" | "json" | "yaml" | "md" | "plain";

export interface Token {
  text: string;
  kind: TokenKind;
}

interface LangSpec {
  lineComments: string[];
  blockComment?: [string, string];
  strings: string[];
  keywords: Set<string>;
  /** Capitalised identifiers are highlighted as types (ts/go). */
  typeByCapital: boolean;
  /** `ident(` marks the identifier as a function call. */
  functionCalls: boolean;
}

const TS_KEYWORDS = new Set(
  ("const let var function return if else for while do switch case break continue " +
    "new class extends implements interface type enum import export from as default " +
    "async await yield try catch finally throw typeof instanceof in of void this super " +
    "null undefined true false public private protected readonly static get set namespace " +
    "declare abstract satisfies keyof infer never unknown any").split(" "),
);

const GO_KEYWORDS = new Set(
  ("func var const type struct interface map chan package import return if else for range " +
    "switch case default break continue go defer select fallthrough goto nil true false iota " +
    "make new len cap append panic recover").split(" "),
);

const PY_KEYWORDS = new Set(
  ("def class return if elif else for while import from as pass break continue with try except " +
    "finally raise lambda yield global nonlocal in is not and or None True False async await del " +
    "assert print self").split(" "),
);

const RB_KEYWORDS = new Set(
  ("def class module return if elsif else unless while until for do end begin rescue ensure raise " +
    "yield self nil true false and or not then case when require require_relative attr_accessor " +
    "attr_reader attr_writer new super puts").split(" "),
);

const JSON_KEYWORDS = new Set(["true", "false", "null"]);

const YAML_KEYWORDS = new Set([
  "true",
  "false",
  "null",
  "yes",
  "no",
  "on",
  "off",
  "~",
]);

const SPECS: Record<Exclude<Lang, "md">, LangSpec> = {
  ts: {
    lineComments: ["//"],
    blockComment: ["/*", "*/"],
    strings: ['"', "'", "`"],
    keywords: TS_KEYWORDS,
    typeByCapital: true,
    functionCalls: true,
  },
  go: {
    lineComments: ["//"],
    blockComment: ["/*", "*/"],
    strings: ['"', "`"],
    keywords: GO_KEYWORDS,
    typeByCapital: true,
    functionCalls: true,
  },
  py: {
    lineComments: ["#"],
    strings: ['"', "'"],
    keywords: PY_KEYWORDS,
    typeByCapital: false,
    functionCalls: true,
  },
  rb: {
    lineComments: ["#"],
    strings: ['"', "'"],
    keywords: RB_KEYWORDS,
    typeByCapital: false,
    functionCalls: true,
  },
  json: {
    lineComments: [],
    strings: ['"'],
    keywords: JSON_KEYWORDS,
    typeByCapital: false,
    functionCalls: false,
  },
  yaml: {
    lineComments: ["#"],
    strings: ['"', "'"],
    keywords: YAML_KEYWORDS,
    typeByCapital: false,
    functionCalls: false,
  },
  plain: {
    lineComments: [],
    strings: [],
    keywords: new Set<string>(),
    typeByCapital: false,
    functionCalls: false,
  },
};

const EXT_TO_LANG: Record<string, Lang> = {
  ts: "ts",
  tsx: "ts",
  mts: "ts",
  cts: "ts",
  js: "ts",
  jsx: "ts",
  mjs: "ts",
  cjs: "ts",
  go: "go",
  py: "py",
  pyi: "py",
  rb: "rb",
  rake: "rb",
  gemspec: "rb",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  md: "md",
  markdown: "md",
};

export function langForPath(path: string): Lang {
  const base = path.split("/").pop() ?? path;
  if (base === "Gemfile" || base === "Rakefile") return "rb";
  if (base === "Dockerfile") return "plain";
  const dot = base.lastIndexOf(".");
  if (dot < 0) return "plain";
  const ext = base.slice(dot + 1).toLowerCase();
  return EXT_TO_LANG[ext] ?? "plain";
}

function isIdentStart(ch: string): boolean {
  return /[A-Za-z_$@]/.test(ch);
}

function isIdentPart(ch: string): boolean {
  return /[A-Za-z0-9_$]/.test(ch);
}

function isDigit(ch: string): boolean {
  return ch >= "0" && ch <= "9";
}

function nextNonSpace(line: string, from: number): string {
  for (let i = from; i < line.length; i++) {
    if (!/\s/.test(line[i]!)) return line[i]!;
  }
  return "";
}

function classifyIdent(
  word: string,
  spec: LangSpec,
  followedByCall: boolean,
): TokenKind {
  if (spec.keywords.has(word)) return "keyword";
  if (followedByCall && spec.functionCalls) return "function";
  if (spec.typeByCapital && /^[A-Z]/.test(word)) return "type";
  return "plain";
}

function tokenizeGeneric(line: string, spec: LangSpec): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  const n = line.length;
  while (i < n) {
    const ch = line[i]!;

    // whitespace
    if (/\s/.test(ch)) {
      let j = i + 1;
      while (j < n && /\s/.test(line[j]!)) j++;
      tokens.push({ text: line.slice(i, j), kind: "plain" });
      i = j;
      continue;
    }

    // line comment
    const lc = spec.lineComments.find((c) => line.startsWith(c, i));
    if (lc) {
      tokens.push({ text: line.slice(i), kind: "comment" });
      break;
    }

    // block comment (line-local)
    if (spec.blockComment && line.startsWith(spec.blockComment[0], i)) {
      const close = line.indexOf(spec.blockComment[1], i + spec.blockComment[0].length);
      const end = close < 0 ? n : close + spec.blockComment[1].length;
      tokens.push({ text: line.slice(i, end), kind: "comment" });
      i = end;
      continue;
    }

    // string
    if (spec.strings.includes(ch)) {
      let j = i + 1;
      while (j < n) {
        if (line[j] === "\\") {
          j += 2;
          continue;
        }
        if (line[j] === ch) {
          j++;
          break;
        }
        j++;
      }
      tokens.push({ text: line.slice(i, j), kind: "string" });
      i = j;
      continue;
    }

    // number
    if (isDigit(ch) || (ch === "." && isDigit(line[i + 1] ?? ""))) {
      let j = i + 1;
      while (j < n && /[0-9a-fA-FxXoObBeE._]/.test(line[j]!)) j++;
      tokens.push({ text: line.slice(i, j), kind: "number" });
      i = j;
      continue;
    }

    // identifier / keyword
    if (isIdentStart(ch)) {
      let j = i + 1;
      while (j < n && isIdentPart(line[j]!)) j++;
      const word = line.slice(i, j);
      const followedByCall = nextNonSpace(line, j) === "(";
      tokens.push({ text: word, kind: classifyIdent(word, spec, followedByCall) });
      i = j;
      continue;
    }

    // punctuation (single char)
    tokens.push({ text: ch, kind: "punct" });
    i++;
  }
  return tokens;
}

function tokenizeYaml(line: string): Token[] {
  const spec = SPECS.yaml;
  // Highlight a leading `key:` as a type, then tokenize the value generically.
  const keyMatch = line.match(/^(\s*(?:-\s+)?)([A-Za-z0-9_.-]+)(\s*:)(\s|$)/);
  if (keyMatch) {
    const [, indent, key, colon] = keyMatch;
    const head: Token[] = [];
    if (indent) head.push({ text: indent, kind: "plain" });
    head.push({ text: key!, kind: "type" });
    head.push({ text: colon!, kind: "punct" });
    const rest = line.slice((indent?.length ?? 0) + key!.length + colon!.length);
    return [...head, ...tokenizeGeneric(rest, spec)];
  }
  return tokenizeGeneric(line, spec);
}

function tokenizeMarkdown(line: string): Token[] {
  if (/^\s*#{1,6}\s/.test(line)) return [{ text: line, kind: "keyword" }];
  if (/^\s*>/.test(line)) return [{ text: line, kind: "comment" }];

  const tokens: Token[] = [];
  const bullet = line.match(/^(\s*(?:[-*+]|\d+\.)\s+)/);
  let i = 0;
  if (bullet) {
    tokens.push({ text: bullet[1]!, kind: "punct" });
    i = bullet[1]!.length;
  }
  const n = line.length;
  let plainStart = i;
  const flushPlain = (end: number) => {
    if (end > plainStart) tokens.push({ text: line.slice(plainStart, end), kind: "plain" });
  };
  while (i < n) {
    if (line[i] === "`") {
      const close = line.indexOf("`", i + 1);
      const end = close < 0 ? n : close + 1;
      flushPlain(i);
      tokens.push({ text: line.slice(i, end), kind: "string" });
      i = end;
      plainStart = i;
      continue;
    }
    i++;
  }
  flushPlain(n);
  return tokens;
}

/** Tokenize a single line for the given language. */
export function tokenizeLine(line: string, lang: Lang): Token[] {
  if (line === "") return [];
  if (lang === "md") return tokenizeMarkdown(line);
  if (lang === "yaml") return tokenizeYaml(line);
  return tokenizeGeneric(line, SPECS[lang]);
}
