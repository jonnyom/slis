import { describe, expect, test } from "bun:test";
import { embeddedTerminalInputSequence } from "./input";

describe("embeddedTerminalInputSequence", () => {
  test("turns Shift+Enter into a newline without changing Enter", () => {
    expect(embeddedTerminalInputSequence("\x1b[13;2u")).toBe("\n");
    expect(embeddedTerminalInputSequence("\r")).toBe("\r");
  });
});
