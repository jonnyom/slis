import { describe, expect, test } from "bun:test";
import { normalizeKeyName } from "./keys";
import type { KeyEvent } from "@opentui/core";

function key(name: string, shift = false): KeyEvent {
  return { name, shift } as unknown as KeyEvent;
}

describe("normalizeKeyName", () => {
  test("kitty: shifted letter (lowercase name + shift) folds to uppercase", () => {
    expect(normalizeKeyName(key("r", true))).toBe("R");
    expect(normalizeKeyName(key("i", true))).toBe("I");
    expect(normalizeKeyName(key("n", true))).toBe("N");
  });

  test("legacy: raw uppercase name passes through", () => {
    expect(normalizeKeyName(key("R", false))).toBe("R");
  });

  test("unshifted lowercase is left alone", () => {
    expect(normalizeKeyName(key("r", false))).toBe("r");
  });

  test("shifted symbols and digits are not folded", () => {
    expect(normalizeKeyName(key("!", true))).toBe("!");
    expect(normalizeKeyName(key("1", true))).toBe("1");
    expect(normalizeKeyName(key("?", true))).toBe("?");
  });

  test("named keys pass through", () => {
    expect(normalizeKeyName(key("escape", false))).toBe("escape");
    expect(normalizeKeyName(key("return", true))).toBe("return");
  });
});
