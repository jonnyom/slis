import type { MouseEvent } from "@opentui/core";
import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";

export class EmbeddedTerminalRenderable extends GhosttyTerminalRenderable {
  private followsOutput = true;

  followLatestOutput(): void {
    this.followsOutput = true;
    this.scrollY = Math.max(0, this.scrollHeight - this.height);
  }

  override onMouseEvent(event: MouseEvent): void {
    if (event.defaultPrevented) return;
    super.onMouseEvent(event);
    if (event.scroll?.direction === "up") this.followsOutput = false;
    if (event.scroll?.direction === "down") {
      this.followsOutput = this.scrollY === Math.max(0, this.scrollHeight - this.height);
    }
  }

  protected override renderSelf(buffer: unknown): void {
    super.renderSelf(buffer);
    if (this.followsOutput) {
      const bottom = Math.max(0, this.scrollHeight - this.height);
      if (this.scrollY !== bottom) this.scrollY = bottom;
    }
    this.positionCursorInViewport();
  }

  private positionCursorInViewport(): void {
    const cursorContext = this.ctx as typeof this.ctx & {
      getCursorState?: () => { x: number; y: number; visible: boolean };
    };
    const cursor = cursorContext.getCursorState?.();
    if (!cursor?.visible) return;
    const cursorX = cursor.x - this.scrollX;
    const cursorY = cursor.y - this.scrollY;
    const cursorInsideViewport =
      cursorX >= this.x + 1 &&
      cursorX <= this.x + this.width &&
      cursorY >= this.y + 1 &&
      cursorY <= this.y + this.height;
    if (!cursorInsideViewport) {
      cursorContext.setCursorPosition(0, 0, false);
      return;
    }
    cursorContext.setCursorPosition(
      cursorX,
      cursorY,
      true,
    );
  }
}
