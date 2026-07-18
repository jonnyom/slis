import { describe, expect, test } from "bun:test";
import type { PrStackEntry, Slice } from "../rpc/types";
import type { SliceView } from "../state/derive";
import { hasFailingCi, listHints } from "./browser.hints";

function view(extra: Partial<Slice> = {}, vextra: Partial<SliceView> = {}): SliceView {
  return {
    slice: { name: "s", base: "", active: false, stale: false, members: [], ...extra },
    status: "none",
    ...vextra,
  };
}

function pr(extra: Partial<PrStackEntry> = {}): PrStackEntry {
  return { repo: "r", branch: "b", number: 1, ...extra };
}

const keys = (hints: { key: string }[]) => hints.map((h) => h.key);
const label = (hints: { key: string; label: string }[], key: string) =>
  hints.find((h) => h.key === key)?.label;

describe("hasFailingCi", () => {
  test("true when a PR reports ci fail or a non-zero fail count", () => {
    expect(hasFailingCi(view({}, { prs: [pr({ ci: "fail" })] }))).toBe(true);
    expect(hasFailingCi(view({}, { prs: [pr({ ci_fail: 2 })] }))).toBe(true);
  });
  test("false with no PRs, passing CI, or a PR that has no number", () => {
    expect(hasFailingCi(view())).toBe(false);
    expect(hasFailingCi(view({}, { prs: [pr({ ci: "pass" })] }))).toBe(false);
    expect(hasFailingCi(view({}, { prs: [pr({ number: undefined, ci: "fail" })] }))).toBe(false);
  });
});

describe("listHints (contextual browser hint bar)", () => {
  test("baseline exposes separate agent and shell terminals", () => {
    const h = listHints(view(), false);
    expect(keys(h)).toEqual(["enter", "a", "C", "V", "t", "w", "space", "/", ","]);
    expect(label(h, "a")).toBe("agent");
    expect(label(h, "C")).toBe("launch");
    expect(label(h, "V")).toBe("review");
    expect(label(h, "t")).toBe("shell");
    expect(label(h, ",")).toBe("config");
  });

  test("P1 — waiting-input relabels the attach key `a answer`", () => {
    const h = listHints(view({}, { status: "waiting-input" }), false);
    expect(label(h, "a")).toBe("answer");
  });

  test("M4 — a red-CI slice surfaces `v why` and `F fix`", () => {
    const h = listHints(view({}, { prs: [pr({ ci: "fail" })] }), false);
    expect(keys(h)).toContain("v");
    expect(keys(h)).toContain("F");
    expect(label(h, "v")).toBe("why");
    expect(label(h, "F")).toBe("fix");
  });

  test("D3 — a slice needing restack surfaces `R stack`", () => {
    const restacking = view(
      {},
      {
        show: {
          name: "s",
          base: "",
          active: false,
          members: [
            {
              repo: "r",
              branch: "b",
              worktree_path: "/w",
              tip_sha: "abc",
              stack: [{ name: "b", depth: 0, trunk: false, needs_restack: true }],
            },
          ],
        },
      },
    );
    const h = listHints(restacking, false);
    expect(keys(h)).toContain("R");
    expect(label(h, "R")).toBe("stack");
  });

  test("M3 — while searching, n/N appear as `match` and space/search drop", () => {
    const h = listHints(view(), true);
    expect(keys(h)).toContain("n/N");
    expect(label(h, "n/N")).toBe("match");
    expect(keys(h)).not.toContain("/");
    expect(keys(h)).not.toContain("space");
  });

  test("no `n/N match` hint when no search is active", () => {
    expect(keys(listHints(view(), false))).not.toContain("n/N");
  });
});
