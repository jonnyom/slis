import { describe, expect, test } from "bun:test";
import { editText } from "./textinput";

describe("editText", () => {
  test("appends a printable character via its sequence", () => {
    expect(editText("ab", { name: "c", sequence: "c" })).toBe("abc");
  });
  test("appends a space", () => {
    expect(editText("a", { name: "space", sequence: " " })).toBe("a ");
  });
  test("appends uppercase / punctuation from the raw sequence", () => {
    expect(editText("", { name: "a", sequence: "A", shift: true } as never)).toBe("A");
    expect(editText("x", { name: "/", sequence: "/" })).toBe("x/");
  });
  test("backspace trims the last character", () => {
    expect(editText("abc", { name: "backspace" })).toBe("ab");
    expect(editText("", { name: "backspace" })).toBe("");
  });
  test("ctrl-u clears the line", () => {
    expect(editText("abc", { name: "u", ctrl: true })).toBe("");
  });
  test("ignores control keys (enter, escape, arrows)", () => {
    expect(editText("ab", { name: "return", sequence: "\r" })).toBe("ab");
    expect(editText("ab", { name: "escape", sequence: "" })).toBe("ab");
    expect(editText("ab", { name: "left" })).toBe("ab");
  });
  test("ignores ctrl/meta-modified printable keys", () => {
    expect(editText("ab", { name: "c", sequence: "c", ctrl: true })).toBe("ab");
    expect(editText("ab", { name: "v", sequence: "v", meta: true })).toBe("ab");
  });
});
