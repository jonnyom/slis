import { afterEach, expect, test } from "bun:test";
import { parseColor, type BoxRenderable } from "@opentui/core";
import { createTestRenderer, type TestRendererSetup } from "@opentui/core/testing";
import { createRoot, flushSync, type Root } from "@opentui/react";
import { theme } from "../theme";
import { InitialScreen, workspaceDiagnosis } from "./initialscreen";
import { BOLD } from "./ui";

let setup: TestRendererSetup | null = null;
let root: Root | null = null;

afterEach(() => {
  root?.unmount();
  setup?.renderer.destroy();
  root = null;
  setup = null;
});

test("initial workspace failure is shown in a centered dialog", async () => {
  let quitCount = 0;
  setup = await createTestRenderer({ width: 120, height: 30 });
  root = createRoot(setup.renderer);
  flushSync(() => root!.render(
    <InitialScreen
      connected={false}
      error={'slis: workspace not found — run `slis init` first: LoadWorkspace: read "/Users/ed/.config/slis/workspace.yaml": open /Users/ed/.config/slis/workspace.yaml: no such file or directory'}
      onQuit={() => quitCount++}
    />,
  ));
  await setup.flush();

  const dialog = setup.renderer.root.findDescendantById("initial-workspace-error");
  const heading = setup.renderer.root.findDescendantById("initial-workspace-heading");
  const instruction = setup.renderer.root.findDescendantById("initial-workspace-instruction");
  const diagnosis = setup.renderer.root.findDescendantById("initial-workspace-diagnosis");
  const footer = setup.renderer.root.findDescendantById("initial-workspace-footer");
  expect(dialog).toBeDefined();
  expect(heading).toBeDefined();
  expect(instruction).toBeDefined();
  expect(diagnosis).toBeDefined();
  expect(footer).toBeDefined();
  expect(dialog!.width).toBeLessThan(120);
  expect(dialog!.x).toBe((120 - dialog!.width) / 2);
  expect(Math.abs(dialog!.y - (30 - dialog!.height) / 2)).toBeLessThanOrEqual(0.5);
  expect((dialog as BoxRenderable).backgroundColor.equals(parseColor(theme.bg))).toBe(true);
  expect((dialog as BoxRenderable).borderColor.equals(parseColor(theme.bad))).toBe(true);
  expect((dialog as BoxRenderable).titleColor?.equals(parseColor(theme.bad))).toBe(true);
  expect((dialog as BoxRenderable).title).toBe("× Workspace unavailable");
  expect(heading!.y).toBe(dialog!.y + 1);
  expect(instruction!.y).toBe(heading!.y + heading!.height);
  expect(diagnosis!.y).toBeGreaterThan(instruction!.y);
  expect(footer!.y).toBeGreaterThan(diagnosis!.y + diagnosis!.height - 1);
  expect(footer!.y + footer!.height).toBe(dialog!.y + dialog!.height - 1);
  expect(setup.captureCharFrame()).toContain("Workspace unavailable");
  expect(setup.captureCharFrame()).not.toContain("✗ Workspace unavailable");
  expect(setup.captureCharFrame()).toContain("Workspace not found");
  expect(setup.captureCharFrame()).toContain("run `slis init` first");
  expect(setup.captureCharFrame()).not.toContain("Diagnosis");
  expect(setup.captureCharFrame()).toContain(
    "Slis looked for the workspace configuration at",
  );
  expect(setup.captureCharFrame()).not.toContain("LoadWorkspace");
  expect(setup.captureCharFrame()).not.toContain("Press q");

  const diagnosisSpans = setup.captureSpans().lines
    .slice(diagnosis!.y, diagnosis!.y + diagnosis!.height)
    .flatMap((line) => line.spans);
  const highlightedPath = diagnosisSpans
    .filter((span) => span.fg.equals(parseColor(theme.text)))
    .map((span) => span.text)
    .join("");
  expect(highlightedPath).toContain("/Users/ed/.config/slis/workspace.yaml");

  const footerSpans = setup.captureSpans().lines[footer!.y]!.spans;
  for (const shortcut of ["q", "esc", "ctrl+c"]) {
    const span = footerSpans.find((candidate) => candidate.text === shortcut);
    expect(span).toBeDefined();
    expect((span!.attributes & BOLD) !== 0).toBe(true);
  }
  const label = footerSpans.find((candidate) => candidate.text.includes("quit"));
  expect(label).toBeDefined();
  expect((label!.attributes & BOLD) === 0).toBe(true);

  setup.mockInput.pressKey("q");
  await setup.flush();
  expect(quitCount).toBe(1);
});

test("workspace diagnosis removes the duplicated recovery instruction", () => {
  expect(workspaceDiagnosis(
    "slis: workspace not found — run `slis init` first: LoadWorkspace: read \"/Users/ed/.config/slis/workspace.yaml\": open /Users/ed/.config/slis/workspace.yaml: no such file or directory",
  )).toBe(
    "Slis looked for the workspace configuration at /Users/ed/.config/slis/workspace.yaml, but no file exists there.",
  );
  expect(workspaceDiagnosis("sidecar exited (1)")).toBe("sidecar exited (1)");
});
