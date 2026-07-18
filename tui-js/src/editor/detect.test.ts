import { describe, expect, test } from "bun:test";
import { availableEditors, KNOWN_EDITORS } from "./detect";

describe("availableEditors", () => {
  test("returns matches in preference order, not PATH-probe order", () => {
    // zed found before code, but code must still rank ahead of zed.
    const found = new Set(["zed", "code"]);
    const got = availableEditors((b) => found.has(b));
    expect(got.map((e) => e.bin)).toEqual(["code", "zed"]);
  });

  test("none on PATH → empty", () => {
    expect(availableEditors(() => false)).toEqual([]);
  });

  test("all known editors are VS Code family plus zed", () => {
    expect(KNOWN_EDITORS.map((e) => e.bin)).toEqual([
      "cursor",
      "code",
      "code-insiders",
      "codium",
      "windsurf",
      "zed",
    ]);
  });
});
