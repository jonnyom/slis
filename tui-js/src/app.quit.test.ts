import { expect, test } from "bun:test";

test.each(["\x03", "\x1b[99;5u"])("Ctrl+C %j exits through an overlay when reset fails", async (sequence) => {
  const child = Bun.spawn([process.execPath, "-e", `
    import { createElement } from "react";
    import { createTestRenderer } from "@opentui/core/testing";
    import { createRoot, flushSync } from "@opentui/react";
    import { App } from "./src/app";
    const setup = await createTestRenderer({ width: 100, height: 30, kittyKeyboard: true });
    const root = createRoot(setup.renderer);
    flushSync(() => root.render(createElement(App, { initialPrefs: {} })));
    await Bun.sleep(150);
    await setup.flush();
    await setup.mockInput.pressKey("?");
    process.env.SLIS_FAKE = "0";
    setTimeout(() => process.exit(99), 2000);
    setup.renderer.stdin.emit("data", Buffer.from(${JSON.stringify(sequence)}));
  `], {
    cwd: new URL("..", import.meta.url).pathname,
    env: { ...process.env, SLIS_FAKE: "1", SLIS_BIN: "/usr/bin/false" },
    stdout: "pipe",
    stderr: "pipe",
  });
  const [code, stderr] = await Promise.all([child.exited, new Response(child.stderr).text()]);
  expect(code).toBe(1);
  expect(stderr).toContain("Live slice was not reset");
});
