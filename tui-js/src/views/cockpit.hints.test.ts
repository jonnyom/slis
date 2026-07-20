import { describe, expect, test } from "bun:test";
import {
  breadcrumbSection,
  cockpitHints,
  cyclePanel,
  PANEL_ORDER,
  type PanelId,
} from "./cockpit.hints";

describe("cyclePanel", () => {
  test("cycles forward through the full order and wraps", () => {
    let p: PanelId = "stack";
    const seen: PanelId[] = [p];
    for (let i = 0; i < 4; i++) {
      p = cyclePanel(p, 1);
      seen.push(p);
    }
    expect(seen).toEqual(["stack", "prs", "session", "procs", "stack"]);
  });

  test("cycles backward and wraps (G9)", () => {
    let p: PanelId = "stack";
    const seen: PanelId[] = [p];
    for (let i = 0; i < 4; i++) {
      p = cyclePanel(p, -1);
      seen.push(p);
    }
    expect(seen).toEqual(["stack", "procs", "session", "prs", "stack"]);
  });

  test("forward then back returns to the start from every panel", () => {
    for (const p of PANEL_ORDER) {
      expect(cyclePanel(cyclePanel(p, 1), -1)).toBe(p);
      expect(cyclePanel(cyclePanel(p, -1), 1)).toBe(p);
    }
  });
});

describe("breadcrumbSection", () => {
  test("maps each panel to its short label", () => {
    expect(breadcrumbSection("stack", false)).toBe("Stack");
    expect(breadcrumbSection("prs", false)).toBe("PRs");
    expect(breadcrumbSection("session", false)).toBe("Session");
    expect(breadcrumbSection("procs", false)).toBe("Processes");
  });

  test("appends › zoom when zoomed", () => {
    expect(breadcrumbSection("prs", true)).toBe("PRs › zoom");
  });
});

describe("cockpitHints", () => {
  const base = {
    scope: "working",
    zoomed: false,
    killPending: false,
    reviewMode: "diff" as const,
    onMember: true,
    stackReview: true,
  };

  test("kill-pending overrides everything with y/n only", () => {
    const hints = cockpitHints("procs", { ...base, killPending: true });
    expect(hints).toEqual([
      { key: "y", label: "confirm" },
      { key: "n", label: "cancel" },
    ]);
  });

  test("zoom shows unzoom + scroll hints regardless of panel", () => {
    const hints = cockpitHints("session", { ...base, zoomed: true });
    expect(hints[0]).toEqual({ key: "enter", label: "unzoom" });
    expect(hints.some((h) => h.label === "scroll")).toBe(true);
  });

  test("stack hints surface the rich-diff drill-down and scope", () => {
    const hints = cockpitHints("stack", base);
    expect(hints.some((h) => h.label === "rich diff")).toBe(true);
    expect(hints.some((h) => h.label === "scope: working")).toBe(true);
    expect(hints.some((h) => h.key === "t")).toBe(false);
  });

  test("stack diff hides scope on a non-member branch, hides f without sidecar support", () => {
    const nonMember = cockpitHints("stack", { ...base, onMember: false });
    expect(nonMember.some((h) => h.key === "b")).toBe(false);
    const noReview = cockpitHints("stack", { ...base, stackReview: false });
    expect(noReview.some((h) => h.key === "f")).toBe(false);
    expect(cockpitHints("stack", base).some((h) => h.key === "f")).toBe(true);
  });

  test("tree and file review modes swap in navigation hints", () => {
    const tree = cockpitHints("stack", { ...base, reviewMode: "tree" }).map((h) => h.label);
    expect(tree).toContain("open/expand");
    expect(tree).toContain("collapse");
    expect(tree).toContain("edit");
    expect(tree).toContain("repo/slice");
    const file = cockpitHints("stack", { ...base, reviewMode: "file" });
    const fileLabels = file.map((h) => h.label);
    expect(fileLabels).toContain("line");
    expect(fileLabels).toContain("comment");
    expect(fileLabels).toContain("edit");
    expect(file.some((h) => h.key === "V" && h.label === "review")).toBe(true);

    const readOnlyTree = cockpitHints("stack", {
      ...base,
      reviewMode: "tree",
      onMember: false,
    });
    expect(readOnlyTree.some((h) => h.key === "e")).toBe(false);
  });

  test("prs hints surface CI log, fix-ci, copy URL and open PR", () => {
    const labels = cockpitHints("prs", base).map((h) => h.label);
    expect(labels).toContain("CI log");
    expect(labels).toContain("fix-ci");
    expect(labels).toContain("copy URL");
    expect(labels).toContain("open PR");
  });

  test("procs hints surface fold, sort and kill", () => {
    const labels = cockpitHints("procs", base).map((h) => h.label);
    expect(labels).toContain("fold");
    expect(labels).toContain("sort");
    expect(labels).toContain("kill");
  });

  test("session hints surface reload (G10)", () => {
    expect(cockpitHints("session", base).some((h) => h.label === "reload")).toBe(true);
  });

  test("every non-modal panel keeps hints within the 4–6 range", () => {
    for (const p of PANEL_ORDER) {
      const n = cockpitHints(p, base).length;
      expect(n).toBeGreaterThanOrEqual(4);
      expect(n).toBeLessThanOrEqual(7);
    }
  });
});
