// PR-comment rendering helpers, ported from internal/tui/prview.go
// (cleanCommentBody / commentKindLabel / reviewStateLabel / wrapText /
// commentBlock). Pure so the cockpit PR-detail pane can consume parsed blocks and
// the logic is unit-testable. Comment `kind`: 0 issue · 1 review · 2 inline.

import type { PrComment } from "../rpc/types";

const HTML_COMMENT = /<!--[\s\S]*?-->/g;
const MD_IMAGE = /!\[[^\]]*\]\([^)]*\)/g;
const MD_LINK = /\[([^\]]*)\]\([^)]*\)/g;
const HTML_TAG = /<[^>]+>/g;
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g;

const ENTITIES: Record<string, string> = {
  "&amp;": "&",
  "&lt;": "<",
  "&gt;": ">",
  "&quot;": '"',
  "&#39;": "'",
  "&apos;": "'",
  "&nbsp;": " ",
};

function unescapeHtml(s: string): string {
  return s.replace(/&(?:amp|lt|gt|quot|#39|apos|nbsp);/g, (m) => ENTITIES[m] ?? m);
}

// cleanCommentBody strips HTML comments/tags, markdown images, and link URLs
// (keeping the link text), unescapes entities, drops control chars, and collapses
// whitespace — the same normalisation cleanCommentBody does in Go.
export function cleanCommentBody(s: string): string {
  s = s.replace(HTML_COMMENT, "");
  s = unescapeHtml(s);
  s = s.replace(MD_IMAGE, "");
  s = s.replace(MD_LINK, "$1");
  s = s.replace(HTML_TAG, "");
  s = s.replace(CONTROL_CHARS, "");
  return s.trim().split(/\s+/).filter(Boolean).join(" ");
}

// reviewStateLabel renders a review submission's state as a short label.
export function reviewStateLabel(state: string): string {
  switch (state.toUpperCase()) {
    case "APPROVED":
      return "✓ approved";
    case "CHANGES_REQUESTED":
      return "✗ changes";
    case "COMMENTED":
      return "💬 review";
    case "DISMISSED":
      return "dismissed";
    default:
      return "review";
  }
}

// commentKindLabel marks a comment's origin: 💬 issue comment, the review state
// for a review submission, "📝 path:line" for an inline review comment.
export function commentKindLabel(c: PrComment): string {
  switch (c.kind) {
    case 1:
      return reviewStateLabel(c.context ?? "");
    case 2:
      return c.context ? "📝 " + c.context : "📝 inline";
    default:
      return "💬";
  }
}

// wrapText word-wraps s to width columns, returning at least one line.
export function wrapText(s: string, width: number): string[] {
  const w = width < 20 ? 20 : width;
  const words = s.split(/\s+/).filter(Boolean);
  if (words.length === 0) return [""];
  const lines: string[] = [];
  let cur = words[0]!;
  for (const word of words.slice(1)) {
    if (cur.length + 1 + word.length > w) {
      lines.push(cur);
      cur = word;
    } else {
      cur += " " + word;
    }
  }
  lines.push(cur);
  return lines;
}

export interface CommentBlock {
  header: string;
  body: string[];
}

// commentBlocks renders each comment as a header (kind · repo #N · author) plus
// its cleaned, wrapped body — the data the cockpit PR pane paints.
export function commentBlocks(
  repo: string,
  prNumber: number,
  comments: readonly PrComment[],
  width: number,
): CommentBlock[] {
  return comments.map((c) => {
    const author = c.author || "?";
    const header = `${commentKindLabel(c)}  ${repo} #${prNumber} — ${author}`;
    const body = cleanCommentBody(c.body) || "(no text)";
    return { header, body: wrapText(body, width) };
  });
}
