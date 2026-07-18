import { describe, expect, test } from "bun:test";
import { buildStackRows, clampSel, compactStackGroups, stepStackBranch } from "./stacknav";
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

  test("carries per-branch line counts into display rows", () => {
    const v = view();
    v.show!.members[0]!.stack![2]!.added = 12;
    v.show!.members[0]!.stack![2]!.deleted = 4;
    const row = buildStackRows(v).find((item) => item.branch === "jonny/checkout");
    expect({ added: row!.added, deleted: row!.deleted }).toEqual({ added: 12, deleted: 4 });
  });
});

test("stack navigation skips visible trunk context rows", () => {
  const rows = [
    { repo: "a", branch: "main", trunk: true },
    { repo: "a", branch: "feat", trunk: false },
    { repo: "b", branch: "main", trunk: true },
    { repo: "b", branch: "feat", trunk: false },
  ] as ReturnType<typeof buildStackRows>;
  expect(stepStackBranch(rows, 1, 1)).toBe(3);
  expect(stepStackBranch(rows, 3, -1)).toBe(1);
});

describe("compactStackGroups", () => {
  const rows = Array.from({ length: 8 }, (_, i) => ({
    repo: "web",
    branch: i === 0 ? "main" : `stack-${i}`,
    trunk: i === 0,
    needsRestack: false,
    isMember: i === 7,
    depth: i,
    firstOfRepo: i === 0,
  }));

  test("caps a repo and accounts for hidden tail branches", () => {
    const [group] = compactStackGroups(rows, 1, 4);
    expect(group!.rows.map((item) => item.index)).toEqual([0, 1, 2, 3]);
    expect(group!.hiddenBefore).toBe(0);
    expect(group!.hiddenAfter).toBe(4);
  });

  test("keeps a deep selection visible with explicit overflow markers", () => {
    const [group] = compactStackGroups(rows, 6, 4);
    expect(group!.rows.map((item) => item.index)).toContain(6);
    expect(group!.rows[0]!.index).toBe(0);
    expect(group!.hiddenBefore + group!.hiddenAfter).toBe(4);
  });
});

describe("clampSel", () => {
  test("clamps to range and to 0 when empty", () => {
    expect(clampSel(5, 3)).toBe(2);
    expect(clampSel(-1, 3)).toBe(0);
    expect(clampSel(2, 0)).toBe(0);
  });
});
