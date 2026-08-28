import { afterEach, expect, mock, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import type { OverlayApi } from "../overlays/useOverlays";
import { FakeRpcClient } from "../rpc/fake";
import type { DiffResult, DiffScope } from "../rpc/types";
import type { SliceView } from "../state/derive";
import * as tmux from "../term/tmux";

mock.module("../term/tmux", () => ({
  ...tmux,
  listTmuxSessions: () => Promise.resolve([]),
}));

const { Cockpit } = await import("./cockpit");

let setup: TestRendererSetup | null = null;
let root: Root | null = null;
let client: FakeRpcClient | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  client?.close();
  root = null;
  setup = null;
  client = null;
});

class ChangingDiffClient extends FakeRpcClient {
  private diffRequest = 0;

  get diffRequests(): number {
    return this.diffRequest;
  }

  override diff(_params: {
    slice: string;
    scope: DiffScope;
    format: "stat" | "patch" | "both";
  }): Promise<DiffResult> {
    this.diffRequest += 1;
    const path = this.diffRequest === 1 ? "before.ts" : "after.ts";
    return Promise.resolve({
      repos: [
        {
          repo: "nory",
          branch: "isaac/feature",
          stat: { files: [{ path, added: 1, deleted: 0 }], added: 1, deleted: 0 },
        },
      ],
    });
  }
}

const view: SliceView = {
  slice: {
    name: "feature",
    base: "main",
    active: false,
    stale: false,
    members: [
      {
        repo: "nory",
        branch: "isaac/feature",
        worktree_path: "/tmp/nory-feature",
        tip_sha: "0123456789abcdef",
      },
    ],
  },
  status: "running",
  prs: [],
};

const overlays = {
  active: false,
  node: null,
  view: "cockpit",
} as OverlayApi;

async function renderCockpit(nextClient: FakeRpcClient): Promise<void> {
  client = nextClient;
  setup = await createTestRenderer({ width: 120, height: 30 });
  root = createRoot(setup.renderer);

  flushSync(() =>
    root!.render(
      <Cockpit
        enabled
        client={nextClient}
        view={view}
        overlays={overlays}
        width={120}
        height={30}
        gatherable={false}
        knownSlices={["feature"]}
        agents={[]}
        onBack={() => {}}
        onOpenTerm={() => {}}
        onOpenExistingSession={() => {}}
        onConfigureAgents={() => {}}
        onToggleProcs={() => {}}
        onRefresh={() => {}}
        onRefreshSlice={() => Promise.resolve()}
        onQuit={() => {}}
      />,
    ),
  );
  await Promise.resolve();
  await setup.flush();
}

test("visible stack summary polls changed-file stats", async () => {
  await renderCockpit(new ChangingDiffClient());

  expect(setup!.captureCharFrame()).toContain("before.ts");

  await Bun.sleep(5_100);
  await setup!.flush();

  expect(setup!.captureCharFrame()).toContain("after.ts");
}, 7_000);

test("r refreshes changed-file stats immediately", async () => {
  const changingClient = new ChangingDiffClient();
  await renderCockpit(changingClient);

  expect(setup!.captureCharFrame()).toContain("before.ts");

  setup!.mockInput.pressKey("r");
  await Bun.sleep(20);
  await setup!.flush();

  expect(changingClient.diffRequests).toBe(2);
  expect(setup!.captureCharFrame()).toContain("after.ts");
});
