import { describe, expect, test } from "bun:test";
import {
  childPath,
  flattenTree,
  indexChanges,
  parentPath,
  toggled,
  withChildren,
  type ChildrenByPath,
} from "./filetree";

const root: ChildrenByPath = {
  "": [
    { name: "src", type: "tree", size: -1 },
    { name: "README.md", type: "blob", size: 10 },
  ],
  src: [
    { name: "checkout", type: "tree", size: -1 },
    { name: "app.ts", type: "blob", size: 20 },
  ],
  "src/checkout": [{ name: "cart.tsx", type: "blob", size: 30 }],
};

describe("childPath / parentPath", () => {
  test("joins and splits repo-relative paths", () => {
    expect(childPath("", "src")).toBe("src");
    expect(childPath("src", "app.ts")).toBe("src/app.ts");
    expect(parentPath("src/checkout/cart.tsx")).toBe("src/checkout");
    expect(parentPath("README.md")).toBe("");
  });
});

describe("indexChanges", () => {
  test("indexes exact statuses and every non-root ancestor directory", () => {
    const changes = indexChanges([
      { path: "src/components/card.tsx", status: "modified" },
      { path: "README.md", status: "added" },
      { path: "test/old.ts", status: "deleted" },
    ]);

    expect([...changes.files]).toEqual([
      ["src/components/card.tsx", "modified"],
      ["README.md", "added"],
      ["test/old.ts", "deleted"],
    ]);
    expect([...changes.directories]).toEqual(["src/components", "src", "test"]);
  });

  test("deduplicates shared ancestor directories", () => {
    const changes = indexChanges([
      { path: "src/a.ts", status: "added" },
      { path: "src/b.ts", status: "renamed" },
    ]);
    expect([...changes.directories]).toEqual(["src"]);
  });
});

describe("flattenTree", () => {
  test("collapsed root shows only top-level entries", () => {
    const rows = flattenTree(root, new Set());
    expect(rows.map((r) => r.path)).toEqual(["src", "README.md"]);
    expect(rows[0]!.expanded).toBe(false);
  });

  test("expanding a directory reveals its fetched children indented", () => {
    const rows = flattenTree(root, new Set(["src"]));
    expect(rows.map((r) => r.path)).toEqual(["src", "src/checkout", "src/app.ts", "README.md"]);
    expect(rows.find((r) => r.path === "src")!.expanded).toBe(true);
    expect(rows.find((r) => r.path === "src/checkout")!.depth).toBe(1);
  });

  test("nested expansion recurses depth-first", () => {
    const rows = flattenTree(root, new Set(["src", "src/checkout"]));
    expect(rows.map((r) => r.path)).toEqual([
      "src",
      "src/checkout",
      "src/checkout/cart.tsx",
      "src/app.ts",
      "README.md",
    ]);
    expect(rows.find((r) => r.path === "src/checkout/cart.tsx")!.depth).toBe(2);
  });

  test("expanded-but-unfetched directory shows no children", () => {
    const partial: ChildrenByPath = { "": [{ name: "src", type: "tree", size: -1 }] };
    const rows = flattenTree(partial, new Set(["src"]));
    expect(rows.map((r) => r.path)).toEqual(["src"]);
  });
});

describe("toggled / withChildren", () => {
  test("toggled adds then removes without mutating input", () => {
    const a = toggled(new Set(), "src");
    expect([...a]).toEqual(["src"]);
    const b = toggled(a, "src");
    expect([...b]).toEqual([]);
    expect([...a]).toEqual(["src"]);
  });

  test("withChildren stores a level immutably", () => {
    const base: ChildrenByPath = {};
    const next = withChildren(base, "src", [{ name: "x.ts", type: "blob", size: 1 }]);
    expect(next["src"]).toHaveLength(1);
    expect(base["src"]).toBeUndefined();
  });
});
