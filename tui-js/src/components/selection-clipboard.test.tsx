import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { SelectionClipboard } from "./selection-clipboard";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("copies selected TUI text to the system clipboard", async () => {
  setup = await createTestRenderer({ width: 40, height: 4 });
  root = createRoot(setup.renderer);
  const copiedText: string[] = [];
  setup.renderer.copyToClipboardOSC52 = (text) => {
    copiedText.push(text);
    return true;
  };
  flushSync(() =>
    root!.render(
      <>
        <SelectionClipboard />
        <text selectable>alpha beta</text>
      </>,
    ),
  );
  await setup.flush();

  await setup.mockMouse.drag(0, 0, 5, 0);

  expect(copiedText).toEqual(["alpha"]);
});

test("opens a visible URL clicked anywhere in the TUI", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  flushSync(() =>
    root!.render(
      <>
        <SelectionClipboard onOpen={(url) => openedUrls.push(url)} />
        <text selectable>Visit https://example.com/docs</text>
      </>,
    ),
  );
  await setup.flush();

  await setup.mockMouse.click(10, 0);

  expect(openedUrls).toEqual(["https://example.com/docs"]);
});

test("opens a visible URL clicked in an embedded terminal", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  flushSync(() =>
    root!.render(
      <SelectionClipboard onOpen={(url) => openedUrls.push(url)} />,
    ),
  );
  const terminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "Open https://example.com/docs",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
  });
  setup.renderer.root.add(terminal);
  await setup.flush();

  await setup.mockMouse.click(12, 0);

  expect(openedUrls).toEqual(["https://example.com/docs"]);
});
