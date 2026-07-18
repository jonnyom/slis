export type WheelDirection = "up" | "down" | "left" | "right";

/**
 * Encode a wheel event using the SGR mouse protocol understood by tmux.
 * Coordinates are one-based terminal-cell coordinates.
 */
export function tmuxWheelSequence(
  direction: WheelDirection,
  column: number,
  row: number,
  modifiers: { shift?: boolean; alt?: boolean; ctrl?: boolean } = {},
): string {
  const wheelCode = { up: 64, down: 65, left: 66, right: 67 }[direction];
  const modifierCode =
    (modifiers.shift ? 4 : 0) + (modifiers.alt ? 8 : 0) + (modifiers.ctrl ? 16 : 0);
  const x = Math.max(1, Math.floor(column));
  const y = Math.max(1, Math.floor(row));
  return `\x1b[<${wheelCode + modifierCode};${x};${y}M`;
}
