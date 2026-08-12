import type { MouseEvent } from "@opentui/core";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";

export class EmbeddedTerminalRenderable extends GhosttyTerminalRenderable {
  override onMouseEvent(_event: MouseEvent): void {}
}
