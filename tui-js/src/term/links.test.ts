import { expect, test } from "bun:test";
import {
  activateVisibleUrlAtPosition,
  Osc8LinkTracker,
  visibleUrlAtPosition,
} from "./links";
import { terminalDataToStyledText } from "ghostty-opentui/terminal-buffer";

test("finds the visible URL under a terminal cell", () => {
  const text = "status\nSee https://example.com/pull/42 for details";

  expect(visibleUrlAtPosition(text, 1, 8)).toBe(
    "https://example.com/pull/42",
  );
  expect(visibleUrlAtPosition(text, 1, 2)).toBeNull();
});

test("excludes punctuation after a visible terminal URL", () => {
  const text = "Open https://example.com/docs.";

  expect(visibleUrlAtPosition(text, 0, 12)).toBe(
    "https://example.com/docs",
  );
  expect(visibleUrlAtPosition(text, 0, 29)).toBeNull();
});

test("activates the visible terminal URL under a click", () => {
  const openedUrls: string[] = [];

  expect(
    activateVisibleUrlAtPosition(
      "Open https://example.com/docs",
      0,
      12,
      (url) => openedUrls.push(url),
    ),
  ).toBe(true);
  expect(openedUrls).toEqual(["https://example.com/docs"]);
});

test("tracks an OSC 8 destination behind a visible label", () => {
  const tracker = new Osc8LinkTracker();
  tracker.observe("Open \x1b]8;;https://example.com/hidden\x1b\\documentation\x1b]8;;\x1b\\");

  expect(tracker.urlAtPosition("Open documentation", 0, 8)).toBe(
    "https://example.com/hidden",
  );
});

test("tracks an OSC 8 link split across terminal output chunks", () => {
  const tracker = new Osc8LinkTracker();
  const encoded = new TextEncoder().encode(
    "\x1b]8;;https://example.com/hidden\x1b\\documentation\x1b]8;;\x1b\\",
  );
  tracker.observe(encoded.slice(0, 24));
  tracker.observe(encoded.slice(24));

  expect(tracker.urlAtPosition("documentation", 0, 4)).toBe(
    "https://example.com/hidden",
  );
});

test("does not activate an unsupported OSC 8 protocol", () => {
  const tracker = new Osc8LinkTracker();
  tracker.observe("\x1b]8;;custom-tool://run\x1b\\dangerous action\x1b]8;;\x1b\\");

  expect(tracker.urlAtPosition("dangerous action", 0, 4)).toBeNull();
});

test("tracks an OSC 8 link at every output chunk boundary", () => {
  const encoded = new TextEncoder().encode(
    "\x1b]8;;https://example.com/hidden\x1b\\documentation\x1b]8;;\x1b\\",
  );
  for (let boundary = 1; boundary < encoded.length; boundary++) {
    const tracker = new Osc8LinkTracker();
    tracker.observe(encoded.slice(0, boundary));
    tracker.observe(encoded.slice(boundary));
    expect(tracker.urlAtPosition("documentation", 0, 4)).toBe("https://example.com/hidden");
  }
});

test("does not guess between OSC 8 destinations with the same label", () => {
  const tracker = new Osc8LinkTracker();
  tracker.observe("\x1b]8;;https://example.com/first\x1b\\details\x1b]8;;\x1b\\");
  tracker.observe("\x1b]8;;https://example.com/second\x1b\\details\x1b]8;;\x1b\\");

  expect(tracker.urlAtPosition("details", 0, 3)).toBeNull();
  expect(tracker.aliases).toEqual([]);
});

test("marks visible terminal URLs as native hyperlinks", () => {
  const url = "https://github.com/Noryai/nory/pull/9649";
  const styled = terminalDataToStyledText({
    cols: 80,
    rows: 1,
    cursor: [0, 0],
    cursorVisible: false,
    cursorStyle: "default",
    offset: 0,
    totalLines: 1,
    lines: [{ spans: [{ text: `Open ${url}`, fg: null, bg: null, flags: 0, width: 47 }] }],
  });

  expect(styled.chunks.some((chunk) => chunk.link?.url === url)).toBe(true);
});

test("marks an OSC 8 label as a native hyperlink", () => {
  const url = "https://example.com/hidden";
  const styled = terminalDataToStyledText({
    cols: 80,
    rows: 1,
    cursor: [0, 0],
    cursorVisible: false,
    cursorStyle: "default",
    offset: 0,
    totalLines: 1,
    lines: [{ spans: [{ text: "Open documentation", fg: null, bg: null, flags: 0, width: 18 }] }],
  }, undefined, undefined, [{ text: "documentation", url }]);

  expect(styled.chunks.some((chunk) => chunk.text === "documentation" && chunk.link?.url === url)).toBe(true);
});

test("coalesces adjacent terminal text with the default style", () => {
  const styled = terminalDataToStyledText({
    cols: 80,
    rows: 2,
    cursor: [0, 0],
    cursorVisible: false,
    cursorStyle: "default",
    offset: 0,
    totalLines: 2,
    lines: [
      { spans: [{ text: "first", fg: null, bg: null, flags: 0, width: 5 }] },
      { spans: [{ text: "second", fg: null, bg: null, flags: 0, width: 6 }] },
    ],
  });

  expect(styled.chunks).toHaveLength(1);
  expect(styled.chunks[0]?.text).toBe("first\nsecond");
  expect(styled.chunks[0]?.fg).toBeUndefined();
});

test("preserves explicit terminal colors while coalescing spans", () => {
  const styled = terminalDataToStyledText({
    cols: 80,
    rows: 1,
    cursor: [0, 0],
    cursorVisible: false,
    cursorStyle: "default",
    offset: 0,
    totalLines: 1,
    lines: [{
      spans: [
        { text: "plain ", fg: null, bg: null, flags: 0, width: 6 },
        { text: "red ", fg: "#ff0000", bg: null, flags: 0, width: 4 },
        { text: "text", fg: "#ff0000", bg: null, flags: 0, width: 4 },
      ],
    }],
  });

  expect(styled.chunks).toHaveLength(2);
  expect(styled.chunks[0]?.text).toBe("plain ");
  expect(styled.chunks[0]?.fg).toBeUndefined();
  expect(styled.chunks[1]?.text).toBe("red text");
  expect(styled.chunks[1]?.fg).toBeDefined();
});
