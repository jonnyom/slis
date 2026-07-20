import { describe, expect, test } from "bun:test";
import { stackHelpItems, stackOverlayAction } from "./stack";

describe("stackOverlayAction", () => {
  test("opens contextual help from the stack actions modal", () => {
    expect(stackOverlayAction("?", true)).toBe("context-help");
  });

  test("rejects gather before invoking the mutation for a standalone slice", () => {
    expect(stackOverlayAction("g", false)).toBe("gather-unavailable");
    expect(stackOverlayAction("g", true)).toBe("gather");
  });
});

describe("stackHelpItems", () => {
  test("explains stack actions and their side effects", () => {
    const items = stackHelpItems(true);
    expect(items.map((item) => item.key)).toEqual(["r", "p", "m", "s", "g", "x"]);
    expect(items.find((item) => item.key === "g")?.detail).toContain("worktrees untouched");
    expect(items.find((item) => item.key === "s")?.detail).toContain("delete merged branches");
  });

  test("omits gather for a standalone target", () => {
    expect(stackHelpItems(false).some((item) => item.key === "g")).toBe(false);
  });
});
