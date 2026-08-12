import { describe, expect, test } from "bun:test";
import type { PrComment } from "../rpc/types";
import {
  cleanCommentBody,
  commentBlocks,
  commentKindLabel,
  diffHunkContext,
  indexInlineComments,
  inlineCommentPreview,
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

describe("inlineCommentPreview", () => {
  test("keeps short comments and truncates long comments for the diff callout", () => {
    expect(inlineCommentPreview("Short review.")).toEqual({
      body: "Short review.",
      truncated: false,
    });
    const preview = inlineCommentPreview("review ".repeat(100));
    expect(preview.truncated).toBe(true);
    expect(preview.body.endsWith("…")).toBe(true);
    expect(preview.body.length).toBeLessThanOrEqual(321);
  });
});

describe("indexInlineComments", () => {
  test("groups live GitHub comments by branch, file, side, and line", () => {
    const indexed = indexInlineComments(
      [
        { repo: "web", branch: "feature", pr: 12, author: "a", body: "new", url: "", kind: 2, path: "src/a.ts", line: 14, side: "RIGHT" },
        { repo: "web", branch: "feature", pr: 12, author: "b", body: "old", url: "", kind: 2, path: "src/old.ts", line: 9, side: "LEFT" },
        { repo: "web", branch: "other", pr: 13, author: "c", body: "wrong branch", url: "", kind: 2, path: "src/a.ts", line: 14, side: "RIGHT" },
      ],
      "web",
      "feature",
      "src/a.ts",
      "src/old.ts",
    );
    expect(indexed.new.get(14)?.map((comment) => comment.body)).toEqual(["new"]);
    expect(indexed.old.get(9)?.map((comment) => comment.body)).toEqual(["old"]);
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

  test("includes compact code context for inline comments", () => {
    const blocks = commentBlocks(
      "web",
      8107,
      [{
        author: "rev",
        body: "This breaks.",
        url: "",
        kind: 2,
        path: "src/cart.ts",
        line: 14,
        diff_hunk: "@@ -12,3 +12,3 @@\n before\n-const old = 1\n+const next = 2\n after",
      }],
      80,
    );
    expect(blocks[0]!.code).toEqual(["  before", "- const old = 1", "+ const next = 2", "  after"]);
    expect(diffHunkContext("@@ header\n+a\n+b\n+c\n+d\n+e\n+f")).toHaveLength(5);
  });
});
