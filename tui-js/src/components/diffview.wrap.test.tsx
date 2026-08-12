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

const longLine = "const result = reconcileEmployeesWithProviderRecordsAndPreserveEveryRelevantFieldForTheNextBatchAndKeepTheRenderedTailVisible_END();";
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
      githubComments={[]}
      onCycleScope={() => {}}
      onToggleMode={() => {}}
      onClose={() => {}}
      onQuit={() => {}}
      onAttach={() => {}}
      onLaunchAgent={() => {}}
      onLaunchOtherAgent={() => {}}
      onConfigureAgents={() => {}}
      onComment={() => {}}
      onReview={() => {}}
      onOpenPrComments={() => {}}
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
  expect(setup.captureCharFrame().replace(/\s+/g, "")).toContain("TailVisible_END");

  const scrollboxes = renderables.filter(
    (renderable): renderable is ScrollBoxRenderable => renderable instanceof ScrollBoxRenderable,
  );
  expect(scrollboxes).toHaveLength(2);
  expect(scrollboxes.every((scrollbox) => !scrollbox.horizontalScrollBar.visible)).toBe(true);
});

test("renders a GitHub review comment below its diff line", async () => {
  setup = await createTestRenderer({ width: 100, height: 18 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <DiffView
      enabled={false}
      repos={[{ repo: "nory", branch: "feature", patch }]}
      scope="parent"
      mode="unified"
      width={100}
      height={18}
      comments={[]}
      githubComments={[{
        repo: "nory",
        branch: "feature",
        pr: 42,
        author: "teammate",
        body: "Keep the original value here.",
        url: "https://github.com/acme/repo/pull/42#discussion_r1",
        kind: 2,
        path: "src/reconcile.ts",
        line: 1,
        side: "RIGHT",
      }]}
      onCycleScope={() => {}}
      onToggleMode={() => {}}
      onClose={() => {}}
      onQuit={() => {}}
      onAttach={() => {}}
      onLaunchAgent={() => {}}
      onLaunchOtherAgent={() => {}}
      onConfigureAgents={() => {}}
      onComment={() => {}}
      onReview={() => {}}
      onOpenPrComments={() => {}}
    />,
  ));
  await setup.flush();

  const frame = setup.captureCharFrame();
  expect(frame).toContain("REVIEW");
  expect(frame).toContain("✎1");
  expect(setup.renderer.root.findDescendantById("github-comment-42-1-0")).toBeDefined();
});

test("shows teammate comment count and opens its PR summary", async () => {
  let opened: { repo: string; branch: string } | undefined;
  setup = await createTestRenderer({ width: 100, height: 18 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <DiffView
      enabled
      repos={[{ repo: "nory", branch: "feature", patch }]}
      scope="parent"
      mode="unified"
      width={100}
      height={18}
      comments={[]}
      githubComments={[{
        repo: "nory",
        branch: "feature",
        pr: 42,
        author: "teammate",
        body: "Keep the original value here.",
        url: "https://github.com/acme/repo/pull/42#discussion_r1",
        kind: 2,
        path: "src/reconcile.ts",
        line: 1,
        side: "RIGHT",
      }]}
      onCycleScope={() => {}}
      onToggleMode={() => {}}
      onClose={() => {}}
      onQuit={() => {}}
      onAttach={() => {}}
      onLaunchAgent={() => {}}
      onLaunchOtherAgent={() => {}}
      onConfigureAgents={() => {}}
      onComment={() => {}}
      onReview={() => {}}
      onOpenPrComments={(repo, branch) => { opened = { repo, branch }; }}
    />,
  ));
  await setup.flush();

  expect(setup.captureCharFrame()).toContain("1 inline comment");
  setup.mockInput.pressKey("2");
  await setup.flush();
  expect(opened).toEqual({ repo: "nory", branch: "feature" });
});
