import { afterEach, expect, test } from "bun:test";
import { ScrollBoxRenderable } from "@opentui/core";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { DiffView } from "./diffview";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

const longLine = "const result = reconcileEmployeesWithProviderRecordsAndPreserveEveryRelevantFieldForTheNextBatch();";
const patch = [
  "diff --git a/src/reconcile.ts b/src/reconcile.ts",
  "--- a/src/reconcile.ts",
  "+++ b/src/reconcile.ts",
  "@@ -1 +1 @@",
  `-${longLine}`,
  `+${longLine} updated`,
].join("\n");

test("split diff wraps long source lines and has no horizontal scrollbars", async () => {
  setup = await createTestRenderer({ width: 100, height: 18 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <DiffView
      enabled={false}
      repos={[{ repo: "nory", branch: "feature", patch }]}
      scope="working"
      mode="split"
      width={100}
      height={18}
      comments={[]}
      onCycleScope={() => {}}
      onToggleMode={() => {}}
      onClose={() => {}}
      onQuit={() => {}}
      onAttach={() => {}}
      onLaunchAgent={() => {}}
      onConfigureAgents={() => {}}
      onComment={() => {}}
      onReview={() => {}}
    />,
  ));
  await setup.flush();

  const renderables = setup.renderer.root
    .getChildren()
    .flatMap(function collect(renderable): typeof renderable[] {
      const nested = "getChildren" in renderable ? renderable.getChildren() : [];
      return [renderable, ...nested.flatMap(collect)];
    });
  const diffLine = renderables.find((renderable) => renderable.id === "diffline-1");
  expect(renderables.map((renderable) => renderable.id)).toContain("diffline-1");
  expect(diffLine?.height).toBeGreaterThan(1);
  const leftCell = diffLine!.getChildren()[0]!;
  expect(leftCell.getChildren()).toHaveLength(2);
  expect(leftCell.getChildren()[0]!.height).toBe(leftCell.height);
  expect(leftCell.getChildren()[1]!.height).toBeGreaterThan(1);

  const scrollboxes = renderables.filter(
    (renderable): renderable is ScrollBoxRenderable => renderable instanceof ScrollBoxRenderable,
  );
  expect(scrollboxes).toHaveLength(2);
  expect(scrollboxes.every((scrollbox) => !scrollbox.horizontalScrollBar.visible)).toBe(true);
});
