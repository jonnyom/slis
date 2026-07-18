// Entry point: create the OpenTUI renderer and mount the React app.
// Run with `bun run start` (real `slis rpc` sidecar) or `bun run start:fake`.

import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { App } from "./app";
import { ErrorBoundary } from "./components/errorboundary";
import { setTheme } from "./theme";
import { loadPrefs, normalizeThemePreference } from "./prefs";

const initialPrefs = await loadPrefs();
const savedTheme = normalizeThemePreference(initialPrefs.theme);
const requestedTheme = process.env.SLIS_THEME?.trim().toLowerCase();

// Environment overrides are temporary; without one, restore the user's last
// palette before the first frame so startup never flashes the default theme.
if (!requestedTheme && savedTheme !== "auto") setTheme(savedTheme);

// exitOnCtrlC is false so ctrl+c reaches an embedded terminal tab (interrupting
// the agent) instead of quitting slis. The browser/cockpit quit with `q`.
const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  targetFps: 30,
});

// OpenTUI asks the terminal for its foreground/background via OSC and infers
// light vs dark. Explicit preferences and NO_COLOR always win; unsupported
// terminals simply keep the safe Midnight fallback.
const autoTheme = requestedTheme
  ? requestedTheme === "auto" || requestedTheme === "system"
  : savedTheme === "auto";
let initialThemeMode: "dark" | "light" | null = null;
if (autoTheme) {
  initialThemeMode = await renderer.waitForThemeMode(300);
  if (initialThemeMode) setTheme(initialThemeMode === "light" ? "light" : "midnight");
}

const quitAfterCrash = () => {
  renderer.destroy();
  process.exit(1);
};

createRoot(renderer).render(
  <ErrorBoundary onQuit={quitAfterCrash}>
    <App initialPrefs={initialPrefs} initialThemeMode={initialThemeMode} />
  </ErrorBoundary>,
);
