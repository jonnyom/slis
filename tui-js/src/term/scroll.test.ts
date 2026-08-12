import { afterEach, describe, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { EmbeddedTerminalRenderable } from "./embedded";

let setup: TestRendererSetup | null = null;

afterEach(() => {
  setup?.renderer.destroy();
  setup = null;
});

describe("embedded terminal scrolling", () => {
  test("renders consecutive agent lines at column zero", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
    });
    setup.renderer.root.add(terminal);

    terminal.feed("SLIS_CODEX_FIRST\nSLIS_CODEX_SECOND\n");

    expect(
      terminal
        .getText()
        .split("\n")
        .filter((line) => line.includes("SLIS_CODEX_")),
    ).toEqual(["SLIS_CODEX_FIRST", "SLIS_CODEX_SECOND"]);
  });

  test("returns to column zero across a line feed", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
    });
    setup.renderer.root.add(terminal);

    terminal.feed("\x1b[6G\nX");

    expect(terminal.getText().split("\n")[1]).toBe("X");
  });

  test("forwards the wheel without scrolling retained Ghostty history", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    let forwardedWheelEvents = 0;
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
      onMouseScroll: (event) => {
        forwardedWheelEvents++;
        event.preventDefault();
        event.stopPropagation();
      },
    });
    setup.renderer.root.add(terminal);
    terminal.feed(Array.from({ length: 30 }, (_, index) => `row ${index}\r\n`).join(""));
    await setup.renderOnce();
    expect(terminal.scrollHeight).toBeGreaterThan(terminal.height);
    expect(terminal.scrollY).toBe(0);

    await setup.mockMouse.scroll(5, 3, "down");

    expect(forwardedWheelEvents).toBe(1);
    expect(terminal.scrollY).toBe(0);
  });
});
