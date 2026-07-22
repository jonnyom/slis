import { afterEach, expect, test } from "bun:test";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { previewLineTokens } from "../components/codepreview";
import { CommentComposerOverlay } from "./overlays";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("comment composer presents code, input, then actions", async () => {
  setup = await createTestRenderer({ width: 100, height: 30 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <CommentComposerOverlay
      ctx={{
        slice: "checkout",
        repo: "nory",
        branch: "feature",
        file: "worker.py",
        line: 12,
        side: "new",
        hunk: "+def calculate_total(amount: int):",
      }}
      text="Use a domain name"
    />,
  ));
  await setup.flush();

  const preview = setup.renderer.root.findDescendantById("comment-code-preview");
  const input = setup.renderer.root.findDescendantById("comment-input-control");
  const actions = setup.renderer.root.findDescendantById("modal-actions");
  expect(preview).toBeDefined();
  expect(input).toBeDefined();
  expect(actions).toBeDefined();
  expect(input!.y).toBeGreaterThan(preview!.y);
  expect(actions!.y).toBeGreaterThan(input!.y);
});

test("code preview tokenizes content using the file language", () => {
  const preview = previewLineTokens("+def calculate_total(amount: int):", "worker.py");
  expect(preview.prefix).toBe("+");
  expect(preview.tokens.some((token) => token.kind === "keyword" && token.text === "def")).toBe(true);
  expect(preview.tokens.map((token) => token.text).join("")).toBe("def calculate_total(amount: int):");
});
