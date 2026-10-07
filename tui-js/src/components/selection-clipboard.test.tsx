import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { EmbeddedTerminalRenderable } from "../term/embedded";
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

test("opens an OSC 8 link clicked in an embedded terminal", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  flushSync(() =>
    root!.render(
      <SelectionClipboard onOpen={(url) => openedUrls.push(url)} />,
    ),
  );
  const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    persistent: true,
  });
  setup.renderer.root.add(terminal);
  terminal.feed("Open \x1b]8;;https://example.com/hidden\x1b\\documentation\x1b]8;;\x1b\\");
  await setup.flush();

  await setup.mockMouse.click(8, 0);

  expect(openedUrls).toEqual(["https://example.com/hidden"]);
});

test("opens a URL from the visible row after terminal output scrolls", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const openedUrls: string[] = [];
  flushSync(() => root!.render(
    <SelectionClipboard onOpen={(url) => openedUrls.push(url)} />,
  ));
  const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    persistent: true,
    limit: 500,
    limitFromEnd: true,
    selectable: true,
    zIndex: 100,
  });
  setup.renderer.root.add(terminal);
  terminal.feed(Array.from({ length: 20 }, (_, index) =>
    `Open https://example.com/row-${index}\r\n`,
  ).join(""));
  await setup.renderOnce();
  await setup.renderOnce();
  await setup.mockMouse.scroll(2, 2, "up");
  await setup.renderOnce();
  const visibleUrl = setup.captureCharFrame().split("\n")[0]!.trim().slice(5);
  expect(terminal.scrollY).toBeGreaterThan(0);

  await setup.mockMouse.click(12, 0);

  expect(openedUrls).toEqual([visibleUrl]);
});

test("copies only the top terminal when selectable panes overlap", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
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
        <text selectable position="absolute" top={0} left={0}>hidden-prefix</text>
      </>,
    ),
  );
  const terminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "https://github.com/Noryai/nory/pull/9649",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    position: "absolute",
    top: 0,
    left: 0,
    zIndex: 100,
  });
  setup.renderer.root.add(terminal);
  await setup.flush();

  await setup.mockMouse.drag(0, 0, 42, 0);

  expect(copiedText).toEqual(["https://github.com/Noryai/nory/pull/9649"]);
});

test("copies only the visible terminal when hidden tabs share its layer", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const copiedText: string[] = [];
  setup.renderer.copyToClipboardOSC52 = (text) => {
    copiedText.push(text);
    return true;
  };
  flushSync(() => root!.render(<SelectionClipboard />));
  const hiddenTerminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "unrelated hidden terminal output",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    visible: false,
    zIndex: 100,
  });
  const visibleTerminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "intended visible terminal output",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    zIndex: 100,
  });
  setup.renderer.root.add(hiddenTerminal);
  setup.renderer.root.add(visibleTerminal);
  await setup.flush();

  await setup.mockMouse.drag(0, 0, 8, 0);

  expect(copiedText).toEqual(["intended"]);
});

test("ignores a terminal after its tab becomes hidden", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const copiedText: string[] = [];
  setup.renderer.copyToClipboardOSC52 = (text) => {
    copiedText.push(text);
    return true;
  };
  flushSync(() => root!.render(<SelectionClipboard />));
  const hiddenTerminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "unrelated hidden terminal output",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    visible: true,
    zIndex: 100,
  });
  const visibleTerminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "intended visible terminal output",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    visible: false,
    zIndex: 100,
  });
  setup.renderer.root.add(hiddenTerminal);
  setup.renderer.root.add(visibleTerminal);
  await setup.flush();
  hiddenTerminal.visible = false;
  visibleTerminal.visible = true;
  await setup.flush();

  await setup.mockMouse.drag(0, 0, 8, 0);

  expect(copiedText).toEqual(["intended"]);
});

test("copies terminal cells from the visible selection bounds", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  const copiedText: string[] = [];
  setup.renderer.copyToClipboardOSC52 = (text) => {
    copiedText.push(text);
    return true;
  };
  flushSync(() => root!.render(<SelectionClipboard />));
  const terminal = new GhosttyTerminalRenderable(setup.renderer, {
    ansi: "unrelated prior output\r\nThe direct delete failed\r\nMySQL outage\r\nNo DELETE was issued",
    width: 80,
    height: 4,
    cols: 80,
    rows: 4,
    selectable: true,
    zIndex: 100,
  });
  terminal.getSelectedText = () =>
    "unrelated prior output\nThe direct delete failed\nMySQL outage\nNo DELETE was issued";
  setup.renderer.root.add(terminal);
  await setup.flush();

  await setup.mockMouse.drag(0, 1, 20, 3);

  expect(copiedText).toEqual([
    "The direct delete failed\nMySQL outage\nNo DELETE was issued",
  ]);
});
