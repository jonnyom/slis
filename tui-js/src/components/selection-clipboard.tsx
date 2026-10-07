import { TextBufferRenderable } from "@opentui/core";
import { useRenderer, useSelectionHandler } from "@opentui/react";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { openUrl } from "../rpc/mutate";
import { activateVisibleUrlAtPosition } from "../term/links";

type SelectionPoint = { x: number; y: number };
type TerminalLinkSource = {
  terminalUrlAtPosition: (row: number, column: number) => string | null;
};

function openSelectedUrl(url: string): void {
  void openUrl(url);
}

function terminalLinkSource(renderable: TextBufferRenderable): TerminalLinkSource | null {
  if (!("terminalUrlAtPosition" in renderable)) return null;
  const source = renderable as TextBufferRenderable & Partial<TerminalLinkSource>;
  return typeof source.terminalUrlAtPosition === "function"
    ? source as TextBufferRenderable & TerminalLinkSource
    : null;
}

function activateTrackedTerminalUrl(
  url: string | null | undefined,
  onOpen: (url: string) => void,
): boolean {
  if (!url) return false;
  onOpen(url);
  return true;
}

function stringIndexAtCellColumn(text: string, targetColumn: number): number {
  if (targetColumn <= 0) return 0;
  let column = 0;
  let index = 0;
  for (const character of text) {
    const nextColumn = column + Math.max(0, Bun.stringWidth(character));
    if (nextColumn > targetColumn) break;
    column = nextColumn;
    index += character.length;
  }
  return index;
}

function orderedSelectionPoints(
  anchor: SelectionPoint,
  focus: SelectionPoint,
): [SelectionPoint, SelectionPoint] {
  if (anchor.y < focus.y || (anchor.y === focus.y && anchor.x <= focus.x)) {
    return [anchor, focus];
  }
  return [focus, anchor];
}

export function terminalTextWithinSelection(
  terminal: GhosttyTerminalRenderable,
  anchor: SelectionPoint,
  focus: SelectionPoint,
): string {
  const [start, end] = orderedSelectionPoints(
    {
      x: anchor.x - terminal.screenX + terminal.scrollX,
      y: anchor.y - terminal.screenY + terminal.scrollY,
    },
    {
      x: focus.x - terminal.screenX + terminal.scrollX,
      y: focus.y - terminal.screenY + terminal.scrollY,
    },
  );
  const lines = terminal.plainText.split("\n");
  const firstRow = Math.max(0, start.y);
  const lastRow = Math.min(lines.length - 1, end.y);
  if (firstRow > lastRow) return "";

  return lines
    .slice(firstRow, lastRow + 1)
    .map((line, rowOffset) => {
      const row = firstRow + rowOffset;
      const startColumn = row === start.y ? Math.max(0, start.x) : 0;
      const endColumn = row === end.y ? Math.max(0, end.x) : Bun.stringWidth(line);
      return line.slice(
        stringIndexAtCellColumn(line, startColumn),
        stringIndexAtCellColumn(line, endColumn),
      );
    })
    .join("\n");
}

export function SelectionClipboard({
  onOpen = openSelectedUrl,
}: {
  onOpen?: (url: string) => void;
} = {}): null {
  const renderer = useRenderer();

  useSelectionHandler((selection) => {
    const isClick =
      selection.anchor.x === selection.focus.x &&
      selection.anchor.y === selection.focus.y;
    const textRenderable = selection.touchedRenderables
      .filter((renderable) => renderable.visible)
      .filter((renderable): renderable is TextBufferRenderable =>
        renderable instanceof TextBufferRenderable,
      )
      .sort((left, right) => right.zIndex - left.zIndex)[0];
    const terminalRow = textRenderable
      ? selection.anchor.y - textRenderable.screenY + textRenderable.scrollY
      : 0;
    const terminalColumn = textRenderable
      ? selection.anchor.x - textRenderable.screenX + textRenderable.scrollX
      : 0;
    const trackedTerminalUrl = textRenderable
      ? terminalLinkSource(textRenderable)?.terminalUrlAtPosition(terminalRow, terminalColumn)
      : null;
    if (
      isClick &&
      textRenderable &&
      (activateTrackedTerminalUrl(trackedTerminalUrl, onOpen) ||
        activateVisibleUrlAtPosition(
          textRenderable.plainText,
          terminalRow,
          terminalColumn,
          onOpen,
        ))
    ) {
      renderer.clearSelection();
      return;
    }

    const selectedRenderables = selection.selectedRenderables.filter(
      (renderable) => renderable.visible,
    );
    const highestZIndex = Math.max(...selectedRenderables.map((renderable) => renderable.zIndex));
    const selectedText = highestZIndex > 0
      ? selectedRenderables
          .filter((renderable) => renderable.zIndex === highestZIndex)
          .map((renderable) =>
            renderable instanceof GhosttyTerminalRenderable
              ? terminalTextWithinSelection(
                  renderable,
                  selection.anchor,
                  selection.focus,
                )
              : renderable.getSelectedText(),
          )
          .join("")
      : selection.getSelectedText();
    if (selectedText) renderer.copyToClipboardOSC52(selectedText);
  });

  return null;
}
