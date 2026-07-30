import { TextBufferRenderable } from "@opentui/core";
import { useRenderer, useSelectionHandler } from "@opentui/react";
import { openUrl } from "../rpc/mutate";
import { activateVisibleUrlAtPosition } from "../term/links";

function openSelectedUrl(url: string): void {
  void openUrl(url);
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
    const textRenderable = selection.touchedRenderables.find(
      (renderable) => renderable instanceof TextBufferRenderable,
    ) as TextBufferRenderable | undefined;
    if (
      isClick &&
      textRenderable &&
      activateVisibleUrlAtPosition(
        textRenderable.plainText,
        selection.anchor.y - textRenderable.screenY,
        selection.anchor.x - textRenderable.screenX,
        onOpen,
      )
    ) {
      renderer.clearSelection();
      return;
    }

    const selectedText = selection.getSelectedText();
    if (selectedText) renderer.copyToClipboardOSC52(selectedText);
  });

  return null;
}
