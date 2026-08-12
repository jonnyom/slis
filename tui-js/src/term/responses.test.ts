import { expect, test } from "bun:test";

import { TerminalModeTracker, TerminalQueryResponder } from "./responses";

const encoder = new TextEncoder();

test("answers zmx terminal startup queries", () => {
  const responder = new TerminalQueryResponder();
  expect(responder.observe(encoder.encode("\x1b]11;?\x1b\\\x1b[6n"))).toEqual([
    "\x1b]11;rgb:0000/0000/0000\x1b\\",
    "\x1b[1;1R",
  ]);
});

test("tracks bracketed paste mode across output chunks", () => {
  const tracker = new TerminalModeTracker();
  tracker.observe(encoder.encode("\x1b[?20"));
  tracker.observe(encoder.encode("04h"));
  expect(tracker.bracketedPaste).toBe(true);
  tracker.observe(encoder.encode("\x1b[?2004l"));
  expect(tracker.bracketedPaste).toBe(false);
});

test("answers fragmented terminal queries once complete", () => {
  const responder = new TerminalQueryResponder();
  expect(responder.observe(encoder.encode("output\x1b]11;"))).toEqual([]);
  expect(responder.observe(encoder.encode("?\x1b\\"))).toEqual([
    "\x1b]11;rgb:0000/0000/0000\x1b\\",
  ]);
});
