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

  test("shows the latest terminal rows and scrolls retained history", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
      onMouseScroll: (event) => event.stopPropagation(),
    });
    setup.renderer.root.add(terminal);
    terminal.feed(Array.from({ length: 30 }, (_, index) => `row ${index}\r\n`).join(""));
    await setup.renderOnce();
    await setup.renderOnce();
    expect(terminal.scrollHeight).toBeGreaterThan(terminal.height);
    expect(terminal.scrollY).toBe(terminal.scrollHeight - terminal.height);
    expect(setup.captureCharFrame()).toContain("row 29");

    await setup.mockMouse.scroll(5, 3, "up");

    const scrolledPosition = terminal.scrollY;
    expect(scrolledPosition).toBeLessThan(terminal.scrollHeight - terminal.height);

    terminal.feed("row 30\r\n");
    await setup.renderOnce();

    expect(terminal.scrollY).toBe(scrolledPosition);
  });

  test("does not scroll retained history when the wheel is claimed", async () => {
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
    const initialScrollY = terminal.scrollY;

    await setup.mockMouse.scroll(5, 3, "up");

    expect(forwardedWheelEvents).toBe(1);
    expect(terminal.scrollY).toBe(initialScrollY);
  });

  test("positions the native cursor relative to retained-history scrolling", async () => {
    setup = await createTestRenderer({ width: 40, height: 6 });
    const terminal = new EmbeddedTerminalRenderable(setup.renderer, {
      width: 40,
      height: 6,
      cols: 40,
      rows: 6,
      persistent: true,
      showCursor: true,
      focusable: true,
    });
    setup.renderer.root.add(terminal);
    terminal.focus();
    terminal.feed(Array.from({ length: 30 }, (_, index) => `row ${index}\r\n`).join(""));
    await setup.renderOnce();
    await setup.renderOnce();

    const [cursorColumn, cursorRow] = terminal.getCursor();

    expect(setup.renderer.getCursorState()).toMatchObject({
      x: terminal.x + cursorColumn + 1,
      y: terminal.y + cursorRow + 1,
      visible: true,
    });

    await setup.mockMouse.scroll(2, 2, "up");
    await setup.renderOnce();

    expect(setup.renderer.getCursorState().visible).toBe(false);
  });
});
