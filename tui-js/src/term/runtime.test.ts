import { expect, test } from "bun:test";

import { applySessionRuntimeLabels } from "./runtime";
import type { SessionGroup } from "./slis";
import type { TabEntry } from "./tabs";

test("runtime labels replace stale launch metadata", () => {
  const tab = {
    kind: "session",
    slice: "feature",
    opts: {
      slice: "feature",
      kind: "agent",
      tabID: "root",
      tabTitle: "root",
      members: [],
      active: false,
      wsRoot: "/work",
      sessionOpts: {},
      launchAgent: false,
      agent: "claude",
      harness: "claude",
      agentLabel: "Claude Code",
    },
  } satisfies TabEntry;
  const groups: SessionGroup[] = [{
    id: "feature",
    active_tab_id: "root",
    tabs: [{ id: "root", kind: "root", title: "root", cwd: "/work", agent: "Codex", label: "Codex" }],
  }];

  const updated = applySessionRuntimeLabels([tab], groups);
  expect(updated[0]?.kind === "session" && updated[0].opts.runtimeLabel).toBe("Codex");
});
