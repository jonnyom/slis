import { describe, expect, test } from "bun:test";
import {
  embeddedTerminalInputSequence,
  embeddedTerminalPasteSequence,
  embeddedTerminalWriteSequence,
} from "./input";

describe("embeddedTerminalInputSequence", () => {
  test("turns Shift+Enter into a newline without changing Enter", () => {
    expect(embeddedTerminalInputSequence("\x1b[13;2u")).toBe("\n");
    expect(embeddedTerminalInputSequence("\r")).toBe("\r");
  });
});

test("sends raw paste when attached terminal did not enable bracketed paste", () => {
  const bytes = new TextEncoder().encode("paste");
  expect(embeddedTerminalPasteSequence(bytes, false)).toEqual(bytes);
});

test("converts paste bytes to one PTY string without changing escapes", () => {
  const bytes = embeddedTerminalPasteSequence(new TextEncoder().encode("paste"));
  expect(embeddedTerminalWriteSequence(bytes)).toBe("\x1b[200~paste\x1b[201~");
});

describe("embeddedTerminalPasteSequence", () => {
  test("preserves the terminal bracketed paste envelope", () => {
    const bytes = new TextEncoder().encode("claude --teleport session_test");

    expect(embeddedTerminalPasteSequence(bytes)).toEqual(
      new TextEncoder().encode(
        "\x1b[200~claude --teleport session_test\x1b[201~",
      ),
    );
  });
});
