import { describe, expect, test } from "bun:test";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import {
  loadPrefs,
  normalizeDiffScope,
  normalizeThemePreference,
  prefsPath,
  updatePrefs,
} from "./prefs";

describe("prefs", () => {
  test("uses the XDG state path when configured", () => {
    expect(prefsPath({ XDG_STATE_HOME: "/state", HOME: "/home/me" })).toBe(
      "/state/slis/prefs.json",
    );
  });

  test("falls back to the home state directory", () => {
    expect(prefsPath({ HOME: "/home/me" })).toBe("/home/me/.local/state/slis/prefs.json");
  });

  test("normalizes legacy and invalid diff scopes", () => {
    expect(normalizeDiffScope("parent")).toBe("parent");
    expect(normalizeDiffScope("working")).toBe("working");
    expect(normalizeDiffScope("dirty")).toBe("working");
    expect(normalizeDiffScope(undefined)).toBe("working");
    expect(normalizeDiffScope("wat" as never)).toBe("working");
  });

  test("accepts known themes and defaults unknown values to auto", () => {
    expect(normalizeThemePreference("violet")).toBe("violet");
    expect(normalizeThemePreference("wat")).toBe("auto");
  });

  test("merges and atomically persists preferences", async () => {
    const dir = await mkdtemp(join(tmpdir(), "slis-prefs-"));
    const path = join(dir, "nested", "prefs.json");
    try {
      await mkdir(join(dir, "nested"));
      await Bun.write(path, JSON.stringify({ split_diff: false, future_key: 7 }));
      await loadPrefs(path);
      await updatePrefs({ split_diff: true, theme: "violet", agent: "codex" }, path);
      expect(await Bun.file(path).json()).toEqual({
        split_diff: true,
        future_key: 7,
        theme: "violet",
        agent: "codex",
      });
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
