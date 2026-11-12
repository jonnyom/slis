import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, useKeyboard, type Root } from "@opentui/react";
import { useState } from "react";
import { TerminalLayer, type TabEntry } from "./tabs";
import { TermManager } from "./manager";
import { DOCK_TYPING_BURST_MILLISECONDS } from "./typing-intent";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

function KeyboardOwner({ onKey }: { onKey: (name: string) => void }) {
  useKeyboard((key) => onKey(key.name));
  return null;
}

function sessionEntry(): TabEntry {
  return {
    kind: "session",
    slice: "feature",
    opts: {
      slice: "feature",
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
  };
}

async function renderTypingDock({ shown = true }: { shown?: boolean } = {}) {
  setup = await createTestRenderer({ width: 120, height: 24, kittyKeyboard: true });
  root = createRoot(setup.renderer);
  const terminalWrites: string[] = [];
  const slisKeys: string[] = [];
  let focusCount = 0;
  const session = {
    async attach() {},
    write(sequence: string) {
      terminalWrites.push(sequence);
    },
    paste() {},
    resize() {},
    onExit() {
      return () => {};
    },
    detach() {},
  };
  const manager = {
    session: () => session,
    get: () => session,
    sessionApplicationHandlesMouse: () => false,
    detach() {},
  } as unknown as TermManager;
  const entry = sessionEntry();
  let setDockShown: ((dockShown: boolean) => void) | undefined;
  function TypingDockHarness() {
    const [dockShown, setDockShownState] = useState(shown);
    setDockShown = setDockShownState;
    return (
      <>
        <KeyboardOwner onKey={(name) => slisKeys.push(name)} />
        <TerminalLayer
          tabs={[entry]}
          active="session:feature:agent"
          shown={dockShown}
          focused={false}
          statuses={{}}
          width={60}
          height={24}
          left={60}
          manager={manager}
          onBack={() => {}}
          onUnfocus={() => {}}
          onSelectTab={() => {}}
          onFocus={() => focusCount++}
          onSessionExit={() => {}}
          onCommandExit={() => {}}
        />
      </>
    );
  }
  flushSync(() => root!.render(<TypingDockHarness />));
  await setup.flush();
  return {
    terminalWrites,
    slisKeys,
    focusCount: () => focusCount,
    setShown: async (dockShown: boolean) => {
      flushSync(() => setDockShown!(dockShown));
      await setup!.flush();
    },
  };
}

test("rapid typing focuses the visible dock and sends buffered input to its terminal", async () => {
  const result = await renderTypingDock();

  await setup!.mockInput.typeText("hel", 20);
  await setup!.flush();

  expect(result.focusCount()).toBe(1);
  expect(result.terminalWrites).toEqual(["h", "e", "l"]);
  expect(result.slisKeys).toEqual([]);
});

test("continuous typing focuses the visible dock when each key arrives quickly", async () => {
  const result = await renderTypingDock();

  await setup!.mockInput.typeText("hello", 100);
  await setup!.flush();

  expect(result.focusCount()).toBe(1);
  expect(result.terminalWrites).toEqual(["h", "e", "l", "l", "o"]);
  expect(result.slisKeys).toEqual([]);
});

test("one printable shortcut returns to Slis after the typing window", async () => {
  const result = await renderTypingDock();

  setup!.mockInput.pressKey("r");
  expect(result.slisKeys).toEqual([]);
  await Bun.sleep(DOCK_TYPING_BURST_MILLISECONDS + 20);
  await setup!.flush();

  expect(result.focusCount()).toBe(0);
  expect(result.terminalWrites).toEqual([]);
  expect(result.slisKeys).toEqual(["r"]);
});

test("rapid navigation keys remain Slis commands", async () => {
  const result = await renderTypingDock();

  await setup!.mockInput.typeText("jkl", 20);
  await Bun.sleep(DOCK_TYPING_BURST_MILLISECONDS + 20);
  await setup!.flush();

  expect(result.focusCount()).toBe(0);
  expect(result.terminalWrites).toEqual([]);
  expect(result.slisKeys).toEqual(["j", "k", "l"]);
});

test("modified keys bypass dock typing detection", async () => {
  const result = await renderTypingDock();

  setup!.mockInput.pressKey("x", { ctrl: true });
  await setup!.flush();

  expect(result.focusCount()).toBe(0);
  expect(result.terminalWrites).toEqual([]);
  expect(result.slisKeys).toEqual(["x"]);
});

test("a hidden dock does not capture rapid typing", async () => {
  const result = await renderTypingDock({ shown: false });

  await setup!.mockInput.typeText("hel", 20);
  await setup!.flush();

  expect(result.focusCount()).toBe(0);
  expect(result.terminalWrites).toEqual([]);
  expect(result.slisKeys).toEqual(["h", "e", "l"]);
});

test("hiding the dock discards a pending typing decision", async () => {
  const result = await renderTypingDock();

  setup!.mockInput.pressKey("r");
  await result.setShown(false);
  await Bun.sleep(DOCK_TYPING_BURST_MILLISECONDS + 20);
  await setup!.flush();

  expect(result.focusCount()).toBe(0);
  expect(result.terminalWrites).toEqual([]);
  expect(result.slisKeys).toEqual([]);
});
