import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { SelectionClipboard } from "./selection-clipboard";
import { TerminalLink } from "./terminal-link";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("clicking a TUI hyperlink opens its URL", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  const url = "https://github.com/Noryai/nory/pull/8584";
  flushSync(() =>
    root!.render(
      <>
        <SelectionClipboard onOpen={(openedUrl) => openedUrls.push(openedUrl)} />
        <TerminalLink url={url} />
      </>,
    ),
  );
  await setup.flush();

  await setup.mockMouse.click(5, 0);

  expect(openedUrls).toEqual([url]);
});

test("dragging across a TUI hyperlink selects without opening it", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  const url = "https://github.com/Noryai/nory/pull/8584";
  flushSync(() =>
    root!.render(
      <>
        <SelectionClipboard onOpen={(openedUrl) => openedUrls.push(openedUrl)} />
        <TerminalLink url={url} />
      </>,
    ),
  );
  await setup.flush();

  await setup.mockMouse.drag(0, 0, 10, 0);

  expect(openedUrls).toEqual([]);
  expect(setup.renderer.hasSelection).toBe(true);
});
