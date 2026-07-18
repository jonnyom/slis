// Entry point: create the OpenTUI renderer and mount the React app.
// Run with `bun run start` (real `slis rpc` sidecar) or `bun run start:fake`.

import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { App } from "./app";

// exitOnCtrlC is false so ctrl+c reaches an embedded terminal tab (interrupting
// the agent) instead of quitting slis. The browser/cockpit quit with `q`.
const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  targetFps: 30,
});

createRoot(renderer).render(<App />);
