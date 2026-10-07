import { expect, test } from "bun:test";
import { removeMissingSliceTabs, reconcileTabSelection } from "./slice-tabs";
import { tabKey, type TabEntry } from "./tabs";

function session(slice: string): TabEntry {
  return {
    kind: "session", slice,
    opts: {
      slice, kind: "agent", tabID: "agent", tabTitle: "agent", members: [],
      active: false, wsRoot: "/workspace", sessionOpts: {}, launchAgent: false,
      agent: "claude", harness: "claude",
    },
  };
}

test("removed slice closes its tabs and clears active and docked selection", () => {
  const removed = session("PAY-681");
  const retained = session("PAY-683");
  const remaining = removeMissingSliceTabs([removed, retained], ["PAY-681", "PAY-683"], ["PAY-683"]);
  expect(remaining).toEqual([retained]);
  expect(reconcileTabSelection(remaining, tabKey(removed), tabKey(removed))).toEqual({ active: null, docked: null });
});

test("refresh preserves unrelated commands and newly opened sessions", () => {
  const command: TabEntry = { kind: "command", id: "cmd:1", title: "remove", argv: [], exited: false };
  const tabs = [command, session("new-slice")];
  expect(removeMissingSliceTabs(tabs, ["PAY-681"], [])).toBe(tabs);
});
