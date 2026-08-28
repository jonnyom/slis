import { expect, test } from "bun:test";

import { applySessionRuntimeLabels, SessionRuntimeRefresher } from "./runtime";
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

test("runtime refreshes never overlap and stop cancels the active refresh", async () => {
  const resolvers: Array<(groups: SessionGroup[]) => void> = [];
  const signals: AbortSignal[] = [];
  const applied: SessionGroup[][] = [];
  const errors: unknown[] = [];
  const refresher = new SessionRuntimeRefresher(
    (signal) => {
      signals.push(signal);
      return new Promise<SessionGroup[]>((resolve) => resolvers.push(resolve));
    },
    (groups) => applied.push(groups),
    (error) => errors.push(error),
    1,
  );

  refresher.activity();
  await Bun.sleep(10);
  expect(signals).toHaveLength(1);

  for (let index = 0; index < 20; index += 1) refresher.activity();
  await Bun.sleep(10);
  expect(signals).toHaveLength(1);

  resolvers[0]!([]);
  await Bun.sleep(10);
  expect(signals).toHaveLength(2);
  expect(applied).toEqual([[]]);

  refresher.stop();
  expect(signals[1]?.aborted).toBe(true);
  expect(errors).toEqual([]);
});

test("runtime refresh stops after an unexpected scan failure", async () => {
  const failure = new Error("scan failed");
  const errors: unknown[] = [];
  let loadCount = 0;
  const refresher = new SessionRuntimeRefresher(
    async () => {
      loadCount += 1;
      throw failure;
    },
    () => {},
    (error) => errors.push(error),
    1,
  );

  refresher.activity();
  await Bun.sleep(10);
  refresher.activity();
  await Bun.sleep(10);

  expect(loadCount).toBe(1);
  expect(errors).toEqual([failure]);
});
