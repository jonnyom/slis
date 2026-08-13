import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { useDelayedSessionAttachment } from "./attachment";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

function AttachmentState({ shown, suspendable }: { shown: boolean; suspendable: boolean }) {
  const attached = useDelayedSessionAttachment(shown, suspendable, 20);
  return <text>{attached ? "attached" : "detached"}</text>;
}

test("hidden session attachment suspends and resumes", async () => {
  setup = await createTestRenderer({ width: 20, height: 2 });
  root = createRoot(setup.renderer);
  const renderState = (shown: boolean) =>
    flushSync(() => root!.render(<AttachmentState shown={shown} suspendable />));

  renderState(true);
  await setup.flush();
  expect(setup.captureCharFrame()).toContain("attached");

  renderState(false);
  await Bun.sleep(40);
  await setup.flush();
  expect(setup.captureCharFrame()).toContain("detached");

  renderState(true);
  await setup.flush();
  expect(setup.captureCharFrame()).toContain("attached");
});

test("quick session tab switching keeps the attachment alive", async () => {
  setup = await createTestRenderer({ width: 20, height: 2 });
  root = createRoot(setup.renderer);
  const renderState = (shown: boolean) =>
    flushSync(() => root!.render(<AttachmentState shown={shown} suspendable />));

  renderState(true);
  renderState(false);
  await Bun.sleep(5);
  renderState(true);
  await Bun.sleep(30);
  await setup.flush();

  expect(setup.captureCharFrame()).toContain("attached");
});

test("hidden command terminals stay attached", async () => {
  setup = await createTestRenderer({ width: 20, height: 2 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(<AttachmentState shown={false} suspendable={false} />));
  await Bun.sleep(40);
  await setup.flush();

  expect(setup.captureCharFrame()).toContain("attached");
});
