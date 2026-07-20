import { afterEach, describe, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { EMBEDDED_TERMINAL_SELECTABLE } from "./tabs";

let setup: TestRendererSetup | null = null;

afterEach(() => {
  setup?.renderer.destroy();
  setup = null;
});

describe("embedded terminal selection", () => {
  test("mouse drags do not create OpenTUI rectangular selections", async () => {
    setup = await createTestRenderer({ width: 40, height: 12 });
    const terminal = new GhosttyTerminalRenderable(setup.renderer, {
      ansi: "alpha\nbeta\ngamma",
      width: 40,
      height: 12,
      selectable: EMBEDDED_TERMINAL_SELECTABLE,
    });
    setup.renderer.root.add(terminal);
    await setup.flush();
    await setup.mockMouse.drag(1, 1, 6, 10);
    expect(setup.renderer.hasSelection).toBe(false);
  });
});
