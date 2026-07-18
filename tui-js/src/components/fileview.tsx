// File-content pane (F3): renders a file at a branch's revision with line
// numbers and single-line syntax highlighting (reusing the differ's tokenizer).
// Binary files and read errors show a one-line notice instead of content.

import type { ReactNode } from "react";
import type { FileResult } from "../rpc/types";
import { langForPath, tokenizeLine } from "../diff/tokenize";
import { color, colorForKind, glyph } from "../theme";
import { BOLD, DIM } from "./ui";

// contentLines splits file content into display lines, dropping a single
// trailing empty line from a final newline so the last line isn't a blank row.
// Exported so the cockpit can drive a line cursor over the same line set.
export function contentLines(content: string): string[] {
  const lines = content.split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

export function FileView({
  file,
  error,
  loading,
  cursor = -1,
  marked,
}: {
  file: FileResult | null;
  error: string | null;
  loading: boolean;
  // 0-based cursor line (F2 review anchor); -1 = no cursor.
  cursor?: number;
  // 1-based line numbers carrying a pending comment (✎ gutter marker).
  marked?: Set<number>;
}): ReactNode {
  if (error) {
    return (
      <text fg={color.missing} wrapMode="none">
        {error}
      </text>
    );
  }
  if (!file || loading) {
    return (
      <text fg={color.dim} attributes={DIM}>
        loading file…
      </text>
    );
  }
  if (file.binary) {
    return (
      <text fg={color.dim} attributes={DIM}>
        binary file · {file.size} bytes — not shown
      </text>
    );
  }

  const lang = langForPath(file.path);
  const lines = contentLines(file.content ?? "");
  if (lines.length === 0) {
    return (
      <text fg={color.dim} attributes={DIM}>
        empty file
      </text>
    );
  }
  const gutterW = String(lines.length).length;

  return (
    <>
      {lines.map((line, i) => {
        const tokens = line === "" ? [] : tokenizeLine(line, lang);
        const isCursor = i === cursor;
        const hasComment = marked?.has(i + 1) ?? false;
        return (
          <text key={i} id={`fileline-${i}`} wrapMode="none">
            <span fg={color.cursorBar} attributes={BOLD}>
              {isCursor ? glyph.focusBar : " "}
            </span>
            {hasComment ? (
              <span fg={color.candidate} attributes={BOLD}>
                {glyph.comment}
              </span>
            ) : (
              <span> </span>
            )}
            <span fg={color.dim}>{String(i + 1).padStart(gutterW) + " "}</span>
            {tokens.length === 0 ? (
              line === "" ? " " : line
            ) : (
              tokens.map((t, j) => (
                <span key={j} fg={isCursor ? color.white : colorForKind(t.kind)}>
                  {t.text}
                </span>
              ))
            )}
          </text>
        );
      })}
    </>
  );
}
