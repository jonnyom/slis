import { expect, test } from "bun:test";
import {
  activateVisibleUrlAtPosition,
  visibleUrlAtPosition,
} from "./links";

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
