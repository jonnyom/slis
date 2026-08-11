import { afterEach, describe, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
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
});
