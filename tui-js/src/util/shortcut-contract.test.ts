import { describe, expect, test } from "bun:test";
import { SHORTCUT_CONTEXTS, shortcutAction, type ShortcutContext } from "./shortcut-contract";

describe("shortcut contract", () => {
  test("a key belongs to only one action within each keyboard context", () => {
    for (const [context, bindings] of Object.entries(SHORTCUT_CONTEXTS)) {
      const owners = new Map<string, string>();
      for (const binding of bindings) {
        for (const key of binding.keys) {
          expect(
            owners.get(key),
            `${context}: ${JSON.stringify(key)} is claimed by both ${owners.get(key)} and ${binding.action}`,
          ).toBeUndefined();
          owners.set(key, binding.action);
        }
      }
    }
  });

  test("global slice actions stay consistent across browser, cockpit, and diff", () => {
    const contexts: ShortcutContext[] = ["browser", "cockpit", "diff"];
    const invariant = {
      a: "attach-agent",
      C: "launch-agent",
      V: "pending-review",
      ",": "configure-agents",
    };
    for (const context of contexts) {
      for (const [key, action] of Object.entries(invariant)) {
        expect(shortcutAction(context, key), `${context} ${key}`).toBe(action);
      }
    }
  });

  test("nested review views preserve terminals, review, and configuration keys", () => {
    for (const context of ["cockpit.file", "cockpit.tree"] as const) {
      expect(shortcutAction(context, "a")).toBe("attach-agent");
      expect(shortcutAction(context, "C")).toBe("launch-agent");
      expect(shortcutAction(context, "t")).toBe("open-shell");
      expect(shortcutAction(context, "V")).toBe("pending-review");
      expect(shortcutAction(context, ",")).toBe("configure-agents");
    }
  });

  test("d is destructive only in slice views and never configures an agent", () => {
    expect(shortcutAction("browser", "d")).toBe("clear-slice");
    expect(shortcutAction("cockpit", "d")).toBe("clear-slice");
    expect(shortcutAction("agent.launch", "d")).toBeUndefined();
    expect(shortcutAction("agent.configure", "d")).toBeUndefined();
  });

  test("launch and configuration are distinct modal contracts", () => {
    expect(shortcutAction("agent.launch", "enter")).toBe("choose");
    expect(shortcutAction("agent.configure", "enter")).toBe("set-default");
    expect(shortcutAction("agent.configure", "1")).toBeUndefined();
  });

  test("the PR panel owns a focused-URL copy action", () => {
    expect(shortcutAction("cockpit.prs", "y")).toBe("copy-pr-url");
  });

  test("protected action keys are resolved through the contract in keyboard owners", async () => {
    const roots = [
      new URL("../views/browser.tsx", import.meta.url),
      new URL("../views/cockpit.tsx", import.meta.url),
      new URL("../components/diffview.tsx", import.meta.url),
    ];
    for (const file of roots) {
      const source = await Bun.file(file).text();
      expect(source).not.toMatch(/name === "(?:C|V|,)"/);
      expect(source).not.toContain('name === "d" && !key.ctrl');
      expect(source).toContain("shortcutAction(");
    }
  });
});
