// File-tree browser pane (F3): renders the flattened, lazily-expanded tree of a
// branch's revision. Pure presentation — expand/collapse state and fetching live
// in the cockpit; the flat rows come from state/filetree.flattenTree.

import type { ReactNode } from "react";
import type { FileRow } from "../state/filetree";
import { color, glyph } from "../theme";
import { BOLD, DIM } from "./ui";

function sizeLabel(size: number): string {
  if (size < 0) return "";
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KB`;
}

export function FileTree({
  rows,
  sel,
  loading,
}: {
  rows: FileRow[];
  sel: number;
  loading: boolean;
}): ReactNode {
  if (rows.length === 0) {
    return (
      <text fg={color.dim} attributes={DIM}>
        {loading ? "loading tree…" : "empty tree"}
      </text>
    );
  }
  return (
    <>
      {rows.map((row, i) => {
        const selected = i === sel;
        const isDir = row.type === "tree";
        const twisty = isDir ? (row.expanded ? "▾ " : "▸ ") : "  ";
        const nameColor = isDir ? color.repoHeader : color.fg;
        return (
          <text key={row.path} wrapMode="none">
            <span fg={color.cursorBar}>{selected ? glyph.focusBar : " "}</span>
            <span fg={color.dim}>{"  ".repeat(row.depth)}</span>
            <span fg={isDir ? color.synced : color.dim}>{twisty}</span>
            <span fg={nameColor} attributes={selected ? BOLD : 0}>
              {row.name}
              {isDir ? "/" : ""}
            </span>
            {!isDir && row.size >= 0 ? (
              <span fg={color.dim}>{"  " + sizeLabel(row.size)}</span>
            ) : null}
          </text>
        );
      })}
    </>
  );
}
