import { describe, expect, test } from "bun:test";
import { tmuxWheelSequence } from "./mouse";

describe("tmuxWheelSequence", () => {
  test("encodes vertical wheel events using one-based SGR coordinates", () => {
    expect(tmuxWheelSequence("up", 4, 7)).toBe("\x1b[<64;4;7M");
    expect(tmuxWheelSequence("down", 4, 7)).toBe("\x1b[<65;4;7M");
  });

  test("clamps coordinates and preserves keyboard modifiers", () => {
    expect(tmuxWheelSequence("up", 0, -2, { shift: true, ctrl: true })).toBe(
      "\x1b[<84;1;1M",
    );
  });
});
