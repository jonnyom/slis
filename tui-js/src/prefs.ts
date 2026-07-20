// Small user-level UI preferences shared with the legacy Go TUI. Keeping these
// in XDG state avoids rewriting the hand-edited workspace.yaml for every toggle.

import { mkdirSync, renameSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import type { DiffScope } from "./rpc/types";
import type { ThemeName } from "./theme";

export type ThemePreference = "auto" | ThemeName;

export interface UiPrefs {
  split_diff?: boolean;
  diff_scope?: DiffScope | "dirty";
  theme?: ThemePreference;
  agent?: string;
  // Kept when an old Go TUI preference file is loaded and rewritten.
  diff_vs_trunk?: boolean;
  [key: string]: unknown;
}

export function prefsPath(env: Record<string, string | undefined> = process.env): string {
  const stateBase = env.XDG_STATE_HOME
    ? env.XDG_STATE_HOME
    : env.HOME
      ? join(env.HOME, ".local", "state")
      : ".slis-state";
  return join(stateBase, "slis", "prefs.json");
}

let cachedPrefs: UiPrefs = {};

export async function loadPrefs(path = prefsPath()): Promise<UiPrefs> {
  try {
    const parsed = await Bun.file(path).json();
    cachedPrefs = parsed && typeof parsed === "object" ? (parsed as UiPrefs) : {};
  } catch {
    cachedPrefs = {};
  }
  return { ...cachedPrefs };
}

export function updatePrefs(patch: Partial<UiPrefs>, path = prefsPath()): void {
  cachedPrefs = { ...cachedPrefs, ...patch };
  try {
    mkdirSync(dirname(path), { recursive: true });
    const temp = `${path}.${process.pid}.tmp`;
    writeFileSync(temp, JSON.stringify(cachedPrefs, null, 2) + "\n", { mode: 0o644 });
    renameSync(temp, path);
  } catch {
    // Preferences are best-effort: the active session still keeps the choice.
  }
}

export function normalizeDiffScope(value: UiPrefs["diff_scope"]): DiffScope {
  if (value === "dirty") return "working";
  if (value === "parent" || value === "trunk" || value === "working") return value;
  return "working";
}

export function normalizeThemePreference(value: unknown): ThemePreference {
  if (
    value === "auto" ||
    value === "midnight" ||
    value === "violet" ||
    value === "light" ||
    value === "mono" ||
    value === "mono-light"
  ) {
    return value;
  }
  return "auto";
}
