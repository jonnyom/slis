// File-content pane (F3): renders a file at a branch's revision with line
// numbers and single-line syntax highlighting (reusing the differ's tokenizer).
// Binary files and read errors show a one-line notice instead of content.

import type { ReactNode } from "react";
import type { FileResult } from "../rpc/types";
import { langForPath, tokenizeLine } from "../diff/tokenize";
import { color, colorForKind } from "../theme";
import { DIM } from "./ui";

// contentLines splits file content into display lines, dropping a single
// trailing empty line from a final newline so the last line isn't a blank row.
function contentLines(content: string): string[] {
  const lines = content.split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

export function FileView({
  file,
  error,
  loading,
}: {
  file: FileResult | null;
  error: string | null;
  loading: boolean;
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
        return (
          <text key={i} wrapMode="none">
            <span fg={color.dim}>{String(i + 1).padStart(gutterW) + " "}</span>
            {tokens.length === 0 ? (
              line === "" ? " " : line
            ) : (
              tokens.map((t, j) => (
                <span key={j} fg={colorForKind(t.kind)}>
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
