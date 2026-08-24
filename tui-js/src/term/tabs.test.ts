import { describe, expect, test } from "bun:test";

import {
  adjacentTabKey,
  dockRefocusAction,
  nextSessionTabID,
  sessionTabKeyForSlice,
  tabBarLabel,
  terminalPresentation,
  tabKey,
  tabLabel,
  type TabEntry,
} from "./tabs";

describe("Slis terminal tabs", () => {
  test("keys and labels a terminal by group and explicit tab", () => {
    const entry = {
      kind: "session",
      slice: "feature",
      opts: {
        slice: "feature",
        kind: "shell",
        tabID: "repo-api",
        tabTitle: "api",
        members: [],
        active: false,
        wsRoot: "/workspace",
        sessionOpts: {},
        launchAgent: false,
        agent: "",
        harness: "claude",
      },
    } satisfies TabEntry;

    expect(tabKey(entry)).toBe("session:feature:repo-api");
    expect(tabLabel(entry)).toBe("feature · api");

    const root = {
      ...entry,
      opts: { ...entry.opts, tabID: "root", tabTitle: "root" },
    } satisfies TabEntry;
    expect(adjacentTabKey([root, entry], tabKey(root), 1)).toBe(tabKey(entry));
    expect(adjacentTabKey([root, entry], tabKey(root), -1)).toBe(tabKey(entry));
    expect(tabBarLabel(entry, [root, entry])).toBe("api");

    const runningCodex = {
      ...root,
      opts: { ...root.opts, runtimeLabel: "Codex" },
    } satisfies TabEntry;
    const idleClaude = {
      ...entry,
      opts: { ...entry.opts, runtimeLabel: "~/nory/web-app" },
    } satisfies TabEntry;
    expect(tabBarLabel(runningCodex, [runningCodex, idleClaude])).toBe("Codex");
    expect(tabBarLabel(idleClaude, [runningCodex, idleClaude])).toBe("~/nory/web-app");
  });

  test("allocates another stable id for repeated agent and shell tabs", () => {
    expect(nextSessionTabID("shell", ["root", "shell", "shell-2"])).toBe("shell-3");
    expect(nextSessionTabID("agent-codex", ["agent-codex", "agent-codex-3"])).toBe("agent-codex-2");
    expect(nextSessionTabID("agent-claude", ["root", "shell"])).toBe("agent-claude");
  });
});

describe("session terminal presentation", () => {
  test("ctrl+g refocuses a visible session dock", () => {
    const dock = {
      kind: "session",
      slice: "checkout",
      opts: {
        slice: "checkout",
        kind: "agent",
        tabID: "agent",
        tabTitle: "agent",
        members: [],
        active: false,
        wsRoot: "/workspace",
        sessionOpts: {},
        launchAgent: false,
        agent: "",
        harness: "claude",
      },
    } satisfies TabEntry;
    const command = {
      kind: "command",
      id: "command:create",
      title: "create",
      argv: [],
      exited: false,
    } satisfies TabEntry;

    expect(dockRefocusAction("g", true, dock)).toEqual({
      activeTab: "session:checkout:agent",
      terminalFocused: true,
    });
    expect(dockRefocusAction("g", true, null)).toBeNull();
    expect(dockRefocusAction("g", false, dock)).toBeNull();
    expect(dockRefocusAction("g", true, command)).toBeNull();
  });

  test("keeps a session dock visible with and without terminal focus", () => {
    expect(terminalPresentation(180, true, true, true)).toEqual({
      shown: true,
      docked: true,
      contentWidth: 105,
      terminalLeft: 105,
      terminalWidth: 75,
    });
    expect(terminalPresentation(180, true, false, false)).toEqual({
      shown: true,
      docked: true,
      contentWidth: 105,
      terminalLeft: 105,
      terminalWidth: 75,
    });
  });

  test("uses full screen for command focus and narrow terminals", () => {
    expect(terminalPresentation(180, true, true, false)).toEqual({
      shown: true,
      docked: false,
      contentWidth: 180,
      terminalLeft: 0,
      terminalWidth: 180,
    });
    expect(terminalPresentation(90, true, false, false).shown).toBe(false);
    expect(terminalPresentation(90, true, true, true)).toEqual({
      shown: true,
      docked: false,
      contentWidth: 90,
      terminalLeft: 0,
      terminalWidth: 90,
    });
  });

  test("gives modal layers the full app viewport", () => {
    expect(terminalPresentation(180, true, false, false, true)).toEqual({
      shown: false,
      docked: false,
      contentWidth: 180,
      terminalLeft: 0,
      terminalWidth: 180,
    });
  });
});

describe("focused slice session tab", () => {
  const session = (slice: string, tabID: string, kind: "agent" | "shell" = "agent"): TabEntry => ({
    kind: "session",
    slice,
    opts: {
      slice,
      kind,
      tabID,
      tabTitle: tabID,
      members: [],
      active: false,
      wsRoot: "/workspace",
      sessionOpts: {},
      launchAgent: false,
      agent: "",
      harness: "claude",
    },
  });

  test("restores the preferred agent for the focused slice", () => {
    const checkoutDefault = session("checkout", "agent");
    const checkoutCodex = session("checkout", "agent-codex");
    const checkoutShell = session("checkout", "shell", "shell");
    const payments = session("payments", "agent");
    const tabs = [checkoutDefault, checkoutCodex, checkoutShell, payments];
    expect(sessionTabKeyForSlice(tabs, "checkout", tabKey(checkoutShell))).toBe(tabKey(checkoutShell));
    expect(sessionTabKeyForSlice(tabs, "payments", tabKey(checkoutShell))).toBe(tabKey(payments));
    expect(sessionTabKeyForSlice(tabs, "missing", tabKey(checkoutShell))).toBeNull();
  });
});
