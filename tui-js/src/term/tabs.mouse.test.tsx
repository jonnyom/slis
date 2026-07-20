import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { TabBar } from "./tabs";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("clicking the terminal back control leaves the terminal", async () => {
  setup = await createTestRenderer({ width: 80, height: 4 });
  root = createRoot(setup.renderer);
  let backCount = 0;
  flushSync(() =>
    root!.render(
      <TabBar tabs={[]} active={null} statuses={{}} onBack={() => backCount++} />,
    ),
  );
  await setup.flush();

  const back = setup.renderer.root.findDescendantById("term-back");
  expect(back).toBeDefined();
  await setup.mockMouse.click(back!.screenX, back!.screenY);
  expect(backCount).toBe(1);
});
