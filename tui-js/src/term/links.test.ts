import { expect, test } from "bun:test";
import {
  activateVisibleUrlAtPosition,
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
