import { afterEach, describe, expect, test } from "bun:test";
import { BoxRenderable } from "@opentui/core";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { EmbeddedTerminalRenderable } from "./embedded";
import {
  EMBEDDED_TERMINAL_BACKGROUND,
  EMBEDDED_TERMINAL_SELECTABLE,
} from "./tabs";

let setup: TestRendererSetup | null = null;

afterEach(() => {
  setup?.renderer.destroy();
  setup = null;
});

describe("embedded terminal selection", () => {
  test("mouse drags select terminal text", async () => {
    setup = await createTestRenderer({ width: 40, height: 12 });
    const terminal = new GhosttyTerminalRenderable(setup.renderer, {
      ansi: "alpha\nbeta\ngamma",
      width: 40,
      height: 12,
      selectable: EMBEDDED_TERMINAL_SELECTABLE,
    });
    setup.renderer.root.add(terminal);
    await setup.flush();
    await setup.mockMouse.drag(0, 0, 5, 0);

    expect(setup.renderer.hasSelection).toBe(true);
    expect(setup.renderer.getSelection()?.getSelectedText()).toBe("alpha");
  });

  test("uses an opaque background to clear cells after reflow", async () => {
    setup = await createTestRenderer({ width: 40, height: 12 });
    const terminal = new GhosttyTerminalRenderable(setup.renderer, {
      width: 40,
      height: 12,
      bg: EMBEDDED_TERMINAL_BACKGROUND,
    });
    setup.renderer.root.add(terminal);

    expect(terminal.bg.a).toBe(1);
  });

  test("copies text from the visible scrollback rows", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
      limit: 500,
      limitFromEnd: true,
      selectable: EMBEDDED_TERMINAL_SELECTABLE,
      onMouseScroll: (event) => event.stopPropagation(),
    });
    setup.renderer.root.add(terminal);
    terminal.feed(Array.from({ length: 30 }, (_, index) => `line-${index}\r\n`).join(""));
    await setup.renderOnce();
    await setup.renderOnce();
    await setup.mockMouse.scroll(2, 2, "up");
    await setup.renderOnce();
    const firstVisibleLine = setup.captureCharFrame().split("\n")[0]!.trimEnd();

    await setup.mockMouse.drag(0, 0, firstVisibleLine.length, 0);

    expect(setup.renderer.getSelection()?.getSelectedText()).toBe(firstVisibleLine);
  });

  test("copies text from a terminal inside an offset pane", async () => {
    setup = await createTestRenderer({ width: 60, height: 6 });
    const pane = new BoxRenderable(setup.renderer, {
      position: "absolute",
      left: 20,
      top: 1,
      width: 40,
      height: 4,
    });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      ansi: "prefix intended suffix",
      width: 40,
      height: 4,
      cols: 40,
      rows: 4,
      selectable: EMBEDDED_TERMINAL_SELECTABLE,
    });
    pane.add(terminal);
    setup.renderer.root.add(pane);
    await setup.flush();

    await setup.mockMouse.drag(27, 1, 35, 1);

    expect(setup.renderer.getSelection()?.getSelectedText()).toBe("intended");
  });
});
