import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { SessionCloseConfirmation } from "./sessionoverlay";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("session close confirmation is a centered modal", async () => {
  setup = await createTestRenderer({ width: 120, height: 30 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(<SessionCloseConfirmation target="slis/checkout" />));
  await setup.flush();

  const dialog = setup.renderer.root.findDescendantById("session-close-confirmation");
  expect(dialog).toBeDefined();
  expect(dialog!.width).toBeLessThan(120);
  expect(dialog!.x).toBe((120 - dialog!.width) / 2);
  expect(Math.abs(dialog!.y - (30 - dialog!.height) / 2)).toBeLessThanOrEqual(0.5);
});
