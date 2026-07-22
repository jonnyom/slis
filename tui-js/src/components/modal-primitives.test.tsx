import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { Card } from "./card";
import { KillConfirm } from "./procview";
import { TextField } from "./textfield";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("card separates subtitle metadata from its content", async () => {
  setup = await createTestRenderer({ width: 80, height: 20 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <Card title="Dialog" subtitle="repo · branch" width={60}>
      <text id="modal-body">body</text>
    </Card>,
  ));
  await setup.flush();

  const subtitle = setup.renderer.root.findDescendantById("modal-subtitle");
  const body = setup.renderer.root.findDescendantById("modal-body");
  expect(subtitle).toBeDefined();
  expect(body!.y).toBeGreaterThan(subtitle!.y + subtitle!.height);
});

test("text field gives its label and value distinct structure", async () => {
  setup = await createTestRenderer({ width: 80, height: 20 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <TextField id="slice-name" label="New slice name" lines={["checkout"]} />,
  ));
  await setup.flush();

  const label = setup.renderer.root.findDescendantById("slice-name-label");
  const control = setup.renderer.root.findDescendantById("slice-name-control");
  expect(label).toBeDefined();
  expect(control).toBeDefined();
  expect(control!.height).toBeGreaterThan(1);
  expect(control!.y).toBe(label!.y + label!.height);
});

test("process termination uses a centered confirmation dialog", async () => {
  setup = await createTestRenderer({ width: 100, height: 24 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <KillConfirm target={{ pid: 42, cmd: "worker --run", subtree: false }} />,
  ));
  await setup.flush();

  const dialog = setup.renderer.root.findDescendantById("process-kill-confirmation");
  expect(dialog).toBeDefined();
  expect(dialog!.x).toBe((100 - dialog!.width) / 2);
});
