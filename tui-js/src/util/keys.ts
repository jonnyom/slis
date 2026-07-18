// Keyboard-name normalization. Under the kitty keyboard protocol (negotiated by
// modern terminals — ghostty, kitty, recent iTerm/WezTerm) OpenTUI reports a
// shifted letter as the LOWERCASE name plus `shift=true` (e.g. shift+R →
// name "r", shift true), so a bare `name === "R"` comparison never matches. A
// legacy terminal instead sends the raw uppercase byte (name "R", shift false).
// Collapsing both to the uppercase name lets handlers keep the natural
// `name === "R"` form and work under either protocol. Only single a–z letters
// are folded; digits, symbols and named keys pass through untouched.

import type { KeyEvent } from "@opentui/core";

export function normalizeKeyName(key: KeyEvent): string {
  const name = key.name;
  if (key.shift && name.length === 1 && name >= "a" && name <= "z") {
    return name.toUpperCase();
  }
  return name;
}

// Ctrl+C can arrive either as the raw ETX byte (handled globally in app.tsx)
// or as a normalized KeyEvent under terminal keyboard protocols. Keeping this
// second guard in React views prevents normalized ctrl+c from falling through
// to the browser's plain `c` create-slice binding.
export function isQuitKey(key: KeyEvent, normalizedName = normalizeKeyName(key)): boolean {
  return normalizedName === "q" || (key.ctrl === true && normalizedName.toLowerCase() === "c");
}
