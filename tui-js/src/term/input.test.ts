import { describe, expect, test } from "bun:test";
import {
  embeddedTerminalInputSequence,
  embeddedTerminalPasteSequence,
} from "./input";

describe("embeddedTerminalInputSequence", () => {
  test("turns Shift+Enter into a newline without changing Enter", () => {
    expect(embeddedTerminalInputSequence("\x1b[13;2u")).toBe("\n");
    expect(embeddedTerminalInputSequence("\r")).toBe("\r");
  });
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
