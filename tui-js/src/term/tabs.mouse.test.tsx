import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { TabBar, TerminalLayer } from "./tabs";
import { TermManager } from "./manager";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("clicking the terminal back control leaves the terminal", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  let backCount = 0;
  flushSync(() =>
    root!.render(
      <TabBar
        tabs={[]}
        active={null}
        statuses={{}}
        onBack={() => backCount++}
        onSelectTab={() => {}}
        onFocus={() => {}}
      />,
    ),
  );
  await setup.flush();

  const back = setup.renderer.root.findDescendantById("term-back");
  expect(back).toBeDefined();
  await setup.mockMouse.click(back!.screenX, back!.screenY);
  expect(backCount).toBe(1);
});

test("clicking dock hide removes the dock without closing its session", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  let hideCount = 0;
  flushSync(() =>
    root!.render(
      <TabBar
        tabs={[]}
        active={null}
        statuses={{}}
        onBack={() => {}}
        onHide={() => hideCount++}
        onSelectTab={() => {}}
        onFocus={() => {}}
      />,
    ),
  );
  await setup.flush();

  const hide = setup.renderer.root.findDescendantById("term-hide");
  expect(hide).toBeDefined();
  await setup.mockMouse.click(hide!.screenX, hide!.screenY);
  expect(hideCount).toBe(1);
});

test("clicking a terminal tab selects and focuses it", async () => {
  setup = await createTestRenderer({ width: 100, height: 4 });
  root = createRoot(setup.renderer);
  let selected = "";
  let focusCount = 0;
  const entry = {
    kind: "session" as const,
    slice: "feature",
    opts: {
      slice: "feature",
      kind: "agent" as const,
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
  };
  flushSync(() =>
    root!.render(
      <TabBar
        tabs={[entry]}
        active={null}
        statuses={{}}
        onBack={() => {}}
        onSelectTab={(key) => { selected = key; }}
        onFocus={() => focusCount++}
      />,
    ),
  );
  await setup.flush();

  const tab = setup.renderer.root.findDescendantById("term-tab-session:feature:agent");
  expect(tab).toBeDefined();
  await setup.mockMouse.click(tab!.screenX, tab!.screenY);
  expect(selected).toBe("session:feature:agent");
  expect(focusCount).toBe(1);
});

test("terminal tabs share a compact rail with an independent close control", async () => {
  setup = await createTestRenderer({ width: 100, height: 6 });
  root = createRoot(setup.renderer);
  let selected = "";
  let closed = "";
  const entry = {
    kind: "session" as const,
    slice: "feature",
    opts: {
      slice: "feature",
      kind: "agent" as const,
      tabID: "agent",
      tabTitle: "claude",
      members: [],
      active: false,
      wsRoot: "/workspace",
      sessionOpts: {},
      launchAgent: false,
      agent: "",
      harness: "claude",
    },
  };
  flushSync(() =>
    root!.render(
      <TabBar
        tabs={[entry]}
        active="session:feature:agent"
        statuses={{}}
        onBack={() => {}}
        onSelectTab={(key) => { selected = key; }}
        onCloseTab={(key) => { closed = key; }}
        onFocus={() => {}}
      />,
    ),
  );
  await setup.flush();

  const frame = setup.captureCharFrame();
  expect(frame).toContain("TERM");
  expect(frame).toContain("▎");
  expect(frame).toContain("─");
  expect(frame).not.toContain("╭");
  expect(frame).not.toContain("╯");
  const close = setup.renderer.root.findDescendantById("term-close-session:feature:agent");
  expect(close).toBeDefined();
  await setup.mockMouse.click(close!.screenX, close!.screenY);
  expect(closed).toBe("session:feature:agent");
  expect(selected).toBe("");
});

test("ctrl+q leaves the focused terminal", async () => {
  setup = await createTestRenderer({ width: 80, height: 24, kittyKeyboard: true });
  root = createRoot(setup.renderer);
  let backCount = 0;
  flushSync(() =>
    root!.render(
      <TerminalLayer
        tabs={[]}
        active={null}
        shown
        focused
        statuses={{}}
        width={80}
        height={24}
        left={0}
        manager={new TermManager()}
        onBack={() => backCount++}
        onSelectTab={() => {}}
        onFocus={() => {}}
        onSessionExit={() => {}}
        onCommandExit={() => {}}
      />,
    ),
  );
  await setup.flush();

  setup.mockInput.pressKey("q", { ctrl: true });
  await setup.flush();

  expect(backCount).toBe(1);
});

test("clicking a visible terminal pane focuses it", async () => {
  setup = await createTestRenderer({ width: 80, height: 24 });
  root = createRoot(setup.renderer);
  let focusCount = 0;
  const manager = new TermManager();
  flushSync(() =>
    root!.render(
      <TerminalLayer
        tabs={[{ kind: "command", id: "cmd:test", title: "test", argv: ["/bin/sleep", "1"], exited: false }]}
        active="cmd:test"
        shown
        focused={false}
        statuses={{}}
        width={80}
        height={24}
        left={0}
        manager={manager}
        onBack={() => {}}
        onSelectTab={() => {}}
        onFocus={() => focusCount++}
        onSessionExit={() => {}}
        onCommandExit={() => {}}
      />,
    ),
  );
  await setup.flush();

  const pane = setup.renderer.root.findDescendantById("term-pane-cmd:test");
  expect(pane).toBeDefined();
  await setup.mockMouse.click(pane!.screenX + 2, pane!.screenY + 2);
  expect(focusCount).toBe(1);
});
