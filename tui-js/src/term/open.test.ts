import { expect, test } from "bun:test";

import { TerminalOpenCoordinator } from "./open";

test("terminal opening shows progress and ignores repeated shortcuts", async () => {
  const loadingStates: Array<string | null> = [];
  let actionCount = 0;
  let releaseFirstAction: (() => void) | undefined;
  const coordinator = new TerminalOpenCoordinator(
    (label) => loadingStates.push(label),
    10,
  );

  const firstOpen = coordinator.run("opening feature agent…", async () => {
    actionCount += 1;
    await new Promise<void>((resolve) => {
      releaseFirstAction = resolve;
    });
  });
  const repeatedOpen = await coordinator.run("opening feature agent…", async () => {
    actionCount += 1;
  });

  expect(repeatedOpen).toBe(false);
  expect(actionCount).toBe(1);
  expect(loadingStates).toEqual(["opening feature agent…"]);

  releaseFirstAction!();
  expect(await firstOpen).toBe(true);
  expect(loadingStates).toEqual(["opening feature agent…", null]);
  expect(await coordinator.run("opening feature agent…", async () => {
    actionCount += 1;
  })).toBe(false);

  await Bun.sleep(15);
  expect(await coordinator.run("opening feature agent…", async () => {
    actionCount += 1;
  })).toBe(true);
  expect(actionCount).toBe(2);
});
