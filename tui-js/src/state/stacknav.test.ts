import { describe, expect, test } from "bun:test";
import { buildStackRows, clampSel } from "./stacknav";
import type { SliceView } from "./derive";

function view(): SliceView {
  return {
    status: "none",
    slice: {
      name: "checkout",
      base: "",
      active: false,
      stale: false,
      members: [
        { repo: "web", branch: "jonny/checkout", worktree_path: "/w", tip_sha: "a" },
        { repo: "api", branch: "jonny/checkout", worktree_path: "/a", tip_sha: "b" },
      ],
    },
    show: {
      name: "checkout",
      base: "",
      active: false,
      members: [
        {
          repo: "web",
          branch: "jonny/checkout",
          worktree_path: "/w",
          tip_sha: "a",
          stack: [
            { name: "main", depth: 0, trunk: true, needs_restack: false },
            { name: "jonny/checkout-base", depth: 1, trunk: false, needs_restack: false },
            { name: "jonny/checkout", depth: 2, trunk: false, needs_restack: true },
          ],
        },
        {
          repo: "api",
          branch: "jonny/checkout",
          worktree_path: "/a",
          tip_sha: "b",
          stack: [
            { name: "master", depth: 0, trunk: true, needs_restack: false },
            { name: "jonny/checkout", depth: 1, trunk: false, needs_restack: false },
          ],
        },
      ],
    },
  };
}

describe("buildStackRows", () => {
  test("flattens every branch across repos, member-by-member", () => {
    const rows = buildStackRows(view());
    expect(rows.map((r) => `${r.repo}:${r.branch}`)).toEqual([
      "web:main",
      "web:jonny/checkout-base",
      "web:jonny/checkout",
      "api:master",
      "api:jonny/checkout",
    ]);
  });

  test("marks the member branch, trunk, restack, and repo-group starts", () => {
    const rows = buildStackRows(view());
    const member = rows.filter((r) => r.isMember).map((r) => `${r.repo}:${r.branch}`);
    expect(member).toEqual(["web:jonny/checkout", "api:jonny/checkout"]);
    expect(rows.find((r) => r.branch === "main")!.trunk).toBe(true);
    expect(rows.find((r) => r.repo === "web" && r.branch === "jonny/checkout")!.needsRestack).toBe(true);
    expect(rows.filter((r) => r.firstOfRepo).map((r) => r.repo)).toEqual(["web", "api"]);
  });

  test("repo without stack data contributes one member row", () => {
    const v = view();
    v.show = undefined;
    const rows = buildStackRows(v);
    expect(rows).toHaveLength(2);
    expect(rows.every((r) => r.isMember && r.firstOfRepo)).toBe(true);
  });
});

describe("clampSel", () => {
  test("clamps to range and to 0 when empty", () => {
    expect(clampSel(5, 3)).toBe(2);
    expect(clampSel(-1, 3)).toBe(0);
    expect(clampSel(2, 0)).toBe(0);
  });
});
