import { describe, expect, test } from "bun:test";
import type { PrComment } from "../rpc/types";
import {
  cleanCommentBody,
  commentBlocks,
  commentKindLabel,
  reviewStateLabel,
  wrapText,
} from "./comments";

describe("cleanCommentBody", () => {
  test("strips html comments, tags, images, and keeps link text", () => {
    const raw =
      "<!-- hidden -->Please **fix** ![img](x.png) see [the docs](http://d) now <b>bold</b>";
    expect(cleanCommentBody(raw)).toBe("Please **fix** see the docs now bold");
  });
  test("unescapes entities and collapses whitespace", () => {
    expect(cleanCommentBody("a &amp; b\n\n  c")).toBe("a & b c");
  });
});

describe("commentKindLabel", () => {
  const c = (kind?: number, context?: string): PrComment => ({
    author: "x",
    body: "",
    url: "",
    kind,
    context,
  });
  test("issue comment (kind 0/undefined) → 💬", () => {
    expect(commentKindLabel(c())).toBe("💬");
    expect(commentKindLabel(c(0))).toBe("💬");
  });
  test("review (kind 1) uses the review-state label", () => {
    expect(commentKindLabel(c(1, "CHANGES_REQUESTED"))).toBe("✗ changes");
    expect(commentKindLabel(c(1, "APPROVED"))).toBe("✓ approved");
  });
  test("inline (kind 2) shows the path:line context", () => {
    expect(commentKindLabel(c(2, "src/x.ts:14"))).toBe("📝 src/x.ts:14");
    expect(commentKindLabel(c(2))).toBe("📝 inline");
  });
});

describe("reviewStateLabel", () => {
  test("known and unknown states", () => {
    expect(reviewStateLabel("commented")).toBe("💬 review");
    expect(reviewStateLabel("DISMISSED")).toBe("dismissed");
    expect(reviewStateLabel("SOMETHING")).toBe("review");
  });
});

describe("wrapText", () => {
  test("wraps on word boundaries within width", () => {
    // Width floors at 20 (matching Go); 12-char words each land on their own line.
    expect(wrapText("aaaaaaaaaaaa bbbbbbbbbbbb cccccccccccc", 20)).toEqual([
      "aaaaaaaaaaaa",
      "bbbbbbbbbbbb",
      "cccccccccccc",
    ]);
  });
  test("empty input yields one blank line", () => {
    expect(wrapText("   ", 40)).toEqual([""]);
  });
});

describe("commentBlocks", () => {
  test("builds header + wrapped body per comment", () => {
    const blocks = commentBlocks(
      "web",
      8107,
      [{ author: "rev", body: "This breaks the empty-cart case.", url: "", kind: 2, context: "cart.tsx:14" }],
      80,
    );
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.header).toBe("📝 cart.tsx:14  web #8107 — rev");
    expect(blocks[0]!.body).toEqual(["This breaks the empty-cart case."]);
  });
  test("missing author and empty body fall back", () => {
    const blocks = commentBlocks("api", 1, [{ author: "", body: "", url: "" }], 80);
    expect(blocks[0]!.header).toBe("💬  api #1 — ?");
    expect(blocks[0]!.body).toEqual(["(no text)"]);
  });
});
