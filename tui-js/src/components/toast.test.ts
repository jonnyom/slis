import { describe, expect, test } from "bun:test";
import { TOAST_MAX, makeToast, toastReducer, type ToastItem } from "./toast";
import { badgeFor } from "../theme";

function item(id: number): ToastItem {
  return { id, message: `m${id}`, glyph: "✓", color: "#fff" };
}

describe("toastReducer", () => {
  test("push appends", () => {
    const s = toastReducer([], { type: "push", toast: item(1) });
    expect(s.map((t) => t.id)).toEqual([1]);
  });

  test("dismiss removes by id, leaving the rest", () => {
    let s = [item(1), item(2), item(3)];
    s = toastReducer(s, { type: "dismiss", id: 2 });
    expect(s.map((t) => t.id)).toEqual([1, 3]);
  });

  test("dismiss of an unknown id is a no-op", () => {
    const s = toastReducer([item(1)], { type: "dismiss", id: 99 });
    expect(s.map((t) => t.id)).toEqual([1]);
  });

  test("queue is capped at TOAST_MAX, dropping the oldest", () => {
    let s: ToastItem[] = [];
    for (let i = 1; i <= TOAST_MAX + 2; i++) {
      s = toastReducer(s, { type: "push", toast: item(i) });
    }
    expect(s.length).toBe(TOAST_MAX);
    // oldest two dropped
    expect(s[0]?.id).toBe(3);
    expect(s[s.length - 1]?.id).toBe(TOAST_MAX + 2);
  });
});

describe("makeToast", () => {
  test("resolves glyph + color from the badge state", () => {
    const t = makeToast("copied", "ci-pass");
    expect(t).toMatchObject({ message: "copied", ...pick(badgeFor("ci-pass")) });
  });

  test("defaults to a success (ci-pass) badge", () => {
    const t = makeToast("done");
    expect(t.color).toBe(badgeFor("ci-pass").color);
  });
});

function pick(spec: { glyph: string; color: string }): { glyph: string; color: string } {
  return { glyph: spec.glyph, color: spec.color };
}
