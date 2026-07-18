// Entry point: create the OpenTUI renderer and mount the React app.
// Run with `bun run start` (real `slis rpc` sidecar) or `bun run start:fake`.

import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { App } from "./app";

const renderer = await createCliRenderer({
  exitOnCtrlC: true,
  targetFps: 30,
});

createRoot(renderer).render(<App />);
