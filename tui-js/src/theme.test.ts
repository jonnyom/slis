import { describe, expect, test } from "bun:test";
import type { PrStackEntry, Slice } from "./rpc/types";
import type { SliceView } from "./state/derive";
import { attention, badgeFor, glyph, resultStatusStyle, theme } from "./theme";

function view(extra: Partial<Slice> = {}, vextra: Partial<SliceView> = {}): SliceView {
  return {
    slice: { name: "s", base: "", active: false, stale: false, members: [], ...extra },
    status: "none",
    ...vextra,
  };
}

function pr(extra: Partial<PrStackEntry> = {}): PrStackEntry {
  return { repo: "r", branch: "b", number: 1, ...extra };
}

describe("attention", () => {
  test("waiting-input is needs-you (attn, ⏸)", () => {
    const a = attention(view({}, { status: "waiting-input" }));
    expect(a.level).toBe(3);
    expect(a.color).toBe(theme.attn);
    expect(a.glyph).toBe(glyph.waiting);
    expect(a.bold).toBe(true);
  });

  test("changes-requested is needs-you (bad, ✗)", () => {
    const a = attention(view({}, { prs: [pr({ review_decision: "CHANGES_REQUESTED" })] }));
    expect(a.level).toBe(3);
    expect(a.color).toBe(theme.bad);
    expect(a.glyph).toBe(glyph.changes);
  });

  test("CI fail is needs-you (bad, ✗)", () => {
    const a = attention(view({}, { prs: [pr({ ci: "fail" })] }));
    expect(a.level).toBe(3);
    expect(a.color).toBe(theme.bad);
  });

  test("ci_fail count also triggers needs-you", () => {
    const a = attention(view({}, { prs: [pr({ ci_fail: 2 })] }));
    expect(a.level).toBe(3);
    expect(a.color).toBe(theme.bad);
  });

  test("live/swapped-in is active (good, ●)", () => {
    const a = attention(view({ active: true }));
    expect(a.level).toBe(2);
    expect(a.color).toBe(theme.good);
    expect(a.glyph).toBe(glyph.live);
  });

  test("running is active (good, ●)", () => {
    const a = attention(view({}, { status: "running" }));
    expect(a.level).toBe(2);
    expect(a.color).toBe(theme.good);
  });

  test("session done is info (merged, ✦)", () => {
    const a = attention(view({}, { status: "done" }));
    expect(a.level).toBe(1);
    expect(a.color).toBe(theme.merged);
    expect(a.glyph).toBe(glyph.done);
  });

  test("all PRs merged is info ready-to-clear (good, ♻)", () => {
    const a = attention(view({}, { prs: [pr({ state: "MERGED" })] }));
    expect(a.level).toBe(1);
    expect(a.color).toBe(theme.good);
    expect(a.glyph).toBe(glyph.ready);
  });

  test("open PR is info in-review (focus, ✓)", () => {
    const a = attention(view({}, { prs: [pr({ state: "OPEN" })] }));
    expect(a.level).toBe(1);
    expect(a.color).toBe(theme.focus);
    expect(a.glyph).toBe(glyph.inReview);
  });

  test("nothing pending is idle (textDim, ·)", () => {
    const a = attention(view());
    expect(a.level).toBe(0);
    expect(a.color).toBe(theme.textDim);
    expect(a.glyph).toBe(glyph.idle);
    expect(a.bold).toBe(false);
  });

  test("needs-you beats active — waiting wins over a live slice", () => {
    const a = attention(view({ active: true }, { status: "waiting-input" }));
    expect(a.level).toBe(3);
  });

  test("active beats info — live wins over an open PR", () => {
    const a = attention(view({ active: true }, { prs: [pr({ state: "OPEN" })] }));
    expect(a.level).toBe(2);
  });
});

describe("badgeFor", () => {
  test("only ever emits the five-hue tokens", () => {
    const allowed = new Set<string>([
      theme.good,
      theme.attn,
      theme.bad,
      theme.merged,
      theme.focus,
      theme.textDim,
    ]);
    const states = [
      "live",
      "running",
      "waiting",
      "done",
      "dirty",
      "stale",
      "restack",
      "ready",
      "ci-pass",
      "ci-fail",
      "ci-pending",
      "approved",
      "changes",
      "merged",
      "idle",
    ] as const;
    for (const s of states) {
      expect(allowed.has(badgeFor(s).color)).toBe(true);
    }
  });

  test("ci-fail maps to bad ✗, ci-pass to good ✓, ci-pending to attn ⋯", () => {
    expect(badgeFor("ci-fail")).toMatchObject({ color: theme.bad, glyph: glyph.ciFail });
    expect(badgeFor("ci-pass")).toMatchObject({ color: theme.good, glyph: glyph.ciPass });
    expect(badgeFor("ci-pending")).toMatchObject({ color: theme.attn, glyph: glyph.ciPending });
  });

  test("waiting is attn ⏸; live is good ●; merged is merged ✦", () => {
    expect(badgeFor("waiting")).toMatchObject({ color: theme.attn, glyph: glyph.waiting });
    expect(badgeFor("live")).toMatchObject({ color: theme.good, glyph: glyph.live });
    expect(badgeFor("merged")).toMatchObject({ color: theme.merged, glyph: glyph.done });
  });
});

describe("distinct restack / stale / overlap glyphs (D1)", () => {
  test("restack ⟳, stale ↓ and overlap ⧉ are three distinct marks", () => {
    expect(glyph.restack).toBe("⟳");
    expect(glyph.stale).toBe("↓");
    expect(glyph.overlap).toBe("⧉");
    expect(new Set([glyph.restack, glyph.stale, glyph.overlap]).size).toBe(3);
  });

  test("badgeFor maps stale → ↓ and restack → ⟳ (no shared ⚠)", () => {
    expect(badgeFor("stale").glyph).toBe(glyph.stale);
    expect(badgeFor("restack").glyph).toBe(glyph.restack);
    expect(badgeFor("stale").glyph).not.toBe(glyph.dirty);
  });
});

describe("resultStatusStyle (D2 — refusals are warn, not fake success/error)", () => {
  test("warn is amber ⚠ — never the green success ✓ nor the red error ✗", () => {
    const warn = resultStatusStyle("warn");
    expect(warn.color).toBe(theme.attn);
    expect(warn.glyph).toBe(glyph.dirty);
    expect(warn.color).not.toBe(theme.good);
    expect(warn.color).not.toBe(theme.bad);
    expect(warn.glyph).not.toBe(glyph.inReview);
    expect(warn.glyph).not.toBe(glyph.changes);
  });

  test("success is good ✓ and failure is bad ✗", () => {
    expect(resultStatusStyle("success")).toEqual({ color: theme.good, glyph: glyph.inReview });
    expect(resultStatusStyle("failure")).toEqual({ color: theme.bad, glyph: glyph.changes });
  });
});
