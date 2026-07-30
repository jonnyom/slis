const KITTY_SHIFT_ENTER = "\x1b[13;2u";

export function embeddedTerminalInputSequence(sequence: string): string {
  return sequence === KITTY_SHIFT_ENTER ? "\n" : sequence;
}
