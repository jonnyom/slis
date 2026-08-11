import type { MouseEvent, RenderContext } from "@opentui/core";
import {
  GhosttyTerminalRenderable,
  type GhosttyTerminalOptions,
} from "ghostty-opentui/terminal-buffer";

export class EmbeddedTerminalRenderable extends GhosttyTerminalRenderable {
  constructor(ctx: RenderContext, options: GhosttyTerminalOptions) {
    super(ctx, options);
    if (options.persistent) this.feed("\x1b[20l");
  }

  override onMouseEvent(_event: MouseEvent): void {}
}
