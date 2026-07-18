export function maxScroll(total: number, viewport: number): number {
  return Math.max(0, total - Math.max(0, viewport));
}

export function clampScroll(offset: number, total: number, viewport: number): number {
  return Math.max(0, Math.min(offset, maxScroll(total, viewport)));
}
