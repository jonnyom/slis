import { describe, expect, test } from "bun:test";
import { isQuitKey, isUnmodifiedKey, normalizeKeyName } from "./keys";
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

describe("isQuitKey", () => {
  test("accepts q and normalized ctrl+c", () => {
    expect(isQuitKey(key("q"))).toBe(true);
    expect(isQuitKey({ name: "c", ctrl: true } as KeyEvent)).toBe(true);
  });

  test("reserves ctrl+q for terminal dock hiding", () => {
    expect(isQuitKey({ name: "q", ctrl: true } as KeyEvent)).toBe(false);
  });

  test("plain c remains the create-slice key", () => {
    expect(isQuitKey(key("c"))).toBe(false);
  });
});

describe("isUnmodifiedKey", () => {
  test("rejects ctrl+g as plain hub navigation", () => {
    expect(isUnmodifiedKey({ name: "g", ctrl: true } as KeyEvent)).toBe(false);
    expect(isUnmodifiedKey(key("g"))).toBe(true);
  });
});
