import type { MouseEvent, RenderContext } from "@opentui/core";
import {
  GhosttyTerminalRenderable,
  type GhosttyTerminalOptions,
} from "ghostty-opentui/terminal-buffer";

function visibleHistoryLimit(
  retainedHistoryLimit: number | undefined,
  rows: number,
): number | undefined {
  if (retainedHistoryLimit === undefined) return undefined;
  return Math.min(retainedHistoryLimit, Math.max(1, rows));
}

export class EmbeddedTerminalRenderable extends GhosttyTerminalRenderable {
  private followsOutput = true;
  private retainedHistoryLimit: number | undefined;
  private liveHistoryLimit: number | undefined;
  private pendingScrollUpDistance = 0;

  constructor(ctx: RenderContext, options: GhosttyTerminalOptions) {
    const retainedHistoryLimit = options.limit;
    const liveHistoryLimit = visibleHistoryLimit(retainedHistoryLimit, options.rows ?? 1);
    super(ctx, { ...options, limit: liveHistoryLimit });
    this.retainedHistoryLimit = retainedHistoryLimit;
    this.liveHistoryLimit = liveHistoryLimit;
  }

  override get limit(): number | undefined {
    return super.limit;
  }

  override set limit(value: number | undefined) {
    if (
      value !== undefined &&
      (this.retainedHistoryLimit === undefined || value > this.retainedHistoryLimit)
    ) {
      this.retainedHistoryLimit = value;
      this.liveHistoryLimit = visibleHistoryLimit(value, this.rows);
    }
    if (value === this.retainedHistoryLimit && this.followsOutput) {
      super.limit = this.liveHistoryLimit;
      return;
    }
    super.limit = value;
  }

  override get rows(): number {
    return super.rows;
  }

  override set rows(value: number) {
    super.rows = value;
    this.liveHistoryLimit = visibleHistoryLimit(this.retainedHistoryLimit, value);
    if (this.followsOutput) this.limit = this.liveHistoryLimit;
  }

  followLatestOutput(): void {
    this.followsOutput = true;
    this.pendingScrollUpDistance = 0;
    this.limit = this.liveHistoryLimit;
    this.scrollY = Math.max(0, this.scrollHeight - this.height);
  }

  override onMouseEvent(event: MouseEvent): void {
    if (event.defaultPrevented) return;
    if (
      event.scroll?.direction === "up" &&
      this.retainedHistoryLimit !== undefined &&
      this.limit !== this.retainedHistoryLimit
    ) {
      this.followsOutput = false;
      this.pendingScrollUpDistance += event.scroll.delta;
      this.limit = this.retainedHistoryLimit;
      return;
    }
    super.onMouseEvent(event);
    if (event.scroll?.direction === "up") this.followsOutput = false;
    if (event.scroll?.direction === "down") {
      this.followsOutput = this.scrollY === Math.max(0, this.scrollHeight - this.height);
      if (this.followsOutput) this.limit = this.liveHistoryLimit;
    }
  }

  protected override renderSelf(buffer: unknown): void {
    super.renderSelf(buffer);
    if (this.pendingScrollUpDistance > 0) {
      const bottom = Math.max(0, this.scrollHeight - this.height);
      this.scrollY = bottom - this.pendingScrollUpDistance;
      this.pendingScrollUpDistance = 0;
      super.renderSelf(buffer);
    }
    if (this.followsOutput) {
      const bottom = Math.max(0, this.scrollHeight - this.height);
      if (this.scrollY !== bottom) {
        this.scrollY = bottom;
        super.renderSelf(buffer);
      }
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
